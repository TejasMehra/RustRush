package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"
)

type result struct {
	DurationNS      int64  `json:"duration_ns"`
	OutputBase64    string `json:"output_base64"`
	ClockResolution uint64 `json:"clock_resolution_ns"`
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--resolution" {
		resolution := clockResolution()
		if resolution == 0 {
			fmt.Fprintln(os.Stderr, "monotonic clock resolution could not be measured")
			os.Exit(1)
		}
		fmt.Printf("%d\n", resolution)
		return
	}
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "a workload executable is required")
		os.Exit(2)
	}
	resolution := clockResolution()
	if resolution == 0 {
		fmt.Fprintln(os.Stderr, "monotonic clock resolution could not be measured")
		os.Exit(1)
	}
	workDir, err := os.MkdirTemp("/tmp", "rustrush-run-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer os.RemoveAll(workDir)

	command := exec.Command(os.Args[1], os.Args[2:]...)
	command.Dir = workDir
	command.Env = []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"HOME=" + workDir,
		"TMPDIR=" + workDir,
	}
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	start := time.Now()
	err = command.Run()
	elapsed := time.Since(start)
	if _, writeErr := os.Stderr.Write(stderr.Bytes()); writeErr != nil {
		fmt.Fprintln(os.Stderr, writeErr)
		os.Exit(1)
	}
	if elapsed <= 0 {
		fmt.Fprintln(os.Stderr, "monotonic workload duration was not positive")
		os.Exit(1)
	}
	payload := result{
		DurationNS:      int64(elapsed),
		OutputBase64:    base64.StdEncoding.EncodeToString(stdout.Bytes()),
		ClockResolution: resolution,
	}
	if payload.ClockResolution == 0 {
		fmt.Fprintln(os.Stderr, "monotonic clock resolution could not be measured")
		os.Exit(1)
	}
	if err := json.NewEncoder(os.Stdout).Encode(payload); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			os.Exit(exitError.ExitCode())
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func clockResolution() uint64 {
	var best time.Duration
	previous := time.Now()
	for i := 0; i < 10_000; i++ {
		current := time.Now()
		if delta := current.Sub(previous); delta > 0 && (best == 0 || delta < best) {
			best = delta
		}
		previous = current
	}
	for i := 0; best == 0 && i < 32; i++ {
		time.Sleep(time.Millisecond)
		current := time.Now()
		if delta := current.Sub(previous); delta > 0 && (best == 0 || delta < best) {
			best = delta
		}
		previous = current
	}
	if best <= 0 {
		return 0
	}
	return uint64(best)
}
