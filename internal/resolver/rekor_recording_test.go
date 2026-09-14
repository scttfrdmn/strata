package resolver_test

import (
	"context"
	"strings"
	"testing"

	"github.com/scttfrdmn/strata/internal/trust"
)

// recordingRekorClient records every VerifyEntry call so a test can assert what
// stage 7 passes — the argument trust.FakeRekorClient discards, and whose loss
// let the stage-7 call site stay unmeasured (the note on FakeRekorClient.VerifyEntry,
// #59). verifyErr is returned from every VerifyEntry, so one type drives both the
// success and the failure path. Log is present only to satisfy the RekorClient
// interface; resolution never logs.
type recordingRekorClient struct {
	verifyErr error
	calls     []verifyEntryCall
}

type verifyEntryCall struct {
	logIndex int64
	bundle   *trust.Bundle
}

func (c *recordingRekorClient) Log(context.Context, *trust.Bundle) (int64, error) {
	return 0, nil
}

func (c *recordingRekorClient) VerifyEntry(_ context.Context, logIndex int64, bundle *trust.Bundle) error {
	c.calls = append(c.calls, verifyEntryCall{logIndex: logIndex, bundle: bundle})
	return c.verifyErr
}

// assertErrMentions fails if err is nil or its message does not contain want. It
// lives here rather than in resolver_test.go so that file needs no new import.
func assertErrMentions(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected an error mentioning %q, got nil", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error does not mention %q: %v", want, err)
	}
}
