package sdk_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/yazanabuashour/openplanner/sdk"
)

func TestLocalClientCalendarHelpers(t *testing.T) {
	t.Parallel()

	client := openTempClient(t)
	ctx := context.Background()

	description := "Home calendar"
	color := "#3B82F6"
	created, err := client.EnsureCalendar(ctx, sdk.CalendarInput{
		Name:        " Personal ",
		Description: &description,
		Color:       &color,
	})
	if err != nil {
		t.Fatalf("EnsureCalendar create: %v", err)
	}
	if created.Status != sdk.CalendarWriteStatusCreated {
		t.Fatalf("create status = %q, want %q", created.Status, sdk.CalendarWriteStatusCreated)
	}
	if created.Calendar.Name != "Personal" || created.Calendar.Description == nil || *created.Calendar.Description != description {
		t.Fatalf("created calendar = %#v", created.Calendar)
	}

	repeated, err := client.EnsureCalendar(ctx, sdk.CalendarInput{
		Name:        "Personal",
		Description: &description,
		Color:       &color,
	})
	if err != nil {
		t.Fatalf("EnsureCalendar repeat: %v", err)
	}
	if repeated.Status != sdk.CalendarWriteStatusAlreadyExists || repeated.Calendar.ID != created.Calendar.ID {
		t.Fatalf("repeat result = %#v, want already_exists for id %s", repeated, created.Calendar.ID)
	}

	updatedDescription := "Personal planning"
	updated, err := client.EnsureCalendar(ctx, sdk.CalendarInput{
		Name:        "Personal",
		Description: &updatedDescription,
	})
	if err != nil {
		t.Fatalf("EnsureCalendar update: %v", err)
	}
	if updated.Status != sdk.CalendarWriteStatusUpdated || updated.Calendar.ID != created.Calendar.ID {
		t.Fatalf("update result = %#v, want updated for id %s", updated, created.Calendar.ID)
	}
	if updated.Calendar.Description == nil || *updated.Calendar.Description != updatedDescription {
		t.Fatalf("updated description = %#v, want %q", updated.Calendar.Description, updatedDescription)
	}
	if updated.Calendar.Color == nil || *updated.Calendar.Color != color {
		t.Fatalf("updated color = %#v, want preserved %q", updated.Calendar.Color, color)
	}
}

