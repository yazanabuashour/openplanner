package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/yazanabuashour/openplanner/sdk"
)

func TestRunPlanningTaskEnsureCalendarStatuses(t *testing.T) {
	t.Parallel()

	options := testOptions(t)
	ctx := context.Background()
	description := "Home calendar"

	created, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:       PlanningTaskActionEnsureCalendar,
		CalendarName: "Personal",
		Description:  &description,
	})
	if err != nil {
		t.Fatalf("create calendar: %v", err)
	}
	if created.Rejected || len(created.Writes) != 1 || created.Writes[0].Status != string(sdk.CalendarWriteStatusCreated) {
		t.Fatalf("create result = %#v", created)
	}

	repeated, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:       PlanningTaskActionEnsureCalendar,
		CalendarName: "Personal",
		Description:  &description,
	})
	if err != nil {
		t.Fatalf("repeat calendar: %v", err)
	}
	if repeated.Rejected || repeated.Writes[0].Status != string(sdk.CalendarWriteStatusAlreadyExists) {
		t.Fatalf("repeat result = %#v", repeated)
	}

	updatedDescription := "Personal planning"
	updated, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:       PlanningTaskActionEnsureCalendar,
		CalendarName: "Personal",
		Description:  &updatedDescription,
	})
	if err != nil {
		t.Fatalf("update calendar: %v", err)
	}
	if updated.Rejected || updated.Writes[0].Status != string(sdk.CalendarWriteStatusUpdated) {
		t.Fatalf("update result = %#v", updated)
	}
}

func TestRunPlanningTaskCreateAndListAgenda(t *testing.T) {
	t.Parallel()

	options := testOptions(t)
	ctx := context.Background()
	count := int32(2)

	event, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:       PlanningTaskActionCreateEvent,
		CalendarName: "Work",
		Title:        "Standup",
		StartAt:      "2026-04-16T09:00:00Z",
		EndAt:        "2026-04-16T10:00:00Z",
		Recurrence: &RecurrenceRuleRequest{
			Frequency: "daily",
			Count:     &count,
		},
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	if event.Rejected || len(event.Events) != 1 || event.Events[0].Title != "Standup" {
		t.Fatalf("event result = %#v", event)
	}

	task, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:       PlanningTaskActionCreateTask,
		CalendarName: "Work",
		Title:        "Review notes",
		DueDate:      "2026-04-16",
		Recurrence: &RecurrenceRuleRequest{
			Frequency: "daily",
			Count:     &count,
		},
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if task.Rejected || len(task.Tasks) != 1 {
		t.Fatalf("task result = %#v", task)
	}

	completion, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:         PlanningTaskActionCompleteTask,
		TaskID:         task.Tasks[0].ID,
		OccurrenceDate: "2026-04-16",
	})
	if err != nil {
		t.Fatalf("complete task: %v", err)
	}
	if completion.Rejected || completion.Writes[0].Status != "completed" {
		t.Fatalf("completion result = %#v", completion)
	}

	limit := 10
	agenda, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action: PlanningTaskActionListAgenda,
		From:   "2026-04-16T00:00:00Z",
		To:     "2026-04-18T00:00:00Z",
		Limit:  &limit,
	})
	if err != nil {
		t.Fatalf("list agenda: %v", err)
	}
	if agenda.Rejected || len(agenda.Agenda) != 4 {
		t.Fatalf("agenda result = %#v", agenda)
	}
	if agenda.Agenda[0].Title != "Review notes" || agenda.Agenda[0].CompletedAt == "" {
		t.Fatalf("first agenda item = %#v, want completed dated task", agenda.Agenda[0])
	}
	if agenda.Agenda[1].Title != "Standup" || agenda.Agenda[1].StartAt == "" {
		t.Fatalf("second agenda item = %#v, want timed event", agenda.Agenda[1])
	}
}

