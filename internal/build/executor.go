package build

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
)

// Executor runs a recipe build script in a prepared environment.
type Executor interface {
	Execute(ctx context.Context, scriptPath string, env []string, outputDir string) error
}

// LocalExecutor runs build.sh via bash on the local machine.
type LocalExecutor struct {
	Stdout io.Writer // defaults to os.Stdout
	Stderr io.Writer // defaults to os.Stderr
}

// Execute runs scriptPath via bash with the given env vars appended to the
// process environment. cmd.Dir is set to the parent directory of scriptPath
// so that relative paths inside the script resolve correctly.
func (e *LocalExecutor) Execute(ctx context.Context, scriptPath string, env []string, _ string) error {
	stdout := e.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	stderr := e.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}

	cmd := exec.CommandContext(ctx, "bash", scriptPath)
	cmd.Dir = parentDir(scriptPath)
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("build script %q: %w", scriptPath, err)
	}
	return nil
}

// DryRunExecutor validates and prints what would run; never executes the script.
type DryRunExecutor struct {
	Out io.Writer // defaults to os.Stderr
}

// Execute prints scriptPath and each env var without running anything.
// Always returns nil.
func (e *DryRunExecutor) Execute(_ context.Context, scriptPath string, env []string, _ string) error {
	out := e.Out
	if out == nil {
		out = os.Stderr
	}
	fmt.Fprintf(out, "dry-run: script: %s\n", scriptPath) //nolint:errcheck
	for _, kv := range env {
		fmt.Fprintf(out, "dry-run: env:    %s\n", kv) //nolint:errcheck
	}
	return nil
}

// parentDir returns the directory containing path.
func parentDir(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return path[:i]
		}
	}
	return "."
}
