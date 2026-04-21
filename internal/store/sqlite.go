package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/yazanabuashour/openplanner/internal/domain"
)

var (
	ErrNotFound = errors.New("store: not found")
	ErrConflict = errors.New("store: conflict")
)

type Store struct {
	db *sql.DB
}

type migration struct {
	version int
	name    string
	sql     string
}

var migrations = []migration{
	{
		version: 1,
		name:    "initial schema",
		sql: `
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
		`,
	},
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}

	return store, nil
}

func (store *Store) Close() error {
	return store.db.Close()
}

func (store *Store) CreateCalendar(calendar domain.Calendar) error {
	_, err := store.db.Exec(`
		INSERT INTO calendars (id, name, description, color, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, calendar.ID, calendar.Name, nullableString(calendar.Description), nullableString(calendar.Color), formatTime(calendar.CreatedAt), formatTime(calendar.UpdatedAt))
	if err != nil {
		return mapWriteError(err)
	}

	return nil
}

func (store *Store) ListCalendars() ([]domain.Calendar, error) {
	rows, err := store.db.Query(`
		SELECT id, name, description, color, created_at, updated_at
		FROM calendars
		ORDER BY created_at DESC, id DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list calendars: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	return scanCalendars(rows)
}

func (store *Store) GetCalendar(id string) (domain.Calendar, error) {
	row := store.db.QueryRow(`
		SELECT id, name, description, color, created_at, updated_at
		FROM calendars
		WHERE id = ?
	`, id)

	calendar, err := scanCalendar(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Calendar{}, ErrNotFound
		}

		return domain.Calendar{}, fmt.Errorf("get calendar: %w", err)
	}

	return calendar, nil
}

func (store *Store) UpdateCalendar(calendar domain.Calendar) error {
	result, err := store.db.Exec(`
		UPDATE calendars
		SET name = ?, description = ?, color = ?, updated_at = ?
		WHERE id = ?
	`, calendar.Name, nullableString(calendar.Description), nullableString(calendar.Color), formatTime(calendar.UpdatedAt), calendar.ID)
	if err != nil {
		return mapWriteError(err)
	}

	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrNotFound
	}

	return nil
}

func (store *Store) DeleteCalendar(id string) error {
	var relatedCount int
	if err := store.db.QueryRow(`
		SELECT (
			(SELECT COUNT(*) FROM events WHERE calendar_id = ?) +
			(SELECT COUNT(*) FROM tasks WHERE calendar_id = ?)
		)
	`, id, id).Scan(&relatedCount); err != nil {
		return fmt.Errorf("count calendar refs: %w", err)
	}
	if relatedCount > 0 {
		return ErrConflict
	}

	result, err := store.db.Exec(`DELETE FROM calendars WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete calendar: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrNotFound
	}

	return nil
}

func (store *Store) CreateEvent(event domain.Event) error {
	recurrence, err := marshalRecurrence(event.Recurrence)
	if err != nil {
		return err
	}

	_, err = store.db.Exec(`
		INSERT INTO events (
			id, calendar_id, title, description, location,
			start_at, end_at, start_date, end_date, recurrence_json,
			created_at, updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, event.ID, event.CalendarID, event.Title, nullableString(event.Description), nullableString(event.Location),
		nullableTime(event.StartAt), nullableTime(event.EndAt), nullableString(event.StartDate), nullableString(event.EndDate),
		recurrence, formatTime(event.CreatedAt), formatTime(event.UpdatedAt))
	if err != nil {
		return mapWriteError(err)
	}

	return nil
}

func (store *Store) ListEvents(calendarID string) ([]domain.Event, error) {
	query := `
		SELECT id, calendar_id, title, description, location, start_at, end_at,
		       start_date, end_date, recurrence_json, created_at, updated_at
		FROM events
	`
	args := []any{}
	if calendarID != "" {
		query += ` WHERE calendar_id = ?`
		args = append(args, calendarID)
	}
	query += ` ORDER BY created_at DESC, id DESC`

	rows, err := store.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	return scanEvents(rows)
}

func (store *Store) GetEvent(id string) (domain.Event, error) {
	row := store.db.QueryRow(`
		SELECT id, calendar_id, title, description, location, start_at, end_at,
		       start_date, end_date, recurrence_json, created_at, updated_at
		FROM events
		WHERE id = ?
	`, id)

	event, err := scanEvent(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Event{}, ErrNotFound
		}

		return domain.Event{}, fmt.Errorf("get event: %w", err)
	}

	return event, nil
}

