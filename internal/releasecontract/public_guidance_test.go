package releasecontract_test

import (
	"os"
	"strings"
	"testing"
)

func TestPublicGuidanceUsesJSONRunnerProductSurface(t *testing.T) {
	t.Parallel()

	files := []string{
		"../../README.md",
		"../../CONTRIBUTING.md",
		"../../docs/agent-evals.md",
		"../../docs/agent-eval-results/README.md",
		"../../docs/maintainers.md",
		"../../docs/release-verification.md",
		"../../skills/openplanner/SKILL.md",
	}
	forbidden := []string{
		"openplanner-agentops",
		"go run ./cmd/openplanner",
		"openapi/openapi.yaml",
		"OpenAPI contract is the",
		"public SDK is the product surface",
		"REST API is the product surface",
		"hosted service is the product surface",
		"web UI is the product surface",
		"ships a public SDK",
		"ships a REST API",
		"ships a hosted service",
		"ships a web UI",
	}

	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		for _, bad := range forbidden {
			if strings.Contains(string(content), bad) {
				t.Fatalf("%s contains stale guidance %q", file, bad)
			}
		}
	}
}

func TestProductSurfaceGuidanceIsPresent(t *testing.T) {
	t.Parallel()

	required := map[string][]string{
		"../../README.md": {
			"## Product Surface",
			"installed `openplanner planning` JSON runner",
			"portable `skills/openplanner` payload",
			"not public extension points",
			"not v1 product deliverables",
		},
		"../../CONTRIBUTING.md": {
			"supported product surface is the installed `openplanner` JSON runner",
			"Agent Skills-compatible `skills/openplanner` payload",
			"No compatibility promise is made for a public SDK, REST API, hosted",
		},
		"../../docs/maintainers.md": {
			"library-first research direction is superseded for v1",
			"JSON-runner-first direction",
			"`op-2vv`",
			"installed `openplanner planning` JSON runner",
		},
		"../../docs/agent-evals.md": {
			"OpenHealth AgentOps runner pattern",
			"installed `openplanner planning` JSON runner",
			"The production skill remains the only model-visible",
			"does not generate an OpenPlanner-specific eval",
			"`codex debug prompt-input`",
			"CLI comparison gates",
			"`n/a` unless a separate baseline is approved",
		},
		"../../docs/agent-eval-results/README.md": {
			"production JSON runner",
			"`<run-root>` placeholders",
			"repo-relative artifact paths",
			"OpenHealth AgentOps runner pattern",
		},
	}

	for file, phrases := range required {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		text := string(content)
		for _, phrase := range phrases {
			if !strings.Contains(text, phrase) {
				t.Fatalf("%s missing product-surface guidance %q", file, phrase)
			}
		}
	}
}

func TestLocalDataBackupGuidanceIsPresent(t *testing.T) {
	t.Parallel()

	required := map[string][]string{
		"../../README.md": {
			"docs/local-data-backup.md",
			"backup, restore, and recovery verification guidance",
		},
		"../../docs/local-data-backup.md": {
			"# Local Data Backup And Recovery",
			"${XDG_DATA_HOME:-~/.local/share}/openplanner/openplanner.db",
			"openplanner planning --db <database-path>` wins over",
			"OPENPLANNER_DATABASE_PATH",
			"## File Permissions",
			"`0700`",
			"`0600`",
			"`-journal`, `-wal`, and `-shm`",
			"owner-only mode support",
			"Stop active OpenPlanner runner usage",
			"cp -p <database-path>",
			"Keep `<backup-dir>` private",
			"mv <database-path> <database-path>.before-restore",
			`printf '%s\n' '{"action":"validate"}' | openplanner planning --db <restored-db>`,
			`"action":"list_agenda"`,
			`"action":"list_events"`,
			`"action":"list_tasks"`,
			"Do not edit local OpenPlanner data through SQLite directly",
			"Use `export_icalendar` and `import_icalendar`",
			"database-file backup and restore",
		},
	}

	for file, phrases := range required {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		text := string(content)
		for _, phrase := range phrases {
			if !strings.Contains(text, phrase) {
				t.Fatalf("%s missing local-data backup guidance %q", file, phrase)
			}
		}
	}
}

func TestMaintainerAgentsFileDoesNotCarryProductTaskPolicy(t *testing.T) {
	t.Parallel()

	content, err := os.ReadFile("../../AGENTS.md")
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	text := string(content)
	for _, forbidden := range []string{
		"OpenPlanner User Data Requests",
		"openplanner planning",
		`"action"`,
		"calendar_name",
		"product data agent",
		"ambiguous short date",
		"RFC3339",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("AGENTS.md contains product task policy %q", forbidden)
		}
	}
	for _, required := range []string{
		"Beads Issue Tracker",
		"Session Completion",
		"Do work on the current branch",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("AGENTS.md missing maintainer guidance %q", required)
		}
	}
}

func TestOpenPlannerSkillCarriesProductTaskPolicy(t *testing.T) {
	t.Parallel()

	content, err := os.ReadFile("../../skills/openplanner/SKILL.md")
	if err != nil {
		t.Fatalf("read skill: %v", err)
	}
	text := string(content)
	for _, required := range []string{
		"openplanner planning",
		"calendar_name",
		`"action":"create_event"`,
		`"action":"delete_task"`,
		"ambiguous short date",
		"RFC3339",
		"Do not write local OpenPlanner data through SQLite directly",
		"Do not inspect\nsource files",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("SKILL.md missing product task policy %q", required)
		}
	}
}
