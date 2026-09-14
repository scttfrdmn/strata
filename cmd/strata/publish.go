package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/scttfrdmn/strata/internal/zenodo"
	"github.com/scttfrdmn/strata/spec"
)

func newPublishCmd() *cobra.Command {
	var token string
	var sandbox bool

	cmd := &cobra.Command{
		Use:   "publish <lock.yaml>",
		Short: "Publish a frozen lockfile to Zenodo (mint DOI)",
		Long: `Publish a frozen lockfile to Zenodo via the Deposit API and print the
minted DOI. The lockfile must be fully frozen (all layers have SHA256), free of
a mutable upper layer (not a dirty environment), and signed (carry a Rekor
entry) — publishing mints a permanent DOI, so an unattested environment is
refused. Provide your Zenodo personal access token via --token or ZENODO_TOKEN.`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			tok, err := resolveToken(token)
			if err != nil {
				return err
			}
			lf, err := parsePublishableLockFile(args[0])
			if err != nil {
				return err
			}

			client := &zenodo.Client{Token: tok}
			if sandbox {
				client.BaseURL = "https://sandbox.zenodo.org"
			}

			result, err := client.Deposit(context.Background(), lf)
			if err != nil {
				return fmt.Errorf("publish: %w", err)
			}

			fmt.Printf("published: doi:%s\n", result.DOI)
			fmt.Printf("record:    %s\n", result.RecordURL)
			return nil
		},
	}

	cmd.Flags().StringVar(&token, "token", "", "Zenodo personal access token (overrides ZENODO_TOKEN env var)")
	cmd.Flags().BoolVar(&sandbox, "sandbox", false, "use sandbox.zenodo.org instead of production")
	return cmd
}

// resolveToken returns the token from the flag or ZENODO_TOKEN env var.
func resolveToken(flagValue string) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}
	if t := os.Getenv("ZENODO_TOKEN"); t != "" {
		return t, nil
	}
	return "", fmt.Errorf("publish: Zenodo token required — set ZENODO_TOKEN or use --token")
}

// parsePublishableLockFile reads a lockfile and verifies it may be published:
// frozen (all layers pinned), not dirty (no mutable upper layer), and signed (a
// Rekor entry exists). Publishing mints a permanent DOI, so each unmet condition
// is a refusal, not a warning: a dirty or unsigned lockfile names an environment
// nothing has attested, and the docs (docs/package-management.md) already promise
// publish rejects it.
func parsePublishableLockFile(path string) (*spec.LockFile, error) {
	lf, err := spec.ParseLockFile(path)
	if err != nil {
		return nil, fmt.Errorf("publish: %w", err)
	}
	if err := lf.Validate(); err != nil {
		return nil, fmt.Errorf("publish: %w", err)
	}
	if !lf.IsFrozen() {
		return nil, fmt.Errorf("publish: lockfile is not frozen — run \"strata freeze\" first to pin all layer SHA256s")
	}
	if lf.HasMutableLayer() {
		return nil, fmt.Errorf("publish: lockfile has a mutable upper layer (dirty environment) — run \"strata freeze-layer\" to convert it into a signed squashfs layer before publishing; publishing now would mint a DOI for an environment nothing has attested")
	}
	if !lf.IsSigned() {
		return nil, fmt.Errorf("publish: lockfile is not signed — it carries no Rekor entry, so there is no attestation to publish; sign the lockfile before publishing")
	}
	return lf, nil
}
