package build

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// ErrMksquashfsNotFound is returned when mksquashfs is not on PATH.
type ErrMksquashfsNotFound struct{}

func (e *ErrMksquashfsNotFound) Error() string {
	return "mksquashfs not found on PATH — install squashfs-tools: " +
		"(apt: squashfs-tools, yum/dnf: squashfs-tools, brew: squashfs)"
}

// CreateSquashfs creates a reproducible squashfs image from srcDir at outPath.
// It uses the flags from SquashfsOptions() to ensure reproducibility.
// Returns *ErrMksquashfsNotFound if the mksquashfs binary is absent from PATH.
func CreateSquashfs(ctx context.Context, srcDir, outPath string) error {
	if _, err := exec.LookPath("mksquashfs"); err != nil {
		return &ErrMksquashfsNotFound{}
	}

	args := append([]string{srcDir, outPath}, SquashfsOptions()...)
	cmd := exec.CommandContext(ctx, "mksquashfs", args...)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return fmt.Errorf("mksquashfs: %w: %s", err, msg)
		}
		return fmt.Errorf("mksquashfs: %w", err)
	}
	return nil
}
