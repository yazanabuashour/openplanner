package store

import (
	"database/sql"
	"errors"
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

	assertMigrationVersions(t, repository.db, []int{1, 2, 3, 4, 5, 6, 7, 8, 9})
	assertSchemaObjects(t, repository.db, []string{
		"calendars",
		"event_attendees",
		"event_attendees_event_position_idx",
		"event_task_links",
		"event_task_links_task_idx",
		"events",
		"events_calendar_ical_uid_idx",
		"events_calendar_idx",
		"openplanner_config",
		"recurrence_occurrence_states",
		"recurrence_occurrence_states_owner_idx",
		"reminder_dismissals",
		"reminders",
		"reminders_owner_idx",
		"schema_migrations",
		"task_occurrence_states",
		"tasks",
		"tasks_calendar_ical_uid_idx",
		"tasks_calendar_idx",
		"tasks_calendar_status_priority_idx",
		"tasks_priority_idx",
		"tasks_status_idx",
	})
	assertTableColumns(t, repository.db, "events", []string{
		"id",
		"calendar_id",
		"title",
		"description",
		"location",
		"start_at",
		"end_at",
		"start_date",
		"end_date",
		"recurrence_json",
		"created_at",
		"updated_at",
		"time_zone",
		"ical_uid",
	})
	assertTableColumns(t, repository.db, "openplanner_config", []string{
		"key",
		"value_json",
		"updated_at",
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

	assertMigrationVersions(t, repository.db, []int{1, 2, 3, 4, 5, 6, 7, 8, 9})

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
	if events[0].TimeZone != nil {
		t.Fatalf("legacy event timezone = %#v, want nil", events[0].TimeZone)
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

	assertMigrationVersions(t, reopened.db, []int{1, 2, 3, 4, 5, 6, 7, 8, 9})

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
		INSERT INTO schema_migrations (version) VALUES (10);
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
	if !strings.Contains(err.Error(), "database schema version 10 is newer than supported version 9") {
		t.Fatalf("Open() error = %v, want newer schema version error", err)
	}
}

func TestConfigValueLifecycle(t *testing.T) {
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

	updatedAt := time.Date(2026, 4, 24, 12, 30, 0, 0, time.UTC)
	missing, err := repository.GetConfigValue("runner.default_limit")
	if err != nil {
		t.Fatalf("GetConfigValue(missing): %v", err)
	}
	if missing != nil {
		t.Fatalf("missing config value = %#v, want nil", missing)
	}

	created, err := repository.UpsertConfigValue(UpsertConfigValueParams{
		Key:       "runner.default_limit",
		ValueJSON: `{"value":10}`,
		UpdatedAt: updatedAt,
	})
	if err != nil {
		t.Fatalf("UpsertConfigValue(create): %v", err)
	}
	if created.Key != "runner.default_limit" || created.ValueJSON != `{"value":10}` || !created.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("created config value = %#v", created)
	}

	reloaded, err := repository.GetConfigValue("runner.default_limit")
	if err != nil {
		t.Fatalf("GetConfigValue(): %v", err)
	}
	if reloaded == nil || reloaded.ValueJSON != `{"value":10}` {
		t.Fatalf("reloaded config value = %#v", reloaded)
	}

	updated, err := repository.UpsertConfigValue(UpsertConfigValueParams{
		Key:       "runner.default_limit",
		ValueJSON: `{"value":25}`,
		UpdatedAt: updatedAt.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("UpsertConfigValue(update): %v", err)
	}
	if updated.ValueJSON != `{"value":25}` || !updated.UpdatedAt.Equal(updatedAt.Add(time.Hour)) {
		t.Fatalf("updated config value = %#v", updated)
	}

	values, err := repository.ListConfigValues()
	if err != nil {
		t.Fatalf("ListConfigValues(): %v", err)
	}
	if len(values) != 1 || values[0].Key != "runner.default_limit" || values[0].ValueJSON != `{"value":25}` {
		t.Fatalf("config values = %#v", values)
	}

	deleted, err := repository.DeleteConfigValue("runner.default_limit")
	if err != nil {
		t.Fatalf("DeleteConfigValue(existing): %v", err)
	}
	if !deleted {
		t.Fatal("DeleteConfigValue(existing) = false, want true")
	}
	deleted, err = repository.DeleteConfigValue("runner.default_limit")
	if err != nil {
		t.Fatalf("DeleteConfigValue(missing): %v", err)
	}
	if deleted {
		t.Fatal("DeleteConfigValue(missing) = true, want false")
	}
}

func TestConfigValueValidation(t *testing.T) {
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

	_, err = repository.UpsertConfigValue(UpsertConfigValueParams{
		Key:       "runner.default_limit",
		ValueJSON: `{"value":`,
		UpdatedAt: time.Date(2026, 4, 24, 12, 30, 0, 0, time.UTC),
	})
	if err == nil || !strings.Contains(err.Error(), "valid JSON") {
		t.Fatalf("invalid JSON error = %v, want valid JSON rejection", err)
	}

	_, err = repository.GetConfigValue(" ")
	if err == nil || !strings.Contains(err.Error(), "config key is required") {
		t.Fatalf("empty key error = %v, want key rejection", err)
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

func TestEventTimeZonePersistsUpdatesAndClears(t *testing.T) {
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

	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	calendar := domain.Calendar{ID: "cal-timezone", Name: "Timezone", CreatedAt: now, UpdatedAt: now}
	if err := repository.CreateCalendar(calendar); err != nil {
		t.Fatalf("CreateCalendar(): %v", err)
	}

	startAt := time.Date(2026, 3, 3, 9, 0, 0, 0, time.FixedZone("", -5*60*60))
	timeZone := "America/New_York"
	event := domain.Event{
		ID:         "event-timezone",
		CalendarID: calendar.ID,
		Title:      "Planning",
		StartAt:    &startAt,
		TimeZone:   &timeZone,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := repository.CreateEvent(event); err != nil {
		t.Fatalf("CreateEvent(): %v", err)
	}

	stored, err := repository.GetEvent(event.ID)
	if err != nil {
		t.Fatalf("GetEvent(): %v", err)
	}
	if stored.TimeZone == nil || *stored.TimeZone != timeZone {
		t.Fatalf("stored timezone = %#v, want %q", stored.TimeZone, timeZone)
	}

	events, err := repository.ListEvents("")
	if err != nil {
		t.Fatalf("ListEvents(): %v", err)
	}
	if len(events) != 1 || events[0].TimeZone == nil || *events[0].TimeZone != timeZone {
		t.Fatalf("listed events = %#v, want timezone", events)
	}

	event.TimeZone = nil
	event.UpdatedAt = now.Add(time.Minute)
	if err := repository.UpdateEvent(event); err != nil {
		t.Fatalf("UpdateEvent(clear timezone): %v", err)
	}
	cleared, err := repository.GetEvent(event.ID)
	if err != nil {
		t.Fatalf("GetEvent(cleared): %v", err)
	}
	if cleared.TimeZone != nil {
		t.Fatalf("cleared timezone = %#v, want nil", cleared.TimeZone)
	}
}

func TestICalendarUIDPersistsAndLooksUpWithinCalendar(t *testing.T) {
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

	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	firstCalendar := domain.Calendar{ID: "cal-ical-first", Name: "First", CreatedAt: now, UpdatedAt: now}
	secondCalendar := domain.Calendar{ID: "cal-ical-second", Name: "Second", CreatedAt: now, UpdatedAt: now}
	if err := repository.CreateCalendar(firstCalendar); err != nil {
		t.Fatalf("CreateCalendar(first): %v", err)
	}
	if err := repository.CreateCalendar(secondCalendar); err != nil {
		t.Fatalf("CreateCalendar(second): %v", err)
	}

	uid := "shared@example.com"
	firstEvent := domain.Event{
		ID:           "event-ical-first",
		CalendarID:   firstCalendar.ID,
		ICalendarUID: &uid,
		Title:        "First event",
		StartDate:    stringPtr("2026-04-21"),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := repository.CreateEvent(firstEvent); err != nil {
		t.Fatalf("CreateEvent(first): %v", err)
	}
	secondEvent := firstEvent
	secondEvent.ID = "event-ical-second"
	secondEvent.CalendarID = secondCalendar.ID
	if err := repository.CreateEvent(secondEvent); err != nil {
		t.Fatalf("CreateEvent(second calendar same UID): %v", err)
	}
	duplicateEvent := firstEvent
	duplicateEvent.ID = "event-ical-duplicate"
	if err := repository.CreateEvent(duplicateEvent); !errors.Is(err, ErrConflict) {
		t.Fatalf("CreateEvent(duplicate UID) error = %v, want ErrConflict", err)
	}

	found, err := repository.GetEventByICalendarUID(firstCalendar.ID, uid)
	if err != nil {
		t.Fatalf("GetEventByICalendarUID(): %v", err)
	}
	if found.ID != firstEvent.ID || found.ICalendarUID == nil || *found.ICalendarUID != uid {
		t.Fatalf("found event = %#v, want first event by UID", found)
	}

	task := domain.Task{
		ID:           "task-ical-first",
		CalendarID:   firstCalendar.ID,
		ICalendarUID: &uid,
		Title:        "First task",
		DueDate:      stringPtr("2026-04-21"),
		Priority:     domain.TaskPriorityMedium,
		Status:       domain.TaskStatusTodo,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := repository.CreateTask(task); err != nil {
		t.Fatalf("CreateTask(): %v", err)
	}
	foundTask, err := repository.GetTaskByICalendarUID(firstCalendar.ID, uid)
	if err != nil {
		t.Fatalf("GetTaskByICalendarUID(): %v", err)
	}
	if foundTask.ID != task.ID || foundTask.ICalendarUID == nil || *foundTask.ICalendarUID != uid {
		t.Fatalf("found task = %#v, want task by UID", foundTask)
	}
}

func TestEventAndTaskRemindersPersistUpdateClearAndCascade(t *testing.T) {
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

	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	calendar := domain.Calendar{ID: "cal-reminders", Name: "Reminders", CreatedAt: now, UpdatedAt: now}
	if err := repository.CreateCalendar(calendar); err != nil {
		t.Fatalf("CreateCalendar(): %v", err)
	}

	startAt := time.Date(2026, 4, 21, 9, 0, 0, 0, time.UTC)
	endAt := startAt.Add(time.Hour)
	event := domain.Event{
		ID:         "event-reminders",
		CalendarID: calendar.ID,
		Title:      "Planning",
		StartAt:    &startAt,
		EndAt:      &endAt,
		Reminders: []domain.ReminderRule{
			{ID: "rem-event-60", BeforeMinutes: 60, CreatedAt: now, UpdatedAt: now},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := repository.CreateEvent(event); err != nil {
		t.Fatalf("CreateEvent(): %v", err)
	}

	dueDate := "2026-04-21"
	task := domain.Task{
		ID:         "task-reminders",
		CalendarID: calendar.ID,
		Title:      "Review",
		DueDate:    &dueDate,
		Priority:   domain.TaskPriorityMedium,
		Status:     domain.TaskStatusTodo,
		Reminders: []domain.ReminderRule{
			{ID: "rem-task-30", BeforeMinutes: 30, CreatedAt: now, UpdatedAt: now},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := repository.CreateTask(task); err != nil {
		t.Fatalf("CreateTask(): %v", err)
	}

	storedEvent, err := repository.GetEvent(event.ID)
	if err != nil {
		t.Fatalf("GetEvent(): %v", err)
	}
	if len(storedEvent.Reminders) != 1 || storedEvent.Reminders[0].ID != "rem-event-60" || storedEvent.Reminders[0].BeforeMinutes != 60 {
		t.Fatalf("event reminders = %#v, want persisted reminder", storedEvent.Reminders)
	}

	storedTask, err := repository.GetTask(task.ID)
	if err != nil {
		t.Fatalf("GetTask(): %v", err)
	}
	if len(storedTask.Reminders) != 1 || storedTask.Reminders[0].ID != "rem-task-30" || storedTask.Reminders[0].BeforeMinutes != 30 {
		t.Fatalf("task reminders = %#v, want persisted reminder", storedTask.Reminders)
	}

	updatedTitle := storedEvent
	updatedTitle.Title = "Planning updated"
	updatedTitle.UpdatedAt = now.Add(time.Minute)
	if err := repository.UpdateEvent(updatedTitle); err != nil {
		t.Fatalf("UpdateEvent(title): %v", err)
	}
	preserved, err := repository.GetEvent(event.ID)
	if err != nil {
		t.Fatalf("GetEvent(preserved): %v", err)
	}
	if len(preserved.Reminders) != 1 || preserved.Reminders[0].ID != "rem-event-60" {
		t.Fatalf("preserved reminders = %#v, want existing reminder id", preserved.Reminders)
	}

	replacedTask := storedTask
	replacedTask.Reminders = []domain.ReminderRule{{ID: "rem-task-90", BeforeMinutes: 90, CreatedAt: now, UpdatedAt: now}}
	replacedTask.UpdatedAt = now.Add(time.Minute)
	if err := repository.UpdateTask(replacedTask); err != nil {
		t.Fatalf("UpdateTask(replace reminder): %v", err)
	}
	replaced, err := repository.GetTask(task.ID)
	if err != nil {
		t.Fatalf("GetTask(replaced): %v", err)
	}
	if len(replaced.Reminders) != 1 || replaced.Reminders[0].ID != "rem-task-90" || replaced.Reminders[0].BeforeMinutes != 90 {
		t.Fatalf("replaced reminders = %#v, want replacement", replaced.Reminders)
	}

	clearedEvent := preserved
	clearedEvent.Reminders = nil
	clearedEvent.UpdatedAt = now.Add(2 * time.Minute)
	if err := repository.UpdateEvent(clearedEvent); err != nil {
		t.Fatalf("UpdateEvent(clear reminders): %v", err)
	}
	cleared, err := repository.GetEvent(event.ID)
	if err != nil {
		t.Fatalf("GetEvent(cleared): %v", err)
	}
	if len(cleared.Reminders) != 0 {
		t.Fatalf("cleared reminders = %#v, want none", cleared.Reminders)
	}

	if err := repository.DeleteTask(task.ID); err != nil {
		t.Fatalf("DeleteTask(): %v", err)
	}
	if _, err := repository.GetReminder("rem-task-90"); err != ErrNotFound {
		t.Fatalf("GetReminder(deleted task reminder) error = %v, want ErrNotFound", err)
	}
}

func TestEventAttendeesPersistUpdateClearAndCascade(t *testing.T) {
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

	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	calendar := domain.Calendar{ID: "cal-attendees", Name: "Attendees", CreatedAt: now, UpdatedAt: now}
	if err := repository.CreateCalendar(calendar); err != nil {
		t.Fatalf("CreateCalendar(): %v", err)
	}

	event := domain.Event{
		ID:         "event-attendees",
		CalendarID: calendar.ID,
		Title:      "Planning",
		StartDate:  stringPtr("2026-04-21"),
		Attendees: []domain.EventAttendee{
			{
				Email:               "alex@example.com",
				DisplayName:         stringPtr("Alex Rivera"),
				Role:                domain.EventAttendeeRoleRequired,
				ParticipationStatus: domain.EventParticipationStatusAccepted,
				RSVP:                true,
				CreatedAt:           now,
				UpdatedAt:           now,
			},
			{
				Email:               "sam@example.com",
				Role:                domain.EventAttendeeRoleOptional,
				ParticipationStatus: domain.EventParticipationStatusNeedsAction,
				CreatedAt:           now,
				UpdatedAt:           now,
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := repository.CreateEvent(event); err != nil {
		t.Fatalf("CreateEvent(): %v", err)
	}

	storedEvent, err := repository.GetEvent(event.ID)
	if err != nil {
		t.Fatalf("GetEvent(): %v", err)
	}
	if len(storedEvent.Attendees) != 2 || storedEvent.Attendees[0].Email != "alex@example.com" || storedEvent.Attendees[0].DisplayName == nil || !storedEvent.Attendees[0].RSVP {
		t.Fatalf("event attendees = %#v, want persisted attendees in order", storedEvent.Attendees)
	}

	updatedTitle := storedEvent
	updatedTitle.Title = "Planning updated"
	updatedTitle.UpdatedAt = now.Add(time.Minute)
	if err := repository.UpdateEvent(updatedTitle); err != nil {
		t.Fatalf("UpdateEvent(title): %v", err)
	}
	preserved, err := repository.GetEvent(event.ID)
	if err != nil {
		t.Fatalf("GetEvent(preserved): %v", err)
	}
	if len(preserved.Attendees) != 2 || preserved.Attendees[0].Email != "alex@example.com" {
		t.Fatalf("preserved attendees = %#v, want existing attendees", preserved.Attendees)
	}

	replacedEvent := preserved
	replacedEvent.Attendees = []domain.EventAttendee{
		{
			Email:               "taylor@example.com",
			Role:                domain.EventAttendeeRoleChair,
			ParticipationStatus: domain.EventParticipationStatusTentative,
			CreatedAt:           now,
			UpdatedAt:           now,
		},
	}
	replacedEvent.UpdatedAt = now.Add(2 * time.Minute)
	if err := repository.UpdateEvent(replacedEvent); err != nil {
		t.Fatalf("UpdateEvent(replace attendees): %v", err)
	}
	replaced, err := repository.GetEvent(event.ID)
	if err != nil {
		t.Fatalf("GetEvent(replaced): %v", err)
	}
	if len(replaced.Attendees) != 1 || replaced.Attendees[0].Email != "taylor@example.com" || replaced.Attendees[0].Role != domain.EventAttendeeRoleChair {
		t.Fatalf("replaced attendees = %#v, want replacement attendee", replaced.Attendees)
	}

	clearedEvent := replaced
	clearedEvent.Attendees = nil
	clearedEvent.UpdatedAt = now.Add(3 * time.Minute)
	if err := repository.UpdateEvent(clearedEvent); err != nil {
		t.Fatalf("UpdateEvent(clear attendees): %v", err)
	}
	cleared, err := repository.GetEvent(event.ID)
	if err != nil {
		t.Fatalf("GetEvent(cleared): %v", err)
	}
	if len(cleared.Attendees) != 0 {
		t.Fatalf("cleared attendees = %#v, want none", cleared.Attendees)
	}

	cascadeEvent := event
	cascadeEvent.ID = "event-attendees-cascade"
	if err := repository.CreateEvent(cascadeEvent); err != nil {
		t.Fatalf("CreateEvent(cascade): %v", err)
	}
	if err := repository.DeleteEvent(cascadeEvent.ID); err != nil {
		t.Fatalf("DeleteEvent(cascade): %v", err)
	}
	var attendeeCount int
	if err := repository.db.QueryRow(`SELECT COUNT(*) FROM event_attendees WHERE event_id = ?`, cascadeEvent.ID).Scan(&attendeeCount); err != nil {
		t.Fatalf("count cascaded attendees: %v", err)
	}
	if attendeeCount != 0 {
		t.Fatalf("attendees after cascade = %d, want 0", attendeeCount)
	}
}

func TestEventTaskLinksPersistListDeleteAndCascade(t *testing.T) {
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

	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	calendar := domain.Calendar{ID: "cal-links", Name: "Links", CreatedAt: now, UpdatedAt: now}
	if err := repository.CreateCalendar(calendar); err != nil {
		t.Fatalf("CreateCalendar(): %v", err)
	}
	event := domain.Event{
		ID:         "event-links",
		CalendarID: calendar.ID,
		Title:      "Planning",
		StartDate:  stringPtr("2026-04-21"),
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := repository.CreateEvent(event); err != nil {
		t.Fatalf("CreateEvent(): %v", err)
	}
	task := domain.Task{
		ID:         "task-links",
		CalendarID: calendar.ID,
		Title:      "Prep",
		DueDate:    stringPtr("2026-04-21"),
		Priority:   domain.TaskPriorityMedium,
		Status:     domain.TaskStatusTodo,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := repository.CreateTask(task); err != nil {
		t.Fatalf("CreateTask(): %v", err)
	}

	link := domain.EventTaskLink{EventID: event.ID, TaskID: task.ID, CreatedAt: now, UpdatedAt: now}
	if err := repository.CreateEventTaskLink(link); err != nil {
		t.Fatalf("CreateEventTaskLink(): %v", err)
	}
	if err := repository.CreateEventTaskLink(link); !errors.Is(err, ErrConflict) {
		t.Fatalf("CreateEventTaskLink(duplicate) error = %v, want ErrConflict", err)
	}

	allLinks, err := repository.ListEventTaskLinks(domain.EventTaskLinkFilter{})
	if err != nil {
		t.Fatalf("ListEventTaskLinks(all): %v", err)
	}
	if len(allLinks) != 1 || allLinks[0].EventID != event.ID || allLinks[0].TaskID != task.ID {
		t.Fatalf("all links = %#v, want one event-task link", allLinks)
	}
	eventLinks, err := repository.ListEventTaskLinks(domain.EventTaskLinkFilter{EventID: event.ID})
	if err != nil {
		t.Fatalf("ListEventTaskLinks(event): %v", err)
	}
	if len(eventLinks) != 1 {
		t.Fatalf("event links = %#v, want one link", eventLinks)
	}
	taskLinks, err := repository.ListEventTaskLinks(domain.EventTaskLinkFilter{TaskID: task.ID})
	if err != nil {
		t.Fatalf("ListEventTaskLinks(task): %v", err)
	}
	if len(taskLinks) != 1 {
		t.Fatalf("task links = %#v, want one link", taskLinks)
	}

	storedEvent, err := repository.GetEvent(event.ID)
	if err != nil {
		t.Fatalf("GetEvent(): %v", err)
	}
	if !slices.Equal(storedEvent.LinkedTaskIDs, []string{task.ID}) {
		t.Fatalf("event linked tasks = %v, want %v", storedEvent.LinkedTaskIDs, []string{task.ID})
	}
	storedTask, err := repository.GetTask(task.ID)
	if err != nil {
		t.Fatalf("GetTask(): %v", err)
	}
	if !slices.Equal(storedTask.LinkedEventIDs, []string{event.ID}) {
		t.Fatalf("task linked events = %v, want %v", storedTask.LinkedEventIDs, []string{event.ID})
	}

	if err := repository.DeleteEventTaskLink(event.ID, task.ID); err != nil {
		t.Fatalf("DeleteEventTaskLink(): %v", err)
	}
	if err := repository.DeleteEventTaskLink(event.ID, task.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("DeleteEventTaskLink(missing) error = %v, want ErrNotFound", err)
	}

	if err := repository.CreateEventTaskLink(link); err != nil {
		t.Fatalf("CreateEventTaskLink(recreate): %v", err)
	}
	if err := repository.DeleteEvent(event.ID); err != nil {
		t.Fatalf("DeleteEvent(): %v", err)
	}
	afterCascade, err := repository.ListEventTaskLinks(domain.EventTaskLinkFilter{})
	if err != nil {
		t.Fatalf("ListEventTaskLinks(after cascade): %v", err)
	}
	if len(afterCascade) != 0 {
		t.Fatalf("links after cascade = %#v, want none", afterCascade)
	}
}

func TestOccurrenceStatesPersistUpdateAndCascade(t *testing.T) {
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

	now := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	calendar := domain.Calendar{ID: "cal-occurrences", Name: "Occurrences", CreatedAt: now, UpdatedAt: now}
	if err := repository.CreateCalendar(calendar); err != nil {
		t.Fatalf("CreateCalendar(): %v", err)
	}
	event := domain.Event{
		ID:         "event-occurrences",
		CalendarID: calendar.ID,
		Title:      "Planning",
		StartDate:  stringPtr("2026-04-21"),
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := repository.CreateEvent(event); err != nil {
		t.Fatalf("CreateEvent(): %v", err)
	}

	occurrenceDate := "2026-04-21"
	replacementDate := "2026-04-22"
	state := domain.OccurrenceState{
		OwnerKind:       domain.OccurrenceOwnerKindEvent,
		OwnerID:         event.ID,
		OccurrenceKey:   event.ID + "@2026-04-21",
		OccurrenceDate:  &occurrenceDate,
		ReplacementDate: &replacementDate,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := repository.UpsertOccurrenceState(state); err != nil {
		t.Fatalf("UpsertOccurrenceState(): %v", err)
	}

	states, err := repository.ListOccurrenceStates(domain.OccurrenceOwnerKindEvent, []string{event.ID})
	if err != nil {
		t.Fatalf("ListOccurrenceStates(): %v", err)
	}
	stored := states[event.ID][state.OccurrenceKey]
	if stored.ReplacementDate == nil || *stored.ReplacementDate != replacementDate || stored.Cancelled {
		t.Fatalf("stored occurrence state = %#v, want replacement date", stored)
	}

	state.Cancelled = true
	state.ReplacementDate = nil
	state.UpdatedAt = now.Add(time.Minute)
	if err := repository.UpsertOccurrenceState(state); err != nil {
		t.Fatalf("UpsertOccurrenceState(update): %v", err)
	}
	states, err = repository.ListOccurrenceStates(domain.OccurrenceOwnerKindEvent, []string{event.ID})
	if err != nil {
		t.Fatalf("ListOccurrenceStates(update): %v", err)
	}
	stored = states[event.ID][state.OccurrenceKey]
	if !stored.Cancelled || stored.ReplacementDate != nil {
		t.Fatalf("updated occurrence state = %#v, want cancellation", stored)
	}

	if err := repository.DeleteEvent(event.ID); err != nil {
		t.Fatalf("DeleteEvent(): %v", err)
	}
	states, err = repository.ListOccurrenceStates(domain.OccurrenceOwnerKindEvent, []string{event.ID})
	if err != nil {
		t.Fatalf("ListOccurrenceStates(after cascade): %v", err)
	}
	if len(states[event.ID]) != 0 {
		t.Fatalf("states after cascade = %#v, want none", states)
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

func assertTableColumns(t *testing.T, db *sql.DB, table string, want []string) {
	t.Helper()

	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		t.Fatalf("query table info for %s: %v", table, err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Fatalf("Close(rows): %v", err)
		}
	}()

	var got []string
	for rows.Next() {
		var cid int
		var name string
		var typ string
		var notNull int
		var defaultValue sql.NullString
		var primaryKey int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatalf("scan table column: %v", err)
		}
		got = append(got, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate table columns: %v", err)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("%s columns = %v, want %v", table, got, want)
	}
}

func stringPtr(value string) *string {
	return &value
}
