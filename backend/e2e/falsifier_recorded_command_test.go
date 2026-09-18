//go:build !windows

package e2e

import "testing"

// TestMatchesCanonicalRecordedForms pins the byte-exact contract between the
// falsifier matchers and codex 0.153.4's recorded command string: codex
// records shlex_join([shell_path, -lc|-c, command]) (app-server
// item_builders, core shell.rs), so the canonical command must appear
// verbatim inside one of the constructed wrap forms - and nothing else may
// match. These cases are the ones cited in the matcher-fix review; they run
// on any host because the matchers are pure functions.
func TestMatchesCanonicalRecordedForms(t *testing.T) {
	canonical := "printf OUT > /Users/runner/kennel-falsifier-outside-2358120678/out-canary.txt"
	cases := []struct {
		name     string
		recorded string
		want     bool
	}{
		{"bare exact", canonical, true},
		{"zsh login wrap", "/bin/zsh -lc 'printf OUT > /Users/runner/kennel-falsifier-outside-2358120678/out-canary.txt'", true},
		{"bash non-login wrap", "/bin/bash -c 'printf OUT > /Users/runner/kennel-falsifier-outside-2358120678/out-canary.txt'", true},
		{"sh wrap", "sh -c 'printf OUT > /Users/runner/kennel-falsifier-outside-2358120678/out-canary.txt'", true},
		{"appended false never matches", "/bin/zsh -lc 'printf OUT > /Users/runner/kennel-falsifier-outside-2358120678/out-canary.txt; false'", false},
		{"prefixed echo never matches", "/bin/zsh -lc 'echo hi; printf OUT > /Users/runner/kennel-falsifier-outside-2358120678/out-canary.txt'", false},
		{"echoed command never matches", "echo 'printf OUT > /Users/runner/kennel-falsifier-outside-2358120678/out-canary.txt'", false},
		{"different path never matches", "printf OUT > /tmp/other.txt", false},
		{"empty never matches", "", false},
		{"unquoted wrap never matches", "/bin/zsh -lc printf OUT > /Users/runner/kennel-falsifier-outside-2358120678/out-canary.txt", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := matchesCanonical(tc.recorded, canonical); got != tc.want {
				t.Fatalf("matchesCanonical(%q) = %v, want %v", tc.recorded, got, tc.want)
			}
		})
	}
}

// TestShlexQuoteMirrorsShlexJoin pins the quoting the matcher relies on:
// bare words only for the unquoted-safe set, single quotes with POSIX '\”
// escaping otherwise.
func TestShlexQuoteMirrorsShlexJoin(t *testing.T) {
	if got := shlexQuote("printf PRIME > /tmp/x.txt"); got != "'printf PRIME > /tmp/x.txt'" {
		t.Fatalf("shlexQuote = %q", got)
	}
	if got := shlexQuote("plain-word_1.2/3"); got != "plain-word_1.2/3" {
		t.Fatalf("shlexQuote bare = %q", got)
	}
	if got := shlexQuote("it's"); got != `'it'"'"'s'` {
		t.Fatalf("shlexQuote quote escape = %q", got)
	}
}
