// Copyright 2026 Scott Friedman
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// EC2Config holds configuration for launching EC2 build instances.
type EC2Config struct {
	// Region is the AWS region, e.g. "us-east-1".
	Region string

	// AMIID is the base AMI ID for the build instance (must match target OS/arch).
	AMIID string

	// InstanceType is the EC2 instance type, e.g. "c5.4xlarge" or "c6g.4xlarge".
	InstanceType string

	// SubnetID is the subnet to launch into; AWS picks a subnet if empty.
	SubnetID string

	// SecurityGroupID is the security group ID, e.g. "sg-0fca02f58fafcdad1".
	SecurityGroupID string

	// IAMProfile is the IAM instance profile name, e.g. "strata-builder".
	IAMProfile string

	// BucketURL is the S3 registry URL, e.g. "s3://strata-registry".
	BucketURL string

	// BinaryArch is the strata binary architecture suffix: "amd64" or "arm64".
	BinaryArch string

	// KeyRef is the cosign key reference. If it begins with "s3://", the
	// user-data script downloads it to /root/.strata-keys/cosign.key and
	// passes that local path to --key. Empty = keyless OIDC.
	KeyRef string

	// CosignVersion is the cosign release tag to install on the build instance,
	// e.g. "v3.0.5". Defaults to "v3.0.5".
	CosignVersion string

	// RootVolumeGB overrides the root EBS volume size in GiB.
	// Default 60. Some AMIs (e.g. DLAMIs) require a larger minimum.
	RootVolumeGB int32

	// PollInterval is how often to poll for build completion. Default 30s.
	PollInterval time.Duration

	// UseSpawn launches the build instance through the pinned spore.host spawn
	// CLI instead of the AWS SDK, so the instance gets spawn's TTL/auto-terminate
	// lifecycle and cannot linger after a failed or hung build (#236). It is
	// fire-and-forget: spawn owns the lifecycle, so it pairs with the no-wait
	// launch path, not the tag-polling one.
	UseSpawn bool

	// TTL is the spawn auto-terminate window (e.g. "4h") when UseSpawn is set:
	// the instance terminates after it regardless of build outcome — the hard
	// cost ceiling. Empty lets spawn apply its default idle timeout.
	TTL string
}

// ec2LaunchAPI is the subset of ec2.Client used by EC2Runner.
// Defined as an interface to allow mock injection in tests.
type ec2LaunchAPI interface {
	RunInstances(ctx context.Context, in *ec2.RunInstancesInput, opts ...func(*ec2.Options)) (*ec2.RunInstancesOutput, error)
	TerminateInstances(ctx context.Context, in *ec2.TerminateInstancesInput, opts ...func(*ec2.Options)) (*ec2.TerminateInstancesOutput, error)
	DescribeInstances(ctx context.Context, in *ec2.DescribeInstancesInput, opts ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error)
}

// s3PutAPI is the subset of s3.Client used by EC2Runner to upload recipe files.
type s3PutAPI interface {
	PutObject(ctx context.Context, in *s3.PutObjectInput, opts ...func(*s3.Options)) (*s3.PutObjectOutput, error)
}

// EC2Runner orchestrates single-layer builds on EC2 instances.
// It uploads the recipe, launches an instance that runs `strata build`
// locally, polls for completion via instance tags, and terminates the instance.
type EC2Runner struct {
	cfg EC2Config
	ec2 ec2LaunchAPI
	s3  s3PutAPI
	// spawn is non-nil when cfg.UseSpawn is set: the build instance is launched
	// through the pinned spawn CLI rather than r.ec2.RunInstances (#236).
	spawn *SpawnLauncher
}

// NewEC2Runner creates an EC2Runner backed by real AWS clients using the
// default credential provider chain.
func NewEC2Runner(cfg EC2Config) (*EC2Runner, error) {
	if cfg.PollInterval == 0 {
		cfg.PollInterval = 30 * time.Second
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(cfg.Region))
	if err != nil {
		return nil, fmt.Errorf("ec2runner: loading AWS config: %w", err)
	}
	bucket, _, ok := parseObjectURI(cfg.BucketURL + "/placeholder")
	if !ok {
		return nil, fmt.Errorf("ec2runner: invalid bucket URL %q", cfg.BucketURL)
	}
	_ = bucket
	r := &EC2Runner{
		cfg: cfg,
		ec2: ec2.NewFromConfig(awsCfg),
		s3:  s3.NewFromConfig(awsCfg),
	}
	if cfg.UseSpawn {
		r.spawn = NewSpawnLauncher(cfg, cfg.TTL)
	}
	return r, nil
}

