package spec

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// maxSpecFileBytes is the maximum size accepted for profile and lockfile YAML.
// Rejects unreasonably large files before deserializing.
const maxSpecFileBytes = 10 << 20 // 10 MiB

// ParseProfile reads and validates a Profile from a YAML file.
func ParseProfile(path string) (*Profile, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("reading profile %q: %w", path, err)
	}
	if info.Size() > maxSpecFileBytes {
		return nil, fmt.Errorf("profile file %q too large (%d bytes)", path, info.Size())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading profile %q: %w", path, err)
	}
	return ParseProfileBytes(data)
}

// ParseProfileBytes parses and validates a Profile from YAML bytes.
func ParseProfileBytes(data []byte) (*Profile, error) {
	var p Profile
	// Both software-ref forms — inline "cuda@12.3" and the name/version mapping
	// — are handled by SoftwareRef.UnmarshalYAML, so they need no post-pass
	// here, and they work anywhere a SoftwareRef appears rather than only in a
	// profile's software list.
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parsing profile YAML: %w", err)
	}

	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("invalid profile: %w", err)
	}

	return &p, nil
}

// ParseLockFile reads a LockFile from a YAML file.
func ParseLockFile(path string) (*LockFile, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("reading lockfile %q: %w", path, err)
	}
	if info.Size() > maxSpecFileBytes {
		return nil, fmt.Errorf("lockfile %q too large (%d bytes)", path, info.Size())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading lockfile %q: %w", path, err)
	}
	return ParseLockFileBytes(data)
}

// ParseLockFileBytes parses a LockFile from YAML bytes.
func ParseLockFileBytes(data []byte) (*LockFile, error) {
	var l LockFile
	if err := yaml.Unmarshal(data, &l); err != nil {
		return nil, fmt.Errorf("parsing lockfile YAML: %w", err)
	}
	return &l, nil
}

// MarshalProfile serializes a Profile to YAML bytes.
func MarshalProfile(p *Profile) ([]byte, error) {
	return yaml.Marshal(p)
}

// MarshalLockFile serializes a LockFile to YAML bytes.
func MarshalLockFile(l *LockFile) ([]byte, error) {
	return yaml.Marshal(l)
}
