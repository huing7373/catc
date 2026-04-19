package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestRun_RequiresConfirmInput locks §21.5 CONFIRM guard + §21.8 #9:
// stdin != "CONFIRM" must abort with exit 1 and NOT attempt any
// Mongo / Redis writes. This test runs as a pure unit test (no
// Docker / integration tag) because the CONFIRM check fires before
// any collection access.
func TestRun_RequiresConfirmInput(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		stdin string
	}{
		{"empty", ""},
		{"wrong_phrase", "WRONG\n"},
		{"lowercase_confirm", "confirm\n"},
		{"leading_whitespace", " CONFIRM\n"},
		{"trailing_whitespace", "CONFIRM \n"},
		{"confirm_no_newline", "CONFIRM"}, // reader.ReadString returns io.EOF but with the content
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			in := strings.NewReader(tc.stdin)
			var out, errOut bytes.Buffer
			// db / redis are nil — the CONFIRM check runs before any
			// collection access so nil is safe for this subtest.
			args := runArgs{
				in:            in,
				out:           &out,
				errOut:        &errOut,
				dryRun:        false,
				olderThanDays: 30,
				limit:         10,
			}
			// The "confirm_no_newline" case happens to match — stdin
			// returns io.EOF + "CONFIRM" and reader.ReadString returns
			// that exact string. We expect it to SUCCEED the guard but
			// then fail later on nil db. Skip that branch in this
			// subtest.
			if tc.name == "confirm_no_newline" {
				t.Skip("confirm_no_newline passes the guard — that's a separate path tested by integration tests")
				return
			}
			code := run(args)
			assert.Equal(t, 1, code, "non-CONFIRM stdin must exit non-zero")
			assert.Empty(t, out.String(), "no summary JSON must be written when guard rejects")
			assert.NotEmpty(t, errOut.String(), "stderr must carry a diagnostic")
		})
	}
}

// TestRun_NegativeOlderThanDaysRejected locks a defense-in-depth check
// that runs after CONFIRM but before any Mongo work: a negative
// grace period would let ops accidentally sweep into the future
// (cutoff = now - negative = future), matching every recently-
// requested user.
func TestRun_NegativeOlderThanDaysRejected(t *testing.T) {
	t.Parallel()

	in := strings.NewReader("CONFIRM\n")
	var out, errOut bytes.Buffer
	code := run(runArgs{
		in:            in,
		out:           &out,
		errOut:        &errOut,
		dryRun:        false,
		olderThanDays: -1,
		limit:         10,
	})
	assert.Equal(t, 1, code)
	assert.Contains(t, errOut.String(), "older-than-days")
	assert.Empty(t, out.String())
}

func TestRun_ZeroLimitRejected(t *testing.T) {
	t.Parallel()

	in := strings.NewReader("CONFIRM\n")
	var out, errOut bytes.Buffer
	code := run(runArgs{
		in:            in,
		out:           &out,
		errOut:        &errOut,
		dryRun:        false,
		olderThanDays: 30,
		limit:         0,
	})
	assert.Equal(t, 1, code)
	assert.Contains(t, errOut.String(), "limit")
	assert.Empty(t, out.String())
}

// TestRun_BannerWrittenToStderr locks that the banner (§21.5
// destructive-action warning) lands on stderr, never stdout, so an
// ops engineer piping `... | jq .` sees the banner on their
// terminal rather than losing it.
func TestRun_BannerWrittenToStderr(t *testing.T) {
	t.Parallel()

	in := strings.NewReader("WRONG\n")
	var out, errOut bytes.Buffer
	_ = run(runArgs{
		in:     in,
		out:    &out,
		errOut: &errOut,
		limit:  10,
	})
	assert.Contains(t, errOut.String(), "PERMANENTLY DELETE",
		"banner must appear on stderr")
	assert.NotContains(t, out.String(), "PERMANENTLY DELETE",
		"banner must NOT appear on stdout (piped JSON readers must stay clean)")
}