// newEC2RunnerWithAPIs constructs an EC2Runner with injected APIs for testing.
func newEC2RunnerWithAPIs(cfg EC2Config, ec2API ec2LaunchAPI, s3API s3PutAPI) *EC2Runner {
	if cfg.PollInterval == 0 {
		cfg.PollInterval = 30 * time.Second
	}
	return &EC2Runner{cfg: cfg, ec2: ec2API, s3: s3API}
}

// rootVolumeGB returns the configured root volume size, defaulting to 60 GiB.
func (r *EC2Runner) rootVolumeGB() int32 {
	if r.cfg.RootVolumeGB > 0 {
		return r.cfg.RootVolumeGB
	}
	return 60
}

// LaunchBuildEC2 uploads the recipe to S3 and launches an EC2 build instance,
// returning the instance ID immediately without waiting for the build to complete.
// The instance self-terminates on success or self-stops on failure (for log
// retrieval). Monitor progress via the strata:build-status tag.
//
// When cfg.UseSpawn is set the instance is launched through the pinned spawn CLI
// with a TTL and on-complete=terminate (#236), so a failed or hung build cannot
// linger: spawn owns the lifecycle. The failure log is still uploaded to S3 by
// the user-data before the instance stops, and spawn's TTL reaps it.
func (r *EC2Runner) LaunchBuildEC2(ctx context.Context, jobID string, recipe *Recipe, job *Job) (instanceID string, err error) {
	if err := r.uploadRecipe(ctx, jobID, recipe); err != nil {
		return "", fmt.Errorf("ec2runner: uploading recipe: %w", err)
	}
	userData, err := r.buildUserData(jobID, recipe, job)
	if err != nil {
		return "", fmt.Errorf("ec2runner: generating user-data: %w", err)
	}
	if r.spawn != nil {
		instanceID, err = r.spawn.Launch(ctx, buildInstanceName(recipe, r.cfg.BinaryArch), userData)
		if err != nil {
			return "", fmt.Errorf("ec2runner: launching instance via spawn: %w", err)
		}
		return instanceID, nil
	}
	instanceID, err = r.launchInstance(ctx, userData, recipe)
	if err != nil {
		return "", fmt.Errorf("ec2runner: launching instance: %w", err)
	}
	return instanceID, nil
}

// RunBuildEC2 launches a build instance and polls until completion, then
// terminates it. For fire-and-forget launches, use LaunchBuildEC2.
func (r *EC2Runner) RunBuildEC2(ctx context.Context, jobID string, recipe *Recipe, job *Job) (instanceID string, err error) {
	instanceID, err = r.LaunchBuildEC2(ctx, jobID, recipe, job)
	if err != nil {
		return "", err
	}

	// Poll until build completes (success or failed).
	if pollErr := r.pollUntilComplete(ctx, instanceID); pollErr != nil {
		_, _ = r.ec2.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
			InstanceIds: []string{instanceID},
		})
		return instanceID, fmt.Errorf("ec2runner: build did not complete: %w", pollErr)
	}

	// Best-effort terminate — on success the instance already self-terminated;
	// on failure it self-stopped so we clean it up here.
	if _, terr := r.ec2.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
		InstanceIds: []string{instanceID},
	}); terr != nil {
		fmt.Fprintf(os.Stderr, "ec2runner: warning: terminating %s: %v\n", instanceID, terr)
	}

	return instanceID, nil
}