func (store *Store) UpdateEvent(event domain.Event) error {
	recurrence, err := marshalRecurrence(event.Recurrence)
	if err != nil {
		return err
	}

	result, err := store.db.Exec(`
		UPDATE events
		SET title = ?, description = ?, location = ?, start_at = ?, end_at = ?,
		    start_date = ?, end_date = ?, recurrence_json = ?, updated_at = ?
		WHERE id = ?
	`, event.Title, nullableString(event.Description), nullableString(event.Location),
		nullableTime(event.StartAt), nullableTime(event.EndAt), nullableString(event.StartDate), nullableString(event.EndDate),
		recurrence, formatTime(event.UpdatedAt), event.ID)
	if err != nil {
		return mapWriteError(err)
	}

	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrNotFound
	}

	return nil
}

func (store *Store) DeleteEvent(id string) error {
	result, err := store.db.Exec(`DELETE FROM events WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete event: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrNotFound
	}

	return nil
}

func (store *Store) CreateTask(task domain.Task) error {
	recurrence, err := marshalRecurrence(task.Recurrence)
	if err != nil {
		return err
	}

	_, err = store.db.Exec(`
		INSERT INTO tasks (
			id, calendar_id, title, description, due_at, due_date,
			recurrence_json, completed_at, created_at, updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, task.ID, task.CalendarID, task.Title, nullableString(task.Description), nullableTime(task.DueAt), nullableString(task.DueDate),
		recurrence, nullableTime(task.CompletedAt), formatTime(task.CreatedAt), formatTime(task.UpdatedAt))
	if err != nil {
		return mapWriteError(err)
	}

	return nil
}

func (store *Store) ListTasks(calendarID string) ([]domain.Task, error) {
	query := `
		SELECT id, calendar_id, title, description, due_at, due_date,
		       recurrence_json, completed_at, created_at, updated_at
		FROM tasks
	`
	args := []any{}
	if calendarID != "" {
		query += ` WHERE calendar_id = ?`
		args = append(args, calendarID)
	}
	query += ` ORDER BY created_at DESC, id DESC`

	rows, err := store.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	return scanTasks(rows)
}

func (store *Store) GetTask(id string) (domain.Task, error) {
	row := store.db.QueryRow(`
		SELECT id, calendar_id, title, description, due_at, due_date,
		       recurrence_json, completed_at, created_at, updated_at
		FROM tasks
		WHERE id = ?
	`, id)

	task, err := scanTask(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Task{}, ErrNotFound
		}

		return domain.Task{}, fmt.Errorf("get task: %w", err)
	}

	return task, nil
}

func (store *Store) UpdateTask(task domain.Task) error {
	recurrence, err := marshalRecurrence(task.Recurrence)
	if err != nil {
		return err
	}

	result, err := store.db.Exec(`
		UPDATE tasks
		SET title = ?, description = ?, due_at = ?, due_date = ?,
		    recurrence_json = ?, completed_at = ?, updated_at = ?
		WHERE id = ?
	`, task.Title, nullableString(task.Description), nullableTime(task.DueAt), nullableString(task.DueDate),
		recurrence, nullableTime(task.CompletedAt), formatTime(task.UpdatedAt), task.ID)
	if err != nil {
		return mapWriteError(err)
	}

	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrNotFound
	}

	return nil
}

