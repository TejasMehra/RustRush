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
	case "guide":
		return guide(args[1:])
	case "candidate":
		return candidate(args[1:])
	case "demo":
		return demo(args[1:])
	case "report":
		return report(args[1:])
	case "compare":
		return compare(args[1:])
	case "roi":
		return roi(args[1:])
	case "__workload":
		return workload(args[1:])
	case "__json-workload":
		return jsonWorkload(args[1:])
	case "__database-fixture":
		return databaseFixture(args[1:])
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
  rustrush guide [--json] [path]
  rustrush candidate --baseline DIR --candidate FILE.rs [--input FILE] --allow-execute [--isolation process|container] [--iterations N] [--repetitions N] [--timeout D] [--out DIR]
  rustrush demo [--workload cpu|json|database-fixture] [--iterations N] [--repetitions N] [--out DIR]
  rustrush report <report.json>
  rustrush compare <report-a.json> <report-b.json>
  rustrush roi [assumption flags]

Commands:
  doctor    Check local toolchain availability
  analyze   Inspect source files without executing them
  guide     Check whether your project is supported and list candidate operations (no execution)
  candidate Benchmark your Go main package against your single-file Rust candidate (needs --allow-execute)
  demo      Run a built-in CPU, JSON, or database-fixture workload
  report    Render a saved JSON report as Markdown
  compare   Compare measured throughput from two saved reports
  roi       Model optional infrastructure and engineering cost assumptions