// uploadRecipe uploads build.sh and meta.yaml to S3 under
// build/jobs/<jobID>/recipe/.
func (r *EC2Runner) uploadRecipe(ctx context.Context, jobID string, recipe *Recipe) error {
	bucket, _, ok := parseObjectURI(r.cfg.BucketURL + "/placeholder")
	if !ok {
		return fmt.Errorf("invalid bucket URL %q", r.cfg.BucketURL)
	}

	for _, fname := range []string{"build.sh", "meta.yaml"} {
		localPath := filepath.Join(recipe.Dir, fname)
		data, err := os.ReadFile(localPath)
		if err != nil {
			return fmt.Errorf("reading %s: %w", fname, err)
		}
		key := fmt.Sprintf("build/jobs/%s/recipe/%s", jobID, fname)
		ct := "text/plain"
		if fname == "meta.yaml" {
			ct = "application/yaml"
		}
		if _, err := r.s3.PutObject(ctx, &s3.PutObjectInput{
			Bucket:      aws.String(bucket),
			Key:         aws.String(key),
			Body:        bytes.NewReader(data),
			ContentType: aws.String(ct),
		}); err != nil {
			return fmt.Errorf("uploading %s: %w", fname, err)
		}
	}
	return nil
}

// cosignReleaseDigests pins the SHA-256 of each cosign release binary the build
// pipeline may install, keyed by release tag then linux arch suffix.
//
// The build instance downloads cosign from GitHub releases (see the user-data
// template) and signs every layer with it via KMS. A substituted cosign calls
// kms:Sign with the real strata-builder instance credentials and produces
// bundles that verify against the published key, so an unpinned fetch is the
// weakest link in the KMS-key hardening #32 built (#63). This is what the
// download is checked against before it is trusted.
//
// Provenance: each value is from cosign's published cosign_checksums.txt for
// that tag —
// https://github.com/sigstore/cosign/releases/download/<tag>/cosign_checksums.txt
// — fetched 2026-09-12. To add a version, fetch its checksums file and copy the
// cosign-linux-amd64 and cosign-linux-arm64 lines here.
var cosignReleaseDigests = map[string]map[string]string{
	"v3.0.5": {
		"amd64": "db15cc99e6e4837daabab023742aaddc3841ce57f193d11b7c3e06c8003642b2",
		"arm64": "d098f3168ae4b3aa70b4ca78947329b953272b487727d1722cb3cb098a1a20ab",
	},
}

// cosignReleaseDigest returns the pinned SHA-256 for a cosign release binary, or
// an error if the (version, arch) pair is not pinned. An unpinned version cannot
// be installed: there is no trusted digest to check the download against, and
// fetching the checksum alongside the binary would be circular (#63).
func cosignReleaseDigest(version, arch string) (string, error) {
	byArch, ok := cosignReleaseDigests[version]
	if !ok {
		return "", fmt.Errorf("no pinned cosign digest for version %q: add it to cosignReleaseDigests from that release's cosign_checksums.txt", version)
	}
	digest, ok := byArch[arch]
	if !ok {
		return "", fmt.Errorf("no pinned cosign digest for version %q arch %q", version, arch)
	}
	return digest, nil
}

// buildUserData generates the EC2 user-data shell script.
func (r *EC2Runner) buildUserData(jobID string, recipe *Recipe, job *Job) (string, error) {
	bucket, _, ok := parseObjectURI(r.cfg.BucketURL + "/placeholder")
	if !ok {
		return "", fmt.Errorf("invalid bucket URL %q", r.cfg.BucketURL)
	}

	cosignVersion := r.cfg.CosignVersion
	if cosignVersion == "" {
		cosignVersion = "v3.0.5"
	}
	// Fail early with a clean message if the requested cosign is not pinned,
	// rather than only through the template's cosignDigest func at Execute time.
	if _, err := cosignReleaseDigest(cosignVersion, r.cfg.BinaryArch); err != nil {
		return "", fmt.Errorf("buildUserData: %w", err)
	}

	// If the key is an S3 URI, the user-data downloads it to a local path.
	// Otherwise pass the key ref directly (KMS URI or local path baked in AMI).
	const localKeyPath = "/root/.strata-keys/cosign.key"
	keyS3URI := ""
	keyFlag := ""
	switch {
	case strings.HasPrefix(r.cfg.KeyRef, "s3://"):
		keyS3URI = r.cfg.KeyRef
		keyFlag = " --key " + localKeyPath
	case r.cfg.KeyRef != "":
		keyFlag = " --key " + r.cfg.KeyRef
	}

	// Validate fields that are interpolated into shell scripts.
	for field, val := range map[string]string{
		"RecipeName":    recipe.Meta.Name,
		"RecipeVersion": recipe.Meta.Version,
		"Arch":          job.Base.NormalizedArch(),
	} {
		if !safeNameRe.MatchString(val) {
			return "", fmt.Errorf("buildUserData: %s %q contains unsafe characters", field, val)
		}
	}

	data := struct {
		Bucket        string
		JobID         string
		BinaryArch    string
		Region        string
		OS            string
		Arch          string
		RegistryURL   string
		KeyFlag       string
		KeyS3URI      string
		LocalKeyPath  string
		CosignVersion string
		RecipeName    string
		RecipeVersion string
	}{
		Bucket:        bucket,
		JobID:         jobID,
		BinaryArch:    r.cfg.BinaryArch,
		Region:        r.cfg.Region,
		OS:            job.Base.OS,
		Arch:          job.Base.NormalizedArch(),
		RegistryURL:   r.cfg.BucketURL,
		KeyFlag:       keyFlag,
		KeyS3URI:      keyS3URI,
		LocalKeyPath:  localKeyPath,
		CosignVersion: cosignVersion,
		RecipeName:    recipe.Meta.Name,
		RecipeVersion: recipe.Meta.Version,
	}

	var buf bytes.Buffer
	if err := ec2UserDataTmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing user-data template: %w", err)
	}
	return buf.String(), nil
}

