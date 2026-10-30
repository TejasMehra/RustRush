package rustrush

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

func CheckToolchain() Toolchain {
	var result Toolchain
	if path, err := exec.LookPath("go"); err == nil {
		result.Go = path
	}
	if path, err := findPython(exec.LookPath); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		output, runErr := exec.CommandContext(ctx, path, "--version").Output()
		cancel()
		if runErr == nil {
			result.Python = strings.TrimSpace(string(output))
		}
	}
	if path, err := exec.LookPath("rustc"); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		output, err := exec.CommandContext(ctx, path, "--version").Output()
		if err == nil {
			result.Rustc = strings.TrimSpace(string(output))
		}
	}
	return result
}
