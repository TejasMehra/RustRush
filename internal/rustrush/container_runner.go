package rustrush

import (
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	TimingProcessEndToEnd = "process_end_to_end"
	TimingContainerRun    = "container_process_isolated"
	TimingSteadyState     = "steady_state_in_container" // Legacy report value; never produced by current runs.
)

//go:embed container_timer/main.go
var timerSource []byte

type timedResult struct {
	Output     string
	DurationNS uint64
	Resolution uint64
}

// parseTimedOutput decodes the timer's structured response. The timer wraps
// workload stdout as base64, so workload text cannot spoof or alter timing.
func parseTimedOutput(stdout string) (timedResult, error) {
	decoder := json.NewDecoder(strings.NewReader(stdout))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return timedResult{}, errors.New("in-container timing response is missing or malformed")
	}
	fields := make(map[string]json.RawMessage, 3)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return timedResult{}, errors.New("in-container timing response has a malformed field")
		}
		key, ok := token.(string)
		if !ok {
			return timedResult{}, errors.New("in-container timing response has a non-string field name")
		}
		if _, duplicate := fields[key]; duplicate {
			return timedResult{}, fmt.Errorf("in-container timing response repeats field %q", key)
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return timedResult{}, errors.New("in-container timing response has a malformed field value")
		}
		fields[key] = raw
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') {
		return timedResult{}, errors.New("in-container timing response is not closed")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return timedResult{}, errors.New("in-container timing response contains trailing data")
	}
	for _, key := range []string{"duration_ns", "output_base64", "clock_resolution_ns"} {
		if _, ok := fields[key]; !ok {
			return timedResult{}, fmt.Errorf("in-container timing response is missing %q", key)
		}
	}
	if len(fields) != 3 {
		return timedResult{}, errors.New("in-container timing response contains unknown fields")
	}
	var duration int64
	var encodedOutput *string
	var resolution uint64
	if err := json.Unmarshal(fields["duration_ns"], &duration); err != nil || duration <= 0 {
		return timedResult{}, errors.New("in-container duration is negative, zero, malformed, or out of range")
	}
	if err := json.Unmarshal(fields["output_base64"], &encodedOutput); err != nil || encodedOutput == nil {
		return timedResult{}, errors.New("in-container workload output is malformed")
	}
	if err := json.Unmarshal(fields["clock_resolution_ns"], &resolution); err != nil || resolution == 0 || resolution > math.MaxInt64 {
		return timedResult{}, errors.New("in-container clock resolution is zero, malformed, or out of range")
	}
	output, err := base64.StdEncoding.DecodeString(*encodedOutput)
	if err != nil {
		return timedResult{}, errors.New("in-container workload output encoding is malformed")
	}
	return timedResult{
		Output: strings.TrimSpace(string(output)), DurationNS: uint64(duration), Resolution: resolution,
	}, nil
}

type containerRunner struct {
	engine      *containerEngine
	binDir      string
	inputDir    string
	mu          sync.Mutex
	overheads   map[string][]time.Duration
	wallTimes   map[string][]time.Duration
	failedRuns  map[string][]time.Duration
	resolutions map[string][]uint64
}

func (e *containerEngine) runner(binDir, inputDir string) *containerRunner {
	return &containerRunner{
		engine: e, binDir: binDir, inputDir: inputDir,
		overheads: map[string][]time.Duration{}, wallTimes: map[string][]time.Duration{},
		failedRuns: map[string][]time.Duration{}, resolutions: map[string][]uint64{},
	}
}

