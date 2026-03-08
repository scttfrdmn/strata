package probe

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

// mockSSM is a hand-written mock of ssmAPI used by SSMResolver tests.
type mockSSM struct {
	// value is returned on a successful call.
	value string
	// err is returned when non-nil.
	err error
	// calls counts how many times GetParameter was called.
	calls int
}

func (m *mockSSM) GetParameter(_ context.Context, _ *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
	m.calls++
	if m.err != nil {
		return nil, m.err
	}
	return &ssm.GetParameterOutput{
		Parameter: &ssmtypes.Parameter{Value: aws.String(m.value)},
	}, nil
}

func TestResolveAMI_HappyPath(t *testing.T) {
	t.Parallel()
	want := "ami-real123"
	mock := &mockSSM{value: want}
	r := newSSMResolverWithAPI(mock)

	got, err := r.ResolveAMI(context.Background(), "al2023", "x86_64")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if mock.calls != 1 {
		t.Errorf("expected 1 SSM call, got %d", mock.calls)
	}
}

func TestResolveAMI_UnknownOS(t *testing.T) {
	t.Parallel()
	mock := &mockSSM{} // should never be called
	r := newSSMResolverWithAPI(mock)

	_, err := r.ResolveAMI(context.Background(), "nosuchos", "x86_64")
	if err == nil {
		t.Fatal("expected error for unknown OS, got nil")
	}
	if mock.calls != 0 {
		t.Errorf("expected 0 SSM calls for unknown OS, got %d", mock.calls)
	}
}

func TestResolveAMI_SSMError(t *testing.T) {
	t.Parallel()
	ssmErr := errors.New("ssm: connection refused")
	mock := &mockSSM{err: ssmErr}
	r := newSSMResolverWithAPI(mock)

	_, err := r.ResolveAMI(context.Background(), "al2023", "x86_64")
	if err == nil {
		t.Fatal("expected error from SSM, got nil")
	}
	if !errors.Is(err, ssmErr) {
		t.Errorf("error chain does not contain ssm error: %v", err)
	}
}
