package probe_test

import (
	"context"
	"testing"

	"github.com/scttfrdmn/strata/internal/probe"
	"github.com/scttfrdmn/strata/spec"
)

func TestResolveSSMParam(t *testing.T) {
	tests := []struct {
		os, arch string
		wantErr  bool
		wantSub  string // expected substring in result
	}{
		{"al2023", "x86_64", false, "al2023"},
		{"al2023", "arm64", false, "al2023"},
		{"rocky9", "x86_64", false, "rocky-linux-9"},
		{"ubuntu24", "arm64", false, "ubuntu"},
		{"windows11", "x86_64", true, ""},
		{"al2023", "sparc", true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.os+"/"+tt.arch, func(t *testing.T) {
			got, err := probe.ResolveSSMParam(tt.os, tt.arch)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ResolveSSMParam(%q, %q) want error, got nil", tt.os, tt.arch)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveSSMParam(%q, %q) unexpected error: %v", tt.os, tt.arch, err)
			}
			if tt.wantSub != "" && len(got) == 0 {
				t.Errorf("ResolveSSMParam(%q, %q) returned empty string", tt.os, tt.arch)
			}
		})
	}
}

func TestOSABI(t *testing.T) {
	tests := []struct{ os, want string }{
		{"al2023", "linux-gnu-2.34"},
		{"rocky9", "linux-gnu-2.34"},
		{"rocky10", "linux-gnu-2.34"},
		{"ubuntu24", "linux-gnu-2.35"},
	}
	for _, tt := range tests {
		if got := probe.OSABI[tt.os]; got != tt.want {
			t.Errorf("OSABI[%q] = %q, want %q", tt.os, got, tt.want)
		}
	}
}

func TestStaticResolver(t *testing.T) {
	ctx := context.Background()
	r := &probe.StaticResolver{
		AMIs: map[string]string{
			"al2023/x86_64": "ami-0abc123",
			"al2023/arm64":  "ami-0def456",
		},
	}

	id, err := r.ResolveAMI(ctx, "al2023", "x86_64")
	if err != nil {
		t.Fatalf("ResolveAMI() error: %v", err)
	}
	if id != "ami-0abc123" {
		t.Errorf("ResolveAMI() = %q, want %q", id, "ami-0abc123")
	}

	_, err = r.ResolveAMI(ctx, "rocky9", "x86_64")
	if err == nil {
		t.Error("ResolveAMI for unconfigured OS should return error")
	}
}

func TestMemoryCache(t *testing.T) {
	ctx := context.Background()
	cache := probe.NewMemoryCache()

	// Miss.
	if _, ok := cache.Get(ctx, "ami-0abc123"); ok {
		t.Error("empty cache should have no entries")
	}

	caps := &spec.BaseCapabilities{
		AMIID: "ami-0abc123",
		OS:    "al2023",
		Arch:  "x86_64",
	}
	if err := cache.Set(ctx, "ami-0abc123", caps); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	// Hit.
	got, ok := cache.Get(ctx, "ami-0abc123")
	if !ok {
		t.Fatal("cache should have entry after Set()")
	}
	if got.AMIID != "ami-0abc123" {
		t.Errorf("cached AMIID = %q, want %q", got.AMIID, "ami-0abc123")
	}

	// Different key is still a miss.
	if _, ok := cache.Get(ctx, "ami-other"); ok {
		t.Error("different AMI ID should not be in cache")
	}
}

func TestFakeRunner(t *testing.T) {
	ctx := context.Background()
	caps := &spec.BaseCapabilities{AMIID: "ami-0abc123", OS: "al2023"}
	runner := &probe.FakeRunner{
		Capabilities: map[string]*spec.BaseCapabilities{
			"ami-0abc123": caps,
		},
	}

	got, err := runner.ProbeAMI(ctx, "ami-0abc123", "x86_64")
	if err != nil {
		t.Fatalf("FakeRunner.ProbeAMI() error: %v", err)
	}
	if got.AMIID != caps.AMIID {
		t.Errorf("AMIID = %q, want %q", got.AMIID, caps.AMIID)
	}

	_, err = runner.ProbeAMI(ctx, "ami-unknown", "x86_64")
	if err == nil {
		t.Error("FakeRunner.ProbeAMI for unknown AMI should return error")
	}
}

