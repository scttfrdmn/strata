package zenodo_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/scttfrdmn/strata/internal/zenodo"
	"github.com/scttfrdmn/strata/spec"
)

// frozenLockfile returns a minimal frozen lockfile suitable for deposit tests.
func frozenLockfile() *spec.LockFile {
	return &spec.LockFile{
		ProfileName:   "test-profile",
		StrataVersion: "0.0.0-test",
		Base: spec.ResolvedBase{
			DeclaredOS: "al2023",
			AMIID:      "ami-test123",
			AMISHA256:  strings.Repeat("a", 64),
			Capabilities: spec.BaseCapabilities{
				AMIID:    "ami-test123",
				Family:   "rhel",
				ProbedAt: time.Now(),
			},
		},
		Layers: []spec.ResolvedLayer{
			{
				LayerManifest: spec.LayerManifest{
					ID:      "python-3.11.9-rhel-x86_64",
					Name:    "python",
					Version: "3.11.9",
					SHA256:  strings.Repeat("b", 64),
				},
				MountOrder: 1,
			},
		},
	}
}

// newTestServer constructs an httptest.Server that serves the three Zenodo
// deposit endpoints with the given status codes.
func newTestServer(
	createStatus, uploadStatus, publishStatus int,
	depositID int64,
	doi string,
) *httptest.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/deposit/depositions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(createStatus)
		if createStatus == http.StatusCreated {
			resp := map[string]any{
				"id": depositID,
				"links": map[string]string{
					"html": "https://zenodo.test/deposit/" + "123",
				},
			}
			json.NewEncoder(w).Encode(resp) //nolint:errcheck
		}
	})

	mux.HandleFunc("/api/deposit/depositions/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodPut && strings.Contains(path, "/files/"):
			w.WriteHeader(uploadStatus)
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/actions/publish"):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(publishStatus)
			if publishStatus == http.StatusAccepted {
				resp := map[string]any{
					"doi": doi,
					"links": map[string]string{
						"html": "https://zenodo.test/record/123",
					},
				}
				json.NewEncoder(w).Encode(resp) //nolint:errcheck
			}
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	})

	return httptest.NewServer(mux)
}

func TestDeposit_HappyPath(t *testing.T) {
	const (
		depositID = int64(12345)
		wantDOI   = "10.5281/zenodo.12345"
	)

	srv := newTestServer(http.StatusCreated, http.StatusOK, http.StatusAccepted, depositID, wantDOI)
	defer srv.Close()

	client := &zenodo.Client{
		BaseURL:    srv.URL,
		Token:      "test-token",
		HTTPClient: srv.Client(),
	}

	lf := frozenLockfile()
	result, err := client.Deposit(context.Background(), lf)
	if err != nil {
		t.Fatalf("Deposit() unexpected error: %v", err)
	}
	if result.DOI != wantDOI {
		t.Errorf("DOI = %q, want %q", result.DOI, wantDOI)
	}
	if result.RecordURL == "" {
		t.Error("RecordURL is empty")
	}
}

func TestDeposit_CreateFails(t *testing.T) {
	srv := newTestServer(http.StatusUnauthorized, http.StatusOK, http.StatusAccepted, 0, "")
	defer srv.Close()

	client := &zenodo.Client{
		BaseURL:    srv.URL,
		Token:      "bad-token",
		HTTPClient: srv.Client(),
	}

	_, err := client.Deposit(context.Background(), frozenLockfile())
	if err == nil {
		t.Fatal("expected error from 401 response, got nil")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error should mention status 401, got: %v", err)
	}
}

func TestDeposit_UploadFails(t *testing.T) {
	srv := newTestServer(http.StatusCreated, http.StatusInternalServerError, http.StatusAccepted, 1, "")
	defer srv.Close()

	client := &zenodo.Client{
		BaseURL:    srv.URL,
		Token:      "test-token",
		HTTPClient: srv.Client(),
	}

	_, err := client.Deposit(context.Background(), frozenLockfile())
	if err == nil {
		t.Fatal("expected error from 500 upload response, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should mention status 500, got: %v", err)
	}
}

func TestDeposit_PublishFails(t *testing.T) {
	srv := newTestServer(http.StatusCreated, http.StatusOK, http.StatusBadRequest, 1, "")
	defer srv.Close()

	client := &zenodo.Client{
		BaseURL:    srv.URL,
		Token:      "test-token",
		HTTPClient: srv.Client(),
	}

	_, err := client.Deposit(context.Background(), frozenLockfile())
	if err == nil {
		t.Fatal("expected error from 400 publish response, got nil")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("error should mention status 400, got: %v", err)
	}
}
