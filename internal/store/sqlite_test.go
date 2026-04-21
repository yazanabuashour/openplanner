package store

import (
	"database/sql"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/yazanabuashour/openplanner/internal/domain"
)

func TestOpenCreatesCurrentSchemaAndRecordsInitialMigration(t *testing.T) {
	t.Parallel()

	repository, err := Open(filepath.Join(t.TempDir(), "openplanner.db"))
	if err != nil {
		t.Fatalf("Open(): %v", err)
	}
	defer func() {
		if err := repository.Close(); err != nil {
			t.Fatalf("Close(): %v", err)
		}
	}()

	assertMigrationVersions(t, repository.db, []int{1, 2})
	assertSchemaObjects(t, repository.db, []string{
		"calendars",
		"events",
		"events_calendar_idx",
		"schema_migrations",
		"task_occurrence_states",
		"tasks",
		"tasks_calendar_idx",
		"tasks_calendar_status_priority_idx",
		"tasks_priority_idx",
		"tasks_status_idx",
	})
}

func TestOpenMigratesLegacyBootstrapDatabaseInPlace(t *testing.T) {
	t.Parallel()

	databasePath := filepath.Join(t.TempDir(), "openplanner.db")
	createLegacyBootstrapDatabase(t, databasePath)

	repository, err := Open(databasePath)
	if err != nil {
		t.Fatalf("Open(): %v", err)
	}
	defer func() {
		if err := repository.Close(); err != nil {
			t.Fatalf("Close(): %v", err)
		}
	}()

	assertMigrationVersions(t, repository.db, []int{1, 2})

	calendars, err := repository.ListCalendars()
	if err != nil {
		t.Fatalf("ListCalendars(): %v", err)
	}
	if len(calendars) != 1 || calendars[0].ID != "cal-legacy" || calendars[0].Name != "Legacy" {
		t.Fatalf("calendars = %#v, want legacy calendar", calendars)
	}

	events, err := repository.ListEvents("")
	if err != nil {
		t.Fatalf("ListEvents(): %v", err)
	}
	if len(events) != 1 || events[0].ID != "event-legacy" || events[0].Title != "Legacy event" {
		t.Fatalf("events = %#v, want legacy event", events)
	}

	tasks, err := repository.ListTasks(domain.TaskListParams{})
	if err != nil {
		t.Fatalf("ListTasks(): %v", err)
	}
	if len(tasks) != 1 || tasks[0].ID != "task-legacy" || tasks[0].Title != "Legacy task" {
		t.Fatalf("tasks = %#v, want legacy task", tasks)
	}
	if tasks[0].Priority != domain.TaskPriorityMedium || tasks[0].Status != domain.TaskStatusTodo || len(tasks[0].Tags) != 0 {
		t.Fatalf("legacy task metadata = priority:%q status:%q tags:%v, want medium/todo/[]", tasks[0].Priority, tasks[0].Status, tasks[0].Tags)
	}

	completions, err := repository.ListTaskCompletions([]string{"task-legacy"})
	if err != nil {
		t.Fatalf("ListTaskCompletions(): %v", err)
	}
	completion, ok := completions["task-legacy"]["date:2026-04-20"]
	if !ok {
		t.Fatalf("completions = %#v, want legacy task completion", completions)
	}
	if completion.CompletedAt.IsZero() || completion.OccurrenceDate == nil || *completion.OccurrenceDate != "2026-04-20" {
		t.Fatalf("completion = %#v, want completed legacy occurrence", completion)
	}
}

func TestOpenMigrationRunnerIsIdempotent(t *testing.T) {
	t.Parallel()

	databasePath := filepath.Join(t.TempDir(), "openplanner.db")
	createdAt := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	repository, err := Open(databasePath)
	if err != nil {
		t.Fatalf("Open(first): %v", err)
	}
	if err := repository.CreateCalendar(domain.Calendar{
		ID:        "cal-idempotent",
		Name:      "Idempotent",
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	}); err != nil {
		t.Fatalf("CreateCalendar(): %v", err)
	}
	if err := repository.Close(); err != nil {
		t.Fatalf("Close(first): %v", err)
	}

	reopened, err := Open(databasePath)
	if err != nil {
		t.Fatalf("Open(second): %v", err)
	}
	defer func() {
		if err := reopened.Close(); err != nil {
			t.Fatalf("Close(second): %v", err)
		}
	}()

	assertMigrationVersions(t, reopened.db, []int{1, 2})

	calendars, err := reopened.ListCalendars()
	if err != nil {
		t.Fatalf("ListCalendars(): %v", err)
	}
	if len(calendars) != 1 || calendars[0].ID != "cal-idempotent" {
		t.Fatalf("calendars = %#v, want preserved calendar after reopen", calendars)
	}
}

func TestOpenRejectsDatabaseWithNewerMigrationVersion(t *testing.T) {
	t.Parallel()

	databasePath := filepath.Join(t.TempDir(), "openplanner.db")
	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatalf("sql.Open(): %v", err)
	}
	if _, err := db.Exec(`
		CREATE TABLE schema_migrations (
			version INTEGER PRIMARY KEY
		);
		INSERT INTO schema_migrations (version) VALUES (3);
	`); err != nil {
		t.Fatalf("seed future schema version: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close(seed): %v", err)
	}

	repository, err := Open(databasePath)
	if err == nil {
		_ = repository.Close()
		t.Fatal("Open() error = nil, want newer schema version error")
	}
	if !strings.Contains(err.Error(), "database schema version 3 is newer than supported version 2") {
		t.Fatalf("Open() error = %v, want newer schema version error", err)
	}
}

