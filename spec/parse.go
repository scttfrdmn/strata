package spec

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ParseProfile reads and validates a Profile from a YAML file.
func ParseProfile(path string) (*Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading profile %q: %w", path, err)
	}
	return ParseProfileBytes(data)
}

// ParseProfileBytes parses and validates a Profile from YAML bytes.
func ParseProfileBytes(data []byte) (*Profile, error) {
	var p Profile
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parsing profile YAML: %w", err)
	}

	// Post-process: handle inline "cuda@12.3" style entries.
	// YAML can represent software refs as plain strings or as structs.
	// We support both forms for user convenience.
	if err := normalizeSoftwareRefs(&p); err != nil {
		return nil, fmt.Errorf("normalizing software refs: %w", err)
	}

	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("invalid profile: %w", err)
	}

	return &p, nil
}

// normalizeSoftwareRefs ensures all SoftwareRefs in the profile are
// properly populated. Handles the case where a software ref was
// deserialized from a plain string.
func normalizeSoftwareRefs(p *Profile) error {
	for i, sw := range p.Software {
		// If only Name is set and it looks like an inline string ref
		// ("cuda@12.3" or "formation:..."), re-parse it.
		if sw.Version == "" && sw.Formation == "" && sw.Name != "" {
			parsed, err := ParseSoftwareRef(sw.Name)
			if err != nil {
				return fmt.Errorf("software[%d] %q: %w", i, sw.Name, err)
			}
			p.Software[i] = parsed
		}
	}
	return nil
}

// ParseLockFile reads a LockFile from a YAML file.
func ParseLockFile(path string) (*LockFile, error) {
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
