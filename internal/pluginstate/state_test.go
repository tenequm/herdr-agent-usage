package pluginstate

import (
	"os"
	"path/filepath"
	"syscall"
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
	if err := AtomicWrite(path, []byte("first"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := AtomicWrite(path, []byte("second"), 0o644); err != nil {
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
	legacyCursor := filepath.Join(home, ".cursor", legacyDirName)
	if got := FamilyDir("cursor", legacyCursor); got != legacyCursor {
		t.Fatalf("cursor legacy dir = %q", got)
	}
}

func TestAtomicWriteLegacyRespectsUmaskAndExistingMode(t *testing.T) {
	restore := Configure("")
	defer restore()
	dir := t.TempDir()
	oldUmask := syscall.Umask(0o077)
	defer syscall.Umask(oldUmask)

	path := filepath.Join(dir, "new.json")
	if err := AtomicWrite(path, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("new mode = %v, %v", info, err)
	}
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := AtomicWrite(path, []byte("replacement"), 0o644); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("replacement mode = %v, %v", info, err)
	}
}

func TestEnsureDirTightensOwnedRootAndLeavesExistingChildModes(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "shared")
	if err := os.Mkdir(root, 0o775); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o775); err != nil {
		t.Fatal(err)
	}
	restore := Configure(root)
	defer restore()
	existing := filepath.Join(root, "existing")
	if err := os.Mkdir(existing, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(existing, 0o755); err != nil {
		t.Fatal(err)
	}
	created := filepath.Join(existing, "created")
	if err := EnsureDir(created); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(root); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("root mode = %v, %v", info, err)
	}
	if info, err := os.Stat(existing); err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("existing child mode = %v, %v", info, err)
	}
	if info, err := os.Stat(created); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("created mode = %v, %v", info, err)
	}
	target := filepath.Join(base, "outside")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if err := EnsureDir(filepath.Join(root, "link", "escape")); err == nil {
		t.Fatal("symlinked state child was followed")
	}
}

func TestAtomicWriteFollowsConfiguredRootSymlink(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "target")
	if err := os.Mkdir(target, 0o775); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, 0o775); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(base, "state")
	if err := os.Symlink(target, root); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	restore := Configure(root)
	defer restore()

	path := filepath.Join(root, "claude", "base", "x.json")
	if err := AtomicWrite(path, []byte("state"), 0o644); err != nil {
		t.Fatal(err)
	}
	if raw, err := os.ReadFile(path); err != nil || string(raw) != "state" {
		t.Fatalf("read = %q, %v", raw, err)
	}
	if info, err := os.Stat(target); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("target mode = %v, %v", info, err)
	}

	outside := filepath.Join(base, "outside")
	if err := os.Mkdir(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(target, "link")); err != nil {
		t.Fatal(err)
	}
	if err := EnsureDir(filepath.Join(root, "link", "escape")); err == nil {
		t.Fatal("symlinked state child was followed")
	}
}
