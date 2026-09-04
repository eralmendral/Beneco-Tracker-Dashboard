package scrape

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type Runner struct {
	pythonPath  string
	scriptPath  string
	outputDir   string
	serverURL   string
	ingestToken string
}

func NewRunner(serverURL, ingestToken string) *Runner {
	return &Runner{
		pythonPath:  configuredPythonPath(),
		scriptPath:  configuredScriptPath(),
		outputDir:   configuredOutputDir(),
		serverURL:   serverURL,
		ingestToken: ingestToken,
	}
}

func (r *Runner) Run(ctx context.Context) error {
	command := exec.CommandContext(
		ctx,
		r.pythonPath,
		r.scriptPath,
		"--output-dir",
		r.outputDir,
	)
	command.Env = append(os.Environ(),
		"SERVER_URL="+r.serverURL,
		"INGEST_TOKEN="+r.ingestToken,
	)
	output, err := command.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			return fmt.Errorf("run scraper: %w", err)
		}
		return fmt.Errorf("run scraper: %w: %s", err, message)
	}
	return nil
}

func configuredPythonPath() string {
	if configured := strings.TrimSpace(os.Getenv("SCRAPER_PYTHON")); configured != "" {
		return configured
	}
	candidates := []string{"python3", "python"}
	if runtime.GOOS == "windows" {
		candidates = []string{"python", "python3"}
	}
	for _, candidate := range candidates {
		if path, err := exec.LookPath(candidate); err == nil {
			return path
		}
	}
	return candidates[0]
}

func configuredScriptPath() string {
	if configured := strings.TrimSpace(os.Getenv("SCRAPER_SCRIPT")); configured != "" {
		return configured
	}
	for _, candidate := range []string{
		"/app/scripts/scrape_beneco.py",
		filepath.Join("..", "scripts", "scrape_beneco.py"),
		filepath.Join("scripts", "scrape_beneco.py"),
	} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return "/app/scripts/scrape_beneco.py"
}

func configuredOutputDir() string {
	if configured := strings.TrimSpace(os.Getenv("SCRAPER_OUTPUT_DIR")); configured != "" {
		return configured
	}
	return filepath.Join(os.TempDir(), "beneco-snapshots")
}
