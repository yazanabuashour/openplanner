//go:build unix

package store

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestOpenCreatesDatabaseWithPrivateFileModeDespitePermissiveUmask(t *testing.T) {
	oldUmask := syscall.Umask(0)
	t.Cleanup(func() {
		syscall.Umask(oldUmask)
	})

	databasePath := filepath.Join(t.TempDir(), "openplanner.db")
	repository, err := Open(databasePath)
	if err != nil {
		t.Fatalf("Open(): %v", err)
	}
	if err := repository.Close(); err != nil {
		t.Fatalf("Close(): %v", err)
	}

	assertFileMode(t, databasePath, 0o600)
}

func TestOpenCorrectsExistingDatabaseBroadFileMode(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "openplanner.db")
	if err := os.WriteFile(databasePath, nil, 0o666); err != nil {
		t.Fatalf("WriteFile(): %v", err)
	}
	if err := os.Chmod(databasePath, 0o666); err != nil {
		t.Fatalf("Chmod(broad): %v", err)
	}

	repository, err := Open(databasePath)
	if err != nil {
		t.Fatalf("Open(): %v", err)
	}
	if err := repository.Close(); err != nil {
		t.Fatalf("Close(): %v", err)
	}

	assertFileMode(t, databasePath, 0o600)
}

func assertFileMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%s): %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s mode = %#o, want %#o", path, got, want)
	}
}
