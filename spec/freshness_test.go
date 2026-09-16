// Copyright 2026 Scott Friedman
// SPDX-License-Identifier: Apache-2.0

package spec

import (
	"strings"
	"testing"
	"time"
)

func TestHumanizeAge(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{412 * 24 * time.Hour, "412 days"},
		{24 * time.Hour, "1 day"},
		{47 * time.Hour, "1 day"}, // truncates to whole days
		{3 * time.Hour, "3 hours"},
		{time.Hour, "1 hour"},
		{90 * time.Minute, "1 hour"},
		{5 * time.Minute, "5 minutes"},
		{time.Minute, "1 minute"},
		{30 * time.Second, "30 seconds"},
		{time.Second, "1 second"},
		{-412 * 24 * time.Hour, "412 days"}, // rendered by magnitude
	}
	for _, tc := range cases {
		if got := HumanizeAge(tc.d); got != tc.want {
			t.Errorf("HumanizeAge(%s) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

func TestParseMaxAge(t *testing.T) {
	cases := []struct {
		in      string
		want    time.Duration
		wantErr bool
	}{
		{"", 0, false},   // no bound
		{"0d", 0, false}, // explicit zero is also no bound
		{"30d", 30 * 24 * time.Hour, false},
		{"2w", 2 * 7 * 24 * time.Hour, false},
		{"720h", 720 * time.Hour, false},
		{"90m", 90 * time.Minute, false},
		{"0.5d", 12 * time.Hour, false},
		{"-5d", 0, true}, // negative refused
		{"-1h", 0, true}, // negative refused (ParseDuration path)
		{"banana", 0, true},
		{"30", 0, true}, // no unit
	}
	for _, tc := range cases {
		got, err := ParseMaxAge(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseMaxAge(%q) = %s, nil; want error", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseMaxAge(%q) unexpected error: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseMaxAge(%q) = %s, want %s", tc.in, got, tc.want)
		}
	}
}

// makeLF returns a lockfile resolved at the given time.
func lfResolvedAt(ts time.Time) *LockFile { return &LockFile{ResolvedAt: ts} }

func TestFreshnessNote(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)

	// Unset resolved_at: nothing to say.
	if note := (&LockFile{}).FreshnessNote(now); note != "" {
		t.Errorf("FreshnessNote with zero ResolvedAt = %q, want \"\"", note)
	}

	// A stale lockfile surfaces its age and date.
	lf := lfResolvedAt(now.Add(-412 * 24 * time.Hour))
	note := lf.FreshnessNote(now)
	if !strings.Contains(note, "412 days ago") {
		t.Errorf("FreshnessNote = %q, want it to state 412 days", note)
	}
	if !strings.Contains(note, lf.ResolvedAt.UTC().Format("2006-01-02")) {
		t.Errorf("FreshnessNote = %q, want it to name the resolved date", note)
	}

	// A future-dated lockfile is flagged as a clock problem, not a negative age.
	future := lfResolvedAt(now.Add(48 * time.Hour)).FreshnessNote(now)
	if !strings.Contains(future, "future") {
		t.Errorf("FreshnessNote for a future lockfile = %q, want a clock note", future)
	}
	// The future branch must not fall through to the age wording (which renders
	// by magnitude and would read as if the lockfile were 2 days old).
	if strings.Contains(future, "ago") {
		t.Errorf("FreshnessNote for a future lockfile used the age wording: %q", future)
	}
}

func TestCheckFreshness(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	stale := lfResolvedAt(now.Add(-412 * 24 * time.Hour))
	fresh := lfResolvedAt(now.Add(-1 * time.Hour))

	// Opt-in: no bound never refuses, however old.
	if err := stale.CheckFreshness(0, now); err != nil {
		t.Errorf("CheckFreshness with no bound refused a stale lockfile: %v", err)
	}
	if err := stale.CheckFreshness(-1, now); err != nil {
		t.Errorf("CheckFreshness with negative bound refused: %v", err)
	}

	// A bound refuses a lockfile older than it, and the error is actionable.
	err := stale.CheckFreshness(30*24*time.Hour, now)
	if err == nil {
		t.Fatal("CheckFreshness accepted a lockfile older than the bound")
	}
	for _, want := range []string{"412 days", "30 days", stale.ResolvedAt.UTC().Format("2006-01-02")} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("CheckFreshness error %q missing %q", err, want)
		}
	}

	// A lockfile within the bound passes.
	if err := fresh.CheckFreshness(30*24*time.Hour, now); err != nil {
		t.Errorf("CheckFreshness refused a lockfile within the bound: %v", err)
	}

	// A bound with no resolved_at cannot establish freshness — refuse.
	if err := (&LockFile{}).CheckFreshness(30*24*time.Hour, now); err == nil {
		t.Error("CheckFreshness accepted a lockfile with no resolved_at under a bound")
	}

	// Boundary: exactly at the bound is not "older than", so it passes.
	atBound := lfResolvedAt(now.Add(-30 * 24 * time.Hour))
	if err := atBound.CheckFreshness(30*24*time.Hour, now); err != nil {
		t.Errorf("CheckFreshness refused a lockfile exactly at the bound: %v", err)
	}
}
