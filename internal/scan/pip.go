package scan

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// pipListEntry is a minimal entry from pip list --format=json.
type pipListEntry struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// DetectPip detects pip-installed packages using the provided ExecRunner.
// Silently returns empty if python3 or pip is not available.
func DetectPip(ctx context.Context, runner ExecRunner) ([]DetectedPackage, error) {
	// Get python prefix
	prefixOut, err := runner.Output(ctx, "python3", "-c", "import sys; print(sys.prefix)")
	if err != nil {
		return nil, nil // python3 not available
	}
	prefix := strings.TrimSpace(string(prefixOut))

	// Strategy 1: pip list --format=json
	if pkgs := pipListJSON(ctx, runner, prefix); pkgs != nil {
		return pkgs, nil
	}

	// Strategy 2: walk dist-info directories
	return pipDistInfo(prefix), nil
}

// pipListJSON runs pip list --format=json and parses the output.
func pipListJSON(ctx context.Context, runner ExecRunner, prefix string) []DetectedPackage {
	out, err := runner.Output(ctx, "python3", "-m", "pip", "list", "--format=json")
	if err != nil {
		return nil
	}
	var entries []pipListEntry
	if err := json.Unmarshal(out, &entries); err != nil {
		return nil
	}
	result := make([]DetectedPackage, 0, len(entries))
	for _, e := range entries {
		if e.Name == "" || e.Version == "" {
			continue
		}
		result = append(result, DetectedPackage{
			Name:    e.Name,
			Version: e.Version,
			Prefix:  prefix,
			Source:  SourcePip,
		})
	}
	return result
}

// pipDistInfo walks site-packages for *.dist-info/METADATA files.
func pipDistInfo(prefix string) []DetectedPackage {
	// Find site-packages directories
	var sitePkgDirs []string

	// Walk lib/python*/site-packages
	libDir := filepath.Join(prefix, "lib")
	entries, err := os.ReadDir(libDir)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "python") {
			sitePkgDirs = append(sitePkgDirs, filepath.Join(libDir, e.Name(), "site-packages"))
		}
	}

	if len(sitePkgDirs) == 0 {
		return nil
	}

	var result []DetectedPackage
	seen := make(map[string]bool)

	for _, spDir := range sitePkgDirs {
		entries, err := os.ReadDir(spDir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() || !strings.HasSuffix(e.Name(), ".dist-info") {
				continue
			}
			metaPath := filepath.Join(spDir, e.Name(), "METADATA")
			name, version := parseDistInfoMetadata(metaPath)
			if name == "" || version == "" {
				continue
			}
			key := name + "@" + version
			if !seen[key] {
				seen[key] = true
				result = append(result, DetectedPackage{
					Name:    name,
					Version: version,
					Prefix:  prefix,
					Source:  SourcePip,
				})
			}
		}
	}
	return result
}

// parseDistInfoMetadata parses Name: and Version: from a METADATA file.
func parseDistInfoMetadata(path string) (name, version string) {
	f, err := os.Open(path)
	if err != nil {
		return "", ""
	}
	defer f.Close() //nolint:errcheck

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if name == "" && strings.HasPrefix(line, "Name: ") {
			name = strings.TrimPrefix(line, "Name: ")
		}
		if version == "" && strings.HasPrefix(line, "Version: ") {
			version = strings.TrimPrefix(line, "Version: ")
		}
		if name != "" && version != "" {
			break
		}
		// Stop at blank line (end of headers)
		if line == "" {
			break
		}
	}
	return name, version
}
