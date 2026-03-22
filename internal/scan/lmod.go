package scan

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// luaPrefixRe matches: local base = "/path" or local root = "/path" etc.
var luaPrefixRe = regexp.MustCompile(`(?m)^\s*local\s+(?:base|root|prefix)\s*=\s*"([^"]+)"`)

// luaPrereqRe matches prereq("foo") or depends_on("foo").
var luaPrereqRe = regexp.MustCompile(`(?:prereq|depends_on)\s*\(\s*"([^"]+)"`)

// tclPrefixRe matches: set root /path or set base /path.
var tclPrefixRe = regexp.MustCompile(`(?m)^\s*set\s+(?:root|base|prefix)\s+(/\S+)`)

// tclPrereqRe matches: prereq foo.
var tclPrereqRe = regexp.MustCompile(`(?m)^\s*prereq\s+(\S+)`)

// DetectLmod walks $MODULEPATH and returns detected packages.
// Silently returns empty if MODULEPATH is not set.
func DetectLmod(_ context.Context) ([]DetectedPackage, error) {
	modulePath := os.Getenv("MODULEPATH")
	if modulePath == "" {
		return nil, nil
	}

	var result []DetectedPackage
	seen := make(map[string]bool)

	for _, dir := range strings.Split(modulePath, ":") {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		pkgs := walkModuleDir(dir)
		for _, p := range pkgs {
			key := p.Name + "@" + p.Version
			if !seen[key] {
				seen[key] = true
				result = append(result, p)
			}
		}
	}

	return result, nil
}

// walkModuleDir walks a single module directory looking for modulefiles.
func walkModuleDir(dir string) []DetectedPackage {
	var result []DetectedPackage

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pkgName := e.Name()
		pkgDir := filepath.Join(dir, pkgName)

		vEntries, err := os.ReadDir(pkgDir)
		if err != nil {
			continue
		}

		for _, ve := range vEntries {
			name := ve.Name()
			var version, modPath string

			if !ve.IsDir() {
				// <dir>/<name>/<version>.lua or <version>.tcl
				if strings.HasSuffix(name, ".lua") {
					version = strings.TrimSuffix(name, ".lua")
					modPath = filepath.Join(pkgDir, name)
				} else if strings.HasSuffix(name, ".tcl") {
					version = strings.TrimSuffix(name, ".tcl")
					modPath = filepath.Join(pkgDir, name)
				}
			} else {
				// <dir>/<name>/<version>/<name>.lua or /<name>.tcl
				subDir := filepath.Join(pkgDir, name)
				version = name
				subEntries, err := os.ReadDir(subDir)
				if err != nil {
					continue
				}
				for _, se := range subEntries {
					if se.Name() == pkgName+".lua" || se.Name() == pkgName+".tcl" {
						modPath = filepath.Join(subDir, se.Name())
						break
					}
				}
				if modPath == "" {
					continue
				}
			}

			if version == "" || modPath == "" {
				continue
			}

			pkg := parseModulefile(pkgName, version, modPath)
			result = append(result, pkg)
		}
	}

	return result
}

// parseModulefile extracts prefix and deps from a modulefile.
func parseModulefile(name, version, path string) DetectedPackage {
	p := DetectedPackage{
		Name:           name,
		Version:        version,
		ModulefilePath: path,
		Source:         SourceLmod,
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return p
	}
	content := string(data)

	if strings.HasSuffix(path, ".lua") {
		parseLuaModulefile(content, &p)
	} else {
		parseTclModulefile(content, &p)
	}

	return p
}

// parseLuaModulefile extracts prefix and deps from Lua modulefile content.
func parseLuaModulefile(content string, p *DetectedPackage) {
	if m := luaPrefixRe.FindStringSubmatch(content); m != nil {
		p.Prefix = m[1]
	}
	// Fallback: look for setenv("*_HOME", "/abs/path") patterns
	if p.Prefix == "" {
		homeRe := regexp.MustCompile(`setenv\s*\(\s*"[A-Z_]+_HOME"\s*,\s*"([^"]+)"`)
		if m := homeRe.FindStringSubmatch(content); m != nil {
			p.Prefix = m[1]
		}
	}

	sc := bufio.NewScanner(strings.NewReader(content))
	for sc.Scan() {
		line := sc.Text()
		if m := luaPrereqRe.FindStringSubmatch(line); m != nil {
			p.ModuleDeps = append(p.ModuleDeps, m[1])
		}
	}
}

// parseTclModulefile extracts prefix and deps from TCL modulefile content.
func parseTclModulefile(content string, p *DetectedPackage) {
	if m := tclPrefixRe.FindStringSubmatch(content); m != nil {
		p.Prefix = m[1]
	}

	sc := bufio.NewScanner(strings.NewReader(content))
	for sc.Scan() {
		line := sc.Text()
		if m := tclPrereqRe.FindStringSubmatch(line); m != nil {
			p.ModuleDeps = append(p.ModuleDeps, m[1])
		}
	}
}
