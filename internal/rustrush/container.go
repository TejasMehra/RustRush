package rustrush

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	IsolationProcess   = "process"
	IsolationContainer = "container"

	DefaultContainerGoImage   = "golang:1.24"
	DefaultContainerRustImage = "rust:1"
	DefaultContainerMemory    = "512m"
	DefaultContainerCPUs      = "1"
	DefaultContainerPids      = 256

	containerSrc = "/rr/src"
	containerOut = "/rr/out"
	containerBin = "/rr/bin"
	containerIn  = "/rr/input"
)

var (
	imagePattern  = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/:@-]{0,199}$`)
	memoryPattern = regexp.MustCompile(`^[1-9][0-9]{0,5}[mMgG]$`)
	cpusPattern   = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?$`)
	namePattern   = regexp.MustCompile(`^rustrush-[0-9a-f]{16}$`)
)

// ContainerConfig selects the images and resource limits for container isolation.
// Images are never pulled implicitly; they must already exist locally.
type ContainerConfig struct {
	GoImage   string
	RustImage string
	Memory    string
	CPUs      string
	PidsLimit int
}

func (c ContainerConfig) withDefaults() ContainerConfig {
	if c.GoImage == "" {
		c.GoImage = DefaultContainerGoImage
	}
	if c.RustImage == "" {
		c.RustImage = DefaultContainerRustImage
	}
	if c.Memory == "" {
		c.Memory = DefaultContainerMemory
	}
	if c.CPUs == "" {
		c.CPUs = DefaultContainerCPUs
	}
	if c.PidsLimit == 0 {
		c.PidsLimit = DefaultContainerPids
	}
	return c
}

func (c ContainerConfig) validate() error {
	for label, image := range map[string]string{"Go image": c.GoImage, "Rust image": c.RustImage} {
		if !imagePattern.MatchString(image) {
			return fmt.Errorf("invalid %s %q", label, image)
		}
	}
	if !memoryPattern.MatchString(c.Memory) {
		return fmt.Errorf("invalid container memory limit %q; use a value such as 512m or 1g", c.Memory)
	}
	if cpus, err := strconv.ParseFloat(c.CPUs, 64); !cpusPattern.MatchString(c.CPUs) || err != nil || cpus <= 0 || cpus > 64 {
		return fmt.Errorf("invalid container CPU limit %q; use a positive number up to 64", c.CPUs)
	}
	if c.PidsLimit < 16 || c.PidsLimit > 4096 {
		return fmt.Errorf("invalid container pids limit %d; use 16 to 4096", c.PidsLimit)
	}
	return nil
}

type containerMount struct {
	Source, Target string
	ReadOnly       bool
}

type containerSpec struct {
	Name    string
	Image   string
	User    string
	Mounts  []containerMount
	Env     []string
	Workdir string
	Command []string
	// TmpfsExec allows executing from /tmp; only builds need it.
	TmpfsExec bool
	// Detach starts a long-lived container; used for steady-state measurement.
	Detach bool
	Config ContainerConfig
}

