package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/spf13/cobra"
)

// ec2SnapshotAPI is the narrow EC2 interface needed by snapshotAMI.
// Allows mock injection in tests.
type ec2SnapshotAPI interface {
	CreateImage(ctx context.Context, in *ec2.CreateImageInput, opts ...func(*ec2.Options)) (*ec2.CreateImageOutput, error)
	DescribeImages(ctx context.Context, in *ec2.DescribeImagesInput, opts ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error)
}

func newSnapshotAMICmd() *cobra.Command {
	var (
		instanceID   string
		name         string
		description  string
		noReboot     bool
		wait         bool
		pollInterval time.Duration
		region       string
		outputFmt    string
	)

	cmd := &cobra.Command{
		Use:   "snapshot-ami",
		Short: "Create an AMI from the current (or specified) running instance",
		Long: `Create an EC2 AMI snapshot from a running instance. When run on an EC2
instance without --instance-id, the instance ID is fetched automatically
from IMDS (the instance metadata service).

This is the recommended Stage 1 alternative to a persistent EBS upper: after
installing packages interactively, snapshot the instance as an AMI. The AMI
can be used as the base for subsequent strata build and strata resolve runs
without reinstalling packages on reboot.

Examples:

  # Snapshot the current instance (run on EC2):
  strata snapshot-ami --wait

  # Snapshot a specific instance:
  strata snapshot-ami --instance-id i-0abc123 --name my-snapshot --wait

  # Output JSON for scripting:
  strata snapshot-ami --instance-id i-0abc123 --output json`,
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := awsconfig.LoadDefaultConfig(context.Background())
			if err != nil {
				return fmt.Errorf("snapshot-ami: loading AWS config: %w", err)
			}
			if region != "" {
				cfg.Region = region
			}
			if cfg.Region == "" {
				cfg.Region = "us-east-1"
			}
			client := ec2.NewFromConfig(cfg)
			return runSnapshotAMI(context.Background(), client, instanceID, name, description, noReboot, wait, pollInterval, cfg.Region, outputFmt)
		},
	}

	cmd.Flags().StringVar(&instanceID, "instance-id", "", "EC2 instance ID to snapshot (default: current instance via IMDS)")
	cmd.Flags().StringVar(&name, "name", "", "AMI name (default: strata-snapshot-<timestamp>)")
	cmd.Flags().StringVar(&description, "description", "", "AMI description")
	cmd.Flags().BoolVar(&noReboot, "no-reboot", false, "create AMI without rebooting the instance (may produce inconsistent filesystem)")
	cmd.Flags().BoolVar(&wait, "wait", false, "wait for AMI to reach 'available' state before returning")
	cmd.Flags().DurationVar(&pollInterval, "poll-interval", 30*time.Second, "polling interval when --wait is set")
	cmd.Flags().StringVar(&region, "region", "", "AWS region (default: us-east-1)")
	cmd.Flags().StringVar(&outputFmt, "output", "text", "output format: text or json")

	return cmd
}

func runSnapshotAMI(
	ctx context.Context,
	client ec2SnapshotAPI,
	instanceID, name, description string,
	noReboot, wait bool,
	pollInterval time.Duration,
	region, outputFmt string,
) error {
	// Resolve instance ID from IMDS if not provided.
	if instanceID == "" {
		id, err := fetchInstanceIDFromIMDS(ctx)
		if err != nil {
			return fmt.Errorf("snapshot-ami: fetching instance ID from IMDS: %w\n  (use --instance-id to specify manually)", err)
		}
		instanceID = id
	}

	// Generate name if not provided.
	if name == "" {
		name = "strata-snapshot-" + time.Now().Format("20060102-150405")
	}

	// Create the AMI.
	input := &ec2.CreateImageInput{
		InstanceId:  aws.String(instanceID),
		Name:        aws.String(name),
		NoReboot:    aws.Bool(noReboot),
		Description: aws.String(description),
	}

	out, err := client.CreateImage(ctx, input)
	if err != nil {
		return fmt.Errorf("snapshot-ami: CreateImage: %w", err)
	}
	amiID := aws.ToString(out.ImageId)

	state := "pending"

	// Wait for the AMI to become available if requested.
	if wait {
		for {
			desc, err := client.DescribeImages(ctx, &ec2.DescribeImagesInput{
				ImageIds: []string{amiID},
			})
			if err != nil {
				return fmt.Errorf("snapshot-ami: DescribeImages: %w", err)
			}
			if len(desc.Images) > 0 {
				state = string(desc.Images[0].State)
				if state == string(types.ImageStateAvailable) {
					break
				}
				if state == string(types.ImageStateFailed) {
					return fmt.Errorf("snapshot-ami: AMI %s failed to become available", amiID)
				}
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(pollInterval):
			}
		}
	}

	// Output result.
	if outputFmt == "json" {
		return printSnapshotJSON(amiID, name, state, region)
	}
	return printSnapshotText(amiID, name, state, region)
}

