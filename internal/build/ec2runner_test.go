package build

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/scttfrdmn/strata/spec"
)

// fakeEC2 implements ec2LaunchAPI for testing.
type fakeEC2 struct {
	instanceID  string
	buildStatus string // returned by DescribeInstances tag
	runErr      error
	termErr     error
}

func (f *fakeEC2) RunInstances(_ context.Context, _ *ec2.RunInstancesInput, _ ...func(*ec2.Options)) (*ec2.RunInstancesOutput, error) {
	if f.runErr != nil {
		return nil, f.runErr
	}
	return &ec2.RunInstancesOutput{
		Instances: []ec2types.Instance{
			{InstanceId: aws.String(f.instanceID)},
		},
	}, nil
}

func (f *fakeEC2) TerminateInstances(_ context.Context, _ *ec2.TerminateInstancesInput, _ ...func(*ec2.Options)) (*ec2.TerminateInstancesOutput, error) {
	return &ec2.TerminateInstancesOutput{}, f.termErr
}

func (f *fakeEC2) DescribeInstances(_ context.Context, _ *ec2.DescribeInstancesInput, _ ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	return &ec2.DescribeInstancesOutput{
		Reservations: []ec2types.Reservation{
			{Instances: []ec2types.Instance{
				{
					InstanceId: aws.String(f.instanceID),
					Tags: []ec2types.Tag{
						{Key: aws.String("strata:build-status"), Value: aws.String(f.buildStatus)},
					},
				},
			}},
		},
	}, nil
}

// fakeS3Put implements s3PutAPI for testing.
type fakeS3Put struct {
	objects map[string][]byte // key -> body
}

func (f *fakeS3Put) PutObject(_ context.Context, in *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	if f.objects == nil {
		f.objects = make(map[string][]byte)
	}
	data, _ := io.ReadAll(in.Body)
	f.objects[aws.ToString(in.Key)] = data
	return &s3.PutObjectOutput{}, nil
}

func TestEC2Runner_UploadRecipe(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "build.sh"), []byte("#!/bin/bash\necho hello"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "meta.yaml"), []byte("name: test\nversion: 1.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	recipe := &Recipe{
		Dir:             dir,
		BuildScriptPath: filepath.Join(dir, "build.sh"),
		Meta:            RecipeMeta{Name: "test", Version: "1.0", Family: "rhel"},
	}

	s3Fake := &fakeS3Put{}
	cfg := EC2Config{BucketURL: "s3://strata-test", Region: "us-east-1"}
	runner := newEC2RunnerWithAPIs(cfg, &fakeEC2{instanceID: "i-test"}, s3Fake)

	if err := runner.uploadRecipe(context.Background(), "job-123", recipe); err != nil {
		t.Fatalf("uploadRecipe: %v", err)
	}

	for _, fname := range []string{"build.sh", "meta.yaml"} {
		key := "build/jobs/job-123/recipe/" + fname
		if _, ok := s3Fake.objects[key]; !ok {
			t.Errorf("expected S3 key %q not found", key)
		}
	}
}

func TestEC2Runner_BuildUserData(t *testing.T) {
	recipe := &Recipe{
		Dir:  "/tmp/recipe",
		Meta: RecipeMeta{Name: "openmpi", Version: "5.0.6", Family: "rhel"},
	}
	job := &Job{
		Base:        stubBaseRef("al2023", "x86_64"),
		RegistryURL: "s3://strata-registry",
	}

	cfg := EC2Config{
		BucketURL:  "s3://strata-registry",
		BinaryArch: "amd64",
		Region:     "us-east-1",
	}
	runner := newEC2RunnerWithAPIs(cfg, &fakeEC2{instanceID: "i-test"}, &fakeS3Put{})

	ud, err := runner.buildUserData("job-456", recipe, job)
	if err != nil {
		t.Fatalf("buildUserData: %v", err)
	}

	if !strings.Contains(ud, "strata build") {
		t.Error("user-data missing 'strata build'")
	}
	if !strings.Contains(ud, "job-456") {
		t.Error("user-data missing job ID")
	}
	if !strings.Contains(ud, "strata-registry") {
		t.Error("user-data missing registry URL")
	}
	if !strings.Contains(ud, "amd64") {
		t.Error("user-data missing binary arch")
	}
}

func TestEC2Runner_PollSuccess(t *testing.T) {
	fe := &fakeEC2{instanceID: "i-poll-test", buildStatus: "success"}
	cfg := EC2Config{
		BucketURL:    "s3://strata-test",
		Region:       "us-east-1",
		PollInterval: 10 * time.Millisecond,
	}
	runner := newEC2RunnerWithAPIs(cfg, fe, &fakeS3Put{})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := runner.pollUntilComplete(ctx, "i-poll-test"); err != nil {
		t.Errorf("pollUntilComplete: %v", err)
	}
}

func TestEC2Runner_PollFailure(t *testing.T) {
	fe := &fakeEC2{instanceID: "i-fail-test", buildStatus: "failed"}
	cfg := EC2Config{
		BucketURL:    "s3://strata-test",
		Region:       "us-east-1",
		PollInterval: 10 * time.Millisecond,
	}
	runner := newEC2RunnerWithAPIs(cfg, fe, &fakeS3Put{})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := runner.pollUntilComplete(ctx, "i-fail-test"); err == nil {
		t.Error("expected error for failed build, got nil")
	}
}

func TestBuildArchForEC2(t *testing.T) {
	tests := []struct {
		arch string
		want string
	}{
		{"x86_64", "amd64"},
		{"arm64", "arm64"},
		{"aarch64", "arm64"},
	}
	for _, tc := range tests {
		if got := ArchForEC2(tc.arch); got != tc.want {
			t.Errorf("ArchForEC2(%q) = %q, want %q", tc.arch, got, tc.want)
		}
	}
}

// stubBaseRef creates a spec.BaseRef for testing.
func stubBaseRef(osName, arch string) spec.BaseRef {
	return spec.BaseRef{OS: osName, Arch: arch}
}

// Ensure EC2Runner user-data template produces valid bash (smoke test).
func TestEC2UserDataTemplate(t *testing.T) {
	var buf bytes.Buffer
	err := ec2UserDataTmpl.Execute(&buf, struct {
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
		Bucket:        "strata-registry",
		JobID:         "test-job",
		BinaryArch:    "amd64",
		Region:        "us-east-1",
		OS:            "al2023",
		Arch:          "x86_64",
		RegistryURL:   "s3://strata-registry",
		KeyFlag:       " --key /root/.strata-keys/cosign.key",
		KeyS3URI:      "s3://strata-registry/build/keys/cosign.key",
		LocalKeyPath:  "/root/.strata-keys/cosign.key",
		CosignVersion: "v3.0.5",
		RecipeName:    "gcc",
		RecipeVersion: "13.2.0",
	})
	if err != nil {
		t.Fatalf("template execution failed: %v", err)
	}
	out := buf.String()
	if !strings.HasPrefix(out, "#!/usr/bin/env bash") {
		t.Error("user-data does not start with shebang")
	}
	if !strings.Contains(out, "strata build") {
		t.Error("user-data missing 'strata build'")
	}
}
