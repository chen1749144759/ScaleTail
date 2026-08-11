package unixpkgs

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestNormalizedPackageScriptUsesUnixLineEndings(t *testing.T) {
	source := filepath.Join(t.TempDir(), "postinstall.sh")
	if err := os.WriteFile(source, []byte("#!/bin/sh\r\necho ready\r\n"), 0600); err != nil {
		t.Fatal(err)
	}

	gotPath, err := normalizedPackageScript(t.TempDir(), source)
	if err != nil {
		t.Fatalf("normalizing package script: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(gotPath) })
	got, err := os.ReadFile(gotPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(got, []byte{'\r'}) {
		t.Fatalf("normalized script still contains carriage returns: %q", got)
	}
	if want := []byte("#!/bin/sh\necho ready\n"); !bytes.Equal(got, want) {
		t.Fatalf("normalized script = %q, want %q", got, want)
	}
	info, err := os.Stat(gotPath)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0755 {
		t.Fatalf("normalized script mode = %o, want 0755", info.Mode().Perm())
	}
}
