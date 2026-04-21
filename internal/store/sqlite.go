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
	{
		version: 2,
		name:    "task metadata",
		sql: `
			ALTER TABLE tasks ADD COLUMN priority TEXT NOT NULL DEFAULT 'medium';
			ALTER TABLE tasks ADD COLUMN status TEXT NOT NULL DEFAULT 'todo';
			ALTER TABLE tasks ADD COLUMN tags_json TEXT NOT NULL DEFAULT '[]';

			UPDATE tasks
			SET status = 'done'
			WHERE completed_at IS NOT NULL;

			CREATE INDEX IF NOT EXISTS tasks_status_idx ON tasks(status, created_at DESC, id DESC);
			CREATE INDEX IF NOT EXISTS tasks_priority_idx ON tasks(priority, created_at DESC, id DESC);
			CREATE INDEX IF NOT EXISTS tasks_calendar_status_priority_idx ON tasks(calendar_id, status, priority, created_at DESC, id DESC);
		`,
	},
	{
		version: 3,
		name:    "reminders",
		sql: `
			CREATE TABLE IF NOT EXISTS reminders (
				id TEXT PRIMARY KEY,
				owner_kind TEXT NOT NULL CHECK (owner_kind IN ('event', 'task')),
				owner_id TEXT NOT NULL,
				event_id TEXT REFERENCES events(id) ON DELETE CASCADE,
				task_id TEXT REFERENCES tasks(id) ON DELETE CASCADE,
				before_minutes INTEGER NOT NULL CHECK (before_minutes > 0),
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL,
				CHECK (
					(owner_kind = 'event' AND event_id = owner_id AND task_id IS NULL) OR
					(owner_kind = 'task' AND task_id = owner_id AND event_id IS NULL)
				),
				UNIQUE (owner_kind, owner_id, before_minutes)
			);

			CREATE INDEX IF NOT EXISTS reminders_owner_idx ON reminders(owner_kind, owner_id);

			CREATE TABLE IF NOT EXISTS reminder_dismissals (
				reminder_id TEXT NOT NULL REFERENCES reminders(id) ON DELETE CASCADE,
				occurrence_key TEXT NOT NULL,
				dismissed_at TEXT NOT NULL,
				PRIMARY KEY (reminder_id, occurrence_key)
			);
		`,
	},
	{
		version: 4,
		name:    "event task links",
		sql: `
			CREATE TABLE IF NOT EXISTS event_task_links (
				event_id TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
				task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL,
				PRIMARY KEY (event_id, task_id)
			);

			CREATE INDEX IF NOT EXISTS event_task_links_task_idx ON event_task_links(task_id, event_id);
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

	tx, err := store.db.Begin()
	if err != nil {
		return fmt.Errorf("begin create event: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	_, err = tx.Exec(`
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

	if err := insertReminders(tx, domain.ReminderOwnerKindEvent, event.ID, event.Reminders); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit create event: %w", err)
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

	events, err := scanEvents(rows)
	if err != nil {
		return nil, err
	}
	if err := store.loadEventReminders(events); err != nil {
		return nil, err
	}
	if err := store.loadEventLinkedTaskIDs(events); err != nil {
		return nil, err
	}
	return events, nil
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

	events := []domain.Event{event}
	if err := store.loadEventReminders(events); err != nil {
		return domain.Event{}, err
	}
	if err := store.loadEventLinkedTaskIDs(events); err != nil {
		return domain.Event{}, err
	}

	return events[0], nil
}

