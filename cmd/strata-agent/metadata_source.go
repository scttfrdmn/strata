package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"gopkg.in/yaml.v3"

	"github.com/scttfrdmn/strata/spec"
)

// imdsAPI is the subset of imds.Client used by metadataLockfileSource and
// ec2ReadySignaler. Defined as an interface to allow mock injection in tests.
type imdsAPI interface {
	GetUserData(ctx context.Context, in *imds.GetUserDataInput,
		opts ...func(*imds.Options)) (*imds.GetUserDataOutput, error)
	GetMetadata(ctx context.Context, in *imds.GetMetadataInput,
		opts ...func(*imds.Options)) (*imds.GetMetadataOutput, error)
}

// s3GetAPI is the subset of s3.Client used for single-object fetches.
// Defined as an interface to allow mock injection in tests.
type s3GetAPI interface {
	GetObject(ctx context.Context, in *s3.GetObjectInput,
		opts ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

// metadataLockfileSource acquires the lockfile from EC2 instance metadata.
// Priority: user-data YAML → instance tag strata:lockfile-s3-uri → error.
type metadataLockfileSource struct {
	imds imdsAPI
	s3   s3GetAPI
}

// newMetadataLockfileSource creates a metadataLockfileSource backed by real AWS clients.
func newMetadataLockfileSource() *metadataLockfileSource {
	imdsClient := imds.New(imds.Options{})
	cfg, err := awsconfig.LoadDefaultConfig(context.Background())
	if err != nil {
		// S3 fallback unavailable; user-data path still works.
		return &metadataLockfileSource{imds: imdsClient}
	}
	// All Strata registry resources live in us-east-1. Fall back when the
	// region cannot be resolved from the environment (e.g. early boot).
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	return &metadataLockfileSource{
		imds: imdsClient,
		s3:   s3.NewFromConfig(cfg),
	}
}

// newMetadataLockfileSourceWithAPIs constructs a metadataLockfileSource with
// injected interfaces — used by tests.
func newMetadataLockfileSourceWithAPIs(imdsClient imdsAPI, s3Client s3GetAPI) *metadataLockfileSource {
	return &metadataLockfileSource{imds: imdsClient, s3: s3Client}
}

// Acquire fetches the lockfile from EC2 instance metadata.
// It first checks user-data for a YAML-encoded lockfile, then falls back to
// the strata:lockfile-s3-uri instance tag.
func (s *metadataLockfileSource) Acquire(ctx context.Context) (*spec.LockFile, error) {
	// 1. Try user-data.
	udOut, err := s.imds.GetUserData(ctx, &imds.GetUserDataInput{})
	if err == nil {
		defer udOut.Content.Close() //nolint:errcheck
		data, readErr := io.ReadAll(udOut.Content)
		if readErr == nil {
			var lf spec.LockFile
			if decodeErr := yaml.Unmarshal(data, &lf); decodeErr == nil && lf.ProfileName != "" {
				return &lf, nil
			}
		}
	}

	// 2. Try instance tag strata:lockfile-s3-uri.
	tagOut, tagErr := s.imds.GetMetadata(ctx, &imds.GetMetadataInput{
		Path: "tags/instance/strata:lockfile-s3-uri",
	})
	if tagErr != nil {
		return nil, fmt.Errorf("metadataLockfileSource: no lockfile in user-data or instance tags: %w", tagErr)
	}
	defer tagOut.Content.Close() //nolint:errcheck
	uriBytes, err := io.ReadAll(tagOut.Content)
	if err != nil {
		return nil, fmt.Errorf("metadataLockfileSource: reading tag: %w", err)
	}
	uri := strings.TrimSpace(string(uriBytes))

	// 3. Fetch from S3.
	bucket, key, ok := parseS3URI(uri)
	if !ok {
		return nil, fmt.Errorf("metadataLockfileSource: invalid S3 URI in tag: %q", uri)
	}
	if s.s3 == nil {
		return nil, fmt.Errorf("metadataLockfileSource: S3 client unavailable")
	}
	out, err := s.s3.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &bucket,
		Key:    &key,
	})
	if err != nil {
		return nil, fmt.Errorf("metadataLockfileSource: fetching lockfile from %q: %w", uri, err)
	}
	defer out.Body.Close() //nolint:errcheck
	data, err := io.ReadAll(out.Body)
	if err != nil {
		return nil, fmt.Errorf("metadataLockfileSource: reading S3 object: %w", err)
	}
	var lf spec.LockFile
	if err := yaml.Unmarshal(data, &lf); err != nil {
		return nil, fmt.Errorf("metadataLockfileSource: parsing lockfile YAML: %w", err)
	}
	return &lf, nil
}

// parseS3URI parses "s3://bucket/key" into (bucket, key, true).
// Returns ("", "", false) for any malformed input.
func parseS3URI(uri string) (bucket, key string, ok bool) {
	if !strings.HasPrefix(uri, "s3://") {
		return "", "", false
	}
	rest := uri[len("s3://"):]
	idx := strings.IndexByte(rest, '/')
	if idx < 0 || idx == 0 {
		return "", "", false
	}
	bucket = rest[:idx]
	key = rest[idx+1:]
	if key == "" {
		return "", "", false
	}
	return bucket, key, true
}
