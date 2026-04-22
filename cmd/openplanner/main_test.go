package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yazanabuashour/openplanner/internal/caldav"
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

func TestPlanningRunnerDecodesNullPatchFields(t *testing.T) {
	t.Parallel()

	databasePath := filepath.Join(t.TempDir(), "runner.db")
	var createOut bytes.Buffer
	var createErr bytes.Buffer
	createCode := run([]string{"planning", "--db", databasePath},
		bytes.NewBufferString(`{"action":"create_task","calendar_name":"Work","title":"Review","due_date":"2026-04-16","recurrence":{"frequency":"daily","count":2}}`),
		&createOut, &createErr)
	if createCode != 0 {
		t.Fatalf("create exit = %d, stderr = %s", createCode, createErr.String())
	}

	var createResult runner.PlanningTaskResult
	if err := json.Unmarshal(createOut.Bytes(), &createResult); err != nil {
		t.Fatalf("decode create result: %v", err)
	}
	if len(createResult.Tasks) != 1 {
		t.Fatalf("create result = %#v", createResult)
	}

	input := `{"action":"update_task","task_id":"` + createResult.Tasks[0].ID + `","due_date":null,"due_at":"2026-04-16T11:00:00Z","recurrence":null}`
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run([]string{"planning", "--db", databasePath}, bytes.NewBufferString(input), &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("update exit = %d, stderr = %s", exitCode, stderr.String())
	}

	var result runner.PlanningTaskResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode update result: %v", err)
	}
	if result.Rejected || result.Tasks[0].DueDate != "" || result.Tasks[0].DueAt != "2026-04-16T11:00:00Z" || result.Tasks[0].Recurrence != nil {
		t.Fatalf("result = %#v, want due_date and recurrence cleared", result)
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

func TestCalDAVRunnerPassesDBFlagAndAddr(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "caldav.db")
	var captured caldav.Options
	oldServe := serveCalDAV
	serveCalDAV = func(_ context.Context, options caldav.Options) error {
		captured = options
		return nil
	}
	t.Cleanup(func() {
		serveCalDAV = oldServe
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run([]string{"caldav", "--db", databasePath, "--addr", "127.0.0.1:0"}, strings.NewReader(""), &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit = %d, stderr = %s", exitCode, stderr.String())
	}
	if captured.DatabasePath != databasePath || captured.Addr != "127.0.0.1:0" {
		t.Fatalf("captured options = %#v", captured)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "experimental CalDAV") {
		t.Fatalf("stderr = %q, want startup notice", stderr.String())
	}
}

func TestCalDAVRunnerUsesDatabaseEnvWhenDBFlagOmitted(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "env-caldav.db")
	t.Setenv("OPENPLANNER_DATABASE_PATH", databasePath)
	var captured caldav.Options
	oldServe := serveCalDAV
	serveCalDAV = func(_ context.Context, options caldav.Options) error {
		captured = options
		return nil
	}
	t.Cleanup(func() {
		serveCalDAV = oldServe
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run([]string{"caldav"}, strings.NewReader(""), &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit = %d, stderr = %s", exitCode, stderr.String())
	}
	if captured.DatabasePath != databasePath {
		t.Fatalf("database path = %q, want env path %q", captured.DatabasePath, databasePath)
	}
}

func TestCalDAVRunnerRejectsPositionalArguments(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run([]string{"caldav", "extra"}, strings.NewReader(""), &stdout, &stderr)
	if exitCode == 0 {
		t.Fatal("exit = 0, want non-zero")
	}
	if !strings.Contains(stderr.String(), "does not accept positional") {
		t.Fatalf("stderr = %q, want positional argument error", stderr.String())
	}
}
