package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/scttfrdmn/strata/spec"
)

// ec2TagAPI is the subset of ec2.Client used for instance tagging.
// Defined as an interface to allow mock injection in tests.
type ec2TagAPI interface {
	CreateTags(ctx context.Context, in *ec2.CreateTagsInput,
		opts ...func(*ec2.Options)) (*ec2.CreateTagsOutput, error)
}

// ec2ReadySignaler writes EC2 instance tags and calls sd_notify to report status.
type ec2ReadySignaler struct {
	imds imdsAPI
	ec2  ec2TagAPI
}

// newEC2ReadySignaler creates an ec2ReadySignaler backed by real AWS clients.
func newEC2ReadySignaler() *ec2ReadySignaler {
	imdsClient := imds.New(imds.Options{})
	cfg, err := awsconfig.LoadDefaultConfig(context.Background())
	if err != nil {
		return &ec2ReadySignaler{imds: imdsClient}
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	return &ec2ReadySignaler{
		imds: imdsClient,
		ec2:  ec2.NewFromConfig(cfg),
	}
}

// newEC2ReadySignalerWithAPIs constructs an ec2ReadySignaler with injected
// interfaces — used by tests.
func newEC2ReadySignalerWithAPIs(imdsClient imdsAPI, ec2Client ec2TagAPI) *ec2ReadySignaler {
	return &ec2ReadySignaler{imds: imdsClient, ec2: ec2Client}
}

// getInstanceID fetches the current EC2 instance ID from IMDS.
func (s *ec2ReadySignaler) getInstanceID(ctx context.Context) (string, error) {
	out, err := s.imds.GetMetadata(ctx, &imds.GetMetadataInput{
		Path: "instance-id",
	})
	if err != nil {
		return "", fmt.Errorf("ec2ReadySignaler: getting instance-id: %w", err)
	}
	defer out.Content.Close()      //nolint:errcheck
	const maxInstanceIDBytes = 256 // EC2 instance IDs are 19 bytes; 256 is generous
	data, err := io.ReadAll(io.LimitReader(out.Content, maxInstanceIDBytes))
	if err != nil {
		return "", fmt.Errorf("ec2ReadySignaler: reading instance-id: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

// SignalReady sets strata:status=ready and strata:environment-id instance tags
// and notifies systemd via sd_notify.
func (s *ec2ReadySignaler) SignalReady(ctx context.Context, lockfile *spec.LockFile) error {
	instanceID, err := s.getInstanceID(ctx)
	if err != nil {
		return err
	}
	if s.ec2 != nil {
		if _, err := s.ec2.CreateTags(ctx, &ec2.CreateTagsInput{
			Resources: []string{instanceID},
			Tags: []ec2types.Tag{
				{Key: aws.String("strata:status"), Value: aws.String("ready")},
				{Key: aws.String("strata:environment-id"), Value: aws.String(lockfile.EnvironmentID())},
			},
		}); err != nil {
			return fmt.Errorf("ec2ReadySignaler: setting ready tags: %w", err)
		}
	}
	return sdNotify("READY=1")
}

// SignalFailed sets strata:status=failed and strata:failure-reason instance tags
// and notifies systemd. All operations are best-effort; errors are not propagated
// since SignalFailed is called during an already-failed boot sequence.
func (s *ec2ReadySignaler) SignalFailed(ctx context.Context, reason error) error {
	msg := reason.Error()
	if len(msg) > 256 {
		msg = msg[:256]
	}
	// Best-effort EC2 tagging.
	if s.ec2 != nil {
		if instanceID, err := s.getInstanceID(ctx); err == nil {
			_, _ = s.ec2.CreateTags(ctx, &ec2.CreateTagsInput{ //nolint:errcheck
				Resources: []string{instanceID},
				Tags: []ec2types.Tag{
					{Key: aws.String("strata:status"), Value: aws.String("failed")},
					{Key: aws.String("strata:failure-reason"), Value: aws.String(msg)},
				},
			})
		}
	}
	return sdNotify("STATUS=failed: " + reason.Error())
}

// sdNotify sends a notification to systemd via NOTIFY_SOCKET.
// Returns nil silently if NOTIFY_SOCKET is not set (not running under systemd).
func sdNotify(state string) error {
	sock := os.Getenv("NOTIFY_SOCKET")
	if sock == "" {
		return nil
	}
	conn, err := net.DialUnix("unixgram", nil, &net.UnixAddr{Name: sock, Net: "unixgram"})
	if err != nil {
		return fmt.Errorf("sd_notify: %w", err)
	}
	defer conn.Close() //nolint:errcheck
	_, err = conn.Write([]byte(state))
	return err
}
