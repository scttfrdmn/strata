//go:build linux

package capture

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// NormalizePaths rewrites originalPrefix → newPrefix in dir (in-place).
// Handles: ELF RPATH/interpreter (via patchelf), shebangs, .pc, .la, .cmake files.
// Never errors on individual files — puts failures in warnings slice.
func NormalizePaths(_ context.Context, dir, originalPrefix, newPrefix string) (modified int, warnings []string, err error) {
	hasPatchelf := false
	if _, lookErr := exec.LookPath("patchelf"); lookErr == nil {
		hasPatchelf = true
	}

	walkErr := filepath.Walk(dir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil // skip unreadable entries
		}
		if info.IsDir() {
			return nil
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			warnings = append(warnings, fmt.Sprintf("read %s: %v", path, readErr))
			return nil
		}

		if isELF(data) {
			if hasPatchelf {
				if patchErr := patchELF(path, originalPrefix, newPrefix); patchErr != nil {
					warnings = append(warnings, fmt.Sprintf("patchelf %s: %v", path, patchErr))
				} else {
					modified++
				}
			}
			return nil
		}

		if !isTextFile(data) {
			return nil
		}

		if !strings.Contains(string(data), originalPrefix) {
			return nil
		}

		newData := bytes.ReplaceAll(data, []byte(originalPrefix), []byte(newPrefix))
		if bytes.Equal(newData, data) {
			return nil
		}

		if writeErr := os.WriteFile(path, newData, info.Mode()); writeErr != nil {
			warnings = append(warnings, fmt.Sprintf("write %s: %v", path, writeErr))
			return nil
		}
		modified++
		return nil
	})

	return modified, warnings, walkErr
}

// isELF returns true if data starts with the ELF magic bytes.
func isELF(data []byte) bool {
	return len(data) >= 4 && data[0] == 0x7f && data[1] == 'E' && data[2] == 'L' && data[3] == 'F'
}

// isTextFile returns true if the first 512 bytes contain no null bytes.
func isTextFile(data []byte) bool {
	check := data
	if len(check) > 512 {
		check = check[:512]
	}
	return !bytes.ContainsRune(check, 0)
}

// patchELF uses patchelf to rewrite RPATH and interpreter in an ELF binary.
func patchELF(path, oldPrefix, newPrefix string) error {
	// Get current RPATH
	out, err := exec.Command("patchelf", "--print-rpath", path).Output()
	if err == nil {
		rpath := strings.TrimSpace(string(out))
		if strings.Contains(rpath, oldPrefix) {
			newRpath := strings.ReplaceAll(rpath, oldPrefix, newPrefix)
			if err := exec.Command("patchelf", "--set-rpath", newRpath, path).Run(); err != nil {
				return fmt.Errorf("set-rpath: %w", err)
			}
		}
	}

	// Get current interpreter
	out, err = exec.Command("patchelf", "--print-interpreter", path).Output()
	if err == nil {
		interp := strings.TrimSpace(string(out))
		if strings.Contains(interp, oldPrefix) {
			newInterp := strings.ReplaceAll(interp, oldPrefix, newPrefix)
			if err := exec.Command("patchelf", "--set-interpreter", newInterp, path).Run(); err != nil {
				return fmt.Errorf("set-interpreter: %w", err)
			}
		}
	}

	return nil
}