// launchInstance runs a new EC2 instance with the given user-data script.
// The Name tag encodes the recipe so status queries are self-documenting.
func (r *EC2Runner) launchInstance(ctx context.Context, userData string, recipe *Recipe) (string, error) {
	name := buildInstanceName(recipe, r.cfg.BinaryArch)
	input := &ec2.RunInstancesInput{
		MinCount:     aws.Int32(1),
		MaxCount:     aws.Int32(1),
		ImageId:      aws.String(r.cfg.AMIID),
		InstanceType: ec2types.InstanceType(r.cfg.InstanceType),
		UserData:     aws.String(encodeUserData(userData)),
		IamInstanceProfile: &ec2types.IamInstanceProfileSpecification{
			Name: aws.String(r.cfg.IAMProfile),
		},
		TagSpecifications: []ec2types.TagSpecification{
			{
				ResourceType: ec2types.ResourceTypeInstance,
				Tags: []ec2types.Tag{
					{Key: aws.String("Name"), Value: aws.String(name)},
					{Key: aws.String("strata:recipe"), Value: aws.String(recipe.Meta.Name + "@" + recipe.Meta.Version)},
					{Key: aws.String("strata:arch"), Value: aws.String(r.cfg.BinaryArch)},
					{Key: aws.String("strata:build-status"), Value: aws.String("launching")},
				},
			},
		},
		BlockDeviceMappings: []ec2types.BlockDeviceMapping{
			{
				DeviceName: aws.String("/dev/xvda"),
				Ebs: &ec2types.EbsBlockDevice{
					VolumeSize:          aws.Int32(r.rootVolumeGB()),
					VolumeType:          ec2types.VolumeTypeGp3,
					DeleteOnTermination: aws.Bool(true),
				},
			},
		},
	}

	if r.cfg.SubnetID != "" {
		input.SubnetId = aws.String(r.cfg.SubnetID)
	}
	if r.cfg.SecurityGroupID != "" {
		input.SecurityGroupIds = []string{r.cfg.SecurityGroupID}
	}

	out, err := r.ec2.RunInstances(ctx, input)
	if err != nil {
		return "", fmt.Errorf("RunInstances: %w", err)
	}
	if len(out.Instances) == 0 {
		return "", fmt.Errorf("RunInstances returned no instances")
	}
	return aws.ToString(out.Instances[0].InstanceId), nil
}

// pollUntilComplete waits until the instance's strata:build-status tag is
// "success" or "failed", or ctx is cancelled.
func (r *EC2Runner) pollUntilComplete(ctx context.Context, instanceID string) error {
	ticker := time.NewTicker(r.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			status, err := r.getBuildStatus(ctx, instanceID)
			if err != nil {
				// Transient describe error — keep polling.
				fmt.Fprintf(os.Stderr, "ec2runner: polling %s: %v\n", instanceID, err)
				continue
			}
			switch status {
			case "success":
				return nil
			case "failed":
				return fmt.Errorf("build instance %s reported failure", instanceID)
			default:
				fmt.Fprintf(os.Stderr, "ec2runner: %s status=%s\n", instanceID, status)
			}
		}
	}
}

