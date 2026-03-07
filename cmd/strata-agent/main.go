// Command strata-agent is the Strata instance bootstrap daemon.
//
// It runs as a systemd service (strata-agent.service) at instance boot and
// executes the 6-step boot sequence: acquire lockfile → verify → pull layers
// → mount overlay → configure env → signal ready.
//
// Real AWS implementations (EC2 metadata, S3, CloudWatch) are stubbed with
// TODO markers pending follow-on work. The binary compiles and the agent logic
// is fully tested via the internal/agent package.
package main

import (
	"context"
	"errors"
	"log"

	"github.com/scttfrdmn/strata/internal/agent"
	"github.com/scttfrdmn/strata/internal/trust"
	"github.com/scttfrdmn/strata/spec"
)

func main() {
	ctx := context.Background()

	a, err := agent.New(agent.Config{
		Source:   newMetadataLockfileSource(),
		Fetcher:  newS3LayerFetcher(),
		Verifier: &trust.CosignVerifier{}, // TODO: configure KeyRef or CertIdentity+CertOIDCIssuer
		Signaler: newEC2ReadySignaler(),
	})
	if err != nil {
		log.Fatal(err)
	}
	if err := a.Run(ctx); err != nil {
		log.Fatalf("strata-agent: %v", err)
	}
}

// metadataLockfileSource acquires the lockfile from EC2 instance metadata.
// Priority: user-data → S3 URL in instance tag → direct S3 path.
type metadataLockfileSource struct{}

func newMetadataLockfileSource() agent.LockfileSource {
	return &metadataLockfileSource{}
}

func (s *metadataLockfileSource) Acquire(_ context.Context) (*spec.LockFile, error) {
	// TODO: implement EC2 metadata service lookup
	// 1. GET http://169.254.169.254/latest/user-data → parse as LockFile YAML
	// 2. Fall back to instance tag "strata:lockfile-s3-uri" → fetch from S3
	return nil, errors.New("metadataLockfileSource: not yet implemented")
}

// s3LayerFetcher downloads squashfs layers from S3 to the local layer cache.
type s3LayerFetcher struct{}

func newS3LayerFetcher() agent.LayerFetcher {
	return &s3LayerFetcher{}
}

func (f *s3LayerFetcher) Fetch(_ context.Context, _ spec.ResolvedLayer) (string, error) {
	// TODO: implement S3 download with resume support
	// 1. Check cache at /strata/cache/<layer.SHA256>.sqfs
	// 2. If not present, download from layer.Source (s3://strata-layers/...)
	// 3. Return local cache path
	return "", errors.New("s3LayerFetcher: not yet implemented")
}

// ec2ReadySignaler writes instance tags and calls sd_notify to report status.
type ec2ReadySignaler struct{}

func newEC2ReadySignaler() agent.ReadySignaler {
	return &ec2ReadySignaler{}
}

func (s *ec2ReadySignaler) SignalReady(_ context.Context, lockfile *spec.LockFile) error {
	// TODO: implement
	// 1. Write EC2 tag "strata:status" = "ready"
	// 2. Write EC2 tag "strata:environment-id" = lockfile.EnvironmentID()
	// 3. Call sd_notify("READY=1")
	_ = lockfile
	return errors.New("ec2ReadySignaler.SignalReady: not yet implemented")
}

func (s *ec2ReadySignaler) SignalFailed(_ context.Context, reason error) error {
	// TODO: implement
	// 1. Write EC2 tag "strata:status" = "failed"
	// 2. Write EC2 tag "strata:failure-reason" = reason.Error()
	// 3. Call sd_notify("STATUS=failed: " + reason.Error())
	_ = reason
	return nil
}