// containerArgs returns the docker CLI arguments for a hardened, network-less run.
// The security-relevant flags are fixed here and are not caller-configurable.
func containerArgs(spec containerSpec) ([]string, error) {
	if !namePattern.MatchString(spec.Name) {
		return nil, fmt.Errorf("invalid container name %q", spec.Name)
	}
	if !imagePattern.MatchString(spec.Image) {
		return nil, fmt.Errorf("invalid image %q", spec.Image)
	}
	if spec.User == "" || strings.HasPrefix(spec.User, "0:") || spec.User == "0" || strings.HasPrefix(spec.User, "root") {
		return nil, errors.New("container user must be an explicit non-root uid:gid")
	}
	if len(spec.Command) == 0 {
		return nil, errors.New("container command is required")
	}
	tmpfs := "/tmp:rw,nosuid,noexec,size=256m"
	if spec.TmpfsExec {
		tmpfs = "/tmp:rw,nosuid,exec,size=512m"
	}
	args := []string{"run"}
	if spec.Detach {
		args = append(args, "--detach")
	}
	args = append(args,
		"--rm", "--name", spec.Name, "--pull", "never",
		"--network", "none",
		"--cap-drop", "ALL",
		"--security-opt", "no-new-privileges",
		"--read-only",
		"--user", spec.User,
		"--memory", spec.Config.Memory, "--memory-swap", spec.Config.Memory,
		"--cpus", spec.Config.CPUs,
		"--pids-limit", strconv.Itoa(spec.Config.PidsLimit),
		"--ulimit", "nofile=1024:1024",
		"--tmpfs", tmpfs,
		"--workdir", spec.Workdir,
	)
	for _, mount := range spec.Mounts {
		if !filepath.IsAbs(mount.Source) || strings.ContainsAny(mount.Source, ",\n\r\x00") {
			return nil, fmt.Errorf("unsafe mount source %q", filepath.Base(mount.Source))
		}
		lower := strings.ToLower(filepath.ToSlash(mount.Source))
		if strings.Contains(lower, "docker.sock") || strings.Contains(lower, "docker_engine") || strings.Contains(lower, "/var/run") {
			return nil, errors.New("refusing to mount a container runtime socket")
		}
		if !strings.HasPrefix(mount.Target, "/rr/") {
			return nil, fmt.Errorf("unsafe mount target %q", mount.Target)
		}
		value := "type=bind,src=" + mount.Source + ",dst=" + mount.Target
		if mount.ReadOnly {
			value += ",readonly"
		}
		args = append(args, "--mount", value)
	}
	for _, env := range spec.Env {
		args = append(args, "--env", env)
	}
	args = append(args, spec.Image)
	return append(args, spec.Command...), nil
}

type containerExecFn func(ctx context.Context, args []string) (stdout, stderr string, err error)

type containerEngine struct {
	config        ContainerConfig
	user          string
	exec          containerExecFn
	serverVersion string
	goImageID     string
	rustImageID   string
}

func dockerEnvironment() []string {
	env := minimalEnvironment()
	for _, key := range []string{"DOCKER_HOST", "DOCKER_CONFIG", "DOCKER_CONTEXT", "XDG_RUNTIME_DIR", "APPDATA", "LOCALAPPDATA"} {
		if value, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+value)
		}
	}
	return env
}

func defaultContainerExec(dockerPath string) containerExecFn {
	return func(ctx context.Context, args []string) (string, string, error) {
		command := exec.CommandContext(ctx, dockerPath, args...)
		command.Env = dockerEnvironment()
		var stdout, stderr bytes.Buffer
		command.Stdout, command.Stderr = &stdout, &stderr
		err := command.Run()
		return stdout.String(), stderr.String(), err
	}
}

func containerUser() (string, error) {
	uid := os.Getuid()
	switch {
	case uid == 0:
		return "", errors.New("container isolation refuses to run as the host root user; use an unprivileged account")
	case uid > 0:
		return fmt.Sprintf("%d:%d", uid, os.Getgid()), nil
	default:
		return "65534:65534", nil
	}
}

// newContainerEngine verifies that the container runtime and both images are
// available. Any failure is returned so callers fail closed instead of falling
// back to running on the host.
func newContainerEngine(ctx context.Context, config ContainerConfig, lookPath func(string) (string, error), execFn containerExecFn) (*containerEngine, error) {
	config = config.withDefaults()
	if err := config.validate(); err != nil {
		return nil, err
	}
	user, err := containerUser()
	if err != nil {
		return nil, err
	}
	if execFn == nil {
		if lookPath == nil {
			lookPath = exec.LookPath
		}
		dockerPath, err := lookPath("docker")
		if err != nil {
			return nil, errors.New("container isolation requested but the docker CLI was not found on PATH; refusing to fall back to local execution")
		}
		execFn = defaultContainerExec(dockerPath)
	}
	engine := &containerEngine{config: config, user: user, exec: execFn}
	probeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	stdout, stderr, err := execFn(probeCtx, []string{"version", "--format", "{{.Server.Version}}"})
	if err != nil || strings.TrimSpace(stdout) == "" {
		return nil, fmt.Errorf("container isolation requested but the docker daemon is unavailable (%s); refusing to fall back to local execution", firstLine(stderr, err))
	}
	engine.serverVersion = strings.TrimSpace(stdout)
	for _, image := range []struct {
		name string
		id   *string
	}{{config.GoImage, &engine.goImageID}, {config.RustImage, &engine.rustImageID}} {
		out, errOut, err := execFn(probeCtx, []string{"image", "inspect", "--format", "{{.Id}}", image.name})
		if err != nil || strings.TrimSpace(out) == "" {
			return nil, fmt.Errorf("container image %q is not available locally (%s); pull it first, since RustRush never pulls images implicitly", image.name, firstLine(errOut, err))
		}
		*image.id = strings.TrimSpace(out)
	}
	return engine, nil
}

