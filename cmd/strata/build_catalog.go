package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/scttfrdmn/strata/internal/build"
)

func newBuildCatalogCmd() *cobra.Command {
	var osFlag, arch, reg, key string
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "build-catalog <recipes-dir>",
		Short: "Build all recipes in a catalog in dependency order",
		Long: `Parse all recipes in a directory, resolve their build_requires dependency
graph, and print the build stages in topological order.

Pass --dry-run to print the build plan without launching any builds.
Without --dry-run, each stage is built in order; recipes within the same
stage can be built in parallel (parallel execution is printed but not yet
automated — use the --dry-run output to orchestrate parallel strata build calls).`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			recipesDir := args[0]

			plan, err := build.PlanCatalog(recipesDir)
			if err != nil {
				return fmt.Errorf("planning catalog: %w", err)
			}

			if len(plan.Stages) == 0 {
				fmt.Fprintln(os.Stderr, "no recipes found in", recipesDir)
				return nil
			}

			fmt.Printf("build plan: %d stage(s), %d recipe(s)\n",
				len(plan.Stages), len(plan.Recipes))

			for i, stage := range plan.Stages {
				fmt.Printf("\nstage %d (parallel):\n", i+1)
				for _, name := range stage {
					r := plan.Recipes[name]
					requires := make([]string, 0, len(r.Meta.BuildRequires))
					for _, req := range r.Meta.BuildRequires {
						requires = append(requires, req.String())
					}
					reqStr := ""
					if len(requires) > 0 {
						reqStr = "  build_requires: [" + strings.Join(requires, ", ") + "]"
					}
					fmt.Printf("  %s@%s (%s)%s\n", r.Meta.Name, r.Meta.Version, r.Dir, reqStr)
				}
			}

			if dryRun {
				fmt.Fprintf(os.Stderr, "\ndry-run: to build, run strata build for each recipe above\n")
				return nil
			}

			// Non-dry-run: print the strata build commands for each stage.
			fmt.Printf("\nto build (run stages in order, recipes within a stage in parallel):\n")
			for i, stage := range plan.Stages {
				fmt.Printf("\n# stage %d\n", i+1)
				for _, name := range stage {
					r := plan.Recipes[name]
					fmt.Printf("strata build %s --os %s --arch %s --registry %s",
						r.Dir, osFlag, arch, reg)
					if key != "" {
						fmt.Printf(" --key %s", key)
					}
					fmt.Println()
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&osFlag, "os", "al2023", "target OS (al2023, rocky9, rocky10, ubuntu24)")
	cmd.Flags().StringVar(&arch, "arch", "x86_64", "target architecture (x86_64, arm64)")
	cmd.Flags().StringVar(&reg, "registry", os.Getenv("STRATA_REGISTRY_URL"),
		"S3 registry URL; overrides STRATA_REGISTRY_URL")
	cmd.Flags().StringVar(&key, "key", "", "cosign key file or KMS URI (empty = keyless OIDC)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print build plan without building")
	return cmd
}
