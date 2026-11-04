package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/TejasMehra/RustRush/internal/rustrush"
)

func probeWith(tools map[string]string) rustrush.DoctorProbe {
	return rustrush.DoctorProbe{
		GOOS:    "linux",
		Timeout: time.Second,
		LookPath: func(name string) (string, error) {
			if _, ok := tools[name]; ok {
				return name, nil
			}
			return "", errors.New("not found")
		},
		Run: func(_ context.Context, path string, _ ...string) (string, error) {
			if tools[path] == "FAIL" {
				return "", errors.New("cannot execute")
			}
			return tools[path], nil
		},
	}
}

func TestDoctorRejectsUnexpectedArguments(t *testing.T) {
	tests := [][]string{
		{"doctor", "extra"},
		{"doctor", "one", "two"},
		{"doctor", "--bogus"},
		{"doctor", "extra", "--bogus"},
	}
	for _, args := range tests {
		err := run(args)
		if err == nil || !strings.Contains(err.Error(), "doctor takes no arguments") {
			t.Errorf("run(%v) = %v, want usage error", args, err)
		}
	}
}

func TestRunDoctorExitBehaviour(t *testing.T) {
	tests := []struct {
		name    string
		tools   map[string]string
		wantErr bool
	}{
		{"rust only", map[string]string{"rustc": "rustc 1.80.0"}, false},
		{"rust and runtimes", map[string]string{"rustc": "rustc 1.80.0", "go": "go version go1.22 x", "python3": "Python 3.12.0", "node": "v20.0.0", "docker": ""}, false},
		{"rust, no docker", map[string]string{"rustc": "rustc 1.80.0", "go": "go version go1.22 x"}, false},
		{"rust missing", map[string]string{"go": "go version go1.22 x", "python3": "Python 3.12.0", "node": "v20.0.0", "docker": ""}, true},
		{"rust cannot run", map[string]string{"rustc": "FAIL", "go": "go version go1.22 x"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			err := runDoctor(&out, probeWith(tt.tools))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !strings.Contains(out.String(), "RustRush environment check") {
				t.Fatalf("diagnostics must print even on failure:\n%s", out.String())
			}
		})
	}
}
