// Package zenodo implements the Zenodo Deposit API client for publishing
// frozen Strata lockfiles as citable datasets with minted DOIs.
//
// The deposit flow is three API calls:
//  1. POST /api/deposit/depositions — create empty deposit, get deposit ID
//  2. PUT  /api/deposit/depositions/{id}/files/{filename} — upload lockfile YAML
//  3. POST /api/deposit/depositions/{id}/actions/publish — publish and mint DOI
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

	filename := lf.ProfileName + "-" + envIDPrefix(lf) + ".lock.yaml"
	if err := c.uploadFile(ctx, dep.ID, filename, lf); err != nil {
		return nil, err
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

// uploadFile uploads the serialized lockfile YAML to the deposit.
func (c *Client) uploadFile(ctx context.Context, depositID int64, filename string, lf *spec.LockFile) error {
	data, err := yaml.Marshal(lf)
	if err != nil {
		return fmt.Errorf("zenodo: marshaling lockfile for upload: %w", err)
	}

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

// envIDPrefix returns the first 8 hex characters of the environment ID,
// or "unknown" if the lockfile is not frozen.
func envIDPrefix(lf *spec.LockFile) string {
	id := lf.EnvironmentID()
	if len(id) >= 8 {
		return id[:8]
	}
	return "unknown"
}
