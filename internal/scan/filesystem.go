package scan

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
)

// DefaultFSScanPaths is the default set of directories to scan for installed software.
var DefaultFSScanPaths = []string{"/opt", "/usr/local", "/opt/apps", "/opt/software"}

// versionRe matches version strings starting with a digit.
var versionRe = regexp.MustCompile(`^\d[\d.]*$`)

// DetectFilesystem scans configured paths for <name>/<version>/ layout packages.
// It looks two levels deep and classifies directories as packages if they contain
// a bin/ or lib/ subdirectory and the version matches a numeric pattern.
func DetectFilesystem(_ context.Context, scanPaths []string) ([]DetectedPackage, error) {
	var result []DetectedPackage
	seen := make(map[string]bool)

	for _, base := range scanPaths {
		entries, err := os.ReadDir(base)
		if err != nil {
			continue // path doesn't exist, skip
		}
		for _, nameEntry := range entries {
			if !nameEntry.IsDir() {
				continue
			}
			pkgName := nameEntry.Name()
			nameDir := filepath.Join(base, pkgName)

			versionEntries, err := os.ReadDir(nameDir)
			if err != nil {
				continue
			}
			for _, verEntry := range versionEntries {
				if !verEntry.IsDir() {
					continue
				}
				ver := verEntry.Name()
				if !versionRe.MatchString(ver) {
					continue
				}
				versionDir := filepath.Join(nameDir, ver)
				if !hasUsableDirs(versionDir) {
					continue
				}
				key := pkgName + "@" + ver + "@" + base
				if !seen[key] {
					seen[key] = true
					result = append(result, DetectedPackage{
						Name:    pkgName,
						Version: ver,
						Prefix:  versionDir,
						Source:  SourceFilesystem,
					})
				}
			}
		}
	}

	return result, nil
}

// hasUsableDirs reports whether dir contains bin/ or lib/.
func hasUsableDirs(dir string) bool {
	for _, sub := range []string{"bin", "lib"} {
		info, err := os.Stat(filepath.Join(dir, sub))
		if err == nil && info.IsDir() {
			return true
		}
	}
	return false
}