func (store *Store) DeleteTask(id string) error {
	result, err := store.db.Exec(`DELETE FROM tasks WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete task: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrNotFound
	}

	return nil
}

func (store *Store) MarkTaskCompleted(taskID string, completedAt time.Time) error {
	result, err := store.db.Exec(`
		UPDATE tasks
		SET completed_at = ?, updated_at = ?
		WHERE id = ? AND completed_at IS NULL
	`, formatTime(completedAt), formatTime(completedAt), taskID)
	if err != nil {
		return fmt.Errorf("mark task completed: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrConflict
	}

	return nil
}

func (store *Store) CreateTaskCompletion(completion domain.TaskCompletion) error {
	_, err := store.db.Exec(`
		INSERT INTO task_occurrence_states (
			task_id, occurrence_key, occurrence_at, occurrence_date, completed_at
		)
		VALUES (?, ?, ?, ?, ?)
	`, completion.TaskID, completion.OccurrenceKey, nullableTime(completion.OccurrenceAt), nullableString(completion.OccurrenceDate), formatTime(completion.CompletedAt))
	if err != nil {
		return mapWriteError(err)
	}

	return nil
}

func (store *Store) ListTaskCompletions(taskIDs []string) (map[string]map[string]domain.TaskCompletion, error) {
	completions := make(map[string]map[string]domain.TaskCompletion, len(taskIDs))
	if len(taskIDs) == 0 {
		return completions, nil
	}

	placeholders := strings.TrimRight(strings.Repeat("?,", len(taskIDs)), ",")
	args := make([]any, 0, len(taskIDs))
	for _, taskID := range taskIDs {
		args = append(args, taskID)
	}

	rows, err := store.db.Query(`
		SELECT task_id, occurrence_key, occurrence_at, occurrence_date, completed_at
		FROM task_occurrence_states
		WHERE task_id IN (`+placeholders+`)
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("list task completions: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	for rows.Next() {
		var (
			taskID         string
			occurrenceKey  string
			occurrenceAt   sql.NullString
			occurrenceDate sql.NullString
			completedAt    string
		)
		if err := rows.Scan(&taskID, &occurrenceKey, &occurrenceAt, &occurrenceDate, &completedAt); err != nil {
			return nil, fmt.Errorf("scan task completion: %w", err)
		}

		if _, ok := completions[taskID]; !ok {
			completions[taskID] = map[string]domain.TaskCompletion{}
		}
		completions[taskID][occurrenceKey] = domain.TaskCompletion{
			TaskID:         taskID,
			OccurrenceKey:  occurrenceKey,
			OccurrenceAt:   parseNullableTime(occurrenceAt),
			OccurrenceDate: parseNullableString(occurrenceDate),
			CompletedAt:    parseStoredTime(completedAt),
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate task completions: %w", err)
	}

	return completions, nil
}

func (store *Store) migrate() error {
	if _, err := store.db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY
		);
	`); err != nil {
		return fmt.Errorf("prepare schema migrations: %w", err)
	}

	tx, err := store.db.Begin()
	if err != nil {
		return fmt.Errorf("begin schema migration: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	appliedVersions, err := readAppliedMigrationVersions(tx)
	if err != nil {
		return err
	}
	latestKnownVersion := latestMigrationVersion()
	for version := range appliedVersions {
		if version > latestKnownVersion {
			return fmt.Errorf("migrate schema: database schema version %d is newer than supported version %d", version, latestKnownVersion)
		}
	}

	for _, migration := range migrations {
		if appliedVersions[migration.version] {
			continue
		}

		if _, err := tx.Exec(migration.sql); err != nil {
			return fmt.Errorf("apply schema migration %d (%s): %w", migration.version, migration.name, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, migration.version); err != nil {
			return fmt.Errorf("record schema migration %d (%s): %w", migration.version, migration.name, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit schema migrations: %w", err)
	}

	return nil
}

func readAppliedMigrationVersions(tx *sql.Tx) (map[int]bool, error) {
	rows, err := tx.Query(`SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("read schema migrations: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	appliedVersions := map[int]bool{}
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("scan schema migration: %w", err)
		}
		appliedVersions[version] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate schema migrations: %w", err)
	}

	return appliedVersions, nil
}

func latestMigrationVersion() int {
	latest := 0
	for _, migration := range migrations {
		if migration.version > latest {
			latest = migration.version
		}
	}

	return latest
}

func scanCalendars(rows *sql.Rows) ([]domain.Calendar, error) {
	var calendars []domain.Calendar
	for rows.Next() {
		calendar, err := scanCalendar(rows)
		if err != nil {
			return nil, fmt.Errorf("scan calendar: %w", err)
		}
		calendars = append(calendars, calendar)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate calendars: %w", err)
	}

	return calendars, nil
}

func scanCalendar(scanner interface {
	Scan(dest ...any) error
}) (domain.Calendar, error) {
	var (
		calendar    domain.Calendar
		description sql.NullString
		color       sql.NullString
		createdAt   string
		updatedAt   string
	)
	if err := scanner.Scan(&calendar.ID, &calendar.Name, &description, &color, &createdAt, &updatedAt); err != nil {
		return domain.Calendar{}, err
	}

	calendar.Description = parseNullableString(description)
	calendar.Color = parseNullableString(color)
	calendar.CreatedAt = parseStoredTime(createdAt)
	calendar.UpdatedAt = parseStoredTime(updatedAt)

	return calendar, nil
}

