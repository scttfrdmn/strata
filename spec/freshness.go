// Copyright 2026 Scott Friedman
// SPDX-License-Identifier: Apache-2.0

package spec

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Freshness (#224, threat T8). ResolvedAt is inside the signed lockfile payload
// (see internal/trust.lockfileSigningPayload), so a verified lockfile's age is
// tamper-resistant: an attacker replaying a stale but correctly-signed lockfile
// cannot backdate resolved_at to look current without breaking the signature.
//
// The policy is deliberately "surface always, refuse opt-in". A frozen, cited,
// archived lockfile is *meant* to be old — re-running a years-old cited
// environment is exactly what strata exists for — so there is no default max-age
// that would refuse a stale lockfile. FreshnessNote always states how far behind
// a lockfile is; CheckFreshness refuses one only when the caller sets a bound.
// This is the "minimum honest version" the issue names: rollback and freeze
// become detectable and refusable rather than silent, but a signed
// timestamp/snapshot role (the fuller TUF answer) remains future work.

// Age reports how long ago the lockfile was resolved, relative to now. It is
// only a trustworthy measure after the lockfile's signature has verified, since
// that is what binds ResolvedAt. A future-dated lockfile (clock skew, or a
// forged-forward timestamp) yields a negative Age.
func (l *LockFile) Age(now time.Time) time.Duration {
	return now.Sub(l.ResolvedAt)
}

// FreshnessNote is the always-surfaced disclosure of how far behind a lockfile
// is — the "surface" half of the freshness policy. It returns "" when ResolvedAt
// is unset (nothing to say), and flags a future-dated lockfile rather than
// printing a negative age. strata run, strata verify, and the agent all print it
// so a stale but validly-signed environment is never silently accepted.
func (l *LockFile) FreshnessNote(now time.Time) string {
	if l.ResolvedAt.IsZero() {
		return ""
	}
	date := l.ResolvedAt.UTC().Format("2006-01-02")
	if age := l.Age(now); age < 0 {
		return fmt.Sprintf("environment resolved_at is in the future (%s) — check the local clock", date)
	}
	return fmt.Sprintf("environment resolved %s ago (%s)", HumanizeAge(l.Age(now)), date)
}

// CheckFreshness refuses a lockfile older than maxAge. A maxAge of zero or
// negative means no bound: freshness is opt-in, so the check never refuses and
// only FreshnessNote's surfacing applies. When a bound is set, a lockfile with
// no ResolvedAt is refused (freshness cannot be established), and the error names
// the age, the resolved date, and the bound so the refusal is actionable.
func (l *LockFile) CheckFreshness(maxAge time.Duration, now time.Time) error {
	if maxAge <= 0 {
		return nil
	}
	if l.ResolvedAt.IsZero() {
		return fmt.Errorf("lockfile has no resolved_at, so its freshness cannot be established against max-age %s", HumanizeAge(maxAge))
	}
	if age := l.Age(now); age > maxAge {
		return fmt.Errorf("lockfile resolved %s ago (%s) exceeds max-age %s",
			HumanizeAge(age), l.ResolvedAt.UTC().Format("2006-01-02"), HumanizeAge(maxAge))
	}
	return nil
}

// HumanizeAge renders a duration in its largest whole unit at or above one —
// days, then hours, minutes, seconds — so an age reads "412 days" or "3 hours"
// rather than "9888h0m0s". A negative duration is rendered by magnitude; callers
// that care about direction check the sign themselves (see FreshnessNote).
func HumanizeAge(d time.Duration) string {
	if d < 0 {
		d = -d
	}
	switch {
	case d >= 24*time.Hour:
		return pluralizeUnit(int64(d/(24*time.Hour)), "day")
	case d >= time.Hour:
		return pluralizeUnit(int64(d/time.Hour), "hour")
	case d >= time.Minute:
		return pluralizeUnit(int64(d/time.Minute), "minute")
	default:
		return pluralizeUnit(int64(d/time.Second), "second")
	}
}

// pluralizeUnit renders "1 day" / "412 days".
func pluralizeUnit(n int64, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
}

// ParseMaxAge parses a max-age bound. It accepts a "d" (day) or "w" (week)
// suffix, which Go's time.ParseDuration does not, and otherwise defers to
// time.ParseDuration (so "720h", "30m" work). An empty string means no bound
// (returns 0). The result must not be negative.
func ParseMaxAge(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	var d time.Duration
	switch {
	case strings.HasSuffix(s, "d"):
		days, err := strconv.ParseFloat(strings.TrimSuffix(s, "d"), 64)
		if err != nil {
			return 0, fmt.Errorf("invalid max-age %q (use e.g. 30d, 2w, 720h): %w", s, err)
		}
		d = time.Duration(days * float64(24*time.Hour))
	case strings.HasSuffix(s, "w"):
		weeks, err := strconv.ParseFloat(strings.TrimSuffix(s, "w"), 64)
		if err != nil {
			return 0, fmt.Errorf("invalid max-age %q (use e.g. 30d, 2w, 720h): %w", s, err)
		}
		d = time.Duration(weeks * float64(7*24*time.Hour))
	default:
		parsed, err := time.ParseDuration(s)
		if err != nil {
			return 0, fmt.Errorf("invalid max-age %q (use e.g. 30d, 2w, 720h): %w", s, err)
		}
		d = parsed
	}
	if d < 0 {
		return 0, fmt.Errorf("invalid max-age %q: must not be negative", s)
	}
	return d, nil
}