// getBuildStatus returns the value of the strata:build-status instance tag.
func (r *EC2Runner) getBuildStatus(ctx context.Context, instanceID string) (string, error) {
	out, err := r.ec2.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: []string{instanceID},
	})
	if err != nil {
		return "", err
	}
	for _, res := range out.Reservations {
		for _, inst := range res.Instances {
			for _, tag := range inst.Tags {
				if aws.ToString(tag.Key) == "strata:build-status" {
					return aws.ToString(tag.Value), nil
				}
			}
		}
	}
	return "unknown", nil
}

// buildInstanceName is the Name tag / spawn name for a build instance, shared by
// the SDK and spawn launch paths so both identify the instance identically.
func buildInstanceName(recipe *Recipe, binaryArch string) string {
	return fmt.Sprintf("strata-build-%s-%s-%s", recipe.Meta.Name, recipe.Meta.Version, binaryArch)
}

// parseObjectURI splits an "s3://<bucket>/<key>" URI into bucket and key.
// Returns ("", "", false) for malformed URIs.
func parseObjectURI(uri string) (bucket, key string, ok bool) {
	rest, found := strings.CutPrefix(uri, "s3://")
	if !found || rest == "" {
		return "", "", false
	}
	idx := strings.IndexByte(rest, '/')
	if idx < 0 {
		return "", "", false
	}
	return rest[:idx], rest[idx+1:], true
}