func printSnapshotText(amiID, name, state, region string) error {
	fmt.Printf("snapshot: %s\n", amiID)
	fmt.Printf("  name:   %s\n", name)
	fmt.Printf("  state:  %s\n", state)
	fmt.Printf("  region: %s\n", region)
	fmt.Printf("  hint:   use as base in a profile: base: {os: %s}\n", amiID)
	fmt.Printf("  hint:   or as --ami in: strata build --ec2 --ami %s\n", amiID)
	return nil
}

func printSnapshotJSON(amiID, name, state, region string) error {
	out := struct {
		AMIID  string `json:"ami_id"`
		Name   string `json:"name"`
		State  string `json:"state"`
		Region string `json:"region"`
	}{
		AMIID:  amiID,
		Name:   name,
		State:  state,
		Region: region,
	}
	data, err := json.Marshal(out)
	if err != nil {
		return fmt.Errorf("snapshot-ami: marshaling JSON output: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

// fetchInstanceIDFromIMDS fetches the EC2 instance ID from IMDSv2.
// Uses a token-based request (IMDSv2) as required on AL2023.
func fetchInstanceIDFromIMDS(ctx context.Context) (string, error) {
	const (
		tokenURL     = "http://169.254.169.254/latest/api/token"
		metadataURL  = "http://169.254.169.254/latest/meta-data/instance-id"
		tokenTTLSecs = "21600"
	)

	httpClient := &http.Client{Timeout: 5 * time.Second}

	// Step 1: Get IMDSv2 token.
	tokenReq, err := http.NewRequestWithContext(ctx, http.MethodPut, tokenURL, nil)
	if err != nil {
		return "", fmt.Errorf("creating token request: %w", err)
	}
	tokenReq.Header.Set("X-aws-ec2-metadata-token-ttl-seconds", tokenTTLSecs)

	tokenResp, err := httpClient.Do(tokenReq)
	if err != nil {
		return "", fmt.Errorf("fetching IMDS token (not on EC2?): %w", err)
	}
	defer tokenResp.Body.Close() //nolint:errcheck
	tokenBytes, err := io.ReadAll(tokenResp.Body)
	if err != nil {
		return "", fmt.Errorf("reading IMDS token: %w", err)
	}
	token := string(tokenBytes)

	// Step 2: Fetch instance-id using the token.
	metaReq, err := http.NewRequestWithContext(ctx, http.MethodGet, metadataURL, nil)
	if err != nil {
		return "", fmt.Errorf("creating metadata request: %w", err)
	}
	metaReq.Header.Set("X-aws-ec2-metadata-token", token)

	metaResp, err := httpClient.Do(metaReq)
	if err != nil {
		return "", fmt.Errorf("fetching instance ID from IMDS: %w", err)
	}
	defer metaResp.Body.Close() //nolint:errcheck
	if metaResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("IMDS returned HTTP %d for instance-id", metaResp.StatusCode)
	}
	idBytes, err := io.ReadAll(metaResp.Body)
	if err != nil {
		return "", fmt.Errorf("reading instance ID from IMDS: %w", err)
	}

	instanceID := string(idBytes)
	if instanceID == "" {
		return "", fmt.Errorf("IMDS returned empty instance ID")
	}
	return instanceID, nil
}