func scanEvents(rows *sql.Rows) ([]domain.Event, error) {
	var events []domain.Event
	for rows.Next() {
		event, err := scanEvent(rows)
		if err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate events: %w", err)
	}

	return events, nil
}

func scanEvent(scanner interface {
	Scan(dest ...any) error
}) (domain.Event, error) {
	var (
		event          domain.Event
		description    sql.NullString
		location       sql.NullString
		startAt        sql.NullString
		endAt          sql.NullString
		startDate      sql.NullString
		endDate        sql.NullString
		recurrenceJSON sql.NullString
		createdAt      string
		updatedAt      string
	)
	if err := scanner.Scan(
		&event.ID, &event.CalendarID, &event.Title, &description, &location,
		&startAt, &endAt, &startDate, &endDate, &recurrenceJSON, &createdAt, &updatedAt,
	); err != nil {
		return domain.Event{}, err
	}

	recurrence, err := parseRecurrence(recurrenceJSON)
	if err != nil {
		return domain.Event{}, err
	}
	event.Description = parseNullableString(description)
	event.Location = parseNullableString(location)
	event.StartAt = parseNullableTime(startAt)
	event.EndAt = parseNullableTime(endAt)
	event.StartDate = parseNullableString(startDate)
	event.EndDate = parseNullableString(endDate)
	event.Recurrence = recurrence
	event.CreatedAt = parseStoredTime(createdAt)
	event.UpdatedAt = parseStoredTime(updatedAt)

	return event, nil
}

func scanTasks(rows *sql.Rows) ([]domain.Task, error) {
	var tasks []domain.Task
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tasks: %w", err)
	}

	return tasks, nil
}

func scanTask(scanner interface {
	Scan(dest ...any) error
}) (domain.Task, error) {
	var (
		task           domain.Task
		description    sql.NullString
		dueAt          sql.NullString
		dueDate        sql.NullString
		recurrenceJSON sql.NullString
		completedAt    sql.NullString
		createdAt      string
		updatedAt      string
	)
	if err := scanner.Scan(
		&task.ID, &task.CalendarID, &task.Title, &description, &dueAt, &dueDate,
		&recurrenceJSON, &completedAt, &createdAt, &updatedAt,
	); err != nil {
		return domain.Task{}, err
	}

	recurrence, err := parseRecurrence(recurrenceJSON)
	if err != nil {
		return domain.Task{}, err
	}
	task.Description = parseNullableString(description)
	task.DueAt = parseNullableTime(dueAt)
	task.DueDate = parseNullableString(dueDate)
	task.Recurrence = recurrence
	task.CompletedAt = parseNullableTime(completedAt)
	task.CreatedAt = parseStoredTime(createdAt)
	task.UpdatedAt = parseStoredTime(updatedAt)

	return task, nil
}

func marshalRecurrence(rule *domain.RecurrenceRule) (sql.NullString, error) {
	if rule == nil {
		return sql.NullString{}, nil
	}

	payload, err := json.Marshal(rule)
	if err != nil {
		return sql.NullString{}, fmt.Errorf("marshal recurrence: %w", err)
	}

	return sql.NullString{String: string(payload), Valid: true}, nil
}

func parseRecurrence(value sql.NullString) (*domain.RecurrenceRule, error) {
	if !value.Valid {
		return nil, nil
	}

	var rule domain.RecurrenceRule
	if err := json.Unmarshal([]byte(value.String), &rule); err != nil {
		return nil, fmt.Errorf("parse recurrence: %w", err)
	}

	return &rule, nil
}

func nullableString(value *string) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	if *value == "" {
		return sql.NullString{}
	}

	return sql.NullString{String: *value, Valid: true}
}

func nullableTime(value *time.Time) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}

	return sql.NullString{String: formatTime(*value), Valid: true}
}

func parseNullableString(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}

	result := value.String
	return &result
}

func parseNullableTime(value sql.NullString) *time.Time {
	if !value.Valid {
		return nil
	}

	parsed := parseStoredTime(value.String)
	return &parsed
}

func formatTime(value time.Time) string {
	return value.Format(time.RFC3339Nano)
}

func parseStoredTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}

	return parsed
}

func mapWriteError(err error) error {
	if strings.Contains(err.Error(), "UNIQUE constraint failed") {
		return ErrConflict
	}
	if strings.Contains(err.Error(), "FOREIGN KEY constraint failed") {
		return ErrNotFound
	}

	return fmt.Errorf("store write: %w", err)
}
