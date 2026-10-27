package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/TejasMehra/RustRush/internal/rustrush"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "rustrush:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		printUsage(os.Stdout)
		return nil
	}

	switch args[0] {
	case "doctor":
		return doctor()
	case "analyze":
		return analyze(args[1:])
	case "demo":
		return demo(args[1:])
	case "report":
		return report(args[1:])
	case "compare":
		return compare(args[1:])
	case "__workload":
		return workload(args[1:])
	default:
		printUsage(os.Stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func printUsage(w *os.File) {
	fmt.Fprintln(w, `RustRush — measure whether a Rust rewrite is worth exploring

Usage:
  rustrush doctor
  rustrush analyze [path]
  rustrush demo [--iterations N] [--repetitions N] [--out DIR]
  rustrush report <report.json>
  rustrush compare <report-a.json> <report-b.json>

Commands:
  doctor    Check local toolchain availability
  analyze   Inspect source files without executing them
  demo      Run the built-in CPU workload and write JSON/Markdown reports
  report    Render a saved JSON report as Markdown
  compare   Compare measured throughput from two saved reports

The demo needs Go. If rustc is unavailable, it records the missing candidate
and produces an INCONCLUSIVE report rather than inventing a comparison.`)
}

func doctor() error {
	toolchain := rustrush.CheckToolchain()
	fmt.Printf("Go runtime: %s (%s/%s)\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)
	fmt.Printf("Rust compiler: %s\n", available(toolchain.Rustc, "not found"))
	fmt.Printf("Go compiler: %s\n", available(toolchain.Go, "not found"))
	return nil
}

func available(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func analyze(args []string) error {
	flags := flag.NewFlagSet("analyze", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	jsonOutput := flags.Bool("json", false, "write machine-readable JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	root := "."
	if flags.NArg() > 0 {
		root = flags.Arg(0)
	}
	result, err := rustrush.Analyze(root)
	if err != nil {
		return err
	}
	if *jsonOutput {
		return rustrush.WriteJSON(os.Stdout, result)
	}
	fmt.Printf("Project: %s\nFiles inspected: %d\n", result.Project, result.FilesInspected)
	if len(result.Languages) == 0 {
		fmt.Println("Languages: none detected")
	} else {
		fmt.Printf("Languages: %v\n", result.Languages)
	}
	if len(result.Findings) == 0 {
		fmt.Println("Static hypotheses: none found")
	}
	for _, finding := range result.Findings {
		fmt.Printf("- [%s] %s (%s): %s\n", finding.Category, finding.Description, finding.Location, finding.NextStep)
	}
	fmt.Println("Note: static findings are hypotheses, not measured bottlenecks.")
	return nil
}

func demo(args []string) error {
	flags := flag.NewFlagSet("demo", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	iterations := flags.Uint64("iterations", rustrush.DefaultIterations, "workload iterations per process")
	repetitions := flags.Int("repetitions", rustrush.DefaultRepetitions, "measurement trials per implementation")
	outDir := flags.String("out", "results", "directory for generated reports")
	timeout := flags.Duration("timeout", 30*time.Second, "maximum time allowed for each workload process")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("demo does not accept positional arguments")
	}
	if *iterations == 0 || *repetitions < 3 || *repetitions > 100 {
		return errors.New("iterations must be positive and repetitions must be between 3 and 100")
	}
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate RustRush executable: %w", err)
	}
	report, err := rustrush.RunDemo(context.Background(), rustrush.DemoOptions{
		Iterations:  *iterations,
		Repetitions: *repetitions,
		Timeout:     *timeout,
		Executable:  executable,
		SourceRoot:  filepath.Clean("."),
	})
	if err != nil {
		return err
	}
	if err := rustrush.WriteReport(*outDir, report); err != nil {
		return err
	}
	fmt.Print(rustrush.RenderMarkdown(report))
	fmt.Printf("\nReports written to %s\\latest.json and %s\\latest.md\n", *outDir, *outDir)
	if report.Verdict == rustrush.VerdictInconclusive {
		fmt.Println("The comparison is inconclusive; see limitations in the report.")
	}
	return nil
}

func report(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: rustrush report <report.json>")
	}
	value, err := rustrush.ReadReport(args[0])
	if err != nil {
		return err
	}
	fmt.Print(rustrush.RenderMarkdown(value))
	return nil
}

func compare(args []string) error {
	if len(args) != 2 {
		return errors.New("usage: rustrush compare <report-a.json> <report-b.json>")
	}
	first, err := rustrush.ReadReport(args[0])
	if err != nil {
		return err
	}
	second, err := rustrush.ReadReport(args[1])
	if err != nil {
		return err
	}
	fmt.Print(rustrush.CompareReports(first, second))
	return nil
}

func workload(args []string) error {
	flags := flag.NewFlagSet("__workload", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	iterations := flags.Uint64("iterations", 0, "number of deterministic workload iterations")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *iterations == 0 {
		return errors.New("iterations must be positive")
	}
	fmt.Println(strconv.FormatUint(rustrush.RunWorkload(*iterations), 10))
	return nil
}
