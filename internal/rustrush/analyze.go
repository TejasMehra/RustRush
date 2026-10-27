package rustrush

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const maxSourceFileBytes = 1 << 20

var (
	loopPattern     = regexp.MustCompile(`(?m)\b(for|while)\b`)
	jsonPattern     = regexp.MustCompile(`(?i)(encoding/json|json\.loads|JSON\.parse|serde_json|json\.stringify)`)
	databasePattern = regexp.MustCompile(`(?i)(\.(?:query|execute|exec|prepare)\s*\(|\bSELECT\s+\w+(?:\s*,\s*\w+)*\s+FROM\s+\w+)`)
	networkPattern  = regexp.MustCompile(`(?i)(net/http|requests\.(?:get|post|request)\s*\(|\bfetch\s*\(|axios\.(?:get|post|request)\s*\(|httpx\.(?:get|post|request)\s*\()`)
)

func Analyze(root string) (Analysis, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return Analysis{}, fmt.Errorf("resolve project path: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return Analysis{}, fmt.Errorf("inspect project path: %w", err)
	}
	if !info.IsDir() {
		return Analysis{}, fmt.Errorf("project path %q is not a directory", root)
	}

	result := Analysis{
		Project:   filepath.Base(absolute),
		Languages: []string{},
		Findings:  []Finding{},
	}
	languages := map[string]bool{}
	loopFiles, jsonFiles, databaseFiles, networkFiles := []string{}, []string{}, []string{}, []string{}
	err = filepath.WalkDir(absolute, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("inspect %s: %w", filepath.Base(path), walkErr)
		}
		if entry.IsDir() {
			if path != absolute && (skippedDirectory(entry.Name()) || sensitiveFilename(entry.Name())) {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&fs.ModeSymlink != 0 || sensitiveFilename(entry.Name()) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("inspect file metadata: %w", err)
		}
		if info.Size() < 0 || info.Size() > maxSourceFileBytes {
			return nil
		}
		language := languageFor(path)
		if language == "" {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("read source file %s: %w", entry.Name(), err)
		}
		contents, readErr := io.ReadAll(io.LimitReader(file, maxSourceFileBytes+1))
		closeErr := file.Close()
		if readErr != nil {
			return fmt.Errorf("read source file %s: %w", entry.Name(), readErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close source file %s: %w", entry.Name(), closeErr)
		}
		if len(contents) > maxSourceFileBytes {
			return nil
		}
		rel, err := filepath.Rel(absolute, path)
		if err != nil {
			return fmt.Errorf("make source location relative: %w", err)
		}
		rel = filepath.ToSlash(rel)
		result.FilesInspected++
		languages[language] = true
		signals := inspectPatterns(language, path, contents)
		if signals.loop {
			loopFiles = append(loopFiles, rel)
		}
		if signals.json {
			jsonFiles = append(jsonFiles, rel)
		}
		if signals.database {
			databaseFiles = append(databaseFiles, rel)
		}
		if signals.network {
			networkFiles = append(networkFiles, rel)
		}
		return nil
	})
	if err != nil {
		return Analysis{}, err
	}
	for language := range languages {
		result.Languages = append(result.Languages, language)
	}
	sort.Strings(result.Languages)
	addFinding := func(category, description string, files []string, nextStep string) {
		if len(files) == 0 {
			return
		}
		sort.Strings(files)
		location := strings.Join(files[:min(len(files), 3)], ", ")
		if len(files) > 3 {
			location += fmt.Sprintf(" (+%d files)", len(files)-3)
		}
		result.Findings = append(result.Findings, Finding{
			Category: category, Description: description, Location: location,
			Evidence:   "Static source-pattern match; runtime profile not collected",
			Confidence: "low", NextStep: nextStep,
		})
	}
	addFinding("cpu_hypothesis", "Loop constructs may contain compute-heavy work; source patterns cannot establish runtime cost.", loopFiles, "Profile representative requests and inspect algorithmic complexity before considering a language migration.")
	addFinding("serialization_hypothesis", "Serialization or JSON handling is present.", jsonFiles, "Measure serialization time and allocation pressure with representative payloads.")
	addFinding("database_hypothesis", "Database access patterns are present; query or network latency may dominate.", databaseFiles, "Measure query count and database wait time; optimize queries before evaluating a language rewrite.")
	addFinding("network_hypothesis", "External network calls are present and may dominate request latency.", networkFiles, "Measure dependency latency separately; a Rust rewrite may not reduce remote wait time.")
	return result, nil
}

