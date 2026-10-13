package macaction

import (
	"context"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestSeparateBudgets(t *testing.T) {
	if ApprovalWindow != 15*time.Minute {
		t.Fatal(ApprovalWindow)
	}
	d, e := Timeout(0)
	if e != nil || d != 2*time.Minute {
		t.Fatal(d, e)
	}
	d, e = Timeout(600)
	if e != nil || d != 10*time.Minute {
		t.Fatal(d, e)
	}
	for _, n := range []int64{-1, 601} {
		if _, e = Timeout(n); e == nil {
			t.Fatal(n)
		}
	}
}
func TestRunnerTimeoutKillsDescendants(t *testing.T) {
	start := time.Now()
	result, e := (Runner{Shell: "/bin/sh", Environment: []string{"PATH=/usr/bin:/bin"}}).Run(context.Background(), t.TempDir(), "sleep 30 & wait", 1)
	if e != nil || result.ExitCode != -1 || time.Since(start) > 4*time.Second {
		t.Fatal(result, e, time.Since(start))
	}
}
func TestCaptureAndUTF8Tail(t *testing.T) {
	c := new(capture)
	c.Write([]byte(strings.Repeat("雪", CaptureLimit)))
	if c.local.Len() != CaptureLimit || len(c.tail) > TailLimit {
		t.Fatal(c.local.Len(), len(c.tail))
	}
	tail := Tail(c.tail)
	if !utf8.ValidString(tail) || len(tail) > TailLimit {
		t.Fatal(len(tail))
	}
	if got := ScrubEnvironment([]string{"PATH=/bin", "TOKEN=secret", "KENNEL_OWNER_TOKEN=secret", "HOME=/tmp"}); len(got) != 2 {
		t.Fatal(got)
	}
}

func TestRiskWarningsForDestinations(t *testing.T) {
	for _, command := range []string{`cp file /tmp/file`, `mv -f file ../outside`, `cp -r "source file" "/outside path/file"`, `tee /tmp/output`, `echo > /project/../outside`} {
		if !strings.Contains(strings.Join(Risks(command, "/project"), ","), "write outside project") {
			t.Fatal(command)
		}
	}
	if strings.Contains(strings.Join(Risks(`cp source /project/file`, "/project"), ","), "write outside project") {
		t.Fatal("registered destination warned")
	}
}
