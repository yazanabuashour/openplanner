package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/yazanabuashour/openplanner/internal/runner"
)

func TestPlanningRunnerJSONRoundTripAndDBFlag(t *testing.T) {
	t.Parallel()

	databasePath := filepath.Join(t.TempDir(), "runner.db")
	input := `{"action":"ensure_calendar","calendar_name":"Personal"}`
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{"planning", "--db", databasePath}, bytes.NewBufferString(input), &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit = %d, stderr = %s", exitCode, stderr.String())
	}

	var result runner.PlanningTaskResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode result: %v\n%s", err, stdout.String())
	}
	if result.Rejected || len(result.Calendars) != 1 || result.Calendars[0].Name != "Personal" {
		t.Fatalf("result = %#v", result)
	}
	if _, err := os.Stat(databasePath); err != nil {
		t.Fatalf("database path was not created: %v", err)
	}
}

func TestPlanningRunnerUsesDatabaseEnvWhenDBFlagOmitted(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "env-runner.db")
	t.Setenv("OPENPLANNER_DATABASE_PATH", databasePath)
	input := `{"action":"ensure_calendar","calendar_name":"Personal"}`
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{"planning"}, bytes.NewBufferString(input), &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit = %d, stderr = %s", exitCode, stderr.String())
	}
	if _, err := os.Stat(databasePath); err != nil {
		t.Fatalf("database path from env was not created: %v", err)
	}
}

func TestPlanningRunnerDBFlagOverridesDatabaseEnv(t *testing.T) {
	envPath := filepath.Join(t.TempDir(), "env-runner.db")
	flagPath := filepath.Join(t.TempDir(), "flag-runner.db")
	t.Setenv("OPENPLANNER_DATABASE_PATH", envPath)
	input := `{"action":"ensure_calendar","calendar_name":"Personal"}`
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{"planning", "--db", flagPath}, bytes.NewBufferString(input), &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit = %d, stderr = %s", exitCode, stderr.String())
	}
	if _, err := os.Stat(flagPath); err != nil {
		t.Fatalf("database path from flag was not created: %v", err)
	}
	if _, err := os.Stat(envPath); !os.IsNotExist(err) {
		t.Fatalf("database path from env exists despite --db override: %v", err)
	}
}

func TestPlanningRunnerValidationRejectionIsJSON(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run([]string{"planning", "--db", filepath.Join(t.TempDir(), "runner.db")},
		bytes.NewBufferString(`{"action":"unknown"}`), &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit = %d, stderr = %s", exitCode, stderr.String())
	}

	var result runner.PlanningTaskResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if !result.Rejected || result.RejectionReason == "" {
		t.Fatalf("result = %#v, want JSON rejection", result)
	}
}

func TestPlanningRunnerBadJSONExitsNonZero(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run([]string{"planning"}, bytes.NewBufferString(`{`), &stdout, &stderr)
	if exitCode == 0 {
		t.Fatal("exit = 0, want non-zero")
	}
	if stderr.Len() == 0 {
		t.Fatal("stderr is empty")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
}