func (store *Store) UpdateEvent(event domain.Event) error {
	recurrence, err := marshalRecurrence(event.Recurrence)
	if err != nil {
		return err
	}

	tx, err := store.db.Begin()
	if err != nil {
		return fmt.Errorf("begin update event: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	result, err := tx.Exec(`
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

	if err := replaceRemindersIfChanged(tx, domain.ReminderOwnerKindEvent, event.ID, event.Reminders); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit update event: %w", err)
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
	tagsJSON, err := marshalTags(task.Tags)
	if err != nil {
		return err
	}

	tx, err := store.db.Begin()
	if err != nil {
		return fmt.Errorf("begin create task: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	_, err = tx.Exec(`
		INSERT INTO tasks (
			id, calendar_id, title, description, due_at, due_date,
			recurrence_json, priority, status, tags_json, completed_at, created_at, updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, task.ID, task.CalendarID, task.Title, nullableString(task.Description), nullableTime(task.DueAt), nullableString(task.DueDate),
		recurrence, task.Priority, task.Status, tagsJSON, nullableTime(task.CompletedAt), formatTime(task.CreatedAt), formatTime(task.UpdatedAt))
	if err != nil {
		return mapWriteError(err)
	}

	if err := insertReminders(tx, domain.ReminderOwnerKindTask, task.ID, task.Reminders); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit create task: %w", err)
	}

	return nil
}

func (store *Store) ListTasks(params domain.TaskListParams) ([]domain.Task, error) {
	query := `
		SELECT id, calendar_id, title, description, due_at, due_date,
		       recurrence_json, priority, status, tags_json, completed_at, created_at, updated_at
		FROM tasks
	`
	args := []any{}
	where := []string{}
	if params.CalendarID != "" {
		where = append(where, `calendar_id = ?`)
		args = append(args, params.CalendarID)
	}
	if params.Priority != "" {
		where = append(where, `priority = ?`)
		args = append(args, params.Priority)
	}
	if params.Status != "" {
		where = append(where, `status = ?`)
		args = append(args, params.Status)
	}
	if len(where) > 0 {
		query += ` WHERE ` + strings.Join(where, ` AND `)
	}
	query += ` ORDER BY created_at DESC, id DESC`

	rows, err := store.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	tasks, err := scanTasks(rows)
	if err != nil {
		return nil, err
	}
	if err := store.loadTaskReminders(tasks); err != nil {
		return nil, err
	}
	if err := store.loadTaskLinkedEventIDs(tasks); err != nil {
		return nil, err
	}
	if len(params.Tags) == 0 {
		return tasks, nil
	}

	filtered := make([]domain.Task, 0, len(tasks))
	for _, task := range tasks {
		if taskHasAllTags(task, params.Tags) {
			filtered = append(filtered, task)
		}
	}
	return filtered, nil
}

func (store *Store) GetTask(id string) (domain.Task, error) {
	row := store.db.QueryRow(`
		SELECT id, calendar_id, title, description, due_at, due_date,
		       recurrence_json, priority, status, tags_json, completed_at, created_at, updated_at
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

	tasks := []domain.Task{task}
	if err := store.loadTaskReminders(tasks); err != nil {
		return domain.Task{}, err
	}
	if err := store.loadTaskLinkedEventIDs(tasks); err != nil {
		return domain.Task{}, err
	}

	return tasks[0], nil
}

func (store *Store) UpdateTask(task domain.Task) error {
	recurrence, err := marshalRecurrence(task.Recurrence)
	if err != nil {
		return err
	}
	tagsJSON, err := marshalTags(task.Tags)
	if err != nil {
		return err
	}

	tx, err := store.db.Begin()
	if err != nil {
		return fmt.Errorf("begin update task: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	result, err := tx.Exec(`
		UPDATE tasks
		SET title = ?, description = ?, due_at = ?, due_date = ?,
		    recurrence_json = ?, priority = ?, status = ?, tags_json = ?, completed_at = ?, updated_at = ?
		WHERE id = ?
	`, task.Title, nullableString(task.Description), nullableTime(task.DueAt), nullableString(task.DueDate),
		recurrence, task.Priority, task.Status, tagsJSON, nullableTime(task.CompletedAt), formatTime(task.UpdatedAt), task.ID)
	if err != nil {
		return mapWriteError(err)
	}

	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrNotFound
	}

	if err := replaceRemindersIfChanged(tx, domain.ReminderOwnerKindTask, task.ID, task.Reminders); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit update task: %w", err)
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

func (store *Store) CreateEventTaskLink(link domain.EventTaskLink) error {
	_, err := store.db.Exec(`
		INSERT INTO event_task_links (event_id, task_id, created_at, updated_at)
		VALUES (?, ?, ?, ?)
	`, link.EventID, link.TaskID, formatTime(link.CreatedAt), formatTime(link.UpdatedAt))
	if err != nil {
		return mapWriteError(err)
	}

	return nil
}

func (store *Store) DeleteEventTaskLink(eventID string, taskID string) error {
	result, err := store.db.Exec(`
		DELETE FROM event_task_links
		WHERE event_id = ? AND task_id = ?
	`, eventID, taskID)
	if err != nil {
		return fmt.Errorf("delete event task link: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrNotFound
	}

	return nil
}

func (store *Store) ListEventTaskLinks(filter domain.EventTaskLinkFilter) ([]domain.EventTaskLink, error) {
	query := `
		SELECT event_id, task_id, created_at, updated_at
		FROM event_task_links
	`
	args := []any{}
	where := []string{}
	if filter.EventID != "" {
		where = append(where, `event_id = ?`)
		args = append(args, filter.EventID)
	}
	if filter.TaskID != "" {
		where = append(where, `task_id = ?`)
		args = append(args, filter.TaskID)
	}
	if len(where) > 0 {
		query += ` WHERE ` + strings.Join(where, ` AND `)
	}
	query += ` ORDER BY created_at DESC, event_id DESC, task_id DESC`

	rows, err := store.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list event task links: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	return scanEventTaskLinks(rows)
}

func (store *Store) MarkTaskCompleted(taskID string, completedAt time.Time) error {
	result, err := store.db.Exec(`
		UPDATE tasks
		SET completed_at = ?, status = 'done', updated_at = ?
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

func (store *Store) GetReminder(id string) (domain.ReminderRule, error) {
	row := store.db.QueryRow(`
		SELECT id, before_minutes, created_at, updated_at
		FROM reminders
		WHERE id = ?
	`, id)

	reminder, err := scanReminder(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ReminderRule{}, ErrNotFound
		}

		return domain.ReminderRule{}, fmt.Errorf("get reminder: %w", err)
	}

	return reminder, nil
}

func (store *Store) DismissReminderOccurrence(reminderID string, occurrenceKey string, dismissedAt time.Time) (bool, error) {
	result, err := store.db.Exec(`
		INSERT OR IGNORE INTO reminder_dismissals (reminder_id, occurrence_key, dismissed_at)
		VALUES (?, ?, ?)
	`, reminderID, occurrenceKey, formatTime(dismissedAt))
	if err != nil {
		return false, mapWriteError(err)
	}

	affected, _ := result.RowsAffected()
	return affected == 0, nil
}

func (store *Store) ListReminderDismissals(reminderIDs []string) (map[string]map[string]time.Time, error) {
	dismissals := make(map[string]map[string]time.Time, len(reminderIDs))
	if len(reminderIDs) == 0 {
		return dismissals, nil
	}

	placeholders := strings.TrimRight(strings.Repeat("?,", len(reminderIDs)), ",")
	args := make([]any, 0, len(reminderIDs))
	for _, reminderID := range reminderIDs {
		args = append(args, reminderID)
	}

	rows, err := store.db.Query(`
		SELECT reminder_id, occurrence_key, dismissed_at
		FROM reminder_dismissals
		WHERE reminder_id IN (`+placeholders+`)
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("list reminder dismissals: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	for rows.Next() {
		var reminderID string
		var occurrenceKey string
		var dismissedAt string
		if err := rows.Scan(&reminderID, &occurrenceKey, &dismissedAt); err != nil {
			return nil, fmt.Errorf("scan reminder dismissal: %w", err)
		}
		if _, ok := dismissals[reminderID]; !ok {
			dismissals[reminderID] = map[string]time.Time{}
		}
		dismissals[reminderID][occurrenceKey] = parseStoredTime(dismissedAt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate reminder dismissals: %w", err)
	}

	return dismissals, nil
}

func insertReminders(tx *sql.Tx, ownerKind domain.ReminderOwnerKind, ownerID string, reminders []domain.ReminderRule) error {
	for _, reminder := range reminders {
		eventID := sql.NullString{}
		taskID := sql.NullString{}
		switch ownerKind {
		case domain.ReminderOwnerKindEvent:
			eventID = sql.NullString{String: ownerID, Valid: true}
		case domain.ReminderOwnerKindTask:
			taskID = sql.NullString{String: ownerID, Valid: true}
		}
		if _, err := tx.Exec(`
			INSERT INTO reminders (
				id, owner_kind, owner_id, event_id, task_id, before_minutes, created_at, updated_at
			)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, reminder.ID, ownerKind, ownerID, eventID, taskID, reminder.BeforeMinutes, formatTime(reminder.CreatedAt), formatTime(reminder.UpdatedAt)); err != nil {
			return mapWriteError(err)
		}
	}

	return nil
}

func replaceRemindersIfChanged(tx *sql.Tx, ownerKind domain.ReminderOwnerKind, ownerID string, reminders []domain.ReminderRule) error {
	current, err := listOwnerRemindersTx(tx, ownerKind, ownerID)
	if err != nil {
		return err
	}
	if remindersEqual(current, reminders) {
		return nil
	}

	if _, err := tx.Exec(`
		DELETE FROM reminders
		WHERE owner_kind = ? AND owner_id = ?
	`, ownerKind, ownerID); err != nil {
		return fmt.Errorf("delete owner reminders: %w", err)
	}

	return insertReminders(tx, ownerKind, ownerID, reminders)
}

func listOwnerRemindersTx(tx *sql.Tx, ownerKind domain.ReminderOwnerKind, ownerID string) ([]domain.ReminderRule, error) {
	rows, err := tx.Query(`
		SELECT id, before_minutes, created_at, updated_at
		FROM reminders
		WHERE owner_kind = ? AND owner_id = ?
		ORDER BY before_minutes ASC, id ASC
	`, ownerKind, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list owner reminders: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	return scanReminders(rows)
}

func remindersEqual(left []domain.ReminderRule, right []domain.ReminderRule) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].ID != right[index].ID ||
			left[index].BeforeMinutes != right[index].BeforeMinutes ||
			!left[index].CreatedAt.Equal(right[index].CreatedAt) ||
			!left[index].UpdatedAt.Equal(right[index].UpdatedAt) {
			return false
		}
	}
	return true
}

func (store *Store) loadEventReminders(events []domain.Event) error {
	ownerIDs := make([]string, 0, len(events))
	for _, event := range events {
		ownerIDs = append(ownerIDs, event.ID)
	}
	byOwner, err := store.loadReminders(domain.ReminderOwnerKindEvent, ownerIDs)
	if err != nil {
		return err
	}
	for index := range events {
		events[index].Reminders = byOwner[events[index].ID]
	}
	return nil
}

func (store *Store) loadTaskReminders(tasks []domain.Task) error {
	ownerIDs := make([]string, 0, len(tasks))
	for _, task := range tasks {
		ownerIDs = append(ownerIDs, task.ID)
	}
	byOwner, err := store.loadReminders(domain.ReminderOwnerKindTask, ownerIDs)
	if err != nil {
		return err
	}
	for index := range tasks {
		tasks[index].Reminders = byOwner[tasks[index].ID]
	}
	return nil
}

func (store *Store) loadEventLinkedTaskIDs(events []domain.Event) error {
	eventIDs := make([]string, 0, len(events))
	for _, event := range events {
		eventIDs = append(eventIDs, event.ID)
	}
	byEvent, err := store.loadLinkedTaskIDs(eventIDs)
	if err != nil {
		return err
	}
	for index := range events {
		events[index].LinkedTaskIDs = byEvent[events[index].ID]
	}
	return nil
}

func (store *Store) loadTaskLinkedEventIDs(tasks []domain.Task) error {
	taskIDs := make([]string, 0, len(tasks))
	for _, task := range tasks {
		taskIDs = append(taskIDs, task.ID)
	}
	byTask, err := store.loadLinkedEventIDs(taskIDs)
	if err != nil {
		return err
	}
	for index := range tasks {
		tasks[index].LinkedEventIDs = byTask[tasks[index].ID]
	}
	return nil
}

func (store *Store) loadReminders(ownerKind domain.ReminderOwnerKind, ownerIDs []string) (map[string][]domain.ReminderRule, error) {
	byOwner := make(map[string][]domain.ReminderRule, len(ownerIDs))
	if len(ownerIDs) == 0 {
		return byOwner, nil
	}

	placeholders := strings.TrimRight(strings.Repeat("?,", len(ownerIDs)), ",")
	args := make([]any, 0, len(ownerIDs)+1)
	args = append(args, ownerKind)
	for _, ownerID := range ownerIDs {
		args = append(args, ownerID)
	}

	rows, err := store.db.Query(`
		SELECT owner_id, id, before_minutes, created_at, updated_at
		FROM reminders
		WHERE owner_kind = ? AND owner_id IN (`+placeholders+`)
		ORDER BY owner_id ASC, before_minutes ASC, id ASC
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("load reminders: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	for rows.Next() {
		var ownerID string
		var reminder domain.ReminderRule
		var createdAt string
		var updatedAt string
		if err := rows.Scan(&ownerID, &reminder.ID, &reminder.BeforeMinutes, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan reminder: %w", err)
		}
		reminder.CreatedAt = parseStoredTime(createdAt)
		reminder.UpdatedAt = parseStoredTime(updatedAt)
		byOwner[ownerID] = append(byOwner[ownerID], reminder)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate reminders: %w", err)
	}

	return byOwner, nil
}

func (store *Store) loadLinkedTaskIDs(eventIDs []string) (map[string][]string, error) {
	byEvent := make(map[string][]string, len(eventIDs))
	if len(eventIDs) == 0 {
		return byEvent, nil
	}

	placeholders := strings.TrimRight(strings.Repeat("?,", len(eventIDs)), ",")
	args := make([]any, 0, len(eventIDs))
	for _, eventID := range eventIDs {
		args = append(args, eventID)
	}

	rows, err := store.db.Query(`
		SELECT event_id, task_id
		FROM event_task_links
		WHERE event_id IN (`+placeholders+`)
		ORDER BY event_id ASC, task_id ASC
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("load linked task ids: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	for rows.Next() {
		var eventID string
		var taskID string
		if err := rows.Scan(&eventID, &taskID); err != nil {
			return nil, fmt.Errorf("scan linked task id: %w", err)
		}
		byEvent[eventID] = append(byEvent[eventID], taskID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate linked task ids: %w", err)
	}

	return byEvent, nil
}

func (store *Store) loadLinkedEventIDs(taskIDs []string) (map[string][]string, error) {
	byTask := make(map[string][]string, len(taskIDs))
	if len(taskIDs) == 0 {
		return byTask, nil
	}

	placeholders := strings.TrimRight(strings.Repeat("?,", len(taskIDs)), ",")
	args := make([]any, 0, len(taskIDs))
	for _, taskID := range taskIDs {
		args = append(args, taskID)
	}

	rows, err := store.db.Query(`
		SELECT task_id, event_id
		FROM event_task_links
		WHERE task_id IN (`+placeholders+`)
		ORDER BY task_id ASC, event_id ASC
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("load linked event ids: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	for rows.Next() {
		var taskID string
		var eventID string
		if err := rows.Scan(&taskID, &eventID); err != nil {
			return nil, fmt.Errorf("scan linked event id: %w", err)
		}
		byTask[taskID] = append(byTask[taskID], eventID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate linked event ids: %w", err)
	}

	return byTask, nil
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
		tagsJSON       string
		completedAt    sql.NullString
		createdAt      string
		updatedAt      string
	)
	if err := scanner.Scan(
		&task.ID, &task.CalendarID, &task.Title, &description, &dueAt, &dueDate,
		&recurrenceJSON, &task.Priority, &task.Status, &tagsJSON, &completedAt, &createdAt, &updatedAt,
	); err != nil {
		return domain.Task{}, err
	}

	recurrence, err := parseRecurrence(recurrenceJSON)
	if err != nil {
		return domain.Task{}, err
	}
	tags, err := parseTags(tagsJSON)
	if err != nil {
		return domain.Task{}, err
	}
	task.Description = parseNullableString(description)
	task.DueAt = parseNullableTime(dueAt)
	task.DueDate = parseNullableString(dueDate)
	task.Recurrence = recurrence
	task.Tags = tags
	task.CompletedAt = parseNullableTime(completedAt)
	task.CreatedAt = parseStoredTime(createdAt)
	task.UpdatedAt = parseStoredTime(updatedAt)

	return task, nil
}

func scanReminders(rows *sql.Rows) ([]domain.ReminderRule, error) {
	var reminders []domain.ReminderRule
	for rows.Next() {
		reminder, err := scanReminder(rows)
		if err != nil {
			return nil, fmt.Errorf("scan reminder: %w", err)
		}
		reminders = append(reminders, reminder)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate reminders: %w", err)
	}

	return reminders, nil
}

func scanReminder(scanner interface {
	Scan(dest ...any) error
}) (domain.ReminderRule, error) {
	var reminder domain.ReminderRule
	var createdAt string
	var updatedAt string
	if err := scanner.Scan(&reminder.ID, &reminder.BeforeMinutes, &createdAt, &updatedAt); err != nil {
		return domain.ReminderRule{}, err
	}
	reminder.CreatedAt = parseStoredTime(createdAt)
	reminder.UpdatedAt = parseStoredTime(updatedAt)
	return reminder, nil
}

func scanEventTaskLinks(rows *sql.Rows) ([]domain.EventTaskLink, error) {
	var links []domain.EventTaskLink
	for rows.Next() {
		link, err := scanEventTaskLink(rows)
		if err != nil {
			return nil, fmt.Errorf("scan event task link: %w", err)
		}
		links = append(links, link)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate event task links: %w", err)
	}

	return links, nil
}

func scanEventTaskLink(scanner interface {
	Scan(dest ...any) error
}) (domain.EventTaskLink, error) {
	var link domain.EventTaskLink
	var createdAt string
	var updatedAt string
	if err := scanner.Scan(&link.EventID, &link.TaskID, &createdAt, &updatedAt); err != nil {
		return domain.EventTaskLink{}, err
	}
	link.CreatedAt = parseStoredTime(createdAt)
	link.UpdatedAt = parseStoredTime(updatedAt)
	return link, nil
}

func marshalTags(tags []string) (string, error) {
	if tags == nil {
		tags = []string{}
	}
	payload, err := json.Marshal(tags)
	if err != nil {
		return "", fmt.Errorf("marshal task tags: %w", err)
	}
	return string(payload), nil
}

func parseTags(value string) ([]string, error) {
	if value == "" {
		return []string{}, nil
	}
	var tags []string
	if err := json.Unmarshal([]byte(value), &tags); err != nil {
		return nil, fmt.Errorf("parse task tags: %w", err)
	}
	if tags == nil {
		return []string{}, nil
	}
	return tags, nil
}

func taskHasAllTags(task domain.Task, tags []string) bool {
	if len(tags) == 0 {
		return true
	}
	taskTags := map[string]bool{}
	for _, tag := range task.Tags {
		taskTags[tag] = true
	}
	for _, tag := range tags {
		if !taskTags[tag] {
			return false
		}
	}
	return true
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
