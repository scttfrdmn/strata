// Copyright 2026 Scott Friedman
// SPDX-License-Identifier: Apache-2.0

// Package zenodo implements the Zenodo Deposit API client for publishing
// frozen Strata lockfiles as citable datasets with minted DOIs.
//
// The deposit flow is:
//  1. POST /api/deposit/depositions — create empty deposit, get deposit ID
//  2. PUT  /api/deposit/depositions/{id}/files/{filename} — upload each file of
//     the verification bundle: the lockfile YAML (which carries the set-signature
//     bundle inline), the cosign public key, and a verification manifest
//  3. POST /api/deposit/depositions/{id}/actions/publish — publish and mint DOI
//
// The deposit is self-contained enough to establish authenticity with no access
// to the original registry (#99, P2): the lockfile's inline signature is over the
// whole layer set, cosign.pub is the trust anchor to verify it against, and the
// manifest states that layer bytes are fetched from a registry with the signed
// digests as the binding. Layer bytes themselves are not deposited (too large for
// Zenodo) — the honest position (a) of #99.
//
// Authentication uses a personal access token (Bearer). Sandbox mode points
// at https://sandbox.zenodo.org for testing without publishing to production.
package zenodo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/scttfrdmn/strata/internal/trust"
	"github.com/scttfrdmn/strata/spec"
)

// Client calls the Zenodo Deposit API to mint DOIs for frozen lockfiles.
type Client struct {
	// BaseURL is the Zenodo server URL. Defaults to "https://zenodo.org".
	// Set to "https://sandbox.zenodo.org" for testing.
	BaseURL string

	// Token is the Zenodo personal access token (Bearer auth).
	// Obtained from the ZENODO_TOKEN environment variable or the --token flag.
	Token string

	// HTTPClient is the HTTP client to use. Defaults to a 30-second timeout client.
	HTTPClient *http.Client
}

// DepositResult is returned by Deposit on success.
type DepositResult struct {
	// DOI is the minted persistent identifier, e.g. "10.5281/zenodo.12345".
	DOI string

	// RecordURL is the human-readable record page, e.g. "https://zenodo.org/record/12345".
	RecordURL string
}

// Deposit publishes a frozen lockfile to Zenodo and returns the minted DOI.
// The lockfile should be frozen (all layers have SHA256) before calling this
// method; publish.go enforces that precondition before calling Deposit.
//
// Three Zenodo API calls are made in sequence. If any fails, the deposit is
// left in a draft state and the error is returned immediately.
func (c *Client) Deposit(ctx context.Context, lf *spec.LockFile) (*DepositResult, error) {
	dep, err := c.createDeposit(ctx, lf)
	if err != nil {
		return nil, err
	}

	// The lockfile carries the set-signature bundle inline (lf.Bundle), so
	// depositing it deposits the signature over the whole layer set.
	lockBytes, err := yaml.Marshal(lf)
	if err != nil {
		return nil, fmt.Errorf("zenodo: marshaling lockfile for upload: %w", err)
	}
	lockName := lf.ProfileName + "-" + envIDPrefix(lf) + ".lock.yaml"

	// The verification bundle: the lockfile, the public key that verifies its
	// signature (the embedded trust anchor, #62 — the public half of the signing
	// key, confirmable against KMS by #222), and a manifest telling a third party
	// how to verify with neither the registry nor contact with the publisher (P2).
	files := []struct {
		name string
		data []byte
	}{
		{lockName, lockBytes},
		{"cosign.pub", trust.EmbeddedCosignKey()},
		{"STRATA-VERIFY.md", verifyManifest(lf, lockName)},
	}
	for _, f := range files {
		if err := c.uploadBytes(ctx, dep.ID, f.name, f.data); err != nil {
			return nil, err
		}
	}

	return c.publish(ctx, dep.ID)
}

// depositMeta is the JSON body for the initial deposit creation request.
type depositMeta struct {
	Metadata struct {
		Title       string    `json:"title"`
		UploadType  string    `json:"upload_type"`
		Description string    `json:"description"`
		Creators    []creator `json:"creators"`
	} `json:"metadata"`
}

type creator struct {
	Name string `json:"name"`
}

// createResponse is the decoded body from POST /api/deposit/depositions.
type createResponse struct {
	ID    int64 `json:"id"`
	Links struct {
		HTML string `json:"html"`
	} `json:"links"`
}

// publishResponse is the decoded body from POST .../actions/publish.
type publishResponse struct {
	DOI   string `json:"doi"`
	Links struct {
		HTML string `json:"html"`
	} `json:"links"`
}

// createDeposit creates an empty Zenodo deposit and returns the deposit ID.
func (c *Client) createDeposit(ctx context.Context, lf *spec.LockFile) (*createResponse, error) {
	var meta depositMeta
	meta.Metadata.Title = lf.ProfileName + " Strata Environment"
	meta.Metadata.UploadType = "dataset"
	meta.Metadata.Description = lf.EnvironmentID()
	meta.Metadata.Creators = []creator{{Name: "Strata"}}

	body, err := json.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("zenodo: marshaling deposit metadata: %w", err)
	}

	url := c.baseURL() + "/api/deposit/depositions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("zenodo: building create-deposit request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.Token)

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("zenodo: create deposit: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("zenodo: create deposit: unexpected status %d (check ZENODO_TOKEN)", resp.StatusCode)
	}

	var result createResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("zenodo: decoding create-deposit response: %w", err)
	}
	return &result, nil
}

