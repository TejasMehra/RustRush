package rustrush

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func skipIfRoot(t *testing.T) {
	t.Helper()
	if os.Getuid() == 0 {
		t.Skip("container isolation refuses to run as root")
	}
}

func testSpec(config ContainerConfig) containerSpec {
	return containerSpec{
		Name: "rustrush-0123456789abcdef", Image: "golang:1.24", User: "1000:1000", Workdir: "/tmp",
		Mounts:  []containerMount{{Source: filepath.Join(os.TempDir(), "x"), Target: "/rr/src", ReadOnly: true}},
		Command: []string{"true"}, Config: config.withDefaults(),
	}
}

func TestContainerArgsAreHardened(t *testing.T) {
	args, err := containerArgs(testSpec(ContainerConfig{}))
	if err != nil {
		t.Fatal(err)
	}
	joined := " " + strings.Join(args, " ") + " "
	for _, required := range []string{
		" --network none ", " --cap-drop ALL ", " --security-opt no-new-privileges ", " --read-only ",
		" --user 1000:1000 ", " --pull never ", " --memory 512m ", " --memory-swap 512m ", " --cpus 1 ",
		" --pids-limit 256 ", ",readonly ", "noexec",
	} {
		if !strings.Contains(joined, required) {
			t.Errorf("missing %q in %s", required, joined)
		}
	}
	for _, forbidden := range []string{"--privileged", "docker.sock", "--network host", "--cap-add", "--pid host", "--volumes-from", "--device"} {
		if strings.Contains(joined, forbidden) {
			t.Errorf("forbidden %q in %s", forbidden, joined)
		}
	}
	if args[len(args)-1] != "true" || args[0] != "run" {
		t.Fatalf("unexpected shape: %v", args)
	}
	spec := testSpec(ContainerConfig{})
	spec.TmpfsExec = true
	args, _ = containerArgs(spec)
	if strings.Contains(strings.Join(args, " "), "noexec") {
		t.Fatal("builds need an executable tmpfs")
	}
}

