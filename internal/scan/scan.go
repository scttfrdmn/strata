package scan

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// ExecRunner is injectable for testing.
type ExecRunner interface {
	Output(ctx context.Context, name string, args ...string) ([]byte, error)
}

// RealExecRunner runs real OS commands.
type RealExecRunner struct{}

// Output executes name with args and returns combined output.
func (r *RealExecRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

// Scanner detects installed software from multiple sources.
type Scanner struct {
	LmodEnabled  bool
	CondaEnabled bool
	PipEnabled   bool
	FSEnabled    bool
	FSScanPaths  []string
	Exec         ExecRunner // nil → RealExecRunner
}

// NewScanner returns a Scanner with all standard sources enabled (FSEnabled=false).
func NewScanner() *Scanner {
	return &Scanner{
		LmodEnabled:  true,
		CondaEnabled: true,
		PipEnabled:   true,
		FSEnabled:    false,
		FSScanPaths:  DefaultFSScanPaths,
	}
}

func (s *Scanner) runner() ExecRunner {
	if s.Exec != nil {
		return s.Exec
	}
	return &RealExecRunner{}
}

// Scan runs all enabled sources; unavailable sources are silently skipped.
func (s *Scanner) Scan(ctx context.Context) ([]DetectedPackage, error) {
	var result []DetectedPackage

	if s.LmodEnabled {
		if pkgs, err := DetectLmod(ctx); err == nil {
			result = append(result, pkgs...)
		}
	}

	if s.CondaEnabled {
		if pkgs, err := DetectConda(ctx); err == nil {
			result = append(result, pkgs...)
		}
	}

	if s.PipEnabled {
		if pkgs, err := DetectPip(ctx, s.runner()); err == nil {
			result = append(result, pkgs...)
		}
	}

	if s.FSEnabled {
		paths := s.FSScanPaths
		if len(paths) == 0 {
			paths = DefaultFSScanPaths
		}
		if pkgs, err := DetectFilesystem(ctx, paths); err == nil {
			result = append(result, pkgs...)
		}
	}

	return result, nil
}

// osABIMap maps /etc/os-release ID values to Strata ABI strings.
// NOTE: Keep in sync with internal/probe OSABI map when new OSes are added.
var osABIMap = map[string]string{
	"amzn":      "linux-gnu-2.34",
	"rhel":      "linux-gnu-2.34",
	"centos":    "linux-gnu-2.34",
	"rocky":     "linux-gnu-2.34",
	"almalinux": "linux-gnu-2.34",
	"ubuntu":    "linux-gnu-2.35",
	"debian":    "linux-gnu-2.35",
}

// CurrentABI reads /etc/os-release and maps the OS ID to an ABI string.
// Does NOT import internal/probe to avoid AWS coupling.
func CurrentABI() (string, error) {
	f, err := os.Open("/etc/os-release")
	if err != nil {
		return "", fmt.Errorf("scan: reading /etc/os-release: %w", err)
	}
	defer f.Close() //nolint:errcheck

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "ID=") {
			id := strings.TrimPrefix(line, "ID=")
			id = strings.Trim(id, `"`)
			abi, ok := osABIMap[id]
			if !ok {
				return "", fmt.Errorf("scan: unknown OS ID %q in /etc/os-release", id)
			}
			return abi, nil
		}
	}
	return "", fmt.Errorf("scan: ID not found in /etc/os-release")
}

// CurrentArch maps runtime.GOARCH to "x86_64" or "arm64".
func CurrentArch() string {
	if runtime.GOARCH == "arm64" {
		return "arm64"
	}
	return "x86_64"
}