// uploadBytes uploads one file of the verification bundle to the deposit.
func (c *Client) uploadBytes(ctx context.Context, depositID int64, filename string, data []byte) error {
	url := fmt.Sprintf("%s/api/deposit/depositions/%d/files/%s", c.baseURL(), depositID, filename)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("zenodo: building file-upload request: %w", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.ContentLength = int64(len(data))

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("zenodo: upload file: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("zenodo: upload file: unexpected status %d", resp.StatusCode)
	}
	return nil
}

// publish triggers publication of a draft deposit and returns the minted DOI.
func (c *Client) publish(ctx context.Context, depositID int64) (*DepositResult, error) {
	url := fmt.Sprintf("%s/api/deposit/depositions/%d/actions/publish", c.baseURL(), depositID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return nil, fmt.Errorf("zenodo: building publish request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("zenodo: publish deposit: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusAccepted {
		return nil, fmt.Errorf("zenodo: publish deposit: unexpected status %d", resp.StatusCode)
	}

	var result publishResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("zenodo: decoding publish response: %w", err)
	}
	return &DepositResult{DOI: result.DOI, RecordURL: result.Links.HTML}, nil
}

// baseURL returns the effective Zenodo API base URL.
func (c *Client) baseURL() string {
	if c.BaseURL != "" {
		return strings.TrimRight(c.BaseURL, "/")
	}
	return "https://zenodo.org"
}

// httpClient returns the effective HTTP client, defaulting to a 30-second timeout.
func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 30 * time.Second}
}

// verifyManifest builds the STRATA-VERIFY.md deposited alongside the lockfile: it
// tells a third party, holding only this record, how to establish the
// environment's authenticity (P2). It states plainly what is and is not in the
// deposit, so the honest position (a) of #99 is on the record, not in a promise.
func verifyManifest(lf *spec.LockFile, lockName string) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "# Verifying this Strata environment\n\n")
	fmt.Fprintf(&b, "This Zenodo record is a frozen Strata environment and the material to verify\n")
	fmt.Fprintf(&b, "it independently — with no access to the original registry, no shared secret,\n")
	fmt.Fprintf(&b, "and no contact with the publisher.\n\n")

	fmt.Fprintf(&b, "## Files in this record\n\n")
	fmt.Fprintf(&b, "- `%s` — the lockfile: the exact set of layers (each with a SHA-256 digest),\n", lockName)
	fmt.Fprintf(&b, "  the base image, the packages, and the resolution time that define the\n")
	fmt.Fprintf(&b, "  environment. It carries a cosign signature over the whole set **inline** (the\n")
	fmt.Fprintf(&b, "  `bundle` field), with the transparency-log (Rekor) inclusion proof.\n")
	fmt.Fprintf(&b, "- `cosign.pub` — the public key that signature verifies against. It is the\n")
	fmt.Fprintf(&b, "  public half of Strata's signing key (an AWS KMS key); the correspondence to\n")
	fmt.Fprintf(&b, "  the private half is auditable with `aws kms get-public-key`.\n")
	fmt.Fprintf(&b, "- `STRATA-VERIFY.md` — this file.\n\n")

	fmt.Fprintf(&b, "## Environment\n\n")
	fmt.Fprintf(&b, "- Profile: `%s`\n", lf.ProfileName)
	fmt.Fprintf(&b, "- Environment ID: `%s`\n", lf.EnvironmentID())
	fmt.Fprintf(&b, "- Layers: %d\n", len(lf.Layers))
	if lf.RekorEntry != "" {
		fmt.Fprintf(&b, "- Rekor log index: `%s`\n", lf.RekorEntry)
	}
	fmt.Fprintf(&b, "\n")

	fmt.Fprintf(&b, "## 1. Verify the attestation (registry not required)\n\n")
	fmt.Fprintf(&b, "Run `strata verify %s` — it checks the inline signature against the same\n", lockName)
	fmt.Fprintf(&b, "embedded key as `cosign.pub`. The signature is over the lockfile with its own\n")
	fmt.Fprintf(&b, "`bundle` and `rekor_entry` fields blanked, so it commits to every other field:\n")
	fmt.Fprintf(&b, "the layer set and mount order, the digests, the base, and the packages. A valid\n")
	fmt.Fprintf(&b, "signature proves the whole set was attested as a unit by the holder of the\n")
	fmt.Fprintf(&b, "signing key — so none of the digests below can have been altered, and no layer\n")
	fmt.Fprintf(&b, "can have been added, removed or reordered, without breaking it.\n\n")

	fmt.Fprintf(&b, "## 2. Verify the layer bytes (registry required, digests are the binding)\n\n")
	fmt.Fprintf(&b, "Layer bytes are **not** in this record — they are too large for a citation\n")
	fmt.Fprintf(&b, "record. Each layer's `source` in the lockfile names the registry to fetch it\n")
	fmt.Fprintf(&b, "from; the SHA-256 in the lockfile is the binding. For each layer: fetch the\n")
	fmt.Fprintf(&b, "bytes, compute their SHA-256, and confirm it equals the digest step 1 attested.\n")
	fmt.Fprintf(&b, "Each layer's own attestation `bundle` is likewise at the registry; because the\n")
	fmt.Fprintf(&b, "set signature already attests every digest, byte integrity reduces to this hash\n")
	fmt.Fprintf(&b, "comparison — the registry can supply the bytes but cannot alter them undetected.\n")
	return []byte(b.String())
}

// envIDPrefix returns the first 8 hex characters of the environment ID,
// or "unknown" if the lockfile is not frozen.
func envIDPrefix(lf *spec.LockFile) string {
	id := lf.EnvironmentID()
	if len(id) >= 8 {
		return id[:8]
	}
	return "unknown"
}
