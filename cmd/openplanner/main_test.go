package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/yazanabuashour/openplanner/internal/runner"
)

func TestRunVersion(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run([]string{"--version"}, strings.NewReader(""), &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit = %d, stderr = %s", exitCode, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); !strings.HasPrefix(got, "openplanner ") {
		t.Fatalf("--version output = %q, want openplanner prefix", got)
	}

	stdout.Reset()
	exitCode = run([]string{"version"}, strings.NewReader(""), &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit = %d, stderr = %s", exitCode, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); !strings.HasPrefix(got, "openplanner ") {
		t.Fatalf("version output = %q, want openplanner prefix", got)
	}
}

func TestRunHelp(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run([]string{"--help"}, strings.NewReader(""), &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit = %d, stderr = %s", exitCode, stderr.String())
	}
	for _, want := range []string{
		"openplanner --version",
		"openplanner planning",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("help output missing %q:\n%s", want, stdout.String())
		}
	}
	if strings.Contains(stdout.String(), "caldav") || strings.Contains(stdout.String(), "CalDAV") {
		t.Fatalf("help output mentions removed CalDAV command:\n%s", stdout.String())
	}
}

func TestRunUnknownSubcommand(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run([]string{"caldav"}, strings.NewReader(""), &stdout, &stderr)
	if exitCode != 2 {
		t.Fatalf("exit = %d, want 2", exitCode)
	}
	if !strings.Contains(stderr.String(), `unknown subcommand "caldav"`) {
		t.Fatalf("stderr = %q, want unknown subcommand error", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
}

func TestResolvedVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		linkerVersion string
		info          *debug.BuildInfo
		ok            bool
		want          string
	}{
		{
			name:          "linker version wins",
			linkerVersion: "v0.1.0",
			info:          &debug.BuildInfo{Main: debug.Module{Version: "v0.0.9"}},
			ok:            true,
			want:          "v0.1.0",
		},
		{
			name: "module version",
			info: &debug.BuildInfo{Main: debug.Module{Version: "v0.1.0"}},
			ok:   true,
			want: "v0.1.0",
		},
		{
			name: "development fallback",
			info: &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}},
			ok:   true,
			want: "dev",
		},
		{
			name: "missing build info fallback",
			want: "dev",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := resolvedVersion(tt.linkerVersion, tt.info, tt.ok); got != tt.want {
				t.Fatalf("resolvedVersion = %q, want %q", got, tt.want)
			}
		})
	}
}

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

func TestPlanningRunnerOversizedJSONExitsBeforeOpeningDatabase(t *testing.T) {
	t.Parallel()

	databasePath := filepath.Join(t.TempDir(), "runner.db")
	input := `{"action":"validate","content":"` + strings.Repeat("x", 4<<20) + `"}`
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := run([]string{"planning", "--db", databasePath}, bytes.NewBufferString(input), &stdout, &stderr)
	if exitCode == 0 {
		t.Fatal("exit = 0, want non-zero")
	}
	if !strings.Contains(stderr.String(), "planning request exceeds 4194304 bytes") {
		t.Fatalf("stderr = %q, want request size error", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if _, err := os.Stat(databasePath); !os.IsNotExist(err) {
		t.Fatalf("database path exists after oversized decode rejection: %v", err)
	}
}
