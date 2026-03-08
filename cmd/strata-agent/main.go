// Command strata-agent is the Strata instance bootstrap daemon.
//
// It runs as a systemd service (strata-agent.service) at instance boot and
// executes the 6-step boot sequence: acquire lockfile → verify → pull layers
// → mount overlay → configure env → signal ready.
//
// AWS integrations are implemented in metadata_source.go, s3_fetcher.go, and
// ec2_signaler.go. Unit tests with mocks are in agent_aws_test.go.
package main

import (
	"context"
	"log"

	"github.com/scttfrdmn/strata/internal/agent"
	"github.com/scttfrdmn/strata/internal/trust"
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
