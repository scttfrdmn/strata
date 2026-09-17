// Copyright 2026 Scott Friedman
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/scttfrdmn/strata/spec"
)

// captureBuildEnvironment records the OS toolchain this build compiled against
// (#234, properties B2/B3): the AMI, the dnf releasever, and the NVR of every
// installed package, read on the build instance after the toolchain is
// installed. It is best-effort — on a host without rpm (local dev, dry-run) it
// returns nil, leaving manifest.BuildEnvironment unset rather than
// half-recorded. amiID is a function (IMDS on a real build) called only after
// rpm succeeds, so a non-EC2 build does not pay for a metadata round-trip and the
// capture is testable without EC2.
func captureBuildEnvironment(ctx context.Context, runner commandRunner, amiID func(context.Context) string) *spec.BuildEnvironment {
	out, err := runner.Run(ctx, "rpm", "-qa")
	if err != nil {
		return nil // not an rpm system — nothing to record
	}
	packages := strings.Fields(string(out)) // rpm -qa is one NVR per line, no spaces within
	if len(packages) == 0 {
		return nil
	}
	releasever := ""
	if o, err := runner.Run(ctx, "rpm", "-E", "%{?releasever}"); err == nil {
		releasever = strings.TrimSpace(string(o))
	}
	return spec.NewBuildEnvironment(amiID(ctx), releasever, packages)
}

// imdsAMIID reads the instance's AMI id from IMDSv2, best-effort. It returns ""
// off EC2 or on any failure — a build that is not on EC2 simply does not produce
// a complete BuildEnvironment (Recorded() needs the AMI). The short timeout keeps
// a non-EC2 build from stalling on the link-local address.
func imdsAMIID(ctx context.Context) string {
	const base = "http://169.254.169.254"
	client := &http.Client{Timeout: 2 * time.Second}

	tokReq, err := http.NewRequestWithContext(ctx, http.MethodPut, base+"/latest/api/token", nil)
	if err != nil {
		return ""
	}
	tokReq.Header.Set("X-aws-ec2-metadata-token-ttl-seconds", "60")
	tokResp, err := client.Do(tokReq)
	if err != nil {
		return ""
	}
	defer tokResp.Body.Close() //nolint:errcheck
	tok, _ := io.ReadAll(tokResp.Body)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/latest/meta-data/ami-id", nil)
	if err != nil {
		return ""
	}
	req.Header.Set("X-aws-ec2-metadata-token", string(tok))
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	id, _ := io.ReadAll(resp.Body)
	return strings.TrimSpace(string(id))
}
