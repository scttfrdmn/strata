// Copyright 2026 Scott Friedman
// SPDX-License-Identifier: Apache-2.0

package trust

import (
	"bytes"
	_ "embed" // for the go:embed directive below
	"fmt"
	"os"
)

// embeddedCosignKey is the Strata public signing key, pinned into every binary
// at build time. It is the public half of the AWS KMS key
// alias/strata-signing-key (verified to match its KMS-held private half, #62);
// verification uses only this public half, so it needs no AWS, no KMS, and no
// network beyond the Rekor inclusion proof carried in a bundle. It lives here,
// in the trust package, so both the agent and the CLI verify against one anchor.
//
//go:embed keys/cosign.pub
var embeddedCosignKey []byte

// EmbeddedCosignKey returns the pinned public signing key bytes.
func EmbeddedCosignKey() []byte { return embeddedCosignKey }

// WriteEmbeddedKeyFile materialises the embedded public key to a temp file and
// returns its path and a cleanup func. cosign verify-blob takes a --key path,
// not bytes. An empty embed is an error: a binary built without a trust anchor
// cannot verify, and its callers turn that into a refusal rather than a skip.
func WriteEmbeddedKeyFile() (path string, cleanup func(), err error) {
	if len(bytes.TrimSpace(embeddedCosignKey)) == 0 {
		return "", func() {}, fmt.Errorf("trust: no embedded cosign public key — the binary was built without a trust anchor")
	}
	f, err := os.CreateTemp("", "strata-cosign-*.pub")
	if err != nil {
		return "", func() {}, fmt.Errorf("trust: writing embedded key: %w", err)
	}
	path = f.Name()
	cleanup = func() { _ = os.Remove(path) }
	if _, err := f.Write(embeddedCosignKey); err != nil {
		_ = f.Close()
		cleanup()
		return "", func() {}, fmt.Errorf("trust: writing embedded key: %w", err)
	}
	if err := f.Close(); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("trust: writing embedded key: %w", err)
	}
	return path, cleanup, nil
}

// EmbeddedKeyVerifier returns a Verifier that checks signatures against the
// embedded public key, plus a cleanup func for the temp key file it writes.
// Used by strata verify and the agent to verify a lockfile signature with no
// AWS access — the transparency proof rides in the bundle.
func EmbeddedKeyVerifier() (Verifier, func(), error) {
	path, cleanup, err := WriteEmbeddedKeyFile()
	if err != nil {
		return nil, cleanup, err
	}
	return &CosignVerifier{KeyRef: path}, cleanup, nil
}
