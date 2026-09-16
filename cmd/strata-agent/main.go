// Copyright 2026 Scott Friedman
// SPDX-License-Identifier: Apache-2.0

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
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// layerFetcher is the fetcher surface run needs: the two agent interfaces plus
// the download stats the agent does not track. s3LayerFetcher satisfies it.
type layerFetcher interface {
	agent.LayerFetcher
	agent.BundleFetcher
	Stats() FetchStats
}

// bootSignaler is the signaler surface run needs: the agent's ReadySignaler plus
// getInstanceID for the metrics upload. ec2ReadySignaler satisfies it.
type bootSignaler interface {
	agent.ReadySignaler
	getInstanceID(ctx context.Context) (string, error)
}

// agentDeps are run's injected dependencies. Everything AWS-backed is behind an
// interface so run — and in particular the verifier-refusal path, the most
// security-relevant branch in the agent (#56, #94) — is reachable in a test with
// no cosign, no AWS, and no real boot.
type agentDeps struct {
	source          agent.LockfileSource
	fetcher         layerFetcher
	signaler        bootSignaler
	installer       agent.PackageInstaller
	resolveVerifier func(ctx context.Context) (trust.Verifier, error)
	allowUnverified bool
}

func main() {
	ctx := context.Background()

	fetcher := newS3LayerFetcher()
	signaler := newEC2ReadySignaler()

	// main is the one line run does not cover: it wires the production
	// dependencies and turns run's returned error into an exit. Everything with
	// behaviour lives in run, which a test drives with fakes.
	err := run(ctx, agentDeps{
		source:    newMetadataLockfileSource(),
		fetcher:   fetcher,
		signaler:  signaler,
		installer: agent.ExecPackageInstaller{},
		resolveVerifier: func(ctx context.Context) (trust.Verifier, error) {
			return resolveVerifier(ctx, productionPrereqs(os.Getenv))
		},
		allowUnverified: allowUnverified(os.Getenv),
	})
	if err != nil {
		log.Fatalf("strata-agent: %v", err)
	}
}

// run executes the boot sequence and returns an error instead of exiting, so its
// two security properties are assertable rather than held by reading main: that
// a verifier refusal is fatal (a returned non-nil error), and that the agent is
// signalled before the boot dies.
func run(ctx context.Context, d agentDeps) error {
	// Resolve the verifier before anything else is built. A boot that cannot
	// check authenticity is a boot that stops, and it stops here rather than
	// six steps later with the layers already mounted.
	verifier, err := d.resolveVerifier(ctx)
	if err != nil {
		// Signal before dying. An instance that refuses to boot and says
		// nothing is indistinguishable from an instance that hung, and the
		// operator is paying for it either way. Signalling is best-effort; its
		// failure is logged and must not make the refusal non-fatal.
		if sigErr := d.signaler.SignalFailed(ctx, err); sigErr != nil {
			log.Printf("strata-agent: could not signal boot failure: %v", sigErr)
		}
		return err
	}

	a, err := agent.New(agent.Config{
		Source:        d.source,
		Fetcher:       d.fetcher,
		BundleFetcher: d.fetcher,
		Verifier:      verifier,
		Signaler:      d.signaler,
		// verifier is nil only when the operator opted out above (resolveVerifier
		// errors otherwise), so the agent must be told the nil is deliberate —
		// else it refuses to boot (#93). Passing the same predicate keeps the two
		// decisions in step.
		PackageInstaller: d.installer,
		AllowUnverified:  d.allowUnverified,
		Warnings:         os.Stderr, // boot log records the unattested-packages disclosure (#139)
	})
	if err != nil {
		return err
	}

	metrics, err := a.Run(ctx)
	if metrics != nil {
		// Populate fetch stats from the fetcher (not tracked inside agent.Run).
		stats := d.fetcher.Stats()
		metrics.FetchBytes = stats.BytesDownloaded
		metrics.CachedLayers = stats.CachedLayers
		metrics.DownloadedLayers = stats.DownloadedLayers
		writeBootMetrics(ctx, metrics, d.signaler)
	}
	return err
}

// allowUnverifiedEnv opts the agent out of authenticity verification. Unset —
// or set to anything other than an affirmative value — means verification is
// required and the agent refuses to boot without it.
//
// An environment variable rather than a flag because the agent is a systemd
// unit with no meaningful argv, and because it matches the existing
// STRATA_REGISTRY_BUCKET lever. Setting it is a recorded act in the instance's
// user-data or unit file, which is the property that matters: the previous
// behaviour was a downgrade nobody had to ask for.
const allowUnverifiedEnv = "STRATA_AGENT_ALLOW_UNVERIFIED"

