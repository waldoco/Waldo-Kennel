// Package macaction owns approval-gated Mac actions. Approval expiry is a start
// deadline, never an execution deadline. Durable custody lives in the daemon.
package macaction

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"
)

const ApprovalWindow = 15 * time.Minute
const DefaultTimeout = 2 * time.Minute
const MaximumTimeout = 10 * time.Minute
const CaptureLimit = 1 << 20
const TailLimit = 6144

var ErrInvalidTimeout = errors.New("execution timeout must be between 1 and 600 seconds")
var ErrInvalidProject = errors.New("project must resolve to a registered directory")

// Timeout is independent of the approval deadline and starts at execution.
func Timeout(seconds int64) (time.Duration, error) {
	if seconds == 0 {
		return DefaultTimeout, nil
	}
	if seconds < 1 || seconds > 600 {
		return 0, ErrInvalidTimeout
	}
	return time.Duration(seconds) * time.Second, nil
}

// capture drains indefinitely but stores at most one MiB. Retaining a rolling
// tail independently avoids sending the beginning of a truncated local log.
type capture struct {
	mu    sync.Mutex
	local bytes.Buffer
	tail  []byte
	total int64
}

func (c *capture) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.total += int64(len(p))
	if left := CaptureLimit - c.local.Len(); left > 0 {
		n := len(p)
		if n > left {
			n = left
		}
		_, _ = c.local.Write(p[:n])
	}
	if len(p) >= TailLimit {
		c.tail = append(c.tail[:0], p[len(p)-TailLimit:]...)
	} else {
		c.tail = append(c.tail, p...)
		if len(c.tail) > TailLimit {
			c.tail = append([]byte(nil), c.tail[len(c.tail)-TailLimit:]...)
		}
	}
	return len(p), nil
}
func Tail(raw []byte) string {
	if len(raw) > TailLimit {
		raw = raw[len(raw)-TailLimit:]
	}
	// Both a clipped rune and arbitrary binary output stay bounded and valid.
	for len(raw) > 0 && !utf8.RuneStart(raw[0]) {
		raw = raw[1:]
	}
	s := string(bytes.ToValidUTF8(raw, []byte("�")))
	for len(s) > TailLimit {
		_, n := utf8.DecodeRuneInString(s)
		s = s[n:]
	}
	return s
}

type Execution struct {
	ExitCode    int
	OutputTail  string
	Truncated   bool
	LocalOutput []byte
}
type Runner struct {
	Shell       string
	Environment []string
}

func ScrubEnvironment(env []string) []string {
	allowed := map[string]bool{"PATH": true, "HOME": true, "USER": true, "LOGNAME": true, "LANG": true, "LC_ALL": true, "LC_CTYPE": true, "TERM": true, "TMPDIR": true}
	var out []string
	for _, v := range env {
		for i := 0; i < len(v); i++ {
			if v[i] == '=' {
				if allowed[v[:i]] {
					out = append(out, v)
				}
				break
			}
		}
	}
	return out
}
func (r Runner) Run(ctx context.Context, cwd, command string, seconds int64) (Execution, error) {
	limit, err := Timeout(seconds)
	if err != nil {
		return Execution{}, err
	}
	if !filepath.IsAbs(cwd) {
		return Execution{}, ErrInvalidProject
	}
	stat, err := os.Stat(cwd)
	if err != nil || !stat.IsDir() {
		return Execution{}, ErrInvalidProject
	}
	if !filepath.IsAbs(r.Shell) {
		return Execution{}, errors.New("login shell unavailable")
	}
	// Begin only after the caller durably commits started. Approval time is not
	// subtracted from this independent execution budget.
	runctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	cmd := exec.CommandContext(runctx, r.Shell, "-l", "-c", command)
	cmd.Dir = cwd
	cmd.Env = ScrubEnvironment(r.Environment)
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = time.Second
	var output capture
	cmd.Stdout = &output
	cmd.Stderr = &output
	err = cmd.Run()
	result := Execution{ExitCode: 0, OutputTail: Tail(output.tail), Truncated: output.total > TailLimit, LocalOutput: append([]byte(nil), output.local.Bytes()...)}
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			result.ExitCode = exit.ExitCode()
		} else {
			return result, err
		}
	}
	if runctx.Err() != nil {
		result.ExitCode = -1
	}
	return result, nil
}

var _ io.Writer = (*capture)(nil)