type sourceSignals struct {
	loop     bool
	json     bool
	database bool
	network  bool
}

func inspectPatterns(language, path string, contents []byte) sourceSignals {
	if language != "Go" {
		return sourceSignals{
			loop: loopPattern.Match(contents), json: jsonPattern.Match(contents),
			database: databasePattern.Match(contents), network: networkPattern.Match(contents),
		}
	}

	file, err := parser.ParseFile(token.NewFileSet(), path, contents, 0)
	if err != nil {
		return sourceSignals{
			loop: loopPattern.Match(contents), json: jsonPattern.Match(contents),
			database: databasePattern.Match(contents), network: networkPattern.Match(contents),
		}
	}
	signals := sourceSignals{}
	aliases := make(map[string]string)
	for _, spec := range file.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		name := filepath.Base(importPath)
		if spec.Name != nil {
			name = spec.Name.Name
		}
		aliases[name] = importPath
		switch {
		case importPath == "encoding/json":
			signals.json = true
		case strings.Contains(importPath, "sql") || strings.Contains(importPath, "gorm"):
			signals.database = true
		case strings.Contains(importPath, "http"):
			signals.network = true
		}
	}
	ast.Inspect(file, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.ForStmt, *ast.RangeStmt:
			signals.loop = true
		case *ast.CallExpr:
			selector, ok := typed.Fun.(*ast.SelectorExpr)
			if !ok {
				break
			}
			method := strings.ToLower(selector.Sel.Name)
			switch receiver := selector.X.(type) {
			case *ast.Ident:
				if aliases[receiver.Name] == "encoding/json" && (method == "marshal" || method == "unmarshal" || method == "newencoder" || method == "newdecoder") {
					signals.json = true
				}
				if (receiver.Name == "db" || receiver.Name == "pool" || receiver.Name == "conn") &&
					(method == "query" || method == "queryrow" || method == "exec" || method == "prepare") {
					signals.database = true
				}
				if (receiver.Name == "http" || receiver.Name == "client") &&
					(method == "get" || method == "post" || method == "do" || method == "roundtrip") {
					signals.network = true
				}
			default:
				if method == "query" || method == "execute" || method == "exec" || method == "prepare" {
					signals.database = true
				}
				if method == "get" || method == "post" || method == "do" || method == "roundtrip" {
					signals.network = true
				}
			}
		}
		return true
	})
	return signals
}

func languageFor(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "Go"
	case ".rs":
		return "Rust"
	case ".py":
		return "Python"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "JavaScript"
	case ".ts", ".tsx":
		return "TypeScript"
	case ".java":
		return "Java"
	case ".cs":
		return "C#"
	case ".rb":
		return "Ruby"
	case ".php":
		return "PHP"
	default:
		return ""
	}
}

func skippedDirectory(name string) bool {
	switch strings.ToLower(name) {
	case ".git", ".hg", ".svn", "node_modules", "vendor", "target", "dist", "build", ".venv", "venv", "__pycache__", "coverage":
		return true
	default:
		return false
	}
}

func sensitiveFilename(name string) bool {
	lower := strings.ToLower(name)
	if lower == ".env" || strings.HasPrefix(lower, ".env.") || lower == "secrets" || lower == "credentials" {
		return true
	}
	if strings.Contains(lower, "secret") || strings.Contains(lower, "credential") || strings.Contains(lower, "token") {
		return true
	}
	switch strings.ToLower(filepath.Ext(lower)) {
	case ".pem", ".key", ".p12", ".pfx", ".der":
		return true
	default:
		return false
	}
}
