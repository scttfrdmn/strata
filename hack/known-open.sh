#!/usr/bin/env bash
#
# known-open.sh enumerates the test controls that assert "a defect still
# reproduces" — the ones that must go red when their defect is fixed, so the fix
# cannot land green with the control silently lying. A control of this shape is
# worth exactly the demonstration that it can fail, and that demonstration is
# otherwise a thing a human did once (#148).
#
# It is a finder, not a checker: it prints the population across four enumeration
# axes (no single marker finds them all) so the next such control is discovered
# rather than remembered. Applying each fix and confirming the red is still
# manual; the sweep that found four false controls in this population is on #148.
#
# Run: make known-open
set -uo pipefail
cd "$(dirname "$0")/.."

echo "# Controls that assert a live defect (#148). Verify each can go red when its defect is fixed."
echo

echo "== axis 1: marker phrases =="
grep -rn --include='*_test.go' -E 'knownOpen|assertStillSpurious|no longer reproduces|is no longer refuted' . || true
echo

echo "== axis 2: controls whose NAME asserts an absence a fix would break =="
grep -rn --include='*_test.go' -E '^func Test[A-Za-z0-9_]*(IsSilent|SaysNothing)[A-Za-z0-9_]*\(' . || true
echo

echo "== axis 3: count constants that record a defect population =="
grep -rn --include='*_test.go' -E '^[[:space:]]*(const )?[A-Za-z]+ (=|:=) [0-9]+' . \
  | grep -Ei '#[0-9]+|open|defect|violat|reachable|discriminat' || true
echo

echo "== axis 4: a fix named as the trigger, in prose =="
grep -rn --include='*_test.go' -E 'when #[0-9]+ is fixed|it is fixed|is the instruction' . || true
