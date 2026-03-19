package main

import (
	"context"
	"fmt"
	"os"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/scttfrdmn/strata/internal/build"
	"github.com/scttfrdmn/strata/internal/registry"
	"github.com/scttfrdmn/strata/internal/trust"
	"github.com/scttfrdmn/strata/spec"
)

func newBuildCmd() *cobra.Command {
	var osFlag, arch, reg, key, amiID, instanceType, cacheDir string
	var dryRun, ec2Flag, noWait bool

	cmd := &cobra.Command{
		Use:   "build <recipe-dir>",
		Short: "Build and sign a layer from a recipe, push to registry",
		Long: `Build a layer from a recipe directory, sign it with cosign, and push
the squashfs artifact to the S3 registry.

Local mode (default): runs build.sh on the local machine. build_requires are
not mounted; the layer is marked bootstrap_build=true. Suitable for Tier 0
layers (gcc, LLVM, CUDA) built with the OS system compiler.

EC2 mode (--ec2): launches a fresh EC2 instance, mounts build_requires layers
via OverlayFS, and runs the full pipeline. Required for Tier 1+ layers.

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
				CacheDir:    cacheDir,
				KeyRef:      key,
			}
			if err := job.Validate(); err != nil {
				return err
			}

			var regClient *registry.S3Client
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

			// Wire up build env resolver on Linux when build_requires are present.
			// This enables Stage 3 OverlayFS mounting for Tier 1+ layers.
			if runtime.GOOS == "linux" && !dryRun && regClient != nil && len(recipe.Meta.BuildRequires) > 0 {
				job.EnvResolver = &build.RegistryBuildEnvResolver{Registry: regClient}
			}

			// EC2 mode: upload recipe, launch instance (poll if !noWait).
			if ec2Flag && !dryRun {
				return runBuildEC2(context.Background(), job, recipe, reg, key, amiID, instanceType, noWait)
			}

			var executor build.Executor
			if dryRun {
				executor = &build.DryRunExecutor{Out: os.Stderr}
			} else {
				executor = &build.LocalExecutor{Stdout: os.Stdout, Stderr: os.Stderr}
			}

			signer := &trust.CosignSigner{KeyRef: key}

			var pushReg build.PushRegistry
			if regClient != nil {
				pushReg = regClient
			}

			manifest, err := build.Run(context.Background(), job, recipe, pushReg, executor, signer)
			if err != nil {
				return fmt.Errorf("build failed: %w", err)
			}

			return printBuildResult(manifest, dryRun)
		},
	}

	cmd.Flags().StringVar(&osFlag, "os", "al2023", "target OS (al2023, rocky9, rocky10, ubuntu24)")
	cmd.Flags().StringVar(&arch, "arch", "x86_64", "target architecture (x86_64, arm64)")
	cmd.Flags().StringVar(&reg, "registry", os.Getenv("STRATA_REGISTRY_URL"),
		"S3 registry URL (e.g. s3://my-strata-bucket); overrides STRATA_REGISTRY_URL")
	cmd.Flags().StringVar(&key, "key", "awskms:///alias/strata-signing-key", "cosign key file or KMS URI (default: AWS KMS alias/strata-signing-key)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "validate and print plan without building")
	cmd.Flags().BoolVar(&ec2Flag, "ec2", false, "run build on a fresh EC2 instance (required for Tier 1+ layers)")
	cmd.Flags().BoolVar(&noWait, "no-wait", false, "with --ec2: launch instance and return immediately without polling")
	cmd.Flags().StringVar(&amiID, "ami", "", "EC2 AMI ID for the build instance (required with --ec2)")
	cmd.Flags().StringVar(&instanceType, "instance-type", "", "EC2 instance type (default: c5.4xlarge for x86_64, c6g.4xlarge for arm64)")
	cmd.Flags().StringVar(&cacheDir, "cache-dir", "", "local cache dir for downloaded build env layers (default: $TMPDIR/strata-build-cache)")
	return cmd
}

// runBuildEC2 orchestrates a build on an EC2 instance.
func runBuildEC2(ctx context.Context, job *build.Job, recipe *build.Recipe, reg, key, amiID, instanceType string, noWait bool) error {
	if amiID == "" {
		return fmt.Errorf("--ami is required for --ec2 builds")
	}

	normalizedArch := job.Base.NormalizedArch()
	if instanceType == "" {
		if normalizedArch == "arm64" {
			instanceType = "c6g.4xlarge"
		} else {
			instanceType = "c5.4xlarge"
		}
	}

	// Default security group and IAM profile from well-known strata-builder config.
	cfg := build.EC2Config{
		Region:          "us-east-1",
		AMIID:           amiID,
		InstanceType:    instanceType,
		SecurityGroupID: "sg-0fca02f58fafcdad1",
		IAMProfile:      "strata-builder",
		BucketURL:       reg,
		BinaryArch:      build.ArchForEC2(normalizedArch),
		KeyRef:          key,
	}

	runner, err := build.NewEC2Runner(cfg)
	if err != nil {
		return fmt.Errorf("initializing EC2 runner: %w", err)
	}

	jobID := fmt.Sprintf("%s-%s-%s", recipe.Meta.Name, recipe.Meta.Version, normalizedArch)
	fmt.Fprintf(os.Stderr, "ec2: launching build for %s@%s on %s (%s)\n",
		recipe.Meta.Name, recipe.Meta.Version, amiID, instanceType)

	if noWait {
		instanceID, err := runner.LaunchBuildEC2(ctx, jobID, recipe, job)
		if err != nil {
			return fmt.Errorf("EC2 launch failed: %w", err)
		}
		fmt.Printf("launched: %s  recipe: %s@%s  arch: %s\n",
			instanceID, recipe.Meta.Name, recipe.Meta.Version, normalizedArch)
		fmt.Printf("monitor:  aws ec2 describe-instances --instance-ids %s --query 'Reservations[].Instances[].[Tags[?Key==`strata:build-status`].Value|[0],State.Name]' --output text\n", instanceID)
		return nil
	}

	instanceID, err := runner.RunBuildEC2(ctx, jobID, recipe, job)
	if err != nil {
		return fmt.Errorf("EC2 build failed: %w", err)
	}

	fmt.Printf("ec2:     instance %s completed successfully\n", instanceID)
	fmt.Printf("recipe:  %s@%s\n", recipe.Meta.Name, recipe.Meta.Version)
	fmt.Printf("hint:    run 'strata search %s' to verify the layer is in the registry\n", recipe.Meta.Name)
	return nil
}

// printBuildResult prints the build result summary.
func printBuildResult(manifest *spec.LayerManifest, dryRun bool) error {
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
	if manifest.BootstrapBuild {
		fmt.Printf("%sbootstrap_build: true (Tier 0 — built with OS system compiler)\n", prefix)
	} else if len(manifest.BuiltWith) > 0 {
		fmt.Printf("%sbuilt_with: %d layer(s)\n", prefix, len(manifest.BuiltWith))
	}
	return nil
}