func TestRunPlanningTaskAllDayEventAndListFiltering(t *testing.T) {
	t.Parallel()

	options := testOptions(t)
	ctx := context.Background()

	if _, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:       PlanningTaskActionCreateEvent,
		CalendarName: "Work",
		Title:        "Planning day",
		StartDate:    "2026-04-17",
	}); err != nil {
		t.Fatalf("create work event: %v", err)
	}
	if _, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:       PlanningTaskActionCreateEvent,
		CalendarName: "Personal",
		Title:        "Errand",
		StartDate:    "2026-04-17",
	}); err != nil {
		t.Fatalf("create personal event: %v", err)
	}

	limit := 20
	events, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:       PlanningTaskActionListEvents,
		CalendarName: "Work",
		Limit:        &limit,
	})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if events.Rejected || len(events.Events) != 1 || events.Events[0].Title != "Planning day" {
		t.Fatalf("filtered events = %#v", events)
	}
	if events.Events[0].StartDate != "2026-04-17" {
		t.Fatalf("start date = %q, want 2026-04-17", events.Events[0].StartDate)
	}
}

func TestRunPlanningTaskCalendarIDWriteReturnsCalendarDetails(t *testing.T) {
	t.Parallel()

	options := testOptions(t)
	ctx := context.Background()

	calendar, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:       PlanningTaskActionEnsureCalendar,
		CalendarName: "Work",
	})
	if err != nil {
		t.Fatalf("ensure calendar: %v", err)
	}
	if len(calendar.Calendars) != 1 {
		t.Fatalf("calendar result = %#v", calendar)
	}

	event, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:     PlanningTaskActionCreateEvent,
		CalendarID: calendar.Calendars[0].ID,
		Title:      "Standup",
		StartAt:    "2026-04-16T09:00:00Z",
		EndAt:      "2026-04-16T09:30:00Z",
	})
	if err != nil {
		t.Fatalf("create event by calendar_id: %v", err)
	}
	if event.Rejected || len(event.Calendars) != 1 || event.Calendars[0].Name != "Work" {
		t.Fatalf("event calendar result = %#v, want Work calendar details", event)
	}
	if len(event.Writes) != 1 || event.Writes[0].Kind != "event" {
		t.Fatalf("event writes = %#v, want only event write", event.Writes)
	}

	task, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:     PlanningTaskActionCreateTask,
		CalendarID: calendar.Calendars[0].ID,
		Title:      "Review notes",
		DueDate:    "2026-04-16",
	})
	if err != nil {
		t.Fatalf("create task by calendar_id: %v", err)
	}
	if task.Rejected || len(task.Calendars) != 1 || task.Calendars[0].Name != "Work" {
		t.Fatalf("task calendar result = %#v, want Work calendar details", task)
	}
}

func TestRunPlanningTaskPreservesFractionalSecondTimes(t *testing.T) {
	t.Parallel()

	options := testOptions(t)
	ctx := context.Background()
	count := int32(1)

	event, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:       PlanningTaskActionCreateEvent,
		CalendarName: "Work",
		Title:        "Precise sync",
		StartAt:      "2026-04-16T08:00:00.500Z",
		EndAt:        "2026-04-16T08:30:00.750Z",
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	if event.Rejected || len(event.Events) != 1 {
		t.Fatalf("event result = %#v", event)
	}
	if event.Events[0].StartAt != "2026-04-16T08:00:00.5Z" || event.Events[0].EndAt != "2026-04-16T08:30:00.75Z" {
		t.Fatalf("event times = %#v, want RFC3339Nano precision", event.Events[0])
	}

	task, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:       PlanningTaskActionCreateTask,
		CalendarName: "Work",
		Title:        "Precise review",
		DueAt:        "2026-04-16T09:00:00.500Z",
		Recurrence: &RecurrenceRuleRequest{
			Frequency: "daily",
			Count:     &count,
		},
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if task.Rejected || len(task.Tasks) != 1 {
		t.Fatalf("task result = %#v", task)
	}
	if task.Tasks[0].DueAt != "2026-04-16T09:00:00.5Z" {
		t.Fatalf("task due_at = %q, want RFC3339Nano precision", task.Tasks[0].DueAt)
	}

	completion, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:       PlanningTaskActionCompleteTask,
		TaskID:       task.Tasks[0].ID,
		OccurrenceAt: task.Tasks[0].DueAt,
	})
	if err != nil {
		t.Fatalf("complete task: %v", err)
	}
	if completion.Rejected {
		t.Fatalf("completion result = %#v, want success with returned occurrence_at", completion)
	}

	agenda, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action: PlanningTaskActionListAgenda,
		From:   "2026-04-16T00:00:00Z",
		To:     "2026-04-17T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("list agenda: %v", err)
	}
	if agenda.Rejected || len(agenda.Agenda) != 2 {
		t.Fatalf("agenda result = %#v", agenda)
	}
	if agenda.Agenda[0].StartAt != "2026-04-16T08:00:00.5Z" || agenda.Agenda[0].EndAt != "2026-04-16T08:30:00.75Z" {
		t.Fatalf("agenda event = %#v, want RFC3339Nano precision", agenda.Agenda[0])
	}
	if agenda.Agenda[1].DueAt != "2026-04-16T09:00:00.5Z" || agenda.Agenda[1].CompletedAt == "" {
		t.Fatalf("agenda task = %#v, want precise completed timed task", agenda.Agenda[1])
	}
}

