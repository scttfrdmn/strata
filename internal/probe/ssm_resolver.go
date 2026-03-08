package probe

import (
	"context"
	"fmt"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

// ssmAPI is the subset of ssm.Client used by SSMResolver. Defined as an
// interface to allow mock injection in tests.
type ssmAPI interface {
	GetParameter(ctx context.Context, in *ssm.GetParameterInput,
		opts ...func(*ssm.Options)) (*ssm.GetParameterOutput, error)
}

// SSMResolver implements Resolver by querying AWS SSM Parameter Store.
// Each OS/arch pair maps to an SSM parameter path defined in osAliasSSM.
type SSMResolver struct {
	ssm ssmAPI
}

// NewSSMResolver creates an SSMResolver using the default AWS credential chain.
func NewSSMResolver(ctx context.Context) (*SSMResolver, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("probe: loading AWS config: %w", err)
	}
	return &SSMResolver{ssm: ssm.NewFromConfig(cfg)}, nil
}

// newSSMResolverWithAPI constructs an SSMResolver with a pre-built API — used
// by tests to inject a mock without real AWS credentials.
func newSSMResolverWithAPI(api ssmAPI) *SSMResolver {
	return &SSMResolver{ssm: api}
}

// ResolveAMI queries SSM Parameter Store for the current AMI ID for the given
// OS and arch. Returns an error if the OS/arch pair is not recognized or if
// the SSM call fails.
func (r *SSMResolver) ResolveAMI(ctx context.Context, os, arch string) (string, error) {
	paramPath, err := ResolveSSMParam(os, arch)
	if err != nil {
		return "", err
	}

	withDecryption := true
	out, err := r.ssm.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           &paramPath,
		WithDecryption: &withDecryption,
	})
	if err != nil {
		return "", fmt.Errorf("probe: SSM GetParameter %q: %w", paramPath, err)
	}
	if out.Parameter == nil || out.Parameter.Value == nil {
		return "", fmt.Errorf("probe: SSM parameter %q returned nil value", paramPath)
	}
	return *out.Parameter.Value, nil
}
