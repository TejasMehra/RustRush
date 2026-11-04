package main

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/TejasMehra/RustRush/internal/rustrush"
)

// printBenchmarkResult prints the plain-English answer first. With verbose it
// prints the complete Markdown report instead, which already opens with the
// same answer, so the summary is never shown twice.
func printBenchmarkResult(w io.Writer, report rustrush.Report, outDir string, verbose bool) {
	jsonPath := filepath.Join(outDir, "latest.json")
	markdownPath := filepath.Join(outDir, "latest.md")
	svgPath := filepath.Join(outDir, "latest.svg")
	if verbose {
		fmt.Fprint(w, rustrush.RenderMarkdown(report))
		fmt.Fprintf(w, "\nReports written to %s, %s and %s\n", jsonPath, markdownPath, svgPath)
		if report.Verdict == rustrush.VerdictInconclusive {
			fmt.Fprintln(w, "The comparison is inconclusive; see limitations in the report.")
		}
		return
	}
	fmt.Fprint(w, rustrush.SummarizeResult(report).Text())
	fmt.Fprintf(w, "\nFull report: %s (also %s and %s)\n", markdownPath, jsonPath, svgPath)
	fmt.Fprintln(w, "Run again with --verbose to print the complete technical report here.")
}