func TestRunPlanningTaskValidationRejectionsBeforeDatabaseCreation(t *testing.T) {
	t.Parallel()

	databasePath := filepath.Join(t.TempDir(), "nested", "openplanner.db")
	tests := []struct {
		name    string
		request PlanningTaskRequest
	}{
		{
			name: "ambiguous short date",
			request: PlanningTaskRequest{
				Action:       PlanningTaskActionCreateTask,
				CalendarName: "Work",
				Title:        "Review",
				DueDate:      "04/16",
			},
		},
		{
			name: "year first slash date",
			request: PlanningTaskRequest{
				Action:       PlanningTaskActionCreateEvent,
				CalendarName: "Work",
				Title:        "Planning",
				StartDate:    "2026/04/16",
			},
		},
		{
			name: "invalid rfc3339",
			request: PlanningTaskRequest{
				Action:       PlanningTaskActionCreateEvent,
				CalendarName: "Work",
				Title:        "Planning",
				StartAt:      "2026-04-16 09:00",
			},
		},
		{
			name: "missing title",
			request: PlanningTaskRequest{
				Action:       PlanningTaskActionCreateTask,
				CalendarName: "Work",
				DueDate:      "2026-04-16",
			},
		},
		{
			name: "invalid range",
			request: PlanningTaskRequest{
				Action: PlanningTaskActionListAgenda,
				From:   "2026-04-18T00:00:00Z",
				To:     "2026-04-16T00:00:00Z",
			},
		},
		{
			name: "unsupported recurrence",
			request: PlanningTaskRequest{
				Action:       PlanningTaskActionCreateTask,
				CalendarName: "Work",
				Title:        "Review",
				DueDate:      "2026-04-16",
				Recurrence:   &RecurrenceRuleRequest{Frequency: "hourly"},
			},
		},
		{
			name: "non-positive limit",
			request: PlanningTaskRequest{
				Action: PlanningTaskActionListTasks,
				Limit:  intPtr(0),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := RunPlanningTask(context.Background(), sdk.Options{DatabasePath: databasePath}, test.request)
			if err != nil {
				t.Fatalf("RunPlanningTask() error = %v", err)
			}
			if !result.Rejected || result.RejectionReason == "" {
				t.Fatalf("result = %#v, want rejection", result)
			}
			if _, err := os.Stat(filepath.Dir(databasePath)); !os.IsNotExist(err) {
				t.Fatalf("database directory exists after validation rejection: %v", err)
			}
		})
	}
}

func testOptions(t *testing.T) sdk.Options {
	t.Helper()
	return sdk.Options{DatabasePath: filepath.Join(t.TempDir(), "openplanner.db")}
}

func intPtr(value int) *int {
	return &value
}