The demo runs without rustc. If Rust is unavailable, it records the missing
candidate and produces an INCONCLUSIVE report rather than inventing a comparison.`)
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
	help, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if help {
		return nil
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

func guide(args []string) error {
	flags := flag.NewFlagSet("guide", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	jsonOutput := flags.Bool("json", false, "write machine-readable JSON")
	help, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if help {
		return nil
	}
	root := "."
	if flags.NArg() > 1 {
		return errors.New("guide accepts at most one project path")
	}
	if flags.NArg() == 1 {
		root = flags.Arg(0)
	}
	result, err := rustrush.Guide(root)
	if err != nil {
		return err
	}
	if *jsonOutput {
		return rustrush.WriteJSON(os.Stdout, result)
	}
	fmt.Printf("Project: %s\nSupport level: %s\n", result.Project, result.Support)
	for _, projectType := range result.ProjectTypes {
		fmt.Printf("- Detected %s (%s): %s\n", projectType.Name, projectType.Marker, projectType.Support)
	}
	if len(result.MainPackages) > 0 {
		fmt.Printf("Go main packages: %v\n", result.MainPackages)
	}
	if len(result.Candidates) > 0 {
		fmt.Println("Candidate operations (static hypotheses, not measured bottlenecks):")
		for _, candidate := range result.Candidates {
			fmt.Printf("- %s (%s) loop depth %d %v: %s\n", candidate.Function, candidate.Location, candidate.LoopDepth, candidate.Signals, candidate.Notes)
		}
	}
	fmt.Println("Next steps:")
	for _, step := range result.NextSteps {
		fmt.Printf("  * %s\n", step)
	}
	fmt.Println("Limitations:")
	for _, limitation := range result.Limitations {
		fmt.Printf("  * %s\n", limitation)
	}
	return nil
}

func candidate(args []string) error {
	flags := flag.NewFlagSet("candidate", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	baseline := flags.String("baseline", "", "Go module root containing a main package that accepts --iterations")
	source := flags.String("candidate", "", "single-file dependency-free Rust candidate (.rs)")
	input := flags.String("input", "", "optional input file passed to both programs as --input")
	iterations := flags.Uint64("iterations", rustrush.DefaultIterations, "workload iterations per process")
	repetitions := flags.Int("repetitions", rustrush.DefaultRepetitions, "measurement trials per implementation")
	timeout := flags.Duration("timeout", 30*time.Second, "maximum time for each build and each process run")
	outDir := flags.String("out", "results", "directory for generated reports")
	allow := flags.Bool("allow-execute", false, "consent to building and running the supplied programs locally")
	isolation := flags.String("isolation", rustrush.IsolationProcess, "process (timeout + minimal environment, not a sandbox) or container (hardened Docker; fails closed)")
	goImage := flags.String("container-go-image", rustrush.DefaultContainerGoImage, "local Docker image with the Go toolchain (never pulled implicitly)")
	rustImage := flags.String("container-rust-image", rustrush.DefaultContainerRustImage, "local Docker image with rustc (never pulled implicitly)")
	memory := flags.String("container-memory", rustrush.DefaultContainerMemory, "container memory limit, e.g. 512m or 1g")
	cpus := flags.String("container-cpus", rustrush.DefaultContainerCPUs, "container CPU limit")
	pids := flags.Int("container-pids", rustrush.DefaultContainerPids, "container process limit")
	help, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if help {
		return nil
	}
	if flags.NArg() != 0 {
		return errors.New("candidate does not accept positional arguments")
	}
	report, err := rustrush.RunCandidate(context.Background(), rustrush.CandidateOptions{
		BaselineDir: *baseline, CandidateFile: *source, InputFile: *input,
		Iterations: *iterations, Repetitions: *repetitions, Timeout: *timeout, AllowExecute: *allow,
		Isolation: *isolation,
		Container: rustrush.ContainerConfig{GoImage: *goImage, RustImage: *rustImage, Memory: *memory, CPUs: *cpus, PidsLimit: *pids},
	})
	if err != nil {
		return err
	}
	if err := rustrush.WriteReport(*outDir, report); err != nil {
		return err
	}
	fmt.Print(rustrush.RenderMarkdown(report))
	fmt.Printf("\nReports written to %s and %s\n",
		filepath.Join(*outDir, "latest.json"), filepath.Join(*outDir, "latest.md"))
	if report.Verdict == rustrush.VerdictInconclusive {
		fmt.Println("The comparison is inconclusive; see limitations in the report.")
	}
	return nil
}

func demo(args []string) error {
	flags := flag.NewFlagSet("demo", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	workloadName := flags.String("workload", "cpu", "built-in workload: cpu, json, or database-fixture")
	iterations := flags.Uint64("iterations", rustrush.DefaultIterations, "workload iterations per process")
	repetitions := flags.Int("repetitions", rustrush.DefaultRepetitions, "measurement trials per implementation")
	outDir := flags.String("out", "results", "directory for generated reports")
	timeout := flags.Duration("timeout", 30*time.Second, "maximum time allowed for each workload process")
	help, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if help {
		return nil
	}
	if flags.NArg() != 0 {
		return errors.New("demo does not accept positional arguments")
	}
	if *iterations == 0 || *repetitions < 3 || *repetitions > 100 {
		return errors.New("iterations must be positive and repetitions must be between 3 and 100")
	}
	if *workloadName != "cpu" && *workloadName != "json" && *workloadName != "database-fixture" {
		return fmt.Errorf("unsupported workload %q; choose cpu, json, or database-fixture", *workloadName)
	}
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate RustRush executable: %w", err)
	}
	report, err := rustrush.RunDemo(context.Background(), rustrush.DemoOptions{
		Workload:    *workloadName,
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
	fmt.Printf("\nReports written to %s and %s\n",
		filepath.Join(*outDir, "latest.json"), filepath.Join(*outDir, "latest.md"))
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

func roi(args []string) error {
	flags := flag.NewFlagSet("roi", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	monthlyOperations := flags.Float64("monthly-operations", 0, "user-supplied operations per month")
	baselineCPU := flags.Float64("baseline-cpu-seconds", 0, "user-supplied CPU seconds per baseline operation")
	targetCPU := flags.Float64("target-cpu-seconds", 0, "user-supplied CPU seconds per target operation")
	cpuPrice := flags.Float64("cpu-hour-price", 0, "user-supplied infrastructure price per CPU-hour in USD")
	engineeringHours := flags.Float64("engineering-hours", 0, "estimated migration engineering hours")
	engineeringRate := flags.Float64("engineering-hourly-cost", 0, "engineering cost per hour in USD")
	maintenanceDelta := flags.Float64("monthly-maintenance-cost-delta", 0, "monthly target-minus-current maintenance cost in USD; may be negative")
	migrationCost := flags.Float64("migration-cost", 0, "one-off migration cost in USD (added to engineering cost)")
	reportPath := flags.String("report", "", "optional benchmark report JSON; target CPU time is then estimated from its measured wall-clock ratio")
	scenarios := flags.Bool("scenarios", false, "also run illustrative conservative/expected/optimistic sensitivity scenarios")
	jsonOutput := flags.Bool("json", false, "write machine-readable JSON")
	help, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if help {
		return nil
	}
	if flags.NArg() != 0 {
		return errors.New("roi does not accept positional arguments")
	}
	provided := make(map[string]bool)
	flags.Visit(func(item *flag.Flag) {
		provided[item.Name] = true
	})
	inputs := rustrush.ROIInputs{
		MonthlyOperations:              optionalFloat(*monthlyOperations, provided["monthly-operations"]),
		BaselineCPUSecondsPerOperation: optionalFloat(*baselineCPU, provided["baseline-cpu-seconds"]),
		TargetCPUSecondsPerOperation:   optionalFloat(*targetCPU, provided["target-cpu-seconds"]),
		CPUCostPerHourUSD:              optionalFloat(*cpuPrice, provided["cpu-hour-price"]),
		EngineeringHours:               optionalFloat(*engineeringHours, provided["engineering-hours"]),
		EngineeringCostPerHourUSD:      optionalFloat(*engineeringRate, provided["engineering-hourly-cost"]),
		MonthlyMaintenanceCostDeltaUSD: optionalFloat(*maintenanceDelta, provided["monthly-maintenance-cost-delta"]),
		MigrationCostUSD:               optionalFloat(*migrationCost, provided["migration-cost"]),
	}
	var linked *rustrush.Report
	if *reportPath != "" {
		loaded, err := rustrush.ReadReport(*reportPath)
		if err != nil {
			return err
		}
		linked = &loaded
	}
	var scenarioSet []rustrush.ROIScenario
	if *scenarios {
		scenarioSet = rustrush.DefaultROIScenarios()
	}
	analysis, err := rustrush.AnalyzeROI(inputs, linked, scenarioSet)
	if err != nil {
		return err
	}
	result := analysis.Result
	if *jsonOutput {
		return rustrush.WriteJSON(os.Stdout, analysis)
	}
	fmt.Println("Modeled ROI estimate; all supplied values are assumptions, not benchmark measurements.")
	printOptionalMoney("Current monthly infrastructure cost", result.CurrentMonthlyCostUSD)
	printOptionalMoney("Target monthly infrastructure cost", result.TargetMonthlyCostUSD)
	printOptionalMoney("Monthly infrastructure savings", result.MonthlyInfrastructureSavingsUSD)
	printOptionalMoney("Estimated engineering cost", result.EstimatedEngineeringCostUSD)
	printOptionalMoney("Estimated net monthly savings", result.EstimatedNetMonthlySavingsUSD)
	if result.BreakEvenMonths != nil {
		fmt.Printf("Estimated break-even: %.2f months\n", *result.BreakEvenMonths)
	} else {
		fmt.Println("Estimated break-even: not calculated")
	}
	if len(result.MissingInputs) > 0 {
		fmt.Printf("Missing inputs: %v\n", result.MissingInputs)
	}
	fmt.Printf("Claim: %s (target CPU source: %s)\n", analysis.Claim, analysis.TargetSource)
	if analysis.Report != nil {
		fmt.Printf("Linked report: verdict %s, correctness %s", analysis.Report.Verdict, analysis.Report.Correctness)
		if analysis.Report.MeasuredValuesUsed {
			fmt.Printf(", measured wall-clock ratio %.4f (candidate/baseline)", analysis.Report.WallClockRatio)
		}
		fmt.Println()
	}
	for _, item := range analysis.Scenarios {
		fmt.Printf("Scenario %-12s %s", item.Scenario.Name, item.Claim)
		if item.NetMonthlySavingsUSD != nil {
			fmt.Printf(", net $%.2f/month", *item.NetMonthlySavingsUSD)
		}
		if item.BreakEvenMonths != nil {
			fmt.Printf(", break-even %.2f months", *item.BreakEvenMonths)
		}
		fmt.Println()
	}
	for _, note := range append(result.Notes, analysis.Notes...) {
		fmt.Printf("Note: %s\n", note)
	}
	return nil
}

func optionalFloat(value float64, provided bool) *float64 {
	if !provided {
		return nil
	}
	return &value
}

func printOptionalMoney(label string, value *float64) {
	if value == nil {
		fmt.Printf("%s: not calculated\n", label)
		return
	}
	fmt.Printf("%s: $%.2f\n", label, *value)
}

func parseFlags(flags *flag.FlagSet, args []string) (bool, error) {
	err := flags.Parse(args)
	if errors.Is(err, flag.ErrHelp) {
		return true, nil
	}
	return false, err
}

func workload(args []string) error {
	flags := flag.NewFlagSet("__workload", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	iterations := flags.Uint64("iterations", 0, "number of deterministic workload iterations")
	help, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if help {
		return nil
	}
	if *iterations == 0 {
		return errors.New("iterations must be positive")
	}
	fmt.Println(strconv.FormatUint(rustrush.RunWorkload(*iterations), 10))
	return nil
}

func jsonWorkload(args []string) error {
	flags := flag.NewFlagSet("__json-workload", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	fixture := flags.String("fixture", "", "path to the JSON fixture")
	iterations := flags.Uint64("iterations", 0, "number of JSON transformations")
	help, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if help {
		return nil
	}
	if *fixture == "" || *iterations == 0 {
		return errors.New("fixture path and positive iterations are required")
	}
	output, err := rustrush.RunJSONWorkload(*fixture, *iterations)
	if err != nil {
		return err
	}
	fmt.Println(output)
	return nil
}

func databaseFixture(args []string) error {
	flags := flag.NewFlagSet("__database-fixture", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	fixture := flags.String("fixture", "", "path to the local CSV fixture")
	iterations := flags.Uint64("iterations", 0, "number of filter-and-aggregate iterations")
	help, err := parseFlags(flags, args)
	if err != nil {
		return err
	}
	if help {
		return nil
	}
	if *fixture == "" || *iterations == 0 {
		return errors.New("fixture path and positive iterations are required")
	}
	output, err := rustrush.RunDatabaseFixture(*fixture, *iterations)
	if err != nil {
		return err
	}
	fmt.Println(output)
	return nil
}