func (r *containerRunner) processRunner() processRunner {
	return func(ctx context.Context, executable string, args []string, _ string, _ []string) (Sample, error) {
		role := strings.TrimSuffix(filepath.Base(executable), ".exe")
		var image string
		switch role {
		case "baseline":
			image = r.engine.config.GoImage
		case "candidate":
			image = r.engine.config.RustImage
		default:
			return Sample{}, fmt.Errorf("unexpected executable %q for container runner", role)
		}

		mounts := []containerMount{{r.binDir, containerBin, true}}
		if r.inputDir != "" {
			mounts = append(mounts, containerMount{r.inputDir, containerIn, true})
		}
		command := []string{containerBin + "/timer", containerBin + "/" + role}
		for _, arg := range args {
			if r.inputDir != "" && strings.HasPrefix(arg, r.inputDir) {
				arg = containerIn + "/" + filepath.Base(arg)
			}
			command = append(command, arg)
		}

		start := time.Now()
		stdout, stderr, err := r.engine.run(ctx, containerSpec{
			Image: image, Workdir: "/tmp", Mounts: mounts, Command: command,
		})
		wall := time.Since(start)
		r.mu.Lock()
		r.wallTimes[role] = append(r.wallTimes[role], wall)
		r.mu.Unlock()
		if err != nil {
			r.mu.Lock()
			r.failedRuns[role] = append(r.failedRuns[role], wall)
			r.mu.Unlock()
			return Sample{}, fmt.Errorf("run workload in a fresh container: %w: %s", err, firstLine(stderr, err))
		}
		timed, err := parseTimedOutput(stdout)
		if err != nil {
			r.mu.Lock()
			r.failedRuns[role] = append(r.failedRuns[role], wall)
			r.mu.Unlock()
			return Sample{}, err
		}
		r.mu.Lock()
		r.resolutions[role] = append(r.resolutions[role], timed.Resolution)
		if wall > time.Duration(timed.DurationNS) {
			r.overheads[role] = append(r.overheads[role], wall-time.Duration(timed.DurationNS))
		}
		r.mu.Unlock()
		return Sample{
			DurationNS: timed.DurationNS, TimerResolutionNS: timed.Resolution, Output: timed.Output,
		}, nil
	}
}

func (r *containerRunner) metadata() map[string]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := map[string]string{
		"timing_clock":          "Go time.Since monotonic clock; elapsed-time accuracy and instrumentation overhead are not calibrated",
		"container_exec_method": "one fresh hardened docker run --rm per implementation warm-up/correctness run and measured invocation",
	}
	if len(r.overheads["baseline"])+len(r.overheads["candidate"]) > 0 {
		result["container_run_overhead_ms_estimate_median"] = fmt.Sprintf(
			"baseline/candidate=%s/%s across correctness/warm-up/trial runs; derived residual estimate, not cross-clock calibrated",
			formatMedianMS(r.overheads["baseline"]), formatMedianMS(r.overheads["candidate"]),
		)
	}
	if len(r.wallTimes["baseline"])+len(r.wallTimes["candidate"]) > 0 {
		result["container_invocation_wall_ms_median"] = fmt.Sprintf(
			"baseline/candidate=%s/%s across correctness/warm-up/trial runs; includes Docker/container lifecycle and workload process duration; not a benchmark sample",
			formatMedianMS(r.wallTimes["baseline"]), formatMedianMS(r.wallTimes["candidate"]),
		)
	}
	if len(r.failedRuns["baseline"])+len(r.failedRuns["candidate"]) > 0 {
		result["container_failed_invocation_wall_ms_median"] = fmt.Sprintf(
			"baseline/candidate=%s/%s; failures are invalid observations, not benchmark samples",
			formatMedianMS(r.failedRuns["baseline"]), formatMedianMS(r.failedRuns["candidate"]),
		)
	}
	if len(r.resolutions["baseline"])+len(r.resolutions["candidate"]) > 0 {
		result["timing_resolution_ns"] = fmt.Sprintf(
			"baseline/candidate=%d/%d (empirical minimum tick; not an accuracy guarantee)",
			medianUint64(r.resolutions["baseline"]), medianUint64(r.resolutions["candidate"]),
		)
	}
	return result
}

func formatMedianMS(values []time.Duration) string {
	if len(values) == 0 {
		return "n/a"
	}
	copyValues := append([]time.Duration(nil), values...)
	sort.Slice(copyValues, func(i, j int) bool { return copyValues[i] < copyValues[j] })
	return fmt.Sprintf("%.1f ms", ms(copyValues[len(copyValues)/2]))
}

func medianUint64(values []uint64) uint64 {
	if len(values) == 0 {
		return 0
	}
	copyValues := append([]uint64(nil), values...)
	sort.Slice(copyValues, func(i, j int) bool { return copyValues[i] < copyValues[j] })
	return copyValues[len(copyValues)/2]
}

func ms(duration time.Duration) float64 { return float64(duration) / float64(time.Millisecond) }