func TestContainerArgsRejectUnsafeSpecs(t *testing.T) {
	cases := map[string]func(*containerSpec){
		"root user":        func(s *containerSpec) { s.User = "0:0" },
		"root by name":     func(s *containerSpec) { s.User = "root" },
		"empty user":       func(s *containerSpec) { s.User = "" },
		"bad name":         func(s *containerSpec) { s.Name = "x; rm -rf /" },
		"bad image":        func(s *containerSpec) { s.Image = "--privileged" },
		"no command":       func(s *containerSpec) { s.Command = nil },
		"docker socket":    func(s *containerSpec) { s.Mounts[0].Source = "/var/run/docker.sock" },
		"windows pipe":     func(s *containerSpec) { s.Mounts[0].Source = `C:\x\docker_engine` },
		"comma injection":  func(s *containerSpec) { s.Mounts[0].Source = filepath.Join(os.TempDir(), "a,dst=/etc") },
		"relative source":  func(s *containerSpec) { s.Mounts[0].Source = "relative/dir" },
		"outside /rr/":     func(s *containerSpec) { s.Mounts[0].Target = "/etc" },
		"host root target": func(s *containerSpec) { s.Mounts[0].Target = "/" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			spec := testSpec(ContainerConfig{})
			spec.Mounts = append([]containerMount(nil), spec.Mounts...)
			mutate(&spec)
			if _, err := containerArgs(spec); err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
}

func TestContainerConfigValidation(t *testing.T) {
	for name, config := range map[string]ContainerConfig{
		"image flag injection": {GoImage: "--privileged"},
		"image whitespace":     {RustImage: "rust:1 --privileged"},
		"memory":               {Memory: "lots"},
		"zero-ish memory":      {Memory: "0m"},
		"cpus text":            {CPUs: "all"},
		"cpus zero":            {CPUs: "0"},
		"cpus huge":            {CPUs: "1000"},
		"pids low":             {PidsLimit: 1},
		"pids high":            {PidsLimit: 100000},
	} {
		t.Run(name, func(t *testing.T) {
			if err := config.withDefaults().validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
	if err := (ContainerConfig{}).withDefaults().validate(); err != nil {
		t.Fatal(err)
	}
}

type fakeDocker struct {
	mu          sync.Mutex
	calls       [][]string
	failOn      string
	daemonErr   error
	missing     string
	rmFail      bool
	hang        bool
	hangTimer   bool
	runFailOn   string
	noTiming    bool
	zeroTiming  bool
	delay       time.Duration
	buildDelays map[string]time.Duration
	runs        int
	timingNS    []uint64
}

func (f *fakeDocker) exec(ctx context.Context, args []string) (string, string, error) {
	f.mu.Lock()
	f.calls = append(f.calls, append([]string(nil), args...))
	f.mu.Unlock()
	if args[0] == "run" {
		delay := f.delay
		joined := strings.Join(args, " ")
		for marker, buildDelay := range f.buildDelays {
			if strings.Contains(joined, marker) {
				delay = buildDelay
				break
			}
		}
		if delay > 0 {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return "", "container command canceled before completion", ctx.Err()
			case <-timer.C:
			}
		}
	}
	switch args[0] {
	case "version":
		if f.daemonErr != nil {
			return "", "Cannot connect to the Docker daemon", f.daemonErr
		}
		return "27.0.0\n", "", nil
	case "image":
		if args[len(args)-1] == f.missing {
			return "", "No such image", errors.New("exit status 1")
		}
		return "sha256:abc\n", "", nil
	case "rm":
		if f.rmFail {
			return "", "cleanup failed", errors.New("exit status 1")
		}
		return "", "", nil
	case "run":
		joined := strings.Join(args, " ")
		isTimer := strings.Contains(joined, "/rr/bin/timer")
		if f.hang || (f.hangTimer && isTimer) {
			<-ctx.Done()
			return "", "", ctx.Err()
		}
		if f.failOn != "" && strings.Contains(joined, f.failOn) {
			return "", "boom", errors.New("exit status 1")
		}
		if f.runFailOn != "" && strings.Contains(joined, f.runFailOn) {
			return "", "workload failed", errors.New("exit status 1")
		}
		if strings.Contains(joined, "rustc --version") {
			return "rustc 1.80.0\n", "", nil
		}
		if isTimer {
			f.mu.Lock()
			f.runs++
			n := f.runs
			f.mu.Unlock()
			switch {
			case f.noTiming:
				return "{}\n", "", nil
			case f.zeroTiming:
				return "{\"duration_ns\":0,\"output_base64\":\"NDIK\",\"clock_resolution_ns\":1000}\n", "", nil
			}
			ns := uint64(1_000_000 + n*1000)
			f.mu.Lock()
			f.timingNS = append(f.timingNS, ns)
			f.mu.Unlock()
			return fmt.Sprintf("{\"duration_ns\":%d,\"output_base64\":\"NDIK\",\"clock_resolution_ns\":1000}\n", ns), "", nil
		}
		return "", "", nil
	}
	return "", "", errors.New("unexpected docker call")
}

func (f *fakeDocker) callsNamed(name string) [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var result [][]string
	for _, call := range f.calls {
		if call[0] == name {
			result = append(result, call)
		}
	}
	return result
}
func TestNewContainerEngineFailsClosed(t *testing.T) {
	skipIfRoot(t)
	ctx := context.Background()
	if _, err := newContainerEngine(ctx, ContainerConfig{}, func(string) (string, error) { return "", errors.New("missing") }, nil); err == nil || !strings.Contains(err.Error(), "refusing to fall back") {
		t.Fatalf("missing CLI must fail closed: %v", err)
	}
	if _, err := newContainerEngine(ctx, ContainerConfig{}, nil, (&fakeDocker{daemonErr: errors.New("exit 1")}).exec); err == nil || !strings.Contains(err.Error(), "daemon is unavailable") {
		t.Fatalf("down daemon must fail closed: %v", err)
	}
	if _, err := newContainerEngine(ctx, ContainerConfig{}, nil, (&fakeDocker{missing: DefaultContainerRustImage}).exec); err == nil || !strings.Contains(err.Error(), "never pulls") {
		t.Fatalf("missing image must fail closed: %v", err)
	}
	if _, err := newContainerEngine(ctx, ContainerConfig{Memory: "x"}, nil, (&fakeDocker{}).exec); err == nil {
		t.Fatal("invalid config must fail")
	}
}

func TestRunCandidateContainerModeFailsClosedWithoutRunningAnything(t *testing.T) {
	skipIfRoot(t)
	f := newCandidateFixture(t)
	options := f.options()
	options.Isolation = IsolationContainer
	options.containerExec = (&fakeDocker{daemonErr: errors.New("down")}).exec
	options.runProcess = func(context.Context, string, []string, string, []string) (Sample, error) {
		t.Fatal("must not fall back to local execution")
		return Sample{}, nil
	}
	options.build = func(context.Context, string, []string, string, []string) (string, error) {
		t.Fatal("must not fall back to local builds")
		return "", nil
	}
	if _, err := RunCandidate(context.Background(), options); err == nil || !strings.Contains(err.Error(), "docker daemon is unavailable") {
		t.Fatalf("expected fail-closed error, got %v", err)
	}
	options.Isolation = "chroot"
	if _, err := RunCandidate(context.Background(), options); err == nil || !strings.Contains(err.Error(), "unsupported isolation") {
		t.Fatalf("unknown mode must be rejected: %v", err)
	}
}

func TestRunCandidateContainerModeUsesHardenedContainers(t *testing.T) {
	skipIfRoot(t)
	f := newCandidateFixture(t)
	docker := &fakeDocker{delay: 5 * time.Millisecond}
	options := f.options()
	options.Isolation = IsolationContainer
	options.containerExec = docker.exec
	options.build = func(context.Context, string, []string, string, []string) (string, error) {
		t.Fatal("host builds are forbidden in container mode")
		return "", nil
	}
	before := hashTree(t, filepath.Dir(f.candidate))
	report, err := RunCandidate(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if report.Correctness != "PASSED_FOR_CHECKSUM" || len(report.Measurements) != 2 {
		t.Fatalf("unexpected report: %+v", report)
	}
	if !strings.HasPrefix(report.Environment["isolation"], "container") || report.Environment["rustc"] != "rustc 1.80.0" ||
		!strings.Contains(report.Environment["container_go_image"], "sha256:abc") {
		t.Fatalf("isolation metadata missing: %v", report.Environment)
	}
	runs := docker.callsNamed("run")
	// rustc --version, three builds, two correctness/warm-up runs, and six trials.
	if len(runs) != 1+3+2+6 {
		t.Fatalf("unexpected container count %d", len(runs))
	}
	names := map[string]bool{}
	timerRuns := 0
	for _, args := range runs {
		joined := " " + strings.Join(args, " ") + " "
		for _, required := range []string{" --network none ", " --cap-drop ALL ", " --read-only ", " --pull never ", " --security-opt no-new-privileges "} {
			if !strings.Contains(joined, required) {
				t.Fatalf("container missing %q: %s", required, joined)
			}
		}
		if strings.Contains(joined, f.baseline) || strings.Contains(joined, f.input) || strings.Contains(joined, "--privileged") {
			t.Fatalf("original paths or privileges leaked into %s", joined)
		}
		for i, arg := range args {
			if arg == "--name" && i+1 < len(args) {
				if names[args[i+1]] {
					t.Fatalf("container name reused across invocations: %s", args[i+1])
				}
				names[args[i+1]] = true
			}
		}
		if strings.Contains(joined, "/rr/bin/timer") {
			timerRuns++
			if !strings.Contains(joined, " --rm ") {
				t.Fatalf("timing invocation is not ephemeral: %s", joined)
			}
		}
	}
	if timerRuns != 2+6 {
		t.Fatalf("expected 2 correctness/warm-up runs and 6 timed runs, got %d", timerRuns)
	}
	if !strings.Contains(strings.Join(runs[len(runs)-1], " "), "--input /rr/input/input.txt") {
		t.Fatalf("input path was not translated: %v", runs[len(runs)-1])
	}
	if removed := docker.callsNamed("rm"); len(removed) != 0 {
		t.Fatalf("--rm containers need no normal-session cleanup: %v", removed)
	}
	if report.Environment["timing_mode"] != TimingContainerRun {
		t.Fatalf("timing mode: %v", report.Environment)
	}
	for _, key := range []string{"container_run_overhead_ms_estimate_median", "container_invocation_wall_ms_median", "timing_clock", "timing_resolution_ns"} {
		if report.Environment[key] == "" {
			t.Fatalf("missing %s: %v", key, report.Environment)
		}
	}
	// Samples must equal structured child durations; Docker delay is not in them.
	want := map[int64]bool{}
	docker.mu.Lock()
	for _, ns := range docker.timingNS {
		want[int64(ns)] = true
	}
	docker.mu.Unlock()
	for _, m := range report.Measurements {
		for _, sample := range m.Samples {
			if !want[int64(sample.DurationNS)] {
				t.Fatalf("sample %d is not an in-container timing", sample.DurationNS)
			}
		}
	}
	after := hashTree(t, filepath.Dir(f.candidate))
	for path, sum := range before {
		if after[path] != sum {
			t.Fatalf("%s changed", path)
		}
	}
}

func TestContainerBuildFailureAndTimeout(t *testing.T) {
	skipIfRoot(t)
	f := newCandidateFixture(t)
	docker := &fakeDocker{failOn: "rustc --edition"}
	options := f.options()
	options.Isolation = IsolationContainer
	options.containerExec = docker.exec
	report, err := RunCandidate(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if report.Measurements[1].Status != "build_failed" || report.Verdict != VerdictInconclusive {
		t.Fatalf("unexpected: %+v", report.Measurements)
	}

	hung := &fakeDocker{hang: true}
	engine, err := newContainerEngine(context.Background(), ContainerConfig{}, nil, hung.exec)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, _, err = engine.run(ctx, containerSpec{Image: "golang:1.24", Workdir: "/tmp", Command: []string{"sleep", "1000"}})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline error, got %v", err)
	}
	removed := false
	for _, call := range hung.calls {
		if call[0] == "rm" && call[1] == "--force" && strings.HasPrefix(call[2], "rustrush-") {
			removed = true
		}
	}
	if !removed {
		t.Fatal("timed-out container must be force-removed")
	}
}

func TestContainerTimerBuildHasIndependentTimeoutAndRecovers(t *testing.T) {
	skipIfRoot(t)
	f := newCandidateFixture(t)
	docker := &fakeDocker{buildDelays: map[string]time.Duration{
		"/rr/out/baseline": 60 * time.Millisecond,
		"/rr/out/timer":    60 * time.Millisecond,
	}}
	options := f.options()
	options.Timeout = 100 * time.Millisecond
	options.Isolation = IsolationContainer
	options.containerExec = docker.exec

	report, err := RunCandidate(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if report.Correctness != "PASSED_FOR_CHECKSUM" || report.Verdict == VerdictInconclusive {
		t.Fatalf("timer preparation did not recover after the baseline build: verdict=%s reason=%q measurements=%+v",
			report.Verdict, report.Reason, report.Measurements)
	}
	if len(report.Measurements) != 2 || report.Measurements[0].Status != "measured" || report.Measurements[1].Status != "measured" {
		t.Fatalf("both implementations should have measured samples: %+v", report.Measurements)
	}
}

func TestContainerTimerBuildFailureIsReportedAndCleanedUp(t *testing.T) {
	skipIfRoot(t)
	f := newCandidateFixture(t)
	docker := &fakeDocker{failOn: "-o /rr/out/timer"}
	options := f.options()
	options.Isolation = IsolationContainer
	options.containerExec = docker.exec

	report, err := RunCandidate(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	baseline := report.Measurements[0]
	if baseline.Status != "failed" || baseline.ErrorCount != 1 || report.Verdict != VerdictInconclusive {
		t.Fatalf("timer preparation failure must invalidate the comparison: %+v verdict=%s", baseline, report.Verdict)
	}
	if !strings.Contains(baseline.Error, "boom") || !strings.Contains(baseline.Error, "exit status 1") ||
		!strings.Contains(report.Reason, "boom") || !strings.Contains(RenderMarkdown(report), "boom") {
		t.Fatalf("timer preparation diagnostic was not surfaced: error=%q reason=%q", baseline.Error, report.Reason)
	}

	timerSourceRemoved := false
	for _, args := range docker.callsNamed("run") {
		joined := strings.Join(args, " ")
		if !strings.Contains(joined, "-o /rr/out/timer") {
			continue
		}
		if !strings.Contains(joined, " --rm ") {
			t.Fatalf("failed timer build container must be removed automatically: %s", joined)
		}
		for _, arg := range args {
			if strings.Contains(arg, "dst=/rr/src") {
				for _, part := range strings.Split(arg, ",") {
					if strings.HasPrefix(part, "src=") && strings.Contains(part, "timer-source") {
						_, statErr := os.Stat(strings.TrimPrefix(part, "src="))
						timerSourceRemoved = errors.Is(statErr, os.ErrNotExist)
					}
				}
			}
		}
	}
	if !timerSourceRemoved {
		t.Fatal("timer source directory should be removed after preparation failure")
	}
}

// TestContainerIsolationProbe checks real runtime behaviour. It needs a Docker
// daemon with the default images present and runs only in CI.
func TestContainerIsolationProbe(t *testing.T) {
	if os.Getenv("RUSTRUSH_CONTAINER_TESTS") != "1" {
		t.Skip("set RUSTRUSH_CONTAINER_TESTS=1 with a Docker daemon and local images")
	}
	skipIfRoot(t)
	t.Setenv("RUSTRUSH_PROBE_SECRET", "host-secret-value")
	engine, err := newContainerEngine(context.Background(), ContainerConfig{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	script := `
echo "uid=$(id -u)"
echo "nics=$(ls /sys/class/net | tr '\n' ' ')"
echo "cap=$(grep CapEff /proc/self/status | awk '{print $2}')"
echo "nnp=$(grep NoNewPrivs /proc/self/status | awk '{print $2}')"
(touch /probe 2>/dev/null && echo rootfs=writable) || echo rootfs=readonly
(touch /rr/src/new 2>/dev/null && echo mount=writable) || echo mount=readonly
(touch /tmp/ok 2>/dev/null && echo tmp=writable) || echo tmp=readonly
echo "secret=$(env | grep -c RUSTRUSH_PROBE_SECRET)"
echo "sock=$(ls /var/run/docker.sock 2>/dev/null | wc -l)"
(timeout 3 bash -c 'exec 3<>/dev/tcp/1.1.1.1/80' 2>/dev/null && echo net=reachable) || echo net=blocked
`
	stdout, stderr, err := engine.run(context.Background(), containerSpec{
		Image: engine.config.GoImage, Workdir: "/tmp",
		Mounts:  []containerMount{{Source: dir, Target: "/rr/src", ReadOnly: true}},
		Command: []string{"bash", "-c", script},
	})
	if err != nil {
		t.Fatalf("probe failed: %v %s", err, stderr)
	}
	if strings.Contains(stdout, "uid=0") {
		t.Errorf("container ran as root:\n%s", stdout)
	}
	for _, want := range []string{"nics=lo ", "cap=0000000000000000", "nnp=1", "rootfs=readonly", "mount=readonly", "tmp=writable", "secret=0", "sock=0", "net=blocked"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("expected %q in probe output:\n%s", want, stdout)
		}
	}
}

func containerRun(t *testing.T, docker *fakeDocker) Report {
	t.Helper()
	f := newCandidateFixture(t)
	options := f.options()
	options.Isolation = IsolationContainer
	options.containerExec = docker.exec
	report, err := RunCandidate(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func TestFreshContainerSamplesExcludeHostOverhead(t *testing.T) {
	skipIfRoot(t)
	docker := &fakeDocker{delay: 20 * time.Millisecond}
	report := containerRun(t, docker)
	if report.Environment["container_run_overhead_ms_estimate_median"] == "" ||
		report.Environment["container_invocation_wall_ms_median"] == "" {
		t.Fatalf("container overhead not recorded separately: %v", report.Environment)
	}
	for _, m := range report.Measurements {
		for _, sample := range m.Samples {
			if time.Duration(sample.DurationNS) > 10*time.Millisecond {
				t.Fatalf("sample %d ns includes host-side delay", sample.DurationNS)
			}
		}
	}
}

func TestContainerWorkloadFailureInvalidatesComparison(t *testing.T) {
	skipIfRoot(t)
	report := containerRun(t, &fakeDocker{runFailOn: "/rr/bin/candidate"})
	if report.Verdict != VerdictInconclusive {
		t.Fatalf("verdict %s", report.Verdict)
	}
}

func TestContainerCleanupFailureInvalidatesComparison(t *testing.T) {
	skipIfRoot(t)
	f := newCandidateFixture(t)
	docker := &fakeDocker{hangTimer: true, rmFail: true}
	options := f.options()
	options.Isolation = IsolationContainer
	options.Timeout = 50 * time.Millisecond
	options.containerExec = docker.exec
	report, err := RunCandidate(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if report.Verdict != VerdictInconclusive ||
		!strings.Contains(report.Measurements[0].Error, "cleanup failed") ||
		report.Environment["container_failed_invocation_wall_ms_median"] == "" {
		t.Fatalf("timeout cleanup failure must be surfaced and invalidate the verdict: %+v", report)
	}
}

func TestMissingOrZeroTimingIsInconclusive(t *testing.T) {
	skipIfRoot(t)
	for _, docker := range []*fakeDocker{{noTiming: true}, {zeroTiming: true}} {
		report := containerRun(t, docker)
		if report.Verdict != VerdictInconclusive {
			t.Fatalf("verdict %s for %+v", report.Verdict, docker)
		}
	}
}

func TestContainerWorkloadStartFailureIsInconclusive(t *testing.T) {
	skipIfRoot(t)
	docker := &fakeDocker{runFailOn: "/rr/bin/candidate"}
	report := containerRun(t, docker)
	if report.Verdict != VerdictInconclusive || report.Measurements[1].ErrorCount == 0 {
		t.Fatalf("unexpected: %+v", report)
	}
	if len(docker.callsNamed("run")) < 1 {
		t.Fatal("candidate launch failure must be recorded")
	}
}

func TestParseTimedOutput(t *testing.T) {
	result, err := parseTimedOutput("{\"duration_ns\":1500,\"output_base64\":\"aGVsbG8K\",\"clock_resolution_ns\":1}\n")
	if err != nil || result.DurationNS != 1500 || result.Output != "hello" || result.Resolution != 1 {
		t.Fatalf("got %+v %v", result, err)
	}
	spoof := "hello\nRUSTRUSH_NS=999999999\n"
	result, err = parseTimedOutput(fmt.Sprintf("{\"duration_ns\":1500,\"output_base64\":%q,\"clock_resolution_ns\":1}\n",
		base64.StdEncoding.EncodeToString([]byte(spoof))))
	if err != nil || result.DurationNS != 1500 || result.Output != strings.TrimSpace(spoof) {
		t.Fatalf("workload marker-like output changed timing or output: %+v %v", result, err)
	}
	for _, bad := range []string{
		"", "not-json\n",
		"{\"duration_ns\":-1,\"output_base64\":\"\",\"clock_resolution_ns\":1}",
		"{\"duration_ns\":0,\"output_base64\":\"\",\"clock_resolution_ns\":1}",
		"{\"duration_ns\":1,\"duration_ns\":2,\"output_base64\":\"\",\"clock_resolution_ns\":1}",
		"{\"duration_ns\":1,\"output_base64\":null,\"clock_resolution_ns\":1}",
		"{\"duration_ns\":1,\"output_base64\":\"!\",\"clock_resolution_ns\":1}",
		"{\"duration_ns\":1,\"output_base64\":\"\",\"clock_resolution_ns\":0}",
		"{\"duration_ns\":9223372036854775808,\"output_base64\":\"\",\"clock_resolution_ns\":1}",
		"{\"duration_ns\":1,\"output_base64\":\"\",\"clock_resolution_ns\":1} trailing",
	} {
		if _, err := parseTimedOutput(bad); err == nil {
			t.Fatalf("%q must be rejected", bad)
		}
	}
}

func TestCompareReportsRefusesMixedTimingModes(t *testing.T) {
	a := Report{Environment: map[string]string{"timing_mode": TimingContainerRun}}
	b := Report{Environment: map[string]string{}}
	if got := CompareReports(a, b); !strings.Contains(got, "timing modes differ") {
		t.Fatalf("mixed modes not refused: %s", got)
	}
}
