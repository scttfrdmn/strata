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
minted DOI. The lockfile must be fully frozen (all layers have SHA256).
Provide your Zenodo personal access token via --token or ZENODO_TOKEN.`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			tok, err := resolveToken(token)
			if err != nil {
				return err
			}
			lf, err := parseFrozenLockFile(args[0])
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

// parseFrozenLockFile reads a lockfile and verifies it is frozen.
func parseFrozenLockFile(path string) (*spec.LockFile, error) {
	lf, err := spec.ParseLockFile(path)
	if err != nil {
		return nil, fmt.Errorf("publish: %w", err)
	}
	if !lf.IsFrozen() {
		return nil, fmt.Errorf("publish: lockfile is not frozen — run \"strata freeze\" first to pin all layer SHA256s")
	}
	return lf, nil
}
