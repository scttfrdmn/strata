// Command strata-agent is the Strata instance bootstrap daemon.
//
// It runs as a systemd service (strata-agent.service) at instance boot and
// executes the 6-step boot sequence: acquire lockfile → fetch layers →
// verify bundles → mount overlay → configure env → signal ready.
//
// AWS integrations are implemented in metadata_source.go, s3_fetcher.go, and
// ec2_signaler.go. Unit tests with mocks are in agent_aws_test.go.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/scttfrdmn/strata/internal/agent"
	"github.com/scttfrdmn/strata/internal/trust"
)

// strataRegistryBucket is the S3 bucket used for the Strata registry, public
// keys, and metrics. Configurable via STRATA_REGISTRY_BUCKET env var to
// support non-default deployments.
const defaultRegistryBucket = "strata-registry"

func registryBucket() string {
	if v := os.Getenv("STRATA_REGISTRY_BUCKET"); v != "" {
		return v
	}
	return defaultRegistryBucket
}

func main() {
	ctx := context.Background()

	fetcher := newS3LayerFetcher()
	signaler := newEC2ReadySignaler()

	a, err := agent.New(agent.Config{
		Source:           newMetadataLockfileSource(),
		Fetcher:          fetcher,
		BundleFetcher:    fetcher,
		Verifier:         newCosignVerifier(ctx),
		Signaler:         signaler,
		PackageInstaller: agent.ExecPackageInstaller{},
	})
	if err != nil {
		log.Fatal(err)
	}

	metrics, err := a.Run(ctx)
	if metrics != nil {
		// Populate fetch stats from the fetcher (not tracked inside agent.Run).
		stats := fetcher.Stats()
		metrics.FetchBytes = stats.BytesDownloaded
		metrics.CachedLayers = stats.CachedLayers
		metrics.DownloadedLayers = stats.DownloadedLayers
		writeBootMetrics(ctx, metrics, signaler)
	}
	if err != nil {
		log.Fatalf("strata-agent: %v", err)
	}
}

// newCosignVerifier checks that cosign is available on PATH and downloads the
// public key from S3. Returns nil when either prerequisite is missing so the
// agent degrades gracefully (SHA256 integrity is still enforced).
func newCosignVerifier(ctx context.Context) trust.Verifier {
	if _, err := exec.LookPath("cosign"); err != nil {
		log.Printf("strata-agent: cosign not found on PATH; skipping bundle verification")
		return nil
	}
	keyPath := fetchPublicKey(ctx)
	if keyPath == "" {
		log.Printf("strata-agent: could not fetch cosign public key; skipping bundle verification")
		return nil
	}
	return &trust.CosignVerifier{KeyRef: keyPath}
}

// fetchPublicKey downloads the Strata cosign public key from S3 to a temp
// file and returns its path. Returns "" on any error (non-fatal).
func fetchPublicKey(ctx context.Context) string {
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		log.Printf("strata-agent: fetchPublicKey: loading AWS config: %v", err)
		return ""
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	s3Client := s3.NewFromConfig(cfg)

	const keyObject = "build/keys/cosign.pub"
	out, err := s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(registryBucket()),
		Key:    aws.String(keyObject),
	})
	if err != nil {
		log.Printf("strata-agent: fetchPublicKey: %v", err)
		return ""
	}
	defer out.Body.Close() //nolint:errcheck

	const maxPubKeyBytes = 4096 // cosign public keys are ~500 bytes; 4 KiB is generous
	data, err := io.ReadAll(io.LimitReader(out.Body, maxPubKeyBytes))
	if err != nil {
		log.Printf("strata-agent: fetchPublicKey: reading key: %v", err)
		return ""
	}

	f, err := os.CreateTemp("", "strata-cosign-*.pub")
	if err != nil {
		return ""
	}
	if _, err := f.Write(data); err != nil {
		f.Close()           //nolint:errcheck
		os.Remove(f.Name()) //nolint:errcheck
		return ""
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name()) //nolint:errcheck
		return ""
	}
	return f.Name()
}

// writeBootMetrics logs metrics to stderr, writes to /etc/strata/boot-metrics.json,
// and uploads best-effort to S3 for later analysis.
func writeBootMetrics(ctx context.Context, m *agent.BootMetrics, signaler *ec2ReadySignaler) {
	data, err := json.Marshal(m)
	if err != nil {
		log.Printf("strata-agent: marshaling boot metrics: %v", err)
		return
	}

	// 1. Log single-line JSON to stderr (visible in journalctl -u strata-agent).
	log.Printf("strata-agent: boot metrics: %s", data)

	// 2. Write to /etc/strata/boot-metrics.json.
	const metricsFile = "/etc/strata/boot-metrics.json"
	if mkErr := os.MkdirAll(filepath.Dir(metricsFile), 0o755); mkErr == nil {
		_ = os.WriteFile(metricsFile, data, 0o644)
	}

	// 3. Best-effort S3 upload: s3://strata-registry/metrics/<instance-id>/<ts>.json
	instanceID, err := signaler.getInstanceID(ctx)
	if err != nil {
		return // IMDS unavailable — skip upload
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	s3Client := s3.NewFromConfig(cfg)
	ts := m.StartedAt.UTC().Format(time.RFC3339)
	key := "metrics/" + instanceID + "/" + ts + ".json"
	_, _ = s3Client.PutObject(ctx, &s3.PutObjectInput{ //nolint:errcheck
		Bucket:      aws.String(registryBucket()),
		Key:         &key,
		Body:        bytes.NewReader(data),
		ContentType: aws.String("application/json"),
	})
}
