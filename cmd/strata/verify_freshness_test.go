// Copyright 2026 Scott Friedman
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
	"time"

	"github.com/scttfrdmn/strata/spec"
)

// TestFreshnessFailures is verify's honest-green contract for #224 (T8): a
// diagnostic's failure mode is a silent pass, and a stale but validly-signed
// lockfile is exactly that. With a --max-age bound, an over-age lockfile becomes
// a reported failure; with no bound, age is surfaced elsewhere but never fails.
func TestFreshnessFailures(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	stale := &spec.LockFile{ResolvedAt: now.Add(-400 * 24 * time.Hour)}
	fresh := &spec.LockFile{ResolvedAt: now.Add(-1 * time.Hour)}

	// No bound: even a very old lockfile is not a failure here (surfaced only).
	if f := freshnessFailures(stale, 0, now); len(f) != 0 {
		t.Errorf("freshnessFailures with no bound reported a failure: %v", f)
	}

	// Bound exceeded: a failure, naming freshness and the age.
	f := freshnessFailures(stale, 30*24*time.Hour, now)
	if len(f) != 1 {
		t.Fatalf("freshnessFailures on a stale lockfile = %v, want one failure", f)
	}
	if !strings.Contains(f[0], "freshness") || !strings.Contains(f[0], "400 days") {
		t.Errorf("failure %q does not name freshness and the age", f[0])
	}

	// Within the bound: no failure.
	if f := freshnessFailures(fresh, 30*24*time.Hour, now); len(f) != 0 {
		t.Errorf("freshnessFailures on a fresh lockfile reported a failure: %v", f)
	}
}
