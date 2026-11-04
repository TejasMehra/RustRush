package rustrush

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeTool struct {
	output string
	err    error
	hang   bool
}

// fakeProbe maps executable names to behaviours; a tool absent from the map is
// not on PATH.
func fakeProbe(goos string, tools map[string]fakeTool) DoctorProbe {
	return DoctorProbe{
		GOOS:    goos,
		Timeout: 20 * time.Millisecond,
		LookPath: func(name string) (string, error) {
			if _, ok := tools[name]; ok {
				return name, nil
			}
			return "", errors.New("not found")
		},
		Run: func(ctx context.Context, path string, args ...string) (string, error) {
			tool := tools[path]
			if tool.hang {
				<-ctx.Done()
				return "", ctx.Err()
			}
			return tool.output, tool.err
		},
	}
}

func TestDoctorDetection(t *testing.T) {
	tests := []struct {
		name  string
		check func(DoctorProbe) ToolStatus
		goos  string
		tools map[string]fakeTool
		state ToolState
		text  string
	}{
		{"rust missing", checkRust, "linux", nil, ToolNotFound, ""},
		{"rust ready", checkRust, "linux", map[string]fakeTool{"rustc": {output: "rustc 1.80.0 (abc 2024)\n"}}, ToolReady, "rustc 1.80.0"},
		{"rust fails", checkRust, "linux", map[string]fakeTool{"rustc": {err: errors.New("boom")}}, ToolCouldNotRun, "boom"},
		{"rust times out", checkRust, "linux", map[string]fakeTool{"rustc": {hang: true}}, ToolCouldNotRun, "timed out"},
		{"go missing", checkGo, "linux", nil, ToolNotFound, ""},
		{"go ready", checkGo, "linux", map[string]fakeTool{"go": {output: "go version go1.22.1 linux/amd64"}}, ToolReady, "go1.22.1"},
		{"python ready", checkPython, "linux", map[string]fakeTool{"python3": {output: "Python 3.12.1"}}, ToolReady, "Python 3.12.1"},
		{"python missing", checkPython, "linux", nil, ToolNotFound, ""},
		{"python 2", checkPython, "linux", map[string]fakeTool{"python3": {output: "Python 2.7.18"}}, ToolNeedsNewer, "Python 2.7.18"},
		{"python store stub", checkPython, "windows", map[string]fakeTool{"python": {output: "Python was not found; run without arguments to install from the Microsoft Store", err: errors.New("exit status 9009")}}, ToolCouldNotRun, "Python was not found"},
		{"python windows prefers python", checkPython, "windows", map[string]fakeTool{"python": {output: "Python 3.11.0"}, "python3": {err: errors.New("stub")}}, ToolReady, "Python 3.11.0"},
		{"python linux prefers python3", checkPython, "linux", map[string]fakeTool{"python": {err: errors.New("stub")}, "python3": {output: "Python 3.11.0"}}, ToolReady, "Python 3.11.0"},
		{"python darwin prefers python3", checkPython, "darwin", map[string]fakeTool{"python": {err: errors.New("stub")}, "python3": {output: "Python 3.11.0"}}, ToolReady, "Python 3.11.0"},
		{"python garbage", checkPython, "linux", map[string]fakeTool{"python3": {output: "hello"}}, ToolCouldNotRun, ""},
		{"python times out", checkPython, "linux", map[string]fakeTool{"python3": {hang: true}}, ToolCouldNotRun, "timed out"},
		{"node missing", checkNode, "linux", nil, ToolNotFound, ""},
		{"node ready", checkNode, "linux", map[string]fakeTool{"node": {output: "v20.1.0\n"}}, ToolReady, "Node.js v20"},
		{"node old", checkNode, "linux", map[string]fakeTool{"node": {output: "v16.20.0"}}, ToolNeedsNewer, "v16.20.0"},
		{"node fails", checkNode, "linux", map[string]fakeTool{"node": {err: errors.New("bad")}}, ToolCouldNotRun, "bad"},
		{"node garbage", checkNode, "linux", map[string]fakeTool{"node": {output: "nope"}}, ToolCouldNotRun, ""},
		{"node times out", checkNode, "linux", map[string]fakeTool{"node": {hang: true}}, ToolCouldNotRun, "timed out"},
		{"docker missing", checkDocker, "linux", nil, ToolNotFound, ""},
		{"docker cli", checkDocker, "linux", map[string]fakeTool{"docker": {}}, ToolReady, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.check(fakeProbe(tt.goos, tt.tools))
			if got.State != tt.state || !strings.Contains(got.Detail, tt.text) {
				t.Fatalf("got %+v, want state %s detail containing %q", got, tt.state, tt.text)
			}
		})
	}
}

func TestFindPythonForLookupOrderIsHostIndependent(t *testing.T) {
	both := func(name string) (string, error) { return name, nil }
	onlyPython := func(name string) (string, error) {
		if name == "python" {
			return name, nil
		}
		return "", errors.New("missing")
	}
	tests := []struct {
		goos   string
		lookup func(string) (string, error)
		want   string
	}{
		{"windows", both, "python"},
		{"linux", both, "python3"},
		{"darwin", both, "python3"},
		{"windows", onlyPython, "python"},
		{"linux", onlyPython, "python"},
	}
	for _, tt := range tests {
		got, err := findPythonFor(tt.goos, tt.lookup)
		if err != nil || got != tt.want {
			t.Errorf("%s: got %q, %v; want %q", tt.goos, got, err, tt.want)
		}
	}
	if _, err := findPythonFor("windows", func(string) (string, error) { return "", errors.New("missing") }); err == nil {
		t.Error("expected error when no interpreter exists")
	}
}