func TestOpenBackfillsCompletedLegacyTaskStatus(t *testing.T) {
	t.Parallel()

	databasePath := filepath.Join(t.TempDir(), "openplanner.db")
	createLegacyCompletedTaskDatabase(t, databasePath)

	repository, err := Open(databasePath)
	if err != nil {
		t.Fatalf("Open(): %v", err)
	}
	defer func() {
		if err := repository.Close(); err != nil {
			t.Fatalf("Close(): %v", err)
		}
	}()

	tasks, err := repository.ListTasks(domain.TaskListParams{})
	if err != nil {
		t.Fatalf("ListTasks(): %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("tasks length = %d, want 1", len(tasks))
	}
	if tasks[0].Status != domain.TaskStatusDone || tasks[0].Priority != domain.TaskPriorityMedium {
		t.Fatalf("task metadata = priority:%q status:%q, want medium/done", tasks[0].Priority, tasks[0].Status)
	}
}

func createLegacyBootstrapDatabase(t *testing.T, databasePath string) {
	t.Helper()

	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatalf("sql.Open(): %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Fatalf("Close(seed): %v", err)
		}
	}()

	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY
		);

		CREATE TABLE IF NOT EXISTS calendars (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			description TEXT,
			color TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS events (
			id TEXT PRIMARY KEY,
			calendar_id TEXT NOT NULL REFERENCES calendars(id) ON DELETE RESTRICT,
			title TEXT NOT NULL,
			description TEXT,
			location TEXT,
			start_at TEXT,
			end_at TEXT,
			start_date TEXT,
			end_date TEXT,
			recurrence_json TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);

		CREATE INDEX IF NOT EXISTS events_calendar_idx ON events(calendar_id, created_at DESC, id DESC);

		CREATE TABLE IF NOT EXISTS tasks (
			id TEXT PRIMARY KEY,
			calendar_id TEXT NOT NULL REFERENCES calendars(id) ON DELETE RESTRICT,
			title TEXT NOT NULL,
			description TEXT,
			due_at TEXT,
			due_date TEXT,
			recurrence_json TEXT,
			completed_at TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);

		CREATE INDEX IF NOT EXISTS tasks_calendar_idx ON tasks(calendar_id, created_at DESC, id DESC);

		CREATE TABLE IF NOT EXISTS task_occurrence_states (
			task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
			occurrence_key TEXT NOT NULL,
			occurrence_at TEXT,
			occurrence_date TEXT,
			completed_at TEXT NOT NULL,
			PRIMARY KEY (task_id, occurrence_key)
		);

		INSERT INTO calendars (id, name, description, color, created_at, updated_at)
		VALUES ('cal-legacy', 'Legacy', 'Existing database', '#336699', '2026-04-20T10:00:00Z', '2026-04-20T10:00:00Z');

		INSERT INTO events (
			id, calendar_id, title, description, location, start_at, end_at,
			start_date, end_date, recurrence_json, created_at, updated_at
		)
		VALUES (
			'event-legacy', 'cal-legacy', 'Legacy event', 'Already stored', 'Office',
			'2026-04-20T15:00:00Z', '2026-04-20T16:00:00Z', NULL, NULL, NULL,
			'2026-04-20T10:01:00Z', '2026-04-20T10:01:00Z'
		);

		INSERT INTO tasks (
			id, calendar_id, title, description, due_at, due_date,
			recurrence_json, completed_at, created_at, updated_at
		)
		VALUES (
			'task-legacy', 'cal-legacy', 'Legacy task', 'Already stored',
			NULL, '2026-04-20', NULL, NULL, '2026-04-20T10:02:00Z', '2026-04-20T10:02:00Z'
		);

		INSERT INTO task_occurrence_states (
			task_id, occurrence_key, occurrence_at, occurrence_date, completed_at
		)
		VALUES (
			'task-legacy', 'date:2026-04-20', NULL, '2026-04-20', '2026-04-20T17:00:00Z'
		);
	`); err != nil {
		t.Fatalf("seed legacy database: %v", err)
	}
}

func createLegacyCompletedTaskDatabase(t *testing.T, databasePath string) {
	t.Helper()

	createLegacyBootstrapDatabase(t, databasePath)

	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatalf("sql.Open(): %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Fatalf("Close(seed): %v", err)
		}
	}()

	if _, err := db.Exec(`UPDATE tasks SET completed_at = '2026-04-20T18:00:00Z' WHERE id = 'task-legacy'`); err != nil {
		t.Fatalf("mark legacy task completed: %v", err)
	}
}

func assertMigrationVersions(t *testing.T, db *sql.DB, want []int) {
	t.Helper()

	rows, err := db.Query(`SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Fatalf("Close(rows): %v", err)
		}
	}()

	var got []int
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			t.Fatalf("scan migration version: %v", err)
		}
		got = append(got, version)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate migration versions: %v", err)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("migration versions = %v, want %v", got, want)
	}
}

func assertSchemaObjects(t *testing.T, db *sql.DB, want []string) {
	t.Helper()

	rows, err := db.Query(`
		SELECT name
		FROM sqlite_master
		WHERE type IN ('table', 'index') AND name NOT LIKE 'sqlite_%'
		ORDER BY name
	`)
	if err != nil {
		t.Fatalf("query schema objects: %v", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Fatalf("Close(rows): %v", err)
		}
	}()

	var got []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan schema object: %v", err)
		}
		got = append(got, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate schema objects: %v", err)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("schema objects = %v, want %v", got, want)
	}
}
