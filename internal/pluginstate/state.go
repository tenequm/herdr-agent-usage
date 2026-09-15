// Package pluginstate resolves and writes plugin-owned runtime state.
package pluginstate

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const legacyDirName = "herdr-usagebar"

// LegacyDirName exposes the historical child name without duplicating it in
// provider adapters.
func LegacyDirName() string { return legacyDirName }

var configured struct {
	sync.RWMutex
	root string
}

// Configure sets the process-wide state root. It returns a restoration
// function for tests and scoped setup reporting. Empty selects legacy paths.
func Configure(root string) func() {
	configured.Lock()
	previous := configured.root
	if root != "" {
		root = filepath.Clean(root)
	}
	configured.root = root
	configured.Unlock()
	return func() {
		configured.Lock()
		configured.root = previous
		configured.Unlock()
	}
}

// Root returns the configured state root, or empty in legacy mode.
func Root() string {
	configured.RLock()
	defer configured.RUnlock()
	return configured.root
}

// GlobalDir returns the directory for non-profile state.
func GlobalDir(home string) string {
	if root := Root(); root != "" {
		return root
	}
	return filepath.Join(home, ".claude", legacyDirName)
}

// FamilyDir returns one provider family's state directory. Provider adapters
// supply both their canonical family id and historical directory.
func FamilyDir(familyID, legacyDir string) string {
	if root := Root(); root != "" {
		return filepath.Join(root, familyID)
	}
	return legacyDir
}

// ProfileDir returns one provider profile's state directory.
func ProfileDir(familyID, profileID, legacyDir string) string {
	if Root() != "" {
		return filepath.Join(FamilyDir(familyID, legacyDir), profileID)
	}
	return legacyDir
}

// UpdateCheckDir keeps the historical config-directory location unless a
// state root is configured.
func UpdateCheckDir(legacyDir string) string {
	if root := Root(); root != "" {
		return root
	}
	return legacyDir
}

// SecurePath reports whether path is beneath the configured root.
func SecurePath(path string) bool {
	root := Root()
	if root == "" || path == "" {
		return false
	}
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// DirMode preserves legacy directory permissions outside the configured root
// while enforcing private state beneath it.
func DirMode(path string) os.FileMode {
	if SecurePath(path) {
		return 0o700
	}
	return 0o755
}

// FileMode returns 0600 beneath the configured root and legacyMode elsewhere.
// The explicit legacy mode is load-bearing because historical call sites used
// both 0644 and 0600.
func FileMode(path string, legacyMode os.FileMode) os.FileMode {
	if SecurePath(path) {
		return 0o600
	}
	return legacyMode
}

// EnsureDir creates state directories without changing permissions on paths
// that already existed. Configured-root symlinks are rejected so a child can
// never escape the state boundary.
func EnsureDir(path string) error {
	if !SecurePath(path) {
		return os.MkdirAll(path, DirMode(path))
	}
	root := Root()
	if info, err := os.Lstat(root); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return &os.PathError{Op: "mkdir", Path: root, Err: os.ErrInvalid}
		}
	} else if os.IsNotExist(err) {
		if err := os.MkdirAll(root, 0o700); err != nil {
			return err
		}
	} else {
		return err
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return err
	}
	current := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		switch {
		case statErr == nil && info.Mode()&os.ModeSymlink != 0:
			return &os.PathError{Op: "mkdir", Path: current, Err: os.ErrInvalid}
		case statErr == nil && !info.IsDir():
			return &os.PathError{Op: "mkdir", Path: current, Err: os.ErrInvalid}
		case statErr == nil:
			continue
		case os.IsNotExist(statErr):
			if err := os.Mkdir(current, 0o700); err != nil && !os.IsExist(err) {
				return err
			}
		default:
			return statErr
		}
	}
	return nil
}

func createTempFile(dir string, mode os.FileMode) (*os.File, error) {
	for range 100 {
		var suffix [8]byte
		if _, err := rand.Read(suffix[:]); err != nil {
			return nil, err
		}
		name := filepath.Join(dir, ".usagebar-"+hex.EncodeToString(suffix[:])+".tmp")
		file, err := os.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
		if os.IsExist(err) {
			continue
		}
		return file, err
	}
	return nil, os.ErrExist
}

// AtomicWrite writes data through a temporary file in the destination
// directory and renames it into place. New legacy files respect the process
// umask; replacements retain the destination mode; configured state is 0600.
func AtomicWrite(path string, data []byte, legacyMode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := EnsureDir(dir); err != nil {
		return err
	}
	mode := FileMode(path, legacyMode)
	preserveMode := SecurePath(path)
	if info, err := os.Stat(path); err == nil {
		if !SecurePath(path) {
			mode = info.Mode().Perm()
		}
		preserveMode = true
	}
	temp, err := createTempFile(dir, mode)
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer func() { _ = os.Remove(tempName) }()
	if preserveMode {
		if err := temp.Chmod(mode); err != nil {
			_ = temp.Close()
			return err
		}
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempName, path)
}