// verifierPrereqs are the inputs the verifier decision consults.
//
// They are injected rather than called directly so that every refusal direction
// is reachable in a test with no cosign installed and no AWS credentials — which
// is the state of CI, and was the reason this branch had 0.0% coverage while
// being the most security-relevant one in the agent (#56).
type verifierPrereqs struct {
	// lookPath resolves an executable, like exec.LookPath.
	lookPath func(file string) (string, error)
	// fetchKey returns a local path to the cosign public key, or "" on failure.
	fetchKey func(ctx context.Context) string
	// allowUnverified is the operator's explicit opt-out. False means refuse.
	allowUnverified bool
}

// productionPrereqs wires the real prerequisites. getenv is a parameter so the
// default this function produces — including the closed default for the opt-out
// — is observable in a test rather than only by reading it.
func productionPrereqs(getenv func(string) string) verifierPrereqs {
	return verifierPrereqs{
		lookPath:        exec.LookPath,
		fetchKey:        writeEmbeddedCosignKey,
		allowUnverified: allowUnverified(getenv),
	}
}

// allowUnverified reports whether the operator has explicitly opted out of
// authenticity verification.
//
// Defaults to false for every input, including unset, empty, and unrecognised
// values. A typo in the variable name or its value leaves verification on: the
// direction of error for a security default is towards refusing.
func allowUnverified(getenv func(string) string) bool {
	switch strings.ToLower(strings.TrimSpace(getenv(allowUnverifiedEnv))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// resolveVerifier returns the verifier the agent should boot with.
//
// A non-nil error means the boot must stop. A nil verifier with a nil error
// means the operator explicitly opted out — that pairing is the *only* way a
// nil verifier is returned, and it is the invariant this function exists to
// hold. Previously a nil verifier was returned on any prerequisite failure,
// which made "cosign is not installed" and "the operator accepted the risk"
// the same value.
//
// Why refusing is right here even though it costs boot availability: the agent
// still checks SHA-256 against the lockfile either way, so what is lost when
// verification is skipped is not integrity but *authenticity* — the lockfile
// stops being evidence about who produced the layers, and a lockfile is exactly
// the artifact an attacker would supply. Degrading that silently trades a
// property nobody can observe was lost.
func resolveVerifier(ctx context.Context, p verifierPrereqs) (trust.Verifier, error) {
	v, err := newCosignVerifier(ctx, p)
	if err == nil {
		return v, nil
	}
	if !p.allowUnverified {
		return nil, fmt.Errorf("%w (set %s=1 to boot without authenticity verification)", err, allowUnverifiedEnv)
	}
	log.Printf("strata-agent: WARNING: %s is set: booting WITHOUT authenticity verification (%v). "+
		"SHA-256 integrity against the lockfile is still enforced; layer authorship is not checked.",
		allowUnverifiedEnv, err)
	return nil, nil
}

// newCosignVerifier builds a cosign verifier from the available prerequisites.
//
// It never returns a nil Verifier with a nil error: either both prerequisites
// are present and a verifier is returned, or the missing one is named in an
// error. Returning nil for "cannot verify" is the fail-open this replaces — and
// it is not even a skip, since internal/trust/verify.go calls v.Verify
// unconditionally.
//
// Prerequisites are reported in the order they are checked, and cosign comes
// first deliberately: without the binary the key is useless, so naming the key
// fetch when both are missing would send an operator to the wrong problem.
func newCosignVerifier(ctx context.Context, p verifierPrereqs) (trust.Verifier, error) {
	if _, err := p.lookPath("cosign"); err != nil {
		return nil, fmt.Errorf("cosign not found on PATH: layer signatures cannot be verified: %w", err)
	}
	keyPath := p.fetchKey(ctx)
	if keyPath == "" {
		return nil, fmt.Errorf("the embedded cosign public key is unavailable: layer signatures cannot be verified")
	}
	return &trust.CosignVerifier{KeyRef: keyPath}, nil
}

// writeEmbeddedCosignKey materialises the Strata public signing key — pinned into
// the binary and shared across binaries in internal/trust (#62) — to a temp file
// and returns its path, or "" on failure. cosign (via CosignVerifier.KeyRef)
// wants a file path, not bytes.
//
// An empty embed returns "": a binary built without a trust anchor cannot
// verify, and its caller turns "" into a refusal to boot unless the operator has
// opted out — the same fail-closed path the S3 fetch fed. The agent is
// short-lived, so the temp file is left for the OS to reclaim rather than tracked
// for cleanup.
func writeEmbeddedCosignKey(context.Context) string {
	path, _, err := trust.WriteEmbeddedKeyFile()
	if err != nil {
		log.Printf("strata-agent: %v", err)
		return ""
	}
	return path
}

// writeBootMetrics logs metrics to stderr, writes to /etc/strata/boot-metrics.json,
// and uploads best-effort to S3 for later analysis.
func writeBootMetrics(ctx context.Context, m *agent.BootMetrics, signaler bootSignaler) {
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
