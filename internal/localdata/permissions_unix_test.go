//go:build unix

package localdata

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestEnsurePrivateDirCorrectsBroadMode(t *testing.T) {
	directoryPath := filepath.Join(t.TempDir(), "openplanner")
	if err := os.MkdirAll(directoryPath, 0o777); err != nil {
		t.Fatalf("MkdirAll(): %v", err)
	}
	if err := os.Chmod(directoryPath, 0o777); err != nil {
		t.Fatalf("Chmod(broad): %v", err)
	}

	if err := EnsurePrivateDir(directoryPath); err != nil {
		t.Fatalf("EnsurePrivateDir(): %v", err)
	}

	assertMode(t, directoryPath, 0o700)
}

func TestEnsurePrivateDirIfMissingLeavesExistingMode(t *testing.T) {
	directoryPath := filepath.Join(t.TempDir(), "shared")
	if err := os.MkdirAll(directoryPath, 0o777); err != nil {
		t.Fatalf("MkdirAll(): %v", err)
	}
	if err := os.Chmod(directoryPath, 0o777); err != nil {
		t.Fatalf("Chmod(broad): %v", err)
	}

	if err := EnsurePrivateDirIfMissing(directoryPath); err != nil {
		t.Fatalf("EnsurePrivateDirIfMissing(): %v", err)
	}

	assertMode(t, directoryPath, 0o777)
}

func TestEnsurePrivateDirIfMissingCreatesPrivateDirDespitePermissiveUmask(t *testing.T) {
	oldUmask := syscall.Umask(0)
	t.Cleanup(func() {
		syscall.Umask(oldUmask)
	})

	directoryPath := filepath.Join(t.TempDir(), "new", "openplanner")
	if err := EnsurePrivateDirIfMissing(directoryPath); err != nil {
		t.Fatalf("EnsurePrivateDirIfMissing(): %v", err)
	}

	assertMode(t, directoryPath, 0o700)
}

func TestEnsurePrivateSQLiteFilesCreatesPrivateFileDespitePermissiveUmask(t *testing.T) {
	oldUmask := syscall.Umask(0)
	t.Cleanup(func() {
		syscall.Umask(oldUmask)
	})

	databasePath := filepath.Join(t.TempDir(), "openplanner.db")
	if err := EnsurePrivateSQLiteFiles(databasePath); err != nil {
		t.Fatalf("EnsurePrivateSQLiteFiles(): %v", err)
	}

	assertMode(t, databasePath, 0o600)
}

func TestEnsurePrivateSQLiteFilesCorrectsExistingBroadMode(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "openplanner.db")
	if err := os.WriteFile(databasePath, []byte("not sqlite yet"), 0o666); err != nil {
		t.Fatalf("WriteFile(): %v", err)
	}
	if err := os.Chmod(databasePath, 0o666); err != nil {
		t.Fatalf("Chmod(broad): %v", err)
	}

	if err := EnsurePrivateSQLiteFiles(databasePath); err != nil {
		t.Fatalf("EnsurePrivateSQLiteFiles(): %v", err)
	}

	assertMode(t, databasePath, 0o600)
}

func TestHardenSQLiteSidecarsCorrectsExistingBroadModes(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "openplanner.db")
	for _, suffix := range sqliteSidecarSuffixes {
		sidecarPath := databasePath + suffix
		if err := os.WriteFile(sidecarPath, []byte("sidecar"), 0o666); err != nil {
			t.Fatalf("WriteFile(%s): %v", suffix, err)
		}
		if err := os.Chmod(sidecarPath, 0o666); err != nil {
			t.Fatalf("Chmod(%s): %v", suffix, err)
		}
	}

	if err := HardenSQLiteSidecars(databasePath); err != nil {
		t.Fatalf("HardenSQLiteSidecars(): %v", err)
	}

	for _, suffix := range sqliteSidecarSuffixes {
		assertMode(t, databasePath+suffix, 0o600)
	}
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%s): %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s mode = %#o, want %#o", path, got, want)
	}
}
