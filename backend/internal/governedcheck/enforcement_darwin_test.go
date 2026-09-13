//go:build darwin

package governedcheck

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSeatbeltAllowsSystemPythonRuntimeWithoutWideningNetwork(t *testing.T) {
	enforcement := seatbelt{}
	profile := enforcement.profile()
	if !strings.Contains(profile, `(subpath (param "DEVELOPER_RUNTIME"))`) ||
		!strings.Contains(profile, `/Library/Developer/CommandLineTools`) {
		t.Fatalf("profile omits Apple developer runtime roots:\n%s", profile)
	}
	if !strings.Contains(profile, "(deny network*)") {
		t.Fatalf("profile widened network while allowing interpreter resources:\n%s", profile)
	}
	cmd, err := enforcement.Command(context.Background(), Request{
		Argv: []string{"python3", "-c", "print('governed-python-ok')"},
	}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("system Python could not load inside governed check sandbox: %v\n%s", err, output)
	}
	if strings.TrimSpace(string(output)) != "governed-python-ok" {
		t.Fatalf("system Python output = %q", output)
	}
}

func TestAllowedDeveloperRuntimeRejectsBroadOrUnrelatedRoots(t *testing.T) {
	for _, root := range []string{"/", "/usr", "/Applications/Other.app/Contents/Developer", "/tmp/Xcode_26.6.app/Contents/Developer"} {
		if allowedDeveloperRuntime(root) {
			t.Fatalf("allowedDeveloperRuntime(%q) = true, want false", root)
		}
	}
	for _, root := range []string{"/Library/Developer/CommandLineTools", "/Applications/Xcode.app/Contents/Developer", "/Applications/Xcode_26.6.app/Contents/Developer"} {
		if !allowedDeveloperRuntime(root) {
			t.Fatalf("allowedDeveloperRuntime(%q) = false, want true", root)
		}
	}
	for _, binary := range []string{"/usr/bin/python3", "/usr/local/bin/python3", "/tmp/Xcode_26.6.app/Contents/Developer/usr/bin/python3"} {
		if root, ok := developerRuntimeForBinary(binary); ok {
			t.Fatalf("developerRuntimeForBinary(%q) = (%q, true), want rejection", binary, root)
		}
	}
	for binary, want := range map[string]string{
		"/Library/Developer/CommandLineTools/usr/bin/python3":                                      "/Library/Developer/CommandLineTools",
		"/Applications/Xcode_26.6.app/Contents/Developer/Library/Frameworks/Python3.framework/bin": "/Applications/Xcode_26.6.app/Contents/Developer",
	} {
		if root, ok := developerRuntimeForBinary(binary); !ok || root != want {
			t.Fatalf("developerRuntimeForBinary(%q) = (%q, %v), want (%q, true)", binary, root, ok, want)
		}
	}
}

func TestSeatbeltPythonCannotReadOutsideWorkspace(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside-secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	cmd, err := (seatbelt{}).Command(context.Background(), Request{
		Argv: []string{"python3", "-c", "from pathlib import Path; import sys; print(Path(sys.argv[1]).read_text())", outside},
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("Python read outside workspace succeeded: %s", output)
	}
	if strings.Contains(string(output), "outside-secret") {
		t.Fatalf("Python exposed outside-workspace content: %s", output)
	}
}
