package trust

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
)

// postJSON sends a POST request with JSON body and returns the response.
// The caller is responsible for closing resp.Body.
func postJSON(ctx context.Context, url string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body.Close() //nolint:errcheck
		return nil, fmt.Errorf("unexpected status %d from %s", resp.StatusCode, url)
	}
	return resp, nil
}

// getJSON sends a GET request and returns the response.
// The caller is responsible for closing resp.Body.
func getJSON(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body.Close() //nolint:errcheck
		return nil, fmt.Errorf("unexpected status %d from %s", resp.StatusCode, url)
	}
	return resp, nil
}
