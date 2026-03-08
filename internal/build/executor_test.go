package build

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalExecutor_Success(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "build.sh")
	target := filepath.Join(dir, "created.txt")
	if err := os.WriteFile(script, []byte("#!/bin/bash\ntouch "+target+"\n"), 0o755); err != nil {
		t.Fatalf("writing script: %v", err)
	}

	exec := &LocalExecutor{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}
	if err := exec.Execute(context.Background(), script, nil, dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Errorf("expected file %s to exist: %v", target, err)
	}
}

func TestLocalExecutor_ScriptFailure(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "build.sh")
	if err := os.WriteFile(script, []byte("#!/bin/bash\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("writing script: %v", err)
	}

	exec := &LocalExecutor{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}
	if err := exec.Execute(context.Background(), script, nil, dir); err == nil {
		t.Error("expected non-nil error for exit 1 script")
	}
}

func TestLocalExecutor_ContextCancelled(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "build.sh")
	if err := os.WriteFile(script, []byte("#!/bin/bash\nsleep 60\n"), 0o755); err != nil {
		t.Fatalf("writing script: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	exec := &LocalExecutor{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}
	if err := exec.Execute(ctx, script, nil, dir); err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestLocalExecutor_EnvVarsSet(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "prefix.txt")
	script := filepath.Join(dir, "build.sh")
	content := "#!/bin/bash\necho -n \"$STRATA_PREFIX\" > " + out + "\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatalf("writing script: %v", err)
	}

	exec := &LocalExecutor{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}
	if err := exec.Execute(context.Background(), script, []string{"STRATA_PREFIX=/test/path"}, dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("reading output: %v", err)
	}
	if string(data) != "/test/path" {
		t.Errorf("expected /test/path, got %q", string(data))
	}
}

func TestDryRunExecutor_NeverExecutes(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "build.sh")
	target := filepath.Join(dir, "should-not-exist.txt")
	if err := os.WriteFile(script, []byte("#!/bin/bash\ntouch "+target+"\n"), 0o755); err != nil {
		t.Fatalf("writing script: %v", err)
	}

	var buf bytes.Buffer
	exec := &DryRunExecutor{Out: &buf}
	if err := exec.Execute(context.Background(), script, nil, dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(target); err == nil {
		t.Error("DryRunExecutor must not create files")
	}
}

func TestDryRunExecutor_PrintsScript(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "build.sh")
	if err := os.WriteFile(script, []byte("#!/bin/bash\n"), 0o755); err != nil {
		t.Fatalf("writing script: %v", err)
	}

	var buf bytes.Buffer
	exec := &DryRunExecutor{Out: &buf}
	if err := exec.Execute(context.Background(), script, []string{"FOO=bar"}, dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, script) {
		t.Errorf("expected output to contain script path %q, got:\n%s", script, output)
	}
	if !strings.Contains(output, "FOO=bar") {
		t.Errorf("expected output to contain env var FOO=bar, got:\n%s", output)
	}
}
