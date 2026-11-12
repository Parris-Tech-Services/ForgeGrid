//go:build !windows

package coordinator

import "os"

// POSIX requires a file readable and writable only by its owner.
func securePrivateFile(path string) error {
	return os.Chmod(path, 0600)
}

func privateFileReadable(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0077 == 0
}
