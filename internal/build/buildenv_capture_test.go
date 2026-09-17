// Copyright 2026 Scott Friedman
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"context"
	"errors"
	"testing"
)

// rpmRunner fakes rpm: -qa returns qaOut, -E returns releasever, unless err set.
type rpmRunner struct {
	qaOut      string
	releasever string
	qaErr      error
}

func (r rpmRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	if name == "rpm" && len(args) > 0 && args[0] == "-qa" {
		if r.qaErr != nil {
			return nil, r.qaErr
		}
		return []byte(r.qaOut), nil
	}
	if name == "rpm" && len(args) > 0 && args[0] == "-E" {
		return []byte(r.releasever + "\n"), nil
	}
	return nil, errors.New("unexpected command")
}

func fixedAMI(id string) func(context.Context) string {
	return func(context.Context) string { return id }
}

func TestCaptureBuildEnvironment(t *testing.T) {
	ctx := context.Background()

	// A real rpm host: packages + releasever + AMI → a complete, sorted record.
	env := captureBuildEnvironment(ctx, rpmRunner{
		qaOut:      "zlib-1.2.11-x86_64\ngcc-11.4.1-2.amzn2023.x86_64\nglibc-2.34-x86_64\n",
		releasever: "2023.10.20260302",
	}, fixedAMI("ami-0c421724a94bba6d6"))
	if !env.Recorded() {
		t.Fatal("expected a recorded BuildEnvironment on an rpm host")
	}
	if env.AMIID != "ami-0c421724a94bba6d6" || env.Releasever != "2023.10.20260302" {
		t.Errorf("ami/releasever not captured: %+v", env)
	}
	if len(env.Packages) != 3 || env.Packages[0] != "gcc-11.4.1-2.amzn2023.x86_64" {
		t.Errorf("packages not captured/sorted: %v", env.Packages)
	}

	// A non-rpm host (rpm -qa errors): nil, not a half-filled record.
	if env := captureBuildEnvironment(ctx, rpmRunner{qaErr: errors.New("rpm: not found")}, fixedAMI("ami-x")); env != nil {
		t.Errorf("expected nil off an rpm host, got %+v", env)
	}

	// rpm present but no packages: nil.
	if env := captureBuildEnvironment(ctx, rpmRunner{qaOut: "\n  \n"}, fixedAMI("ami-x")); env != nil {
		t.Errorf("expected nil for empty rpm -qa, got %+v", env)
	}
}
