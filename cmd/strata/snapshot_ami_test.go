package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// mockEC2SnapshotAPI implements ec2SnapshotAPI for testing.
type mockEC2SnapshotAPI struct {
	createImageFunc    func(ctx context.Context, in *ec2.CreateImageInput, opts ...func(*ec2.Options)) (*ec2.CreateImageOutput, error)
	describeImagesFunc func(ctx context.Context, in *ec2.DescribeImagesInput, opts ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error)
}

func (m *mockEC2SnapshotAPI) CreateImage(ctx context.Context, in *ec2.CreateImageInput, opts ...func(*ec2.Options)) (*ec2.CreateImageOutput, error) {
	return m.createImageFunc(ctx, in, opts...)
}

func (m *mockEC2SnapshotAPI) DescribeImages(ctx context.Context, in *ec2.DescribeImagesInput, opts ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
	return m.describeImagesFunc(ctx, in, opts...)
}

func newMockSnapshot(amiID string, states []types.ImageState) *mockEC2SnapshotAPI {
	callCount := 0
	return &mockEC2SnapshotAPI{
		createImageFunc: func(_ context.Context, _ *ec2.CreateImageInput, _ ...func(*ec2.Options)) (*ec2.CreateImageOutput, error) {
			return &ec2.CreateImageOutput{ImageId: aws.String(amiID)}, nil
		},
		describeImagesFunc: func(_ context.Context, _ *ec2.DescribeImagesInput, _ ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
			var state types.ImageState
			if callCount < len(states) {
				state = states[callCount]
			} else {
				state = states[len(states)-1]
			}
			callCount++
			return &ec2.DescribeImagesOutput{
				Images: []types.Image{
					{ImageId: aws.String(amiID), State: state},
				},
			}, nil
		},
	}
}

func TestSnapshotAMIText(t *testing.T) {
	mock := newMockSnapshot("ami-0123456789abcdef0", []types.ImageState{types.ImageStateAvailable})

	// Capture stdout by redirecting — use runSnapshotAMI directly.
	err := runSnapshotAMI(
		context.Background(),
		mock,
		"i-0abc123", // instanceID
		"my-snapshot",
		"test description",
		false, // noReboot
		false, // wait
		30*time.Second,
		"us-east-1",
		"text",
	)
	if err != nil {
		t.Fatalf("runSnapshotAMI: %v", err)
	}
}

func TestSnapshotAMIJSON(t *testing.T) {
	mock := newMockSnapshot("ami-0123456789abcdef0", []types.ImageState{types.ImageStateAvailable})

	err := runSnapshotAMI(
		context.Background(),
		mock,
		"i-0abc123",
		"my-snapshot",
		"",
		false,
		false,
		30*time.Second,
		"us-east-1",
		"json",
	)
	if err != nil {
		t.Fatalf("runSnapshotAMI JSON: %v", err)
	}
}

func TestSnapshotAMIWait(t *testing.T) {
	// Mock returns pending on first call, available on second.
	mock := newMockSnapshot("ami-0abc", []types.ImageState{
		types.ImageStatePending,
		types.ImageStateAvailable,
	})

	err := runSnapshotAMI(
		context.Background(),
		mock,
		"i-0abc123",
		"wait-test",
		"",
		false,
		true,               // wait
		1*time.Millisecond, // very short poll interval for test speed
		"us-east-1",
		"text",
	)
	if err != nil {
		t.Fatalf("runSnapshotAMI wait: %v", err)
	}
}

func TestSnapshotAMIDefaultName(t *testing.T) {
	var capturedName string
	mock := &mockEC2SnapshotAPI{
		createImageFunc: func(_ context.Context, in *ec2.CreateImageInput, _ ...func(*ec2.Options)) (*ec2.CreateImageOutput, error) {
			capturedName = aws.ToString(in.Name)
			return &ec2.CreateImageOutput{ImageId: aws.String("ami-test")}, nil
		},
		describeImagesFunc: func(_ context.Context, _ *ec2.DescribeImagesInput, _ ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
			return &ec2.DescribeImagesOutput{}, nil
		},
	}

	err := runSnapshotAMI(
		context.Background(),
		mock,
		"i-0abc123",
		"", // empty name → should generate one
		"",
		false,
		false,
		30*time.Second,
		"us-east-1",
		"text",
	)
	if err != nil {
		t.Fatalf("runSnapshotAMI default name: %v", err)
	}
	if !strings.HasPrefix(capturedName, "strata-snapshot-") {
		t.Errorf("expected generated name to start with strata-snapshot-, got %q", capturedName)
	}
}

func TestSnapshotAMIFailedState(t *testing.T) {
	mock := newMockSnapshot("ami-fail", []types.ImageState{types.ImageStateFailed})

	err := runSnapshotAMI(
		context.Background(),
		mock,
		"i-0abc123",
		"fail-test",
		"",
		false,
		true, // wait — will see "failed" state
		1*time.Millisecond,
		"us-east-1",
		"text",
	)
	if err == nil {
		t.Error("expected error when AMI enters failed state")
	}
}
