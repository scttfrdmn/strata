// Copyright 2026 Scott Friedman
// SPDX-License-Identifier: Apache-2.0

package zenodo_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/scttfrdmn/strata/internal/trust"
	"github.com/scttfrdmn/strata/internal/zenodo"
)

// recordingDepositServer serves the three deposit endpoints and records every
// uploaded file (name -> bytes), so a test can assert what a deposit actually
// contains rather than only that it succeeded.
func recordingDepositServer(t *testing.T, uploads map[string][]byte) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/deposit/depositions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
			"id":    int64(12345),
			"links": map[string]string{"html": "https://zenodo.test/deposit/123"},
		})
	})
	mux.HandleFunc("/api/deposit/depositions/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/files/"):
			name := r.URL.Path[strings.Index(r.URL.Path, "/files/")+len("/files/"):]
			var buf bytes.Buffer
			_, _ = buf.ReadFrom(r.Body)
			uploads[name] = buf.Bytes()
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/actions/publish"):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
				"doi":   "10.5281/zenodo.12345",
				"links": map[string]string{"html": "https://zenodo.test/record/123"},
			})
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	})
	return httptest.NewServer(mux)
}

// TestDeposit_DepositsVerificationBundle is the evidence for #99: the deposit must
// carry what a third party needs to verify the environment, not the lockfile
// alone. Before this change the deposit was one file — a list of digests and
// dangling registry URIs with nothing to check them against.
func TestDeposit_DepositsVerificationBundle(t *testing.T) {
	uploads := map[string][]byte{}
	srv := recordingDepositServer(t, uploads)
	defer srv.Close()

	client := &zenodo.Client{BaseURL: srv.URL, Token: "test-token", HTTPClient: srv.Client()}
	if _, err := client.Deposit(context.Background(), frozenLockfile()); err != nil {
		t.Fatalf("Deposit() unexpected error: %v", err)
	}

	// The trust anchor must be in the record — without it the deposited signature
	// verifies against nothing, and it must be the *embedded* key (the one strata
	// verify and publish check against), byte-for-byte, not some other key.
	key, ok := uploads["cosign.pub"]
	if !ok {
		t.Fatal("cosign.pub was not deposited — a third party has no key to verify against")
	}
	if !bytes.Equal(key, trust.EmbeddedCosignKey()) {
		t.Error("deposited cosign.pub is not the embedded trust anchor")
	}

	// The manifest must state how to verify and, honestly, that layer bytes come
	// from the registry with the digests as the binding (#99 position a).
	manifest, ok := uploads["STRATA-VERIFY.md"]
	if !ok {
		t.Fatal("STRATA-VERIFY.md was not deposited — the deposit does not say how to verify it")
	}
	for _, want := range []string{"cosign.pub", "strata verify", "registry", "SHA-256", "test-profile"} {
		if !strings.Contains(string(manifest), want) {
			t.Errorf("manifest does not mention %q; a third party cannot follow it", want)
		}
	}

	// The lockfile itself is still deposited (it carries the inline set signature).
	var lockDeposited bool
	for name := range uploads {
		if strings.HasSuffix(name, ".lock.yaml") {
			lockDeposited = true
		}
	}
	if !lockDeposited {
		t.Error("the lockfile was not deposited")
	}
}
