package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/scttfrdmn/strata/internal/build"
	"github.com/scttfrdmn/strata/internal/registry"
	"github.com/scttfrdmn/strata/internal/trust"
	"github.com/scttfrdmn/strata/spec"
)

func newBuildCmd() *cobra.Command {
	var osFlag, arch, reg, key string
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "build <recipe-dir>",
		Short: "Build and sign a layer from a recipe, push to registry",
		Long: `Build a layer from a recipe directory, sign it with cosign, and push
the squashfs artifact to the S3 registry.

Pass --dry-run to validate the recipe and print the build plan without
executing any build steps or requiring AWS credentials.`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			recipeDir := args[0]

			recipe, err := build.ParseRecipe(recipeDir)
			if err != nil {
				return fmt.Errorf("parsing recipe: %w", err)
			}

			job := &build.Job{
				RecipeDir:   recipeDir,
				Base:        spec.BaseRef{OS: osFlag, Arch: arch},
				RegistryURL: reg,
				DryRun:      dryRun,
			}
			if err := job.Validate(); err != nil {
				return err
			}

			var regClient build.PushRegistry
			if !dryRun {
				if reg == "" {
					return fmt.Errorf("--registry (or STRATA_REGISTRY_URL) is required for non-dry-run builds")
				}
				c, regErr := registry.NewS3Client(reg)
				if regErr != nil {
					return fmt.Errorf("registry: %w", regErr)
				}
				regClient = c
			}

			var executor build.Executor
			if dryRun {
				executor = &build.DryRunExecutor{Out: os.Stderr}
			} else {
				executor = &build.LocalExecutor{Stdout: os.Stdout, Stderr: os.Stderr}
			}

			signer := &trust.CosignSigner{KeyRef: key}

			manifest, err := build.Run(context.Background(), job, recipe, regClient, executor, signer)
			if err != nil {
				return fmt.Errorf("build failed: %w", err)
			}

			prefix := ""
			if dryRun {
				prefix = "dry-run: "
			}
			fmt.Printf("%sbuilt:   %s\n", prefix, manifest.ID)
			fmt.Printf("%ssha256:  %s\n", prefix, manifest.SHA256)
			if manifest.Size > 0 {
				fmt.Printf("%ssize:    %.1f MB\n", prefix, float64(manifest.Size)/1e6)
			}
			if manifest.RekorEntry != "" && manifest.RekorEntry != "dry-run" {
				fmt.Printf("%srekor:   %s\n", prefix, manifest.RekorEntry)
			}
			if manifest.Source != "" {
				dir := manifest.Source
				for i := len(dir) - 1; i >= 0; i-- {
					if dir[i] == '/' {
						dir = dir[:i+1]
						break
					}
				}
				fmt.Printf("%spushed:  %s\n", prefix, dir)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&osFlag, "os", "al2023", "target OS (al2023, rocky9, rocky10, rhel9, ubuntu22)")
	cmd.Flags().StringVar(&arch, "arch", "x86_64", "target architecture (x86_64, arm64)")
	cmd.Flags().StringVar(&reg, "registry", os.Getenv("STRATA_REGISTRY_URL"),
		"S3 registry URL (e.g. s3://my-strata-bucket); overrides STRATA_REGISTRY_URL")
	cmd.Flags().StringVar(&key, "key", "", "cosign key file or KMS URI (empty = keyless OIDC)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "validate and print plan without building")
	return cmd
}
