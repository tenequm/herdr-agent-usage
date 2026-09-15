package pluginstate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfiguredPathsAndPrivateAtomicWrite(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	restore := Configure(root)
	defer restore()

	if got := GlobalDir("/home/test"); got != root {
		t.Fatalf("global dir = %q", got)
	}
	wantProfile := filepath.Join(root, "claude", "work")
	if got := ProfileDir("claude", "work", "/legacy"); got != wantProfile {
		t.Fatalf("profile dir = %q", got)
	}
	path := filepath.Join(wantProfile, "state.json")
	if err := AtomicWrite(path, []byte("first")); err != nil {
		t.Fatal(err)
	}
	if err := AtomicWrite(path, []byte("second")); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != "second" {
		t.Fatalf("read = %q, %v", raw, err)
	}
	for _, item := range []struct {
		path string
		mode os.FileMode
	}{{root, 0o700}, {filepath.Join(root, "claude"), 0o700}, {wantProfile, 0o700}, {path, 0o600}} {
		info, err := os.Stat(item.path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != item.mode {
			t.Fatalf("%s mode = %o, want %o", item.path, info.Mode().Perm(), item.mode)
		}
	}
}

func TestAbsentRootPreservesLegacyPaths(t *testing.T) {
	restore := Configure("")
	defer restore()
	home := filepath.Join(string(filepath.Separator), "home", "test")
	if got := GlobalDir(home); got != filepath.Join(home, ".claude", legacyDirName) {
		t.Fatalf("global legacy dir = %q", got)
	}
	if got := ProfileDir("claude", "work", "/legacy/profile"); got != "/legacy/profile" {
		t.Fatalf("profile legacy dir = %q", got)
	}
	if got := CursorDir(home); got != filepath.Join(home, ".cursor", legacyDirName) {
		t.Fatalf("cursor legacy dir = %q", got)
	}
}
