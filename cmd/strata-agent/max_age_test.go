// Copyright 2026 Scott Friedman
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"
	"time"
)

// TestAgentMaxAge_ParsesBound: the freshness bound (#224) is read from
// STRATA_AGENT_MAX_AGE, unset means no bound, and the day/week suffixes work.
func TestAgentMaxAge_ParsesBound(t *testing.T) {
	tests := []struct {
		value string
		want  time.Duration
	}{
		{"", 0}, // unset — the deliberate default: no bound
		{"30d", 30 * 24 * time.Hour},
		{"2w", 2 * 7 * 24 * time.Hour},
		{"720h", 720 * time.Hour},
	}
	for _, tt := range tests {
		got, err := agentMaxAge(func(string) string { return tt.value })
		if err != nil {
			t.Errorf("agentMaxAge(%q) unexpected error: %v", tt.value, err)
			continue
		}
		if got != tt.want {
			t.Errorf("agentMaxAge(%q) = %s, want %s", tt.value, got, tt.want)
		}
	}
}

// TestAgentMaxAge_MalformedIsAnError: a typo in the bound must not silently
// become "no bound" — that would disable a protection the operator asked for.
// main turns this error into a fatal boot refusal (the refuse-direction default).
func TestAgentMaxAge_MalformedIsAnError(t *testing.T) {
	if _, err := agentMaxAge(func(string) string { return "banana" }); err == nil {
		t.Error("agentMaxAge accepted a malformed bound instead of erroring")
	}
	if _, err := agentMaxAge(func(string) string { return "-5d" }); err == nil {
		t.Error("agentMaxAge accepted a negative bound")
	}
}

// TestAgentMaxAge_ReadsTheRightVariable guards against a wrong env var name.
func TestAgentMaxAge_ReadsTheRightVariable(t *testing.T) {
	got, err := agentMaxAge(func(name string) string {
		if name == maxAgeEnv {
			return "30d"
		}
		return ""
	})
	if err != nil {
		t.Fatalf("agentMaxAge: %v", err)
	}
	if got != 30*24*time.Hour {
		t.Errorf("agentMaxAge read the wrong variable: got %s", got)
	}
}
