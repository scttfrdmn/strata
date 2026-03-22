package scan

import (
	"testing"
)

func TestCurrentArch(t *testing.T) {
	arch := CurrentArch()
	if arch != "x86_64" && arch != "arm64" {
		t.Errorf("CurrentArch() = %q, want x86_64 or arm64", arch)
	}
}
