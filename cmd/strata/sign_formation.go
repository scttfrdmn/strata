// Copyright 2026 Scott Friedman
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/scttfrdmn/strata/internal/trust"
	"github.com/scttfrdmn/strata/spec"
)

func newSignFormationCmd() *cobra.Command {
	var key string

	cmd := &cobra.Command{
		Use:   "sign-formation <formation.yaml>",
		Short: "Sign a formation as a unit and log it to Rekor",
		Long: `Sign a formation — its name, version, ordered layer refs and provided
capabilities, as a set — with cosign, record the Sigstore bundle on the
formation, and log it to the Rekor transparency log. This fills the
"pending-initial-build" rekor_entry/bundle placeholders the shipped formations
carry with a real attestation: a formation whose layer set no maintainer
attested has no valid signature, exactly as for a lockfile (see "strata sign").

Signing calls the key (default the Strata KMS signing key) via cosign, which
reads AWS credentials from the environment; export credentials that can kms:Sign
the key before running.`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runSignFormation(args[0], key)
		},
	}

	cmd.Flags().StringVar(&key, "key", defaultSigningKey, "cosign signing key (KMS URI or key file)")
	return cmd
}

func runSignFormation(path, key string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("sign-formation: %w", err)
	}
	var f spec.Formation
	if err := yaml.Unmarshal(data, &f); err != nil {
		return fmt.Errorf("sign-formation: parsing %s: %w", path, err)
	}
	if f.Name == "" || f.Version == "" {
		return fmt.Errorf("sign-formation: %s has no name/version — not a formation", path)
	}
	if len(f.Layers) == 0 {
		return fmt.Errorf("sign-formation: %s declares no layers — nothing to attest", path)
	}

	if err := signFormation(context.Background(), &f, key); err != nil {
		return fmt.Errorf("sign-formation: %w", err)
	}
	// SignedBy is recorded provenance (the key ref), set after signing and
	// excluded from the signed payload — the same treatment a layer manifest gets.
	f.SignedBy = key

	if err := writeYAML(path, &f); err != nil {
		return err
	}
	fmt.Printf("signed formation: %s (rekor log index %s)\n", path, f.RekorEntry)
	return nil
}

// signFormation is the seam production uses cosign+KMS through; a test injects a
// fake signer so the wiring is exercised without AWS or the network.
var signFormation = func(ctx context.Context, f *spec.Formation, key string) error {
	return trust.SignFormation(ctx, f, &trust.CosignSigner{KeyRef: key})
}
