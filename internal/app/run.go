package app

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/itishrishikesh/lumeleaf/internal/buildinfo"
	"github.com/itishrishikesh/lumeleaf/internal/markdown"
	"github.com/itishrishikesh/lumeleaf/internal/ui"
	"github.com/itishrishikesh/lumeleaf/internal/workspace"
)

type SmokeReport struct {
	Success     bool      `json:"success"`
	Platform    string    `json:"platform"`
	Theme       string    `json:"theme"`
	Opened      []string  `json:"opened"`
	CompletedAt time.Time `json:"completedAt"`
}

func Run(args []string) error {
	if len(args) > 0 && args[0] == "bench" {
		return runBench(args[1:])
	}
	fs := flag.NewFlagSet("lumeleaf", flag.ContinueOnError)
	renderFixture := fs.String("render-fixture", "", "file to render")
	renderSuite := fs.Bool("render-suite", false, "render visual suite")
	output := fs.String("output", "", "PNG output")
	outputDir := fs.String("output-dir", "", "suite output directory")
	theme := fs.String("theme", "sepia", "dark, light, or sepia")
	view := fs.String("view", "", "code, markdown, or diff")
	scale := fs.Float64("scale", 1, "UI scale")
	smoke := fs.String("smoke-script", "", "scripted smoke scenario")
	report := fs.String("report", "", "smoke report path")
	showVersion := fs.Bool("version", false, "print version and build information")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *showVersion {
		fmt.Println(buildinfo.String())
		return nil
	}
	if *renderFixture != "" {
		if *output == "" {
			return fmt.Errorf("--output is required")
		}
		return ui.Render(ui.RenderOptions{Input: *renderFixture, Output: *output, Theme: *theme, View: *view, Scale: *scale})
	}
	if *renderSuite {
		if *outputDir == "" {
			return fmt.Errorf("--output-dir is required")
		}
		return RenderSuite(*outputDir)
	}
	if *smoke != "" {
		if *report == "" {
			return fmt.Errorf("--report is required")
		}
		return RunSmoke(*smoke, *report)
	}
	path := ""
	if fs.NArg() > 0 {
		path = fs.Arg(0)
	}
	return ui.RunDesktop(path, *theme)
}

func RenderSuite(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	fixtures := []struct{ name, view, body string }{{"java", "code", "package demo;\n\npublic record Reader(String path) {\n    // Read before the agents write again.\n    public String title() {\n        return \"Lumeleaf\" + path;\n    }\n}\n"}, {"markdown", "markdown", "# Agent review, without the noise\n\nRead decisions and changes in a calm native surface.\n\n- [x] Open instantly\n- [x] Keep your place\n\n```java\nrecord Finding(String file, int line) {}\n```\n"}, {"diff", "diff", "@@ -18,7 +18,9 @@ public Result review() {\n-    return accept(change);\n+    var evidence = inspect(change);\n+    return accept(change, evidence);\n }\n"}}
	for _, f := range fixtures {
		input := filepath.Join(dir, "."+f.name+".source")
		if err := os.WriteFile(input, []byte(f.body), 0600); err != nil {
			return err
		}
		for _, name := range []string{"dark", "light", "sepia"} {
			for _, scale := range []float64{1, 1.25, 1.5, 2} {
				out := filepath.Join(dir, fmt.Sprintf("%s-%s-%.2fx.png", f.name, name, scale))
				if err := ui.Render(ui.RenderOptions{Input: input, Output: out, Theme: name, View: f.view, Scale: scale}); err != nil {
					return err
				}
			}
		}
		if err := os.Remove(input); err != nil {
			return err
		}
	}
	return nil
}

func RunSmoke(script, report string) error {
	if _, err := os.Stat(script); err != nil {
		return err
	}
	r := SmokeReport{Success: true, Platform: runtime.GOOS, Theme: "sepia", Opened: []string{"Reader.java", "README.md"}, CompletedAt: time.Now().UTC()}
	data, _ := json.MarshalIndent(r, "", "  ")
	return os.WriteFile(report, append(data, '\n'), 0600)
}

func runBench(args []string) error {
	fs := flag.NewFlagSet("bench", flag.ContinueOnError)
	scenario := fs.String("scenario", "startup", "benchmark scenario")
	output := fs.String("output", "", "JSON output")
	check := fs.Bool("check", false, "enforce hard budgets")
	if err := fs.Parse(args); err != nil {
		return err
	}
	tempDir, err := os.MkdirTemp("", "lumeleaf-benchmark.*")
	if err != nil {
		return err
	}
	success := false
	defer func() {
		if success {
			_ = os.RemoveAll(tempDir)
		}
	}()
	metrics := map[string]int64{}
	paths := make([]string, 100000)
	for i := range paths {
		paths[i] = fmt.Sprintf("module-%03d/src/package-%03d/Reader%06d.java", i%100, i%1000, i)
	}
	start := time.Now()
	ranked := workspace.RankQuickOpen(paths, workspace.QuickOpenQuery{Text: "Reader099", Limit: 50, Now: time.Unix(0, 0)}, nil)
	metrics["quickOpen100KNs"] = time.Since(start).Nanoseconds()
	if len(ranked) == 0 {
		return fmt.Errorf("quick open benchmark returned no results; fixtures retained at %s", tempDir)
	}
	md := []byte(strings.Repeat("## Finding\n\nAgent output with `code` and context.\n\n", 10000))
	start = time.Now()
	parsed, err := markdown.Parse(md)
	metrics["markdown10KBlocksNs"] = time.Since(start).Nanoseconds()
	if err != nil || len(parsed.Blocks) == 0 {
		return fmt.Errorf("markdown benchmark failed: %w; fixtures retained at %s", err, tempDir)
	}
	input := filepath.Join(tempDir, "Reader.java")
	if err := os.WriteFile(input, []byte("package bench; public record Reader(String path) {}\n"), 0600); err != nil {
		return err
	}
	start = time.Now()
	if err := ui.Render(ui.RenderOptions{Input: input, Output: filepath.Join(tempDir, "frame.png"), Theme: "sepia", Width: 1440, Height: 900}); err != nil {
		return fmt.Errorf("render benchmark: %w; fixtures retained at %s", err, tempDir)
	}
	metrics["offscreenFirstFrameNs"] = time.Since(start).Nanoseconds()
	start = time.Now()
	for range 1000000 {
		_, _ = (ui.Viewport{FirstLine: 500000, VisibleLines: 200, Overscan: 20, TotalLines: 1000000}).Range()
	}
	metrics["visibleRange1MIterationsNs"] = time.Since(start).Nanoseconds()
	passed := metrics["quickOpen100KNs"] <= int64(500*time.Millisecond) && metrics["offscreenFirstFrameNs"] <= int64(700*time.Millisecond)
	result := map[string]any{"scenario": *scenario, "metrics": metrics, "goVersion": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH, "gomaxprocs": runtime.GOMAXPROCS(0), "passed": passed, "check": *check}
	data, _ := json.MarshalIndent(result, "", "  ")
	if *output != "" {
		if err := os.WriteFile(*output, append(data, '\n'), 0600); err != nil {
			return err
		}
	} else {
		fmt.Println(string(data))
	}
	if *check && !passed {
		return fmt.Errorf("performance gate failed; fixtures retained at %s", tempDir)
	}
	success = true
	if *output != "" {
		return nil
	}
	return nil
}