func TestClientGetCapabilities(t *testing.T) {
	ctx := context.Background()
	amiID := "ami-0abc123"

	caps := &spec.BaseCapabilities{
		AMIID: amiID,
		OS:    "al2023",
		Arch:  "x86_64",
		Provides: []spec.Capability{
			{Name: "glibc", Version: "2.34"},
		},
	}

	client := &probe.Client{
		Resolver: &probe.StaticResolver{AMIs: map[string]string{"al2023/x86_64": amiID}},
		Runner:   &probe.FakeRunner{Capabilities: map[string]*spec.BaseCapabilities{amiID: caps}},
		Cache:    probe.NewMemoryCache(),
	}

	// First call probes.
	got, err := client.GetCapabilities(ctx, "al2023", "x86_64")
	if err != nil {
		t.Fatalf("GetCapabilities() error: %v", err)
	}
	if got.AMIID != amiID {
		t.Errorf("AMIID = %q, want %q", got.AMIID, amiID)
	}

	// Second call should use cache (runner would fail for a different AMI).
	got2, err := client.GetCapabilities(ctx, "al2023", "x86_64")
	if err != nil {
		t.Fatalf("GetCapabilities() second call error: %v", err)
	}
	if got2.AMIID != amiID {
		t.Errorf("second call AMIID = %q, want %q", got2.AMIID, amiID)
	}
}

func TestClientResolve(t *testing.T) {
	ctx := context.Background()
	amiID := "ami-0abc123"
	caps := &spec.BaseCapabilities{AMIID: amiID, OS: "al2023"}

	client := &probe.Client{
		Resolver: &probe.StaticResolver{AMIs: map[string]string{"al2023/x86_64": amiID}},
		Runner:   &probe.FakeRunner{Capabilities: map[string]*spec.BaseCapabilities{amiID: caps}},
		Cache:    probe.NewMemoryCache(),
	}

	result, err := client.Resolve(ctx, "al2023", "x86_64")
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	if result.AMIID != amiID {
		t.Errorf("result.AMIID = %q, want %q", result.AMIID, amiID)
	}
	if result.Capabilities == nil {
		t.Error("result.Capabilities should not be nil")
	}
}

func TestKnownBaseCapabilities(t *testing.T) {
	tests := []struct {
		os, arch  string
		wantABI   string
		wantGlibc string
	}{
		{"al2023", "x86_64", "linux-gnu-2.34", "2.34"},
		{"rocky9", "x86_64", "linux-gnu-2.34", "2.34"},
		{"rocky10", "x86_64", "linux-gnu-2.34", "2.39"},
		{"ubuntu24", "x86_64", "linux-gnu-2.35", "2.39"},
	}

	for _, tt := range tests {
		t.Run(tt.os+"/"+tt.arch, func(t *testing.T) {
			caps, err := probe.KnownBaseCapabilities(tt.os, tt.arch, "ami-test")
			if err != nil {
				t.Fatalf("KnownBaseCapabilities(%q, %q) error: %v", tt.os, tt.arch, err)
			}
			if caps.ABI != tt.wantABI {
				t.Errorf("ABI = %q, want %q", caps.ABI, tt.wantABI)
			}
			if !caps.HasCapability("glibc", tt.wantGlibc) {
				t.Errorf("glibc@%s not satisfied by %v", tt.wantGlibc, caps.Provides)
			}
			if !caps.HasCapability("abi", tt.wantABI) {
				t.Errorf("abi=%q not in capabilities", tt.wantABI)
			}
		})
	}

	// Unknown OS should error.
	_, err := probe.KnownBaseCapabilities("windows11", "x86_64", "ami-test")
	if err == nil {
		t.Error("KnownBaseCapabilities for unknown OS should return error")
	}
}
