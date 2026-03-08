package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/scttfrdmn/strata/internal/registry"
)

func newIndexCmd() *cobra.Command {
	var reg string

	cmd := &cobra.Command{
		Use:   "index",
		Short: "Rebuild the registry layer catalog index",
		Long: `Scan all layer manifests in the S3 registry and rewrite
index/layers.yaml. Use after batch builds or to repair index inconsistency.`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if reg == "" {
				return fmt.Errorf("--registry (or STRATA_REGISTRY_URL) is required")
			}

			c, err := registry.NewS3Client(reg)
			if err != nil {
				return fmt.Errorf("registry: %w", err)
			}

			if err := c.RebuildIndex(context.Background()); err != nil {
				return fmt.Errorf("rebuilding index: %w", err)
			}

			fmt.Println("index: rebuilt index/layers.yaml")
			return nil
		},
	}

	cmd.Flags().StringVar(&reg, "registry", os.Getenv("STRATA_REGISTRY_URL"),
		"S3 registry URL (e.g. s3://my-strata-bucket); overrides STRATA_REGISTRY_URL")
	return cmd
}
