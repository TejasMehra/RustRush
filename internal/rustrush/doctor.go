package rustrush

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// ToolState is the beginner-facing state of one dependency.
type ToolState string

const (
	ToolReady       ToolState = "ready"
	ToolNotFound    ToolState = "not_found"
	ToolNeedsNewer  ToolState = "needs_newer_version"
	ToolCouldNotRun ToolState = "could_not_run"
)

// ToolStatus is the result of checking one dependency. Detail holds the
// detected version when Ready, or the reason otherwise.
type ToolStatus struct {
	State  ToolState
	Detail string
}

// DoctorReport is the outcome of CheckEnvironment. It is separate from
// Toolchain so the existing Toolchain JSON shape is untouched.
type DoctorReport struct {
	GOOS   string
	Rust   ToolStatus
	Go     ToolStatus
	Python ToolStatus
	Node   ToolStatus
	// Docker only records whether the CLI exists; the daemon and images are
	// verified later, and fail closed, when --isolation container is used.
	Docker ToolStatus
}

// DoctorProbe lets tests replace the host lookups.
type DoctorProbe struct {
	LookPath func(string) (string, error)
	Run      func(ctx context.Context, path string, args ...string) (string, error)
	GOOS     string
	Timeout  time.Duration
}

func DefaultDoctorProbe() DoctorProbe {
	return DoctorProbe{
		LookPath: exec.LookPath,
		Run: func(ctx context.Context, path string, args ...string) (string, error) {
			output, err := exec.CommandContext(ctx, path, args...).CombinedOutput()
			return string(output), err
		},
		GOOS:    runtime.GOOS,
		Timeout: 3 * time.Second,
	}
}

// CheckEnvironment checks every dependency without running user code, starting
// containers, or pulling images.
func CheckEnvironment(probe DoctorProbe) DoctorReport {
	if probe.Timeout <= 0 {
		probe.Timeout = 3 * time.Second
	}
	return DoctorReport{
		GOOS:   probe.GOOS,
		Rust:   checkRust(probe),
		Go:     checkGo(probe),
		Python: checkPython(probe),
		Node:   checkNode(probe),
		Docker: checkDocker(probe),
	}
}

// runVersion runs a version command and returns its trimmed output, or a
// short failure reason.
func runVersion(probe DoctorProbe, path string, args ...string) (string, string) {
	ctx, cancel := context.WithTimeout(context.Background(), probe.Timeout)
	defer cancel()
	output, err := probe.Run(ctx, path, args...)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
			return "", "timed out"
		}
		return "", firstLine(output, err)
	}
	output = strings.TrimSpace(output)
	if output == "" {
		return "", "it printed no version"
	}
	return firstLine(output, nil), ""
}

func checkRust(probe DoctorProbe) ToolStatus {
	path, err := probe.LookPath("rustc")
	if err != nil || path == "" {
		return ToolStatus{State: ToolNotFound}
	}
	version, failure := runVersion(probe, path, "--version")
	if failure != "" {
		return ToolStatus{State: ToolCouldNotRun, Detail: failure}
	}
	return ToolStatus{State: ToolReady, Detail: version}
}

func checkGo(probe DoctorProbe) ToolStatus {
	path, err := probe.LookPath("go")
	if err != nil || path == "" {
		return ToolStatus{State: ToolNotFound}
	}
	version, failure := runVersion(probe, path, "version")
	if failure != "" {
		return ToolStatus{State: ToolCouldNotRun, Detail: failure}
	}
	if fields := strings.Fields(version); len(fields) >= 3 && fields[0] == "go" && fields[1] == "version" {
		version = fields[2]
	}
	return ToolStatus{State: ToolReady, Detail: version}
}

// checkPython inspects only the interpreter that candidate runs would use
// (findPython), so Ready never overstates what a benchmark would do.
func checkPython(probe DoctorProbe) ToolStatus {
	path, err := findPythonFor(probe.GOOS, probe.LookPath)
	if err != nil {
		return ToolStatus{State: ToolNotFound}
	}
	version, failure := runVersion(probe, path, "--version")
	if failure != "" {
		return ToolStatus{State: ToolCouldNotRun, Detail: failure}
	}
	switch {
	case strings.HasPrefix(version, "Python 3."):
		return ToolStatus{State: ToolReady, Detail: version}
	case strings.HasPrefix(version, "Python "):
		return ToolStatus{State: ToolNeedsNewer, Detail: "Python 3 is required; found " + version}
	}
	return ToolStatus{State: ToolCouldNotRun, Detail: "it did not report a Python version"}
}

