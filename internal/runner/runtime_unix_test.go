//go:build unix

package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRunPlanningTaskExplicitDatabasePathLeavesExistingParentMode(t *testing.T) {
	t.Parallel()

	databaseDir := filepath.Join(t.TempDir(), "shared")
	if err := os.MkdirAll(databaseDir, 0o777); err != nil {
		t.Fatalf("MkdirAll(): %v", err)
	}
	if err := os.Chmod(databaseDir, 0o777); err != nil {
		t.Fatalf("Chmod(broad): %v", err)
	}

	result, err := RunPlanningTask(context.Background(), Options{
		DatabasePath: filepath.Join(databaseDir, "openplanner.db"),
	}, PlanningTaskRequest{
		Action:       PlanningTaskActionEnsureCalendar,
		CalendarName: "Personal",
	})
	if err != nil {
		t.Fatalf("RunPlanningTask(): %v", err)
	}
	if result.Rejected {
		t.Fatalf("result rejected: %s", result.RejectionReason)
	}

	assertMode(t, databaseDir, 0o777)
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
