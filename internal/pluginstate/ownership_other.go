//go:build !aix && !android && !darwin && !dragonfly && !freebsd && !illumos && !ios && !linux && !netbsd && !openbsd && !solaris

package pluginstate

import "os"

func ownedByCurrentUser(os.FileInfo) bool {
	return false
}
