// Command kennel-codex-runtime-identity resolves the executable used by the
// native Codex proof. The default path comes from production discovery; an
// explicit test override is canonicalized with the same Unix symlink rule.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	codexagent "github.com/Pin4sf/Waldo-Kennel/backend/internal/adapters/agent/codex"
)

type identity struct {
	SelectedPath  string `json:"selected_path"`
	CanonicalPath string `json:"canonical_path"`
}

func main() {
	selected := ""
	canonical := ""
	var err error
	if explicit := os.Getenv("KENNEL_CODEX_BIN"); explicit != "" {
		selected, err = exec.LookPath(explicit)
		if err == nil {
			canonical = selected
			if runtime.GOOS != "windows" {
				canonical, err = filepath.EvalSymlinks(selected)
			}
		}
	} else {
		selected, _ = exec.LookPath("codex")
		canonical, err = codexagent.ResolveCodexBinary(context.Background())
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if selected == "" {
		selected = canonical
	}
	if err := json.NewEncoder(os.Stdout).Encode(identity{SelectedPath: selected, CanonicalPath: canonical}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
