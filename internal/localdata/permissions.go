package localdata

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

const (
	privateDirMode  os.FileMode = 0o700
	privateFileMode os.FileMode = 0o600
)

var sqliteSidecarSuffixes = []string{"-journal", "-wal", "-shm"}

// EnsurePrivateDir creates path and tightens it to owner-only access where the
// host filesystem supports POSIX-style mode bits.
func EnsurePrivateDir(path string) error {
	if err := os.MkdirAll(path, privateDirMode); err != nil {
		return err
	}
	return chmodPrivate(path, privateDirMode)
}

// EnsurePrivateDirIfMissing creates path with owner-only access when it does
// not exist, but does not change permissions on an existing directory.
func EnsurePrivateDirIfMissing(path string) error {
	info, err := os.Stat(path)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("%s is not a directory", path)
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(path, privateDirMode); err != nil {
		return err
	}
	return chmodPrivate(path, privateDirMode)
}

// EnsurePrivateSQLiteFiles pre-creates or corrects the main database file.
func EnsurePrivateSQLiteFiles(databasePath string) error {
	file, err := os.OpenFile(databasePath, os.O_RDWR|os.O_CREATE, privateFileMode)
	if err != nil {
		return fmt.Errorf("prepare sqlite database file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close sqlite database file: %w", err)
	}
	if err := chmodPrivate(databasePath, privateFileMode); err != nil {
		return fmt.Errorf("set sqlite database permissions: %w", err)
	}
	return nil
}

// HardenSQLiteSidecars corrects SQLite sidecar files that exist at call time.
func HardenSQLiteSidecars(databasePath string) error {
	for _, suffix := range sqliteSidecarSuffixes {
		path := databasePath + suffix
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("stat sqlite sidecar %s: %w", filepath.Base(path), err)
		}
		if err := chmodPrivate(path, privateFileMode); err != nil {
			return fmt.Errorf("set sqlite sidecar permissions for %s: %w", filepath.Base(path), err)
		}
	}
	return nil
}

func chmodPrivate(path string, mode os.FileMode) error {
	if runtime.GOOS == "windows" {
		_ = os.Chmod(path, mode)
		return nil
	}
	return os.Chmod(path, mode)
}