func checkNode(probe DoctorProbe) ToolStatus {
	path, err := findNode(probe.LookPath)
	if err != nil {
		return ToolStatus{State: ToolNotFound}
	}
	version, failure := runVersion(probe, path, "--version")
	if failure != "" {
		return ToolStatus{State: ToolCouldNotRun, Detail: failure}
	}
	name, parseErr := parseNodeVersion(version)
	if parseErr == nil {
		return ToolStatus{State: ToolReady, Detail: name}
	}
	if nodeVersionPattern.MatchString(version) {
		return ToolStatus{State: ToolNeedsNewer, Detail: fmt.Sprintf("Node.js %d or newer is required; found %s", minNodeMajor, version)}
	}
	return ToolStatus{State: ToolCouldNotRun, Detail: "it did not report a Node.js version"}
}

func checkDocker(probe DoctorProbe) ToolStatus {
	if path, err := probe.LookPath("docker"); err != nil || path == "" {
		return ToolStatus{State: ToolNotFound}
	}
	return ToolStatus{State: ToolReady}
}

// CanCompare reports whether any Rust comparison, including the built-in demo,
// can run. Missing optional runtimes never make this false.
func (r DoctorReport) CanCompare() bool {
	return r.Rust.State == ToolReady
}

// Comparison is one thing the user may want to run.
type Comparison struct {
	Name  string
	Ready bool
	// Fix says what to install when the comparison is unavailable.
	Fix string
}

// Comparisons lists the demo and each baseline language. A comparison is Ready
// only when Rust and, where needed, that language's runtime are Ready. The
// built-in demo uses RustRush itself as its baseline, so it needs only Rust.
func (r DoctorReport) Comparisons() []Comparison {
	rustFix := ""
	if r.Rust.State != ToolReady {
		rustFix = installAdvice("Rust", r.Rust.State, "https://rustup.rs")
	}
	build := func(name string, runtime ToolStatus, runtimeFix string) Comparison {
		var fixes []string
		if rustFix != "" {
			fixes = append(fixes, rustFix)
		}
		if runtimeFix != "" && runtime.State != ToolReady {
			fixes = append(fixes, runtimeFix)
		}
		return Comparison{Name: name, Ready: len(fixes) == 0, Fix: strings.Join(fixes, " and ")}
	}
	pythonFix := installAdvice("Python 3", r.Python.State, "https://www.python.org/downloads/")
	if r.Python.State == ToolCouldNotRun && r.GOOS == "windows" {
		pythonFix += " (on Windows the Microsoft Store \"python\" shortcut can cause this; turn it off under App execution aliases or install Python from python.org)"
	}
	return []Comparison{
		build("Built-in demo", ToolStatus{State: ToolReady}, ""),
		build("Go vs Rust", r.Go, installAdvice("Go", r.Go.State, "https://go.dev/dl/")),
		build("Python vs Rust", r.Python, pythonFix),
		build("Node.js vs Rust", r.Node, installAdvice(fmt.Sprintf("Node.js %d or newer", minNodeMajor), r.Node.State, "https://nodejs.org")),
	}
}

func installAdvice(name string, state ToolState, url string) string {
	if state == ToolCouldNotRun {
		return fmt.Sprintf("repair or reinstall %s from %s", name, url)
	}
	return fmt.Sprintf("install %s from %s", name, url)
}

func (s ToolStatus) text() string {
	switch s.State {
	case ToolReady:
		if s.Detail != "" {
			return "Ready (" + s.Detail + ")"
		}
		return "Ready"
	case ToolNeedsNewer:
		return "Needs newer version (" + s.Detail + ")"
	case ToolCouldNotRun:
		return "Found but could not run (" + s.Detail + ")"
	}
	return "Not found"
}

// RenderDoctor produces the beginner-facing environment check.
func RenderDoctor(r DoctorReport) string {
	var b strings.Builder
	b.WriteString("RustRush environment check\n\n")
	row := func(name, status string) { fmt.Fprintf(&b, "%-16s%s\n", name, status) }
	row("Rust compiler", r.Rust.text())
	row("Go", r.Go.text())
	row("Python", r.Python.text())
	row("Node.js", r.Node.text())
	docker := "Optional - CLI not found (only needed for --isolation container)"
	if r.Docker.State == ToolReady {
		docker = "Optional - CLI found; container mode also needs a running Docker and local images, which are not checked here"
	}
	row("Docker", docker)

	var available, unavailable []string
	for _, comparison := range r.Comparisons() {
		if comparison.Ready {
			available = append(available, "  - "+comparison.Name+"\n")
		} else {
			unavailable = append(unavailable, "  - "+comparison.Name+": "+comparison.Fix+".\n")
		}
	}
	b.WriteString("\nComparisons available:\n")
	if len(available) == 0 {
		b.WriteString("  (none yet)\n")
	}
	for _, line := range available {
		b.WriteString(line)
	}
	if len(unavailable) > 0 {
		b.WriteString("\nNot available:\n")
		for _, line := range unavailable {
			b.WriteString(line)
		}
	}
	if r.CanCompare() {
		b.WriteString("\nTip: run `rustrush demo` for a first comparison.\n")
	}
	return b.String()
}
