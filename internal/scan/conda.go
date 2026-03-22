package scan

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// condaMetaEntry is the minimal fields we need from a conda-meta/*.json file.
type condaMetaEntry struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Build   string `json:"build"`
	Channel string `json:"channel"`
}

// archSuffixRe matches conda channel URL architecture suffixes.
var archSuffixRe = regexp.MustCompile(`/(linux-64|linux-aarch64|osx-64|osx-arm64|win-64)$`)

// DetectConda discovers conda environments and returns installed packages.
// Silently returns empty if conda is not installed or no environments are found.
func DetectConda(_ context.Context) ([]DetectedPackage, error) {
	envPaths := findCondaEnvs()
	if len(envPaths) == 0 {
		return nil, nil
	}

	var result []DetectedPackage
	seen := make(map[string]bool)

	for _, envPath := range envPaths {
		pkgs := readCondaEnv(envPath)
		for _, p := range pkgs {
			key := p.Name + "@" + p.Version + "@" + envPath
			if !seen[key] {
				seen[key] = true
				result = append(result, p)
			}
		}
	}

	return result, nil
}

// findCondaEnvs returns paths to conda environments to scan.
func findCondaEnvs() []string {
	var envPaths []string
	seen := make(map[string]bool)

	add := func(p string) {
		if p != "" && !seen[p] {
			if _, err := os.Stat(filepath.Join(p, "conda-meta")); err == nil {
				seen[p] = true
				envPaths = append(envPaths, p)
			}
		}
	}

	// Active environment
	add(os.Getenv("CONDA_PREFIX"))

	// Find conda root from CONDA_EXE
	condaExe := os.Getenv("CONDA_EXE")
	if condaExe != "" {
		// CONDA_EXE is typically <conda_root>/bin/conda
		condaRoot := filepath.Dir(filepath.Dir(condaExe))
		add(condaRoot)
		scanCondaEnvsDir(condaRoot, add)
	}

	// Well-known default locations
	home, _ := os.UserHomeDir()
	defaults := []string{
		filepath.Join(home, "miniforge3"),
		filepath.Join(home, "miniconda3"),
		filepath.Join(home, "anaconda3"),
		"/opt/conda",
		"/opt/miniforge3",
		"/opt/miniconda3",
	}
	for _, d := range defaults {
		add(d)
		scanCondaEnvsDir(d, add)
	}

	return envPaths
}

// scanCondaEnvsDir adds all subdirectories of <condaRoot>/envs/ that contain conda-meta.
func scanCondaEnvsDir(condaRoot string, add func(string)) {
	envsDir := filepath.Join(condaRoot, "envs")
	entries, err := os.ReadDir(envsDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			add(filepath.Join(envsDir, e.Name()))
		}
	}
}

// readCondaEnv reads all packages from a conda-meta directory.
func readCondaEnv(envPath string) []DetectedPackage {
	metaDir := filepath.Join(envPath, "conda-meta")
	entries, err := os.ReadDir(metaDir)
	if err != nil {
		return nil
	}

	var result []DetectedPackage
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(metaDir, e.Name()))
		if err != nil {
			continue
		}
		var entry condaMetaEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			continue
		}
		if entry.Name == "" || entry.Version == "" {
			continue
		}
		channel := normalizeCondaChannel(entry.Channel)
		result = append(result, DetectedPackage{
			Name:      entry.Name,
			Version:   entry.Version,
			Prefix:    envPath,
			Source:    SourceConda,
			Channel:   channel,
			BuildHash: entry.Build,
		})
	}
	return result
}

// normalizeCondaChannel converts a channel URL to a short name.
// For example, "https://conda.anaconda.org/conda-forge/linux-64" → "conda-forge".
func normalizeCondaChannel(channel string) string {
	if channel == "" {
		return ""
	}
	// Strip arch suffix
	channel = archSuffixRe.ReplaceAllString(channel, "")
	// Take last path segment
	if idx := strings.LastIndex(channel, "/"); idx >= 0 {
		channel = channel[idx+1:]
	}
	return channel
}
