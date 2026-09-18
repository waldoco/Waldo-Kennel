//go:build !windows

package e2e

import (
	"encoding/hex"
	"strings"
	"testing"
)

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

// TestProbeTargetMarkerBindsExactly pins the round-8 contract: the
// KENNEL-PROBE-TARGET marker must carry the HEX ENCODING of the intended
// resolved dir as ONE complete delimited field - raw-text paths, delimiter
// injection, newline framing, embedding relatives, and duplicate or
// injected markers never pass. These are the laundering shapes the round-6
// and round-7 reviews demonstrated live.
func TestProbeTargetMarkerBindsExactly(t *testing.T) {
	dir := "/Users/runner/kennel-falsifier-outside-2358120678"
	enc := hex.EncodeToString([]byte(dir))
	cases := []struct {
		name   string
		output string
		want   bool
	}{
		{"exact encoded marker", "some prelude\nKENNEL-PROBE-TARGET=<" + enc + ">\nsh: x: Operation not permitted\n", true},
		{"raw unencoded path never passes", "KENNEL-PROBE-TARGET=<" + dir + ">\n", false},
		{"uppercase hex never passes", "KENNEL-PROBE-TARGET=<" + strings.ToUpper(enc) + ">\n", false},
		{"round-7 raw newline-split never passes", "KENNEL-PROBE-TARGET=<" + dir + ">\nactor-tail>\n", false},
		{"duplicate marker never passes", "KENNEL-PROBE-TARGET=<" + enc + ">\nKENNEL-PROBE-TARGET=<" + enc + ">\n", false},
		{"conflicting marker never passes", "KENNEL-PROBE-TARGET=<" + enc + ">\nKENNEL-PROBE-TARGET=<" + hex.EncodeToString([]byte("/tmp/other")) + ">\n", false},
		{"encoded suffix-embedding never passes", "KENNEL-PROBE-TARGET=<" + hex.EncodeToString([]byte("/workspace/denied-prefix"+dir)) + ">\n", false},
		{"encoded prefix-embedding never passes", "KENNEL-PROBE-TARGET=<" + hex.EncodeToString([]byte(dir+".evil")) + ">\n", false},
		{"trailing junk never passes", "KENNEL-PROBE-TARGET=<" + enc + "> extra\n", false},
		{"framing close-open never passes", "KENNEL-PROBE-TARGET=<" + enc + "><" + enc + ">\n", false},
		{"no marker never passes", "sh: x: Operation not permitted\n", false},
		{"empty field never passes", "KENNEL-PROBE-TARGET=<>\n", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := probeTargetEvidence(tc.output, dir); got != tc.want {
				t.Fatalf("probeTargetEvidence(%q) = %v, want %v", tc.output, got, tc.want)
			}
		})
	}
}