// encodeUserData base64-encodes a string for EC2 UserData.
// The EC2 API requires base64-encoded UserData.
func encodeUserData(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

// safeNameRe matches recipe names and versions that are safe for embedding
// in shell scripts: letters, digits, dots, underscores, hyphens only.
var safeNameRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// ArchForEC2 returns the binary arch suffix for the strata binary filename.
// x86_64 → "amd64", arm64 → "arm64".
func ArchForEC2(normalizedArch string) string {
	if strings.Contains(normalizedArch, "arm") || strings.Contains(normalizedArch, "aarch") {
		return "arm64"
	}
	return "amd64"
}

// ec2UserDataTmpl is the user-data script template for EC2 build instances.
// On success: rebuilds registry index, tags success, self-terminates.
// On failure: tags failed, self-stops (instance kept for log retrieval).
var ec2UserDataTmpl = template.Must(template.New("userdata").
	Funcs(template.FuncMap{"cosignDigest": cosignReleaseDigest}).
	Parse(`#!/usr/bin/env bash
set -uo pipefail
LOG=/var/log/strata-build.log
exec > >(tee -a "$LOG") 2>&1

# IMDSv2 token (required on AL2023)
TOKEN=$(curl -s -X PUT "http://169.254.169.254/latest/api/token" \
  -H "X-aws-ec2-metadata-token-ttl-seconds: 21600")
INSTANCE_ID=$(curl -s -H "X-aws-ec2-metadata-token: $TOKEN" \
  http://169.254.169.254/latest/meta-data/instance-id)
REGION=$(curl -s -H "X-aws-ec2-metadata-token: $TOKEN" \
  http://169.254.169.254/latest/meta-data/placement/region)

export AWS_DEFAULT_REGION="$REGION"

tag() {
  aws ec2 create-tags --region "$REGION" --resources "$INSTANCE_ID" \
    --tags "Key=strata:build-status,Value=$1" || true
}
tag "running"

# On failure: upload the build log to S3 (SSM run-command is not available in
# every account, so the log must be retrievable without it), tag, and stop.
fail() {
  aws s3 cp "$LOG" "s3://{{.Bucket}}/build/logs/{{.JobID}}.log" --region "$REGION" || true
  tag "failed"
  aws ec2 stop-instances --region "$REGION" --instance-ids "$INSTANCE_ID"
  exit 1
}

# Pin dnf to this AMI's own AL2023 releasever snapshot so the toolchain resolves
# deterministically over time rather than tracking the moving repos (#234, B3).
# dnf's own conf.releasever is the snapshot this fixed AMI shipped with (e.g.
# 2023.10.20260302); rpm -E %{releasever} is empty on AL2023, so ask dnf via its
# Python API, falling back to /etc/os-release VERSION_ID. Exported as
# STRATA_RELEASEVER so strata build records the same value it pins to. If it
# cannot be determined, install unpinned rather than fail.
RELEASEVER=$(python3 -c 'import dnf,sys; sys.stdout.write(dnf.Base().conf.releasever or "")' 2>/dev/null || true)
if [ -z "$RELEASEVER" ]; then RELEASEVER=$(. /etc/os-release 2>/dev/null; printf '%s' "${VERSION_ID:-}"); fi
export STRATA_RELEASEVER="$RELEASEVER"
RELFLAG=""
if [ -n "$RELEASEVER" ]; then RELFLAG="--releasever=$RELEASEVER"; fi
echo "strata: pinning dnf to releasever=${RELEASEVER:-<unset>}"

# Install build toolchain. "Development Tools" group provides gcc, g++, make,
# binutils, glibc-devel, and other essentials needed to compile from source.
# squashfs-tools: mksquashfs to package the build output.
# The -devel packages cover common recipe build deps (Python, R, OpenMPI, etc.).
dnf $RELFLAG groupinstall -y "Development Tools" || fail
dnf $RELFLAG install -y \
  squashfs-tools \
  openssl-devel zlib-devel bzip2-devel libffi-devel xz-devel \
  ncurses-devel readline-devel sqlite-devel \
  pcre2-devel libcurl-devel \
  libpng-devel libjpeg-turbo-devel \
  cairo-devel pango-devel \
  libxml2-devel \
  libevent-devel \
  hwloc-devel || fail

# Download strata binary
aws s3 cp "s3://{{.Bucket}}/build/bin/strata-linux-{{.BinaryArch}}" /usr/local/bin/strata || fail
chmod +x /usr/local/bin/strata

# Download cosign (required for signing layers) and verify it against a pinned
# digest before trusting it. A substituted cosign would sign malicious layers
# with the real KMS key and every bundle would verify downstream (#63), so a
# mismatch aborts the build rather than warning.
curl -fsSL \
  "https://github.com/sigstore/cosign/releases/download/{{.CosignVersion}}/cosign-linux-{{.BinaryArch}}" \
  -o /usr/local/bin/cosign || fail
echo "{{cosignDigest .CosignVersion .BinaryArch}}  /usr/local/bin/cosign" | sha256sum -c - || fail
chmod +x /usr/local/bin/cosign

{{- if .KeyS3URI}}
# Download signing key
mkdir -p "$(dirname "{{.LocalKeyPath}}")"
aws s3 cp "{{.KeyS3URI}}" "{{.LocalKeyPath}}" || fail
chmod 600 "{{.LocalKeyPath}}"
{{- end}}

# Download recipe
RECIPE_DIR="/opt/strata-recipe/{{.RecipeName}}/{{.RecipeVersion}}"
mkdir -p "$RECIPE_DIR"
aws s3 sync "s3://{{.Bucket}}/build/jobs/{{.JobID}}/recipe/" "$RECIPE_DIR/" || fail

# Run build. TMPDIR points at the disk-backed root volume, not /tmp: AL2023
# mounts /tmp as tmpfs (RAM-backed, ~half of RAM), which a large toolkit build
# (e.g. CUDA, ~8 GiB installed) overflows with "no space left on device"
# regardless of the EBS volume size. /var/tmp lives on the root volume.
export COSIGN_PASSWORD=""
export TMPDIR=/var/tmp
if strata build "$RECIPE_DIR" --os {{.OS}} --arch {{.Arch}} \
    --registry '{{.RegistryURL}}'{{.KeyFlag}}; then
  # NOTE: the registry index is rebuilt out-of-band, not here. Concurrent builds
  # each rewriting the single shared index/layers.yaml race and corrupt it (#233),
  # so indexing is decoupled from per-build success; run "strata index" after a
  # batch completes.
  tag "success"
  # Self-terminate on success — no need to keep the instance.
  aws ec2 terminate-instances --region "$REGION" --instance-ids "$INSTANCE_ID"
else
  fail
fi
`))