func TestDoctorDockerIsNeverRunOrProbed(t *testing.T) {
	probe := fakeProbe("linux", map[string]fakeTool{"docker": {}})
	probe.Run = func(context.Context, string, ...string) (string, error) {
		t.Fatal("docker must not be executed by doctor")
		return "", nil
	}
	if checkDocker(probe).State != ToolReady {
		t.Fatal("docker CLI should be detected")
	}
}

func allTools() map[string]fakeTool {
	return map[string]fakeTool{
		"rustc":   {output: "rustc 1.80.0"},
		"go":      {output: "go version go1.22.0 linux/amd64"},
		"python3": {output: "Python 3.12.0"},
		"node":    {output: "v20.0.0"},
		"docker":  {},
	}
}

func without(tools map[string]fakeTool, names ...string) map[string]fakeTool {
	for _, name := range names {
		delete(tools, name)
	}
	return tools
}

func TestDoctorComparisonsAndExitCodes(t *testing.T) {
	tests := []struct {
		name      string
		tools     map[string]fakeTool
		ready     []string
		notReady  []string
		canRun    bool
		notInText []string
	}{
		{"all", allTools(), []string{"Built-in demo", "Go vs Rust", "Python vs Rust", "Node.js vs Rust"}, nil, true, nil},
		{"rust and go", map[string]fakeTool{"rustc": {output: "rustc 1.80.0"}, "go": {output: "go version go1.22.0 x"}}, []string{"Built-in demo", "Go vs Rust"}, []string{"Python vs Rust", "Node.js vs Rust"}, true, nil},
		{"rust and python", map[string]fakeTool{"rustc": {output: "rustc 1.80.0"}, "python3": {output: "Python 3.9.0"}}, []string{"Built-in demo", "Python vs Rust"}, []string{"Go vs Rust", "Node.js vs Rust"}, true, nil},
		{"rust and node", map[string]fakeTool{"rustc": {output: "rustc 1.80.0"}, "node": {output: "v18.0.0"}}, []string{"Built-in demo", "Node.js vs Rust"}, []string{"Go vs Rust", "Python vs Rust"}, true, nil},
		{"rust only", map[string]fakeTool{"rustc": {output: "rustc 1.80.0"}}, []string{"Built-in demo"}, []string{"Go vs Rust", "Python vs Rust", "Node.js vs Rust"}, true, nil},
		{"docker missing keeps exit 0", without(allTools(), "docker"), []string{"Built-in demo", "Go vs Rust"}, nil, true, nil},
		{"no rust, all runtimes", without(allTools(), "rustc"), nil, []string{"Built-in demo", "Go vs Rust", "Python vs Rust", "Node.js vs Rust"}, false, nil},
		{"rust broken", func() map[string]fakeTool {
			tools := allTools()
			tools["rustc"] = fakeTool{err: errors.New("crash")}
			return tools
		}(), nil, []string{"Built-in demo", "Go vs Rust"}, false, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := CheckEnvironment(fakeProbe("linux", tt.tools))
			if report.CanCompare() != tt.canRun {
				t.Fatalf("CanCompare = %v, want %v", report.CanCompare(), tt.canRun)
			}
			ready := map[string]bool{}
			for _, c := range report.Comparisons() {
				ready[c.Name] = c.Ready
				if !c.Ready && c.Fix == "" {
					t.Fatalf("%s unavailable without guidance", c.Name)
				}
			}
			for _, name := range tt.ready {
				if !ready[name] {
					t.Errorf("%s should be available", name)
				}
			}
			for _, name := range tt.notReady {
				if ready[name] {
					t.Errorf("%s should not be available", name)
				}
			}
		})
	}
}

func TestRenderDoctorOutput(t *testing.T) {
	tools := allTools()
	tools["node"] = fakeTool{output: "v16.0.0"}
	delete(tools, "python3")
	out := RenderDoctor(CheckEnvironment(fakeProbe("linux", tools)))
	for _, want := range []string{
		"RustRush environment check",
		"Ready (rustc 1.80.0)",
		"Ready (go1.22.0)",
		"Not found",
		"Needs newer version (Node.js 18 or newer is required; found v16.0.0)",
		"Comparisons available:",
		"  - Built-in demo",
		"  - Go vs Rust",
		"Not available:",
		"Python vs Rust: install Python 3 from",
		"Node.js vs Rust: install Node.js 18 or newer from",
		"CLI found; container mode also needs a running Docker and local images, which are not checked here",
		"rustrush demo",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Container mode is ready") || strings.Contains(strings.ToLower(out), "container mode: ready") {
		t.Errorf("must not claim container mode ready:\n%s", out)
	}
}

func TestRenderDoctorMissingRustAndDocker(t *testing.T) {
	out := RenderDoctor(CheckEnvironment(fakeProbe("linux", map[string]fakeTool{"go": {output: "go version go1.22.0 x"}})))
	for _, want := range []string{"Rust compiler   Not found", "Optional - CLI not found", "(none yet)", "install Rust from https://rustup.rs"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Tip:") {
		t.Errorf("no demo tip without Rust:\n%s", out)
	}
}

func TestRenderDoctorExecutionFailureGuidance(t *testing.T) {
	out := RenderDoctor(CheckEnvironment(fakeProbe("windows", map[string]fakeTool{
		"rustc":  {output: "rustc 1.80.0"},
		"python": {output: "Python was not found; run without arguments to install from the Microsoft Store", err: errors.New("exit 9009")},
	})))
	for _, want := range []string{"Found but could not run", "repair or reinstall Python 3", "Microsoft Store"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}
