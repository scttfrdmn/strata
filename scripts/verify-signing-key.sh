#!/usr/bin/env bash
# Copyright 2026 Scott Friedman
# SPDX-License-Identifier: Apache-2.0
#
# verify-signing-key.sh — assert the public key embedded in the binary
# (internal/trust/keys/cosign.pub) is the public half of the KMS signing key
# alias/strata-signing-key. This is the #62 correspondence: verification uses the
# embedded key with no AWS call, so nothing in the build or the test suite proves
# the embedded bytes still match the key that actually signs. Run it at release,
# with the strata profile, before tagging.
#
# It compares the DER (canonical SubjectPublicKeyInfo) of each key, so PEM
# whitespace/formatting differences cannot mask a real mismatch, and it fails
# loudly and distinctly when it cannot reach KMS — an unreachable key must not
# read as a match (a check whose failure looks like its success is not a check).
set -euo pipefail

KEY_FILE="${KEY_FILE:-internal/trust/keys/cosign.pub}"
KEY_ID="${STRATA_SIGNING_KEY_ID:-alias/strata-signing-key}"
# Default to the strata profile explicitly (CLAUDE.md: the signing key lives in the
# Strata Infrastructure account, reached via --profile strata). Do NOT fall back to
# an ambient AWS_PROFILE — that silently redirects the check to whatever account the
# shell happens to point at, where the alias is "not found" for the wrong reason.
# Override deliberately with STRATA_AWS_PROFILE when the profile name differs.
AWS_PROFILE_ARG="${STRATA_AWS_PROFILE:-strata}"
REGION="${AWS_REGION:-us-east-1}"

fail() { echo "verify-signing-key: $*" >&2; exit 1; }

[ -f "$KEY_FILE" ] || fail "embedded key not found at $KEY_FILE"

# DER-fingerprint the embedded key. A malformed embedded key is an error, not a
# mismatch.
embedded_der="$(openssl pkey -pubin -in "$KEY_FILE" -outform DER 2>/dev/null | openssl dgst -sha256 | awk '{print $NF}')" \
  || fail "could not read $KEY_FILE as a public key"
[ -n "$embedded_der" ] || fail "could not read $KEY_FILE as a public key"

# Fetch the KMS public key. Capture the raw base64 first so an AWS error (expired
# credentials, wrong profile, missing kms:GetPublicKey) surfaces as an instrument
# failure — never as an empty string that would digest to the empty-input hash and
# masquerade as a mismatch.
kms_b64="$(aws --profile "$AWS_PROFILE_ARG" --region "$REGION" kms get-public-key \
  --key-id "$KEY_ID" --query PublicKey --output text 2>&1)" \
  || fail "aws kms get-public-key failed (profile=$AWS_PROFILE_ARG key=$KEY_ID): $kms_b64"
[ -n "$kms_b64" ] && [ "$kms_b64" != "None" ] \
  || fail "aws kms get-public-key returned no PublicKey for $KEY_ID"

kms_der="$(printf '%s' "$kms_b64" | base64 -d 2>/dev/null | openssl dgst -sha256 | awk '{print $NF}')"
[ -n "$kms_der" ] || fail "could not decode the KMS public key returned for $KEY_ID"

if [ "$embedded_der" = "$kms_der" ]; then
  echo "verify-signing-key: OK — embedded $KEY_FILE matches KMS $KEY_ID (SPKI sha256 $embedded_der)"
  exit 0
fi

fail "MISMATCH — embedded $KEY_FILE (SPKI sha256 $embedded_der) is NOT the public half of KMS $KEY_ID (SPKI sha256 $kms_der). The binary would verify against a key that no longer signs; do not release."
