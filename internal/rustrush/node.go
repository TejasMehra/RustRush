package rustrush

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// minNodeMajor is the oldest Node.js major release accepted as a baseline runtime.
const minNodeMajor = 18

const maxNodeBaselineBytes = 2 << 20

var nodeVersionPattern = regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)(?:[-+][0-9A-Za-z.+-]+)?$`)

// normalizeBaselineLanguage maps accepted spellings to the canonical value stored in reports.
func normalizeBaselineLanguage(language string) string {
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "node", "node.js":
		return "nodejs"
	default:
		return strings.ToLower(strings.TrimSpace(language))
	}
}

func baselineLabel(language string) string {
	switch language {
	case "python":
		return "Python"
	case "nodejs":
		return "Node.js"
	default:
		return "Go"
	}
}

func validateNodeBaseline(path string) (string, error) {
	if path == "" {
		return "", errors.New("a Node.js baseline file is required (--baseline)")
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".js", ".cjs", ".mjs":
	default:
		return "", fmt.Errorf("Node.js baseline %q must be a single .js, .cjs, or .mjs file", filepath.Base(path))
	}
	if err := requireRegularFile(path, "Node.js baseline"); err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("inspect Node.js baseline: %w", err)
	}
	if info.Size() > maxNodeBaselineBytes {
		return "", fmt.Errorf("Node.js baseline %q is larger than %d bytes", filepath.Base(path), maxNodeBaselineBytes)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read Node.js baseline: %w", err)
	}
	if len(bytes.TrimSpace(content)) == 0 {
		return "", fmt.Errorf("Node.js baseline %q is empty", filepath.Base(path))
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve Node.js baseline: %w", err)
	}
	return absolute, nil
}

// nodeScriptName keeps the original extension so .mjs files stay ES modules and .cjs files CommonJS.
func nodeScriptName(path string) string {
	return "baseline" + strings.ToLower(filepath.Ext(path))
}

func findNode(lookPath func(string) (string, error)) (string, error) {
	names := []string{"node", "nodejs"}
	if runtime.GOOS == "windows" {
		names = []string{"node"}
	}
	for _, name := range names {
		if path, err := lookPath(name); err == nil && path != "" {
			return path, nil
		}
	}
	return "", fmt.Errorf("a Node.js %d or newer interpreter was not found on PATH; install Node.js LTS and rerun", minNodeMajor)
}

// parseNodeVersion validates `node --version` output such as "v20.15.1".
func parseNodeVersion(output string) (string, error) {
	version := strings.TrimSpace(output)
	match := nodeVersionPattern.FindStringSubmatch(version)
	if match == nil {
		return "", fmt.Errorf("Node.js baseline requires a Node.js %d+ runtime; interpreter reported %q", minNodeMajor, version)
	}
	major, err := strconv.Atoi(match[1])
	if err != nil || major < minNodeMajor {
		return "", fmt.Errorf("Node.js baseline requires Node.js %d or newer; interpreter reported %q", minNodeMajor, version)
	}
	return "Node.js " + version, nil
}

// nodeEnvironment is the minimal environment plus settings that keep runs deterministic and quiet.
// NODE_OPTIONS and NODE_PATH are deliberately not inherited.
func nodeEnvironment() []string {
	return append(minimalEnvironment(), "NODE_NO_WARNINGS=1", "NO_COLOR=1")
}