func firstLine(text string, err error) string {
	text = strings.TrimSpace(text)
	if text == "" && err != nil {
		text = err.Error()
	}
	if index := strings.IndexAny(text, "\r\n"); index >= 0 {
		text = text[:index]
	}
	if len(text) > 200 {
		text = text[:200]
	}
	return text
}

func newContainerName() (string, error) {
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate container name: %w", err)
	}
	return "rustrush-" + hex.EncodeToString(random), nil
}

// run executes one hardened container and force-removes it if the context ends.
func (e *containerEngine) run(ctx context.Context, spec containerSpec) (string, string, error) {
	name, err := newContainerName()
	if err != nil {
		return "", "", err
	}
	spec.Name, spec.User, spec.Config = name, e.user, e.config
	args, err := containerArgs(spec)
	if err != nil {
		return "", "", err
	}
	stdout, stderr, runErr := e.exec(ctx, args)
	if ctx.Err() != nil {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _, _ = e.exec(cleanupCtx, []string{"rm", "--force", name})
		return stdout, stderr, ctx.Err()
	}
	return stdout, stderr, runErr
}

func (e *containerEngine) buildGo(ctx context.Context, srcDir, outDir string) (string, error) {
	stdout, stderr, err := e.run(ctx, containerSpec{
		Image: e.config.GoImage, Workdir: containerSrc, TmpfsExec: true,
		Mounts: []containerMount{{srcDir, containerSrc, true}, {outDir, containerOut, false}},
		Env: []string{"HOME=/tmp", "GOCACHE=/tmp/gocache", "GOFLAGS=-mod=readonly", "GOPROXY=off",
			"GOTOOLCHAIN=local", "CGO_ENABLED=0", "GOWORK=off"},
		Command: []string{"go", "build", "-buildvcs=false", "-o", containerOut + "/baseline", "."},
	})
	return stdout + stderr, err
}

func (e *containerEngine) buildRust(ctx context.Context, srcDir, outDir string) (string, error) {
	stdout, stderr, err := e.run(ctx, containerSpec{
		Image: e.config.RustImage, Workdir: containerSrc, TmpfsExec: true,
		Mounts:  []containerMount{{srcDir, containerSrc, true}, {outDir, containerOut, false}},
		Env:     []string{"HOME=/tmp"},
		Command: []string{"rustc", "--edition=2021", "-C", "opt-level=3", containerSrc + "/candidate.rs", "-o", containerOut + "/candidate"},
	})
	return stdout + stderr, err
}

func (e *containerEngine) rustVersion(ctx context.Context) string {
	stdout, _, err := e.run(ctx, containerSpec{
		Image: e.config.RustImage, Workdir: "/tmp", Command: []string{"rustc", "--version"},
	})
	if err != nil {
		return ""
	}
	return strings.TrimSpace(stdout)
}

func (e *containerEngine) metadata() map[string]string {
	return map[string]string{
		"isolation":            "container (docker): no network, read-only root, all capabilities dropped, no-new-privileges, non-root user, memory/CPU/pids limits; shares the host kernel and is not a validated security boundary",
		"container_server":     e.serverVersion,
		"container_go_image":   e.config.GoImage + " (" + e.goImageID + ")",
		"container_rust_image": e.config.RustImage + " (" + e.rustImageID + ")",
		"container_limits":     fmt.Sprintf("memory=%s cpus=%s pids=%d", e.config.Memory, e.config.CPUs, e.config.PidsLimit),
		"container_host_os":    runtime.GOOS,
	}
}
