// Copyright 2026 Scott Friedman
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/scttfrdmn/strata/internal/trust"
	"github.com/scttfrdmn/strata/spec"
)

// defaultSigningKey is the Strata cosign signing key — the AWS KMS asymmetric
// key whose public half is embedded in every binary (internal/trust). The same
// default the build/freeze-layer/stratify commands use.
const defaultSigningKey = "awskms:///alias/strata-signing-key"

func newSignCmd() *cobra.Command {
	var key string

	cmd := &cobra.Command{
		Use:   "sign <lock.yaml>",
		Short: "Sign a frozen lockfile as a whole and log it to Rekor",
		Long: `Sign the lockfile — the entire resolved layer set, not just the individual
layers — with cosign, and record the Sigstore bundle on the lockfile. Signing the
set is what makes a mix-and-match of individually-valid, individually-signed
layers detectable: a combination no maintainer ever attested has no valid lockfile
signature. The signature is logged to the Rekor transparency log, so it carries
the same auditability as the layers.

The lockfile must be frozen (run "strata freeze" first). Signing calls the key
(default the Strata KMS signing key) via cosign, which reads AWS credentials from
the environment; export credentials that can kms:Sign the key before running.`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runSign(args[0], key)
		},
	}

	cmd.Flags().StringVar(&key, "key", defaultSigningKey, "cosign signing key (KMS URI or key file)")
	return cmd
}

func runSign(path, key string) error {
	lf, err := spec.ParseLockFile(path)
	if err != nil {
		return fmt.Errorf("sign: %w", err)
	}
	if err := lf.Validate(); err != nil {
		return fmt.Errorf("sign: %w", err)
	}
	if !lf.IsFrozen() {
		return fmt.Errorf("sign: lockfile is not frozen — run \"strata freeze\" first so every layer is SHA256-pinned; an unfrozen lockfile does not name a fixed set to sign")
	}

	if err := signLockFile(context.Background(), lf, key); err != nil {
		return fmt.Errorf("sign: %w", err)
	}

	if err := writeYAML(path, lf); err != nil {
		return err
	}
	fmt.Printf("signed: %s (rekor log index %s)\n", path, lf.RekorEntry)
	return nil
}

// signLockFile is the seam production uses cosign+KMS through; a test injects a
// fake signer so the wiring is exercised without AWS or the network.
var signLockFile = func(ctx context.Context, lf *spec.LockFile, key string) error {
	return trust.SignLockFile(ctx, lf, &trust.CosignSigner{KeyRef: key})
}
