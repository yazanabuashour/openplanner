package sdk

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenLocalUsesDirectLocalRuntime(t *testing.T) {
	client, err := OpenLocal(Options{DatabasePath: filepath.Join(t.TempDir(), "openplanner.db")})
	if err != nil {
		t.Fatalf("OpenLocal(): %v", err)
	}
	defer func() {
		if closeErr := client.Close(); closeErr != nil {
			t.Fatalf("Close(): %v", closeErr)
		}
	}()

	if client.service == nil {
		t.Fatal("service = nil")
	}
}

func TestDefaultDataDirUsesXDGDataHome(t *testing.T) {
	xdgDataHome := filepath.Join(t.TempDir(), "xdg-data")
	t.Setenv("XDG_DATA_HOME", xdgDataHome)
	t.Setenv("HOME", filepath.Join(t.TempDir(), "home"))

	dataDir, err := DefaultDataDir()
	if err != nil {
		t.Fatalf("DefaultDataDir(): %v", err)
	}

	want := filepath.Join(xdgDataHome, "openplanner")
	if dataDir != want {
		t.Fatalf("DefaultDataDir() = %q, want %q", dataDir, want)
	}
}

func TestDefaultDataDirFallsBackToHomeLocalShare(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("HOME", home)

	dataDir, err := DefaultDataDir()
	if err != nil {
		t.Fatalf("DefaultDataDir(): %v", err)
	}

	want := filepath.Join(home, ".local", "share", "openplanner")
	if dataDir != want {
		t.Fatalf("DefaultDataDir() = %q, want %q", dataDir, want)
	}
}

func TestOpenLocalUsesDefaultDatabasePath(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("HOME", home)

	databasePath, err := DefaultDatabasePath()
	if err != nil {
		t.Fatalf("DefaultDatabasePath(): %v", err)
	}

	client, err := OpenLocal(Options{})
	if err != nil {
		t.Fatalf("OpenLocal(): %v", err)
	}
	defer func() {
		if closeErr := client.Close(); closeErr != nil {
			t.Fatalf("Close(): %v", closeErr)
		}
	}()

	if _, err := os.Stat(databasePath); err != nil {
		t.Fatalf("default database path missing: %v", err)
	}
}
