//go:build !windows

package codexappserver

import (
	"bufio"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// A dedicated process group makes the app-server and every command it launches
// one owned cancellation unit. Killing only the app-server can orphan the shell
// whose effects Stop is meant to halt.
func configureAppServerProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killAppServerProcessTree terminates the app-server and every process it
// spawned, including one a sandbox or job-control layer moved into its own
// process group or session. A single killpg(-pid) is not enough: live Mac
// evidence showed a sandboxed command's process outliving it, still writing
// to disk seconds after the group kill returned success.
//
// The descendant set is captured by walking the live process table BEFORE
// any signal is sent. Killing the root first and only then asking "who are
// its descendants" is unreliable: once a parent dies its orphaned children
// are reparented (ppid changes), so a post-kill table walk can lose exactly
// the descendant this exists to catch. Every pid in that pre-kill snapshot is
// then killed directly by pid, not only by process group, and quiescence is
// verified against that fixed set rather than re-derived from ppid links.
func killAppServerProcessTree(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	root := cmd.Process.Pid

	tree, err := processTable()
	if err != nil {
		return fmt.Errorf("read process table for pid %d: %w", root, err)
	}
	targets := descendants(tree, root)

	// Belt-and-suspenders group kill first, for the common case where nothing
	// detached; then every captured pid individually.
	_ = syscall.Kill(-root, syscall.SIGKILL)
	for _, pid := range targets {
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		tree, err := processTable()
		if err != nil {
			return fmt.Errorf("read process table while quiescing pid %d: %w", root, err)
		}
		survivors := stillRunning(tree, targets)
		if len(survivors) == 0 {
			return nil
		}
		for _, pid := range survivors {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("process tree for pid %d did not fully quiesce, still running: %v", root, survivors)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

type procInfo struct {
	ppid   int
	zombie bool
}

// descendants returns root and every pid reachable from it by parent links in
// tree, in the shape observed at the moment tree was captured.
func descendants(tree map[int]procInfo, root int) []int {
	byParent := map[int][]int{}
	for pid, info := range tree {
		byParent[info.ppid] = append(byParent[info.ppid], pid)
	}
	var out []int
	queue := []int{root}
	seen := map[int]bool{}
	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		if seen[pid] {
			continue
		}
		seen[pid] = true
		out = append(out, pid)
		queue = append(queue, byParent[pid]...)
	}
	return out
}

// stillRunning reports which of pids are present in tree and not a zombie. A
// zombie is already dead and can produce no further effect; it only awaits
// reaping, which is a resource-cleanup concern, not a quiescence one. kill(pid,
// 0) cannot make this distinction on its own: it still succeeds for a zombie.
func stillRunning(tree map[int]procInfo, pids []int) []int {
	var running []int
	for _, pid := range pids {
		if info, ok := tree[pid]; ok && !info.zombie {
			running = append(running, pid)
		}
	}
	return running
}

// processTable snapshots the system process table as pid -> {ppid, zombie}.
// ps, not /proc, so this works on Darwin as well as Linux.
func processTable() (map[int]procInfo, error) {
	out, err := exec.Command("ps", "-axo", "pid=,ppid=,stat=").Output()
	if err != nil {
		return nil, fmt.Errorf("ps: %w", err)
	}
	table := map[int]procInfo{}
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}
		pid, errPid := strconv.Atoi(fields[0])
		ppid, errPpid := strconv.Atoi(fields[1])
		if errPid != nil || errPpid != nil {
			continue
		}
		table[pid] = procInfo{ppid: ppid, zombie: strings.HasPrefix(fields[2], "Z")}
	}
	return table, scanner.Err()
}
