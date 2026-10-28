package rustrush

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	TimingProcessEndToEnd = "process_end_to_end"
	TimingSteadyState     = "steady_state_in_container"

	timingMarker = "RUSTRUSH_NS="
	// sessionLifetimeSeconds bounds a leaked container if cleanup never runs.
	sessionLifetimeSeconds = "3600"
)

// timedScript runs the workload and prints the in-container wall-clock time of
// that process only (bash EPOCHREALTIME, 1 microsecond resolution). The script
// itself, docker exec dispatch, and container lifecycle are outside the interval.
const timedScript = `[ -n "$EPOCHREALTIME" ] || { echo "bash EPOCHREALTIME is unavailable" >&2; exit 97; }
a=${EPOCHREALTIME/[.,]/}
"$@"
rc=$?
b=${EPOCHREALTIME/[.,]/}
printf '\n` + timingMarker + `%d\n' $(( (b - a) * 1000 ))
exit $rc`

var timingLine = regexp.MustCompile(`^` + timingMarker + `([0-9]+)$`)

// parseTimedOutput splits workload stdout from the trailing timing marker.
// Missing, malformed, or zero timing is an error so it can never become a sample.
func parseTimedOutput(stdout string) (string, uint64, error) {
	index := strings.LastIndex(stdout, "\n"+timingMarker)
	if index < 0 {
		return "", 0, errors.New("in-container timing is missing from the workload output")
	}
	match := timingLine.FindStringSubmatch(strings.TrimSpace(stdout[index+1:]))
	if match == nil {
		return "", 0, errors.New("in-container timing is malformed")
	}
	nanoseconds, err := strconv.ParseUint(match[1], 10, 64)
	if err != nil || nanoseconds == 0 {
		return "", 0, errors.New("in-container timing is zero or out of range")
	}
	return strings.TrimSpace(stdout[:index]), nanoseconds, nil
}

// containerSession holds one pre-started, hardened container per implementation.
// Creation and start-up happen before any timing; each run is a docker exec into
// an already-running container with the same limits, so warm-ups and timed runs
// share identical, already-prepared environments.
type containerSession struct {
	engine   *containerEngine
	inputDir string
	mu       sync.Mutex
	names    map[string]string
	startup  map[string]time.Duration
	dispatch []time.Duration
	closed   bool
}

func (e *containerEngine) startSession(ctx context.Context, binDir, inputDir string) (*containerSession, error) {
	session := &containerSession{engine: e, inputDir: inputDir, names: map[string]string{}, startup: map[string]time.Duration{}}
	for _, item := range []struct{ role, image string }{{"baseline", e.config.GoImage}, {"candidate", e.config.RustImage}} {
		name, err := newContainerName()
		if err != nil {
			session.close()
			return nil, err
		}
		mounts := []containerMount{{binDir, containerBin, true}}
		if inputDir != "" {
			mounts = append(mounts, containerMount{inputDir, containerIn, true})
		}
		args, err := containerArgs(containerSpec{
			Name: name, Image: item.image, User: e.user, Workdir: "/tmp", Mounts: mounts, Detach: true,
			Command: []string{"sleep", sessionLifetimeSeconds}, Config: e.config,
		})
		if err != nil {
			session.close()
			return nil, err
		}
		session.names[item.role] = name
		start := time.Now()
		_, stderr, runErr := e.exec(ctx, args)
		if runErr != nil || ctx.Err() != nil {
			session.close()
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, fmt.Errorf("start %s container: %w: %s", item.role, runErr, firstLine(stderr, nil))
		}
		session.startup[item.role] = time.Since(start)
	}
	return session, nil
}

func (s *containerSession) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for _, name := range s.names {
		_, _, _ = s.engine.exec(ctx, []string{"rm", "--force", name})
	}
}

// runner returns a processRunner whose samples are in-container workload durations.
func (s *containerSession) runner() processRunner {
	return func(ctx context.Context, executable string, args []string, _ string, _ []string) (Sample, error) {
		role := strings.TrimSuffix(filepath.Base(executable), ".exe")
		name, ok := s.names[role]
		if !ok {
			return Sample{}, fmt.Errorf("unexpected executable %q for container runner", role)
		}
		command := []string{"exec", "--workdir", "/tmp", name, "bash", "-c", timedScript, "bash", containerBin + "/" + role}
		for _, arg := range args {
			if s.inputDir != "" && strings.HasPrefix(arg, s.inputDir) {
				arg = containerIn + "/" + filepath.Base(arg)
			}
			command = append(command, arg)
		}
		start := time.Now()
		stdout, stderr, err := s.engine.exec(ctx, command)
		wall := time.Since(start)
		if ctx.Err() != nil {
			// The workload may still be running inside the container; remove it so
			// no later sample can be affected.
			cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_, _, _ = s.engine.exec(cleanup, []string{"rm", "--force", name})
			return Sample{}, ctx.Err()
		}
		if err != nil {
			return Sample{}, fmt.Errorf("run workload in container: %w: %s", err, firstLine(stderr, nil))
		}
		output, nanoseconds, parseErr := parseTimedOutput(stdout)
		if parseErr != nil {
			return Sample{}, parseErr
		}
		if overhead := wall - time.Duration(nanoseconds); overhead > 0 {
			s.mu.Lock()
			s.dispatch = append(s.dispatch, overhead)
			s.mu.Unlock()
		}
		return Sample{DurationNS: nanoseconds, Output: output}, nil
	}
}

// metadata reports the overheads that were deliberately excluded from samples.
func (s *containerSession) metadata() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := map[string]string{
		"timing_resolution":     "1 microsecond (bash EPOCHREALTIME inside the container)",
		"container_startup_ms":  fmt.Sprintf("baseline=%.1f candidate=%.1f (excluded from samples)", ms(s.startup["baseline"]), ms(s.startup["candidate"])),
		"container_exec_method": "docker exec into a pre-started container; the timed interval covers only the workload process",
	}
	if len(s.dispatch) > 0 {
		sorted := append([]time.Duration(nil), s.dispatch...)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
		result["container_exec_dispatch_ms_median"] = fmt.Sprintf("%.1f over %d runs (excluded from samples)", ms(sorted[len(sorted)/2]), len(sorted))
	}
	return result
}

func ms(duration time.Duration) float64 { return float64(duration) / float64(time.Millisecond) }
