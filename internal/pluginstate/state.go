// Package pluginstate resolves and writes plugin-owned runtime state.
package pluginstate

import (
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

// ProfileDir returns one provider profile's state directory.
func ProfileDir(familyID, profileID, legacyDir string) string {
	if root := Root(); root != "" {
		return filepath.Join(root, familyID, profileID)
	}
	return legacyDir
}

// CursorDir returns the Cursor-specific state directory.
func CursorDir(home string) string {
	if root := Root(); root != "" {
		return filepath.Join(root, "cursor")
	}
	return filepath.Join(home, ".cursor", legacyDirName)
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

// DirMode and FileMode preserve legacy permissions outside the configured
// root while enforcing private state beneath it.
func DirMode(path string) os.FileMode {
	if SecurePath(path) {
		return 0o700
	}
	return 0o755
}

func FileMode(path string) os.FileMode {
	if SecurePath(path) {
		return 0o600
	}
	return 0o644
}

// EnsureDir creates a state directory with the mode required for its path.
func EnsureDir(path string) error {
	mode := DirMode(path)
	if err := os.MkdirAll(path, mode); err != nil {
		return err
	}
	if root := Root(); root != "" && SecurePath(path) {
		// MkdirAll does not tighten already-existing ancestors. Walk back to the
		// configured root so a pre-created 0755 root cannot expose child names.
		for current := filepath.Clean(path); ; current = filepath.Dir(current) {
			if err := os.Chmod(current, mode); err != nil {
				return err
			}
			if current == root {
				break
			}
		}
	}
	return nil
}

// AtomicWrite writes data through a temporary file in the destination
// directory and renames it into place.
func AtomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := EnsureDir(dir); err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, ".usagebar-*.tmp")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer func() { _ = os.Remove(tempName) }()
	if err := temp.Chmod(FileMode(path)); err != nil {
		_ = temp.Close()
		return err
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