func TestLocalClientPlanningHelpers(t *testing.T) {
	t.Parallel()

	client := openTempClient(t)
	ctx := context.Background()

	calendar, err := client.EnsureCalendar(ctx, sdk.CalendarInput{Name: "Work"})
	if err != nil {
		t.Fatalf("EnsureCalendar: %v", err)
	}

	startAt := time.Date(2026, 4, 16, 9, 0, 0, 0, time.UTC)
	endAt := startAt.Add(time.Hour)
	timedEvent, err := client.CreateEvent(ctx, sdk.EventInput{
		CalendarID: calendar.Calendar.ID,
		Title:      "Standup",
		StartAt:    &startAt,
		EndAt:      &endAt,
	})
	if err != nil {
		t.Fatalf("CreateEvent timed: %v", err)
	}
	if timedEvent.StartAt == nil || !timedEvent.StartAt.Equal(startAt) {
		t.Fatalf("timed event start = %#v, want %s", timedEvent.StartAt, startAt)
	}

	allDay := "2026-04-17"
	allDayEvent, err := client.CreateEvent(ctx, sdk.EventInput{
		CalendarID: calendar.Calendar.ID,
		Title:      "Planning day",
		StartDate:  &allDay,
	})
	if err != nil {
		t.Fatalf("CreateEvent all-day: %v", err)
	}
	if allDayEvent.StartDate == nil || *allDayEvent.StartDate != allDay {
		t.Fatalf("all-day event date = %#v, want %q", allDayEvent.StartDate, allDay)
	}

	dueAt := time.Date(2026, 4, 16, 11, 0, 0, 0, time.UTC)
	timedTask, err := client.CreateTask(ctx, sdk.TaskInput{
		CalendarID: calendar.Calendar.ID,
		Title:      "Send summary",
		DueAt:      &dueAt,
	})
	if err != nil {
		t.Fatalf("CreateTask timed: %v", err)
	}

	count := int32(2)
	dueDate := "2026-04-16"
	recurringTask, err := client.CreateTask(ctx, sdk.TaskInput{
		CalendarID: calendar.Calendar.ID,
		Title:      "Review notes",
		DueDate:    &dueDate,
		Recurrence: &sdk.RecurrenceRule{
			Frequency: sdk.RecurrenceFrequencyDaily,
			Count:     &count,
		},
	})
	if err != nil {
		t.Fatalf("CreateTask dated recurring: %v", err)
	}

	if _, err := client.CompleteTask(ctx, timedTask.ID, sdk.TaskCompletionInput{}); err != nil {
		t.Fatalf("CompleteTask non-recurring: %v", err)
	}
	completion, err := client.CompleteTask(ctx, recurringTask.ID, sdk.TaskCompletionInput{
		OccurrenceDate: &dueDate,
	})
	if err != nil {
		t.Fatalf("CompleteTask recurring: %v", err)
	}
	if completion.OccurrenceDate == nil || *completion.OccurrenceDate != dueDate {
		t.Fatalf("completion occurrence date = %#v, want %q", completion.OccurrenceDate, dueDate)
	}

	events, err := client.ListEvents(ctx, sdk.ListOptions{CalendarID: calendar.Calendar.ID, Limit: 10})
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(events.Items) != 2 {
		t.Fatalf("event count = %d, want 2", len(events.Items))
	}

	tasks, err := client.ListTasks(ctx, sdk.ListOptions{CalendarID: calendar.Calendar.ID, Limit: 10})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(tasks.Items) != 2 {
		t.Fatalf("task count = %d, want 2", len(tasks.Items))
	}

	agenda, err := client.ListAgenda(ctx, sdk.AgendaOptions{
		From:  time.Date(2026, 4, 16, 0, 0, 0, 0, time.UTC),
		To:    time.Date(2026, 4, 18, 0, 0, 0, 0, time.UTC),
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("ListAgenda: %v", err)
	}
	if len(agenda.Items) != 5 {
		t.Fatalf("agenda count = %d, want 5: %#v", len(agenda.Items), agenda.Items)
	}
	if agenda.Items[0].Title != "Review notes" || agenda.Items[0].CompletedAt == nil {
		t.Fatalf("first agenda item = %#v, want completed recurring task occurrence", agenda.Items[0])
	}
	if agenda.Items[1].Title != "Standup" || agenda.Items[1].Kind != sdk.AgendaItemKindEvent {
		t.Fatalf("second agenda item = %#v, want Standup event", agenda.Items[1])
	}
	if agenda.Items[2].Title != "Send summary" || agenda.Items[2].CompletedAt == nil {
		t.Fatalf("third agenda item = %#v, want completed timed task", agenda.Items[2])
	}
	if agenda.Items[3].Title != "Planning day" || agenda.Items[3].StartDate == nil {
		t.Fatalf("fourth agenda item = %#v, want all-day event", agenda.Items[3])
	}
	if agenda.Items[4].Title != "Review notes" || agenda.Items[4].DueDate == nil || *agenda.Items[4].DueDate != "2026-04-17" {
		t.Fatalf("fifth agenda item = %#v, want second recurring task occurrence", agenda.Items[4])
	}
}

func TestLocalClientUpdatePatchHelpers(t *testing.T) {
	t.Parallel()

	client := openTempClient(t)
	ctx := context.Background()

	description := "Planning"
	color := "#445566"
	calendarWrite, err := client.EnsureCalendar(ctx, sdk.CalendarInput{
		Name:        "Work",
		Description: &description,
		Color:       &color,
	})
	if err != nil {
		t.Fatalf("EnsureCalendar(): %v", err)
	}

	calendar, err := client.UpdateCalendar(ctx, calendarWrite.Calendar.ID, sdk.CalendarPatchInput{
		Description: sdk.SetPatch("Delivery"),
		Color:       sdk.ClearPatch[string](),
	})
	if err != nil {
		t.Fatalf("UpdateCalendar(): %v", err)
	}
	if calendar.Description == nil || *calendar.Description != "Delivery" || calendar.Color != nil {
		t.Fatalf("calendar = %#v, want description set and color cleared", calendar)
	}

	startAt := time.Date(2026, 4, 16, 9, 0, 0, 0, time.UTC)
	endAt := startAt.Add(time.Hour)
	count := int32(2)
	event, err := client.CreateEvent(ctx, sdk.EventInput{
		CalendarID: calendar.ID,
		Title:      "Standup",
		StartAt:    &startAt,
		EndAt:      &endAt,
		Recurrence: &sdk.RecurrenceRule{
			Frequency: sdk.RecurrenceFrequencyDaily,
			Count:     &count,
		},
	})
	if err != nil {
		t.Fatalf("CreateEvent(): %v", err)
	}
	startDate := "2026-04-17"
	updatedEvent, err := client.UpdateEvent(ctx, event.ID, sdk.EventPatchInput{
		Location:   sdk.SetPatch("Room 12"),
		StartAt:    sdk.ClearPatch[time.Time](),
		EndAt:      sdk.ClearPatch[time.Time](),
		StartDate:  sdk.SetPatch(startDate),
		Recurrence: sdk.ClearPatch[sdk.RecurrenceRule](),
	})
	if err != nil {
		t.Fatalf("UpdateEvent(): %v", err)
	}
	if updatedEvent.Location == nil || *updatedEvent.Location != "Room 12" || updatedEvent.StartAt != nil || updatedEvent.StartDate == nil || updatedEvent.Recurrence != nil {
		t.Fatalf("event = %#v, want set location, all-day date, and cleared recurrence", updatedEvent)
	}

	dueDate := "2026-04-16"
	task, err := client.CreateTask(ctx, sdk.TaskInput{
		CalendarID: calendar.ID,
		Title:      "Review",
		DueDate:    &dueDate,
		Recurrence: &sdk.RecurrenceRule{
			Frequency: sdk.RecurrenceFrequencyDaily,
			Count:     &count,
		},
	})
	if err != nil {
		t.Fatalf("CreateTask(): %v", err)
	}
	dueAt := time.Date(2026, 4, 16, 11, 0, 0, 0, time.UTC)
	updatedTask, err := client.UpdateTask(ctx, task.ID, sdk.TaskPatchInput{
		DueDate:    sdk.ClearPatch[string](),
		DueAt:      sdk.SetPatch(dueAt),
		Recurrence: sdk.ClearPatch[sdk.RecurrenceRule](),
	})
	if err != nil {
		t.Fatalf("UpdateTask(): %v", err)
	}
	if updatedTask.DueDate != nil || updatedTask.DueAt == nil || !updatedTask.DueAt.Equal(dueAt) || updatedTask.Recurrence != nil {
		t.Fatalf("task = %#v, want due mode switch and cleared recurrence", updatedTask)
	}
}

func openTempClient(t *testing.T) *sdk.Client {
	t.Helper()

	client, err := sdk.OpenLocal(sdk.Options{
		DatabasePath: filepath.Join(t.TempDir(), "openplanner.db"),
	})
	if err != nil {
		t.Fatalf("OpenLocal(): %v", err)
	}
	t.Cleanup(func() {
		if closeErr := client.Close(); closeErr != nil {
			t.Fatalf("Close(): %v", closeErr)
		}
	})
	return client
}
