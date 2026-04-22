package runner

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
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
	if created.Rejected || len(created.Writes) != 1 || created.Writes[0].Status != "created" {
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
	if repeated.Rejected || repeated.Writes[0].Status != "already_exists" {
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
	if updated.Rejected || updated.Writes[0].Status != "updated" {
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

func TestRunPlanningTaskTimezoneAwareEvents(t *testing.T) {
	t.Parallel()

	options := testOptions(t)
	ctx := context.Background()
	count := int32(2)
	timeZone := "America/New_York"

	event, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:       PlanningTaskActionCreateEvent,
		CalendarName: "Work",
		Title:        "Weekly sync",
		StartAt:      "2026-03-03T09:00:00-05:00",
		TimeZone:     &timeZone,
		Recurrence: &RecurrenceRuleRequest{
			Frequency: "weekly",
			Count:     &count,
		},
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	if event.Rejected || len(event.Events) != 1 || event.Events[0].TimeZone == nil || *event.Events[0].TimeZone != timeZone {
		t.Fatalf("event result = %#v, want timezone", event)
	}

	agenda, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action: PlanningTaskActionListAgenda,
		From:   "2026-03-03T00:00:00Z",
		To:     "2026-03-11T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("list agenda: %v", err)
	}
	if agenda.Rejected || len(agenda.Agenda) != 2 {
		t.Fatalf("agenda = %#v, want two occurrences", agenda)
	}
	if agenda.Agenda[0].StartAt != "2026-03-03T09:00:00-05:00" || agenda.Agenda[1].StartAt != "2026-03-10T09:00:00-04:00" {
		t.Fatalf("agenda = %#v, want local 09:00 across DST", agenda.Agenda)
	}
	if agenda.Agenda[1].TimeZone == nil || *agenda.Agenda[1].TimeZone != timeZone {
		t.Fatalf("agenda timezone = %#v, want %q", agenda.Agenda[1].TimeZone, timeZone)
	}

	preserved, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:  PlanningTaskActionUpdateEvent,
		EventID: event.Events[0].ID,
		Title:   "Renamed sync",
	})
	if err != nil {
		t.Fatalf("update event preserve timezone: %v", err)
	}
	if preserved.Rejected || preserved.Events[0].TimeZone == nil || *preserved.Events[0].TimeZone != timeZone {
		t.Fatalf("preserved event = %#v, want timezone preserved", preserved)
	}

	clearJSON := `{"action":"update_event","event_id":"` + event.Events[0].ID + `","time_zone":null}`
	clearRequest, err := DecodePlanningTaskRequest(bytes.NewBufferString(clearJSON))
	if err != nil {
		t.Fatalf("decode timezone clear: %v", err)
	}
	cleared, err := RunPlanningTask(ctx, options, clearRequest)
	if err != nil {
		t.Fatalf("update event clear timezone: %v", err)
	}
	if cleared.Rejected || cleared.Events[0].TimeZone != nil {
		t.Fatalf("cleared event = %#v, want timezone cleared", cleared)
	}

	second, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:       PlanningTaskActionCreateEvent,
		CalendarName: "Work",
		Title:        "Switch mode",
		StartAt:      "2026-03-03T09:00:00-05:00",
		TimeZone:     &timeZone,
	})
	if err != nil {
		t.Fatalf("create switch event: %v", err)
	}
	switchJSON := `{"action":"update_event","event_id":"` + second.Events[0].ID + `","start_at":null,"start_date":"2026-03-04"}`
	switchRequest, err := DecodePlanningTaskRequest(bytes.NewBufferString(switchJSON))
	if err != nil {
		t.Fatalf("decode switch event: %v", err)
	}
	switched, err := RunPlanningTask(ctx, options, switchRequest)
	if err != nil {
		t.Fatalf("update switch event: %v", err)
	}
	if switched.Rejected || switched.Events[0].StartDate != "2026-03-04" || switched.Events[0].TimeZone != nil {
		t.Fatalf("switched event = %#v, want all-day without timezone", switched)
	}

	rejected, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:   PlanningTaskActionUpdateEvent,
		EventID:  second.Events[0].ID,
		TimeZone: &timeZone,
	})
	if err != nil {
		t.Fatalf("update all-day timezone: %v", err)
	}
	if !rejected.Rejected || rejected.RejectionReason == "" {
		t.Fatalf("rejected result = %#v, want all-day timezone rejection", rejected)
	}
}

func TestRunPlanningTaskOccurrenceActionsAndCompletionKey(t *testing.T) {
	t.Parallel()

	options := testOptions(t)
	ctx := context.Background()
	count := int32(3)

	event, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:       PlanningTaskActionCreateEvent,
		CalendarName: "Work",
		Title:        "Standup",
		StartAt:      "2026-04-16T09:00:00Z",
		EndAt:        "2026-04-16T09:30:00Z",
		Recurrence:   &RecurrenceRuleRequest{Frequency: "daily", Count: &count},
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	task, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:       PlanningTaskActionCreateTask,
		CalendarName: "Work",
		Title:        "Review",
		DueDate:      "2026-04-16",
		Recurrence:   &RecurrenceRuleRequest{Frequency: "daily", Count: &count},
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	canceledEvent, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:       PlanningTaskActionCancelEventOccurrence,
		EventID:      event.Events[0].ID,
		OccurrenceAt: "2026-04-17T09:00:00Z",
	})
	if err != nil {
		t.Fatalf("cancel event occurrence: %v", err)
	}
	if canceledEvent.Rejected || canceledEvent.Writes[0].OccurrenceKey == "" || canceledEvent.Writes[0].Status != "canceled" {
		t.Fatalf("canceled event result = %#v", canceledEvent)
	}
	movedEvent, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:       PlanningTaskActionRescheduleEventOccurrence,
		EventID:      event.Events[0].ID,
		OccurrenceAt: "2026-04-18T09:00:00Z",
		StartAt:      "2026-04-19T11:00:00Z",
	})
	if err != nil {
		t.Fatalf("reschedule event occurrence: %v", err)
	}
	if movedEvent.Rejected || movedEvent.Writes[0].OccurrenceKey == "" || movedEvent.Writes[0].Status != "rescheduled" {
		t.Fatalf("moved event result = %#v", movedEvent)
	}

	canceledTask, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:         PlanningTaskActionCancelTaskOccurrence,
		TaskID:         task.Tasks[0].ID,
		OccurrenceDate: "2026-04-17",
	})
	if err != nil {
		t.Fatalf("cancel task occurrence: %v", err)
	}
	if canceledTask.Rejected || canceledTask.Writes[0].OccurrenceKey == "" {
		t.Fatalf("canceled task result = %#v", canceledTask)
	}
	movedTask, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:         PlanningTaskActionRescheduleTaskOccurrence,
		TaskID:         task.Tasks[0].ID,
		OccurrenceDate: "2026-04-18",
		DueDate:        "2026-04-19",
	})
	if err != nil {
		t.Fatalf("reschedule task occurrence: %v", err)
	}
	if movedTask.Rejected || movedTask.Writes[0].OccurrenceKey == "" {
		t.Fatalf("moved task result = %#v", movedTask)
	}
	completed, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:        PlanningTaskActionCompleteTask,
		TaskID:        task.Tasks[0].ID,
		OccurrenceKey: movedTask.Writes[0].OccurrenceKey,
	})
	if err != nil {
		t.Fatalf("complete by occurrence key: %v", err)
	}
	if completed.Rejected || completed.Writes[0].OccurrenceKey != movedTask.Writes[0].OccurrenceKey {
		t.Fatalf("completion result = %#v, want stable occurrence key", completed)
	}

	limit := 10
	agenda, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action: PlanningTaskActionListAgenda,
		From:   "2026-04-16T00:00:00Z",
		To:     "2026-04-20T00:00:00Z",
		Limit:  &limit,
	})
	if err != nil {
		t.Fatalf("list agenda: %v", err)
	}
	if agenda.Rejected || len(agenda.Agenda) != 4 {
		t.Fatalf("agenda = %#v, want first and moved event/task occurrences", agenda)
	}
	if agenda.Agenda[2].DueDate != "2026-04-19" || agenda.Agenda[2].OccurrenceKey != movedTask.Writes[0].OccurrenceKey || agenda.Agenda[2].CompletedAt == "" {
		t.Fatalf("moved task agenda item = %#v", agenda.Agenda[2])
	}
	if agenda.Agenda[3].StartAt != "2026-04-19T11:00:00Z" || agenda.Agenda[3].OccurrenceKey != movedEvent.Writes[0].OccurrenceKey {
		t.Fatalf("moved event agenda item = %#v", agenda.Agenda[3])
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

func TestRunPlanningTaskExportICalendar(t *testing.T) {
	t.Parallel()

	options := testOptions(t)
	ctx := context.Background()
	count := int32(2)
	timeZone := "America/New_York"

	workCalendar, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:       PlanningTaskActionEnsureCalendar,
		CalendarName: "Work Calendar",
	})
	if err != nil {
		t.Fatalf("ensure work calendar: %v", err)
	}
	if _, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:       PlanningTaskActionEnsureCalendar,
		CalendarName: "Personal",
	}); err != nil {
		t.Fatalf("ensure personal calendar: %v", err)
	}

	event, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:       PlanningTaskActionCreateEvent,
		CalendarName: "Work Calendar",
		Title:        "Weekly sync",
		StartAt:      "2026-03-03T09:00:00-05:00",
		EndAt:        "2026-03-03T10:00:00-05:00",
		TimeZone:     &timeZone,
		Recurrence:   &RecurrenceRuleRequest{Frequency: "weekly", Count: &count},
		Reminders:    []ReminderRuleRequest{{BeforeMinutes: 30}},
		Attendees:    []EventAttendeeRequest{{Email: "alex@example.com", DisplayName: "Alex Rivera", Role: "required", ParticipationStatus: "accepted", RSVP: true}},
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	task, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:       PlanningTaskActionCreateTask,
		CalendarName: "Work Calendar",
		Title:        "Review notes",
		DueDate:      "2026-03-03",
		Recurrence:   &RecurrenceRuleRequest{Frequency: "daily", Count: &count},
		Priority:     "high",
		Tags:         []string{"planning"},
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	movedEvent, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:       PlanningTaskActionRescheduleEventOccurrence,
		EventID:      event.Events[0].ID,
		OccurrenceAt: "2026-03-10T09:00:00-04:00",
		StartAt:      "2026-03-11T11:00:00-04:00",
	})
	if err != nil {
		t.Fatalf("reschedule event occurrence: %v", err)
	}
	completedTask, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:         PlanningTaskActionCompleteTask,
		TaskID:         task.Tasks[0].ID,
		OccurrenceDate: "2026-03-04",
	})
	if err != nil {
		t.Fatalf("complete task occurrence: %v", err)
	}

	export, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:       PlanningTaskActionExportICalendar,
		CalendarName: "Work Calendar",
	})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if export.Rejected || export.ICalendar == nil {
		t.Fatalf("export result = %#v, want iCalendar payload", export)
	}
	if export.ICalendar.ContentType != "text/calendar; charset=utf-8" ||
		export.ICalendar.Filename != "work-calendar.ics" ||
		export.ICalendar.CalendarID != workCalendar.Calendars[0].ID ||
		export.ICalendar.CalendarName != "Work Calendar" ||
		export.ICalendar.EventCount != 1 ||
		export.ICalendar.TaskCount != 1 {
		t.Fatalf("export metadata = %#v", export.ICalendar)
	}
	content := strings.ReplaceAll(export.ICalendar.Content, "\r\n ", "")
	for _, expected := range []string{
		"BEGIN:VCALENDAR\r\n",
		"BEGIN:VEVENT\r\n",
		"BEGIN:VTODO\r\n",
		"DTSTART;TZID=America/New_York:20260303T090000",
		"RRULE:FREQ=WEEKLY;INTERVAL=1;COUNT=2",
		"ATTENDEE;ROLE=REQ-PARTICIPANT;PARTSTAT=ACCEPTED;RSVP=TRUE;CN=\"Alex Rivera\":mailto:alex@example.com",
		"TRIGGER:-PT30M",
		"EXDATE;TZID=America/New_York:20260310T090000",
		"RECURRENCE-ID;TZID=America/New_York:20260310T090000",
		"DUE;VALUE=DATE:20260303",
		"PRIORITY:1",
		"STATUS:COMPLETED",
		"RECURRENCE-ID;VALUE=DATE:20260304",
		"X-OPENPLANNER-OCCURRENCE-KEY:" + movedEvent.Writes[0].OccurrenceKey,
		"X-OPENPLANNER-OCCURRENCE-KEY:" + completedTask.Writes[0].OccurrenceKey,
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("export content missing %q:\n%s", expected, content)
		}
	}

	all, err := RunPlanningTask(ctx, options, PlanningTaskRequest{Action: PlanningTaskActionExportICalendar})
	if err != nil {
		t.Fatalf("export all: %v", err)
	}
	if all.Rejected || all.ICalendar == nil || all.ICalendar.Filename != "openplanner.ics" || all.ICalendar.EventCount != 1 || all.ICalendar.TaskCount != 1 {
		t.Fatalf("all export = %#v", all)
	}
}

func TestRunPlanningTaskExportICalendarFiltersAndRejections(t *testing.T) {
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
	byID, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:     PlanningTaskActionExportICalendar,
		CalendarID: calendar.Calendars[0].ID,
	})
	if err != nil {
		t.Fatalf("export by id: %v", err)
	}
	if byID.Rejected || byID.ICalendar == nil || byID.ICalendar.CalendarName != "Work" {
		t.Fatalf("by id export = %#v", byID)
	}

	tests := []PlanningTaskRequest{
		{Action: PlanningTaskActionExportICalendar, CalendarID: "not-a-ulid"},
		{Action: PlanningTaskActionExportICalendar, CalendarName: "Missing"},
		{Action: PlanningTaskActionExportICalendar, CalendarID: calendar.Calendars[0].ID, CalendarName: "Work"},
		{Action: PlanningTaskActionExportICalendar, From: "2026-04-16T00:00:00Z"},
	}
	for _, request := range tests {
		result, err := RunPlanningTask(ctx, options, request)
		if err != nil {
			t.Fatalf("RunPlanningTask(%#v): %v", request, err)
		}
		if !result.Rejected || result.RejectionReason == "" {
			t.Fatalf("result = %#v, want rejection for %#v", result, request)
		}
	}
}

func TestRunPlanningTaskEventAttendeesRoundTripListAgendaAndUpdate(t *testing.T) {
	t.Parallel()

	options := testOptions(t)
	ctx := context.Background()

	create, err := DecodePlanningTaskRequest(bytes.NewBufferString(`{"action":"create_event","calendar_name":"Work","title":"Planning","start_at":"2026-04-16T09:00:00Z","end_at":"2026-04-16T10:00:00Z","attendees":[{"email":" alex@example.com ","display_name":" Alex Rivera ","role":"required","participation_status":"accepted","rsvp":true},{"email":"sam@example.com"}]}`))
	if err != nil {
		t.Fatalf("decode create event: %v", err)
	}
	event, err := RunPlanningTask(ctx, options, create)
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	if event.Rejected || len(event.Events) != 1 || len(event.Events[0].Attendees) != 2 {
		t.Fatalf("event result = %#v, want attendees", event)
	}
	if event.Events[0].Attendees[0].Email != "alex@example.com" ||
		event.Events[0].Attendees[0].DisplayName == nil ||
		*event.Events[0].Attendees[0].DisplayName != "Alex Rivera" ||
		event.Events[0].Attendees[0].Role != "required" ||
		event.Events[0].Attendees[0].ParticipationStatus != "accepted" ||
		!event.Events[0].Attendees[0].RSVP {
		t.Fatalf("first attendee = %#v, want normalized accepted attendee", event.Events[0].Attendees[0])
	}
	if event.Events[0].Attendees[1].Role != "required" ||
		event.Events[0].Attendees[1].ParticipationStatus != "needs_action" ||
		event.Events[0].Attendees[1].RSVP {
		t.Fatalf("second attendee = %#v, want default attendee metadata", event.Events[0].Attendees[1])
	}

	limit := 10
	events, err := RunPlanningTask(ctx, options, PlanningTaskRequest{Action: PlanningTaskActionListEvents, CalendarName: "Work", Limit: &limit})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if events.Rejected || len(events.Events) != 1 || len(events.Events[0].Attendees) != 2 {
		t.Fatalf("listed events = %#v, want attendees", events)
	}

	agenda, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action: PlanningTaskActionListAgenda,
		From:   "2026-04-16T00:00:00Z",
		To:     "2026-04-17T00:00:00Z",
		Limit:  &limit,
	})
	if err != nil {
		t.Fatalf("list agenda: %v", err)
	}
	if agenda.Rejected || len(agenda.Agenda) != 1 || len(agenda.Agenda[0].Attendees) != 2 {
		t.Fatalf("agenda = %#v, want event attendees", agenda)
	}

	preserve, err := DecodePlanningTaskRequest(bytes.NewBufferString(`{"action":"update_event","event_id":"` + event.Events[0].ID + `","title":"Planning updated"}`))
	if err != nil {
		t.Fatalf("decode preserve update: %v", err)
	}
	preserved, err := RunPlanningTask(ctx, options, preserve)
	if err != nil {
		t.Fatalf("update event title: %v", err)
	}
	if preserved.Rejected || preserved.Events[0].Title != "Planning updated" || len(preserved.Events[0].Attendees) != 2 {
		t.Fatalf("preserved event = %#v, want attendees preserved", preserved)
	}

	replace, err := DecodePlanningTaskRequest(bytes.NewBufferString(`{"action":"update_event","event_id":"` + event.Events[0].ID + `","attendees":[{"email":"taylor@example.com","role":"optional","participation_status":"tentative"}]}`))
	if err != nil {
		t.Fatalf("decode replace attendees: %v", err)
	}
	replaced, err := RunPlanningTask(ctx, options, replace)
	if err != nil {
		t.Fatalf("replace attendees: %v", err)
	}
	if replaced.Rejected || len(replaced.Events[0].Attendees) != 1 ||
		replaced.Events[0].Attendees[0].Email != "taylor@example.com" ||
		replaced.Events[0].Attendees[0].Role != "optional" ||
		replaced.Events[0].Attendees[0].ParticipationStatus != "tentative" {
		t.Fatalf("replaced event = %#v, want replacement attendee", replaced)
	}

	clear, err := DecodePlanningTaskRequest(bytes.NewBufferString(`{"action":"update_event","event_id":"` + event.Events[0].ID + `","attendees":null}`))
	if err != nil {
		t.Fatalf("decode clear attendees: %v", err)
	}
	cleared, err := RunPlanningTask(ctx, options, clear)
	if err != nil {
		t.Fatalf("clear attendees: %v", err)
	}
	if cleared.Rejected || len(cleared.Events[0].Attendees) != 0 {
		t.Fatalf("cleared event = %#v, want no attendees", cleared)
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

func TestRunPlanningTaskUpdateActionsAndClearSemantics(t *testing.T) {
	t.Parallel()

	options := testOptions(t)
	ctx := context.Background()
	count := int32(2)

	calendar, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:       PlanningTaskActionEnsureCalendar,
		CalendarName: "Work",
		Description:  stringPtr("Planning"),
		Color:        stringPtr("#334455"),
	})
	if err != nil {
		t.Fatalf("ensure calendar: %v", err)
	}
	calendarUpdate, err := DecodePlanningTaskRequest(bytes.NewBufferString(`{"action":"update_calendar","calendar_name":"Work","description":"Delivery","color":null}`))
	if err != nil {
		t.Fatalf("decode calendar update: %v", err)
	}
	updatedCalendar, err := RunPlanningTask(ctx, options, calendarUpdate)
	if err != nil {
		t.Fatalf("update calendar: %v", err)
	}
	if updatedCalendar.Rejected || updatedCalendar.Calendars[0].Description == nil || *updatedCalendar.Calendars[0].Description != "Delivery" || updatedCalendar.Calendars[0].Color != nil {
		t.Fatalf("updated calendar = %#v, want description set and color cleared", updatedCalendar)
	}

	event, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:      PlanningTaskActionCreateEvent,
		CalendarID:  calendar.Calendars[0].ID,
		Title:       "Standup",
		StartAt:     "2026-04-16T09:00:00Z",
		EndAt:       "2026-04-16T09:30:00Z",
		Description: stringPtr("Daily sync"),
		Recurrence:  &RecurrenceRuleRequest{Frequency: "daily", Count: &count},
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	eventUpdateJSON := `{"action":"update_event","event_id":"` + event.Events[0].ID + `","description":null,"start_at":null,"end_at":null,"start_date":"2026-04-17","recurrence":null}`
	eventUpdate, err := DecodePlanningTaskRequest(bytes.NewBufferString(eventUpdateJSON))
	if err != nil {
		t.Fatalf("decode event update: %v", err)
	}
	updatedEvent, err := RunPlanningTask(ctx, options, eventUpdate)
	if err != nil {
		t.Fatalf("update event: %v", err)
	}
	if updatedEvent.Rejected || updatedEvent.Events[0].Description != nil || updatedEvent.Events[0].StartAt != "" || updatedEvent.Events[0].StartDate != "2026-04-17" || updatedEvent.Events[0].Recurrence != nil {
		t.Fatalf("updated event = %#v, want explicit clears and all-day date", updatedEvent)
	}

	task, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:       PlanningTaskActionCreateTask,
		CalendarName: "Work",
		Title:        "Review",
		DueDate:      "2026-04-16",
		Recurrence:   &RecurrenceRuleRequest{Frequency: "daily", Count: &count},
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	taskUpdateJSON := `{"action":"update_task","task_id":"` + task.Tasks[0].ID + `","due_date":null,"due_at":"2026-04-16T11:00:00Z","recurrence":null}`
	taskUpdate, err := DecodePlanningTaskRequest(bytes.NewBufferString(taskUpdateJSON))
	if err != nil {
		t.Fatalf("decode task update: %v", err)
	}
	updatedTask, err := RunPlanningTask(ctx, options, taskUpdate)
	if err != nil {
		t.Fatalf("update task: %v", err)
	}
	if updatedTask.Rejected || updatedTask.Tasks[0].DueDate != "" || updatedTask.Tasks[0].DueAt != "2026-04-16T11:00:00Z" || updatedTask.Tasks[0].Recurrence != nil {
		t.Fatalf("updated task = %#v, want due_date and recurrence cleared", updatedTask)
	}
}

func TestRunPlanningTaskTaskMetadataRoundTripAndFilters(t *testing.T) {
	t.Parallel()

	options := testOptions(t)
	ctx := context.Background()

	task, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:       PlanningTaskActionCreateTask,
		CalendarName: "Work",
		Title:        "Review",
		DueDate:      "2026-04-16",
		Priority:     "high",
		Status:       "in_progress",
		Tags:         []string{" Planning ", "review"},
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if task.Rejected || len(task.Tasks) != 1 {
		t.Fatalf("task result = %#v", task)
	}
	if task.Tasks[0].Priority != "high" || task.Tasks[0].Status != "in_progress" || !slices.Equal(task.Tasks[0].Tags, []string{"planning", "review"}) {
		t.Fatalf("created task metadata = %#v", task.Tasks[0])
	}

	if _, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:       PlanningTaskActionCreateTask,
		CalendarName: "Work",
		Title:        "Backlog",
		DueDate:      "2026-04-16",
		Priority:     "low",
		Status:       "todo",
		Tags:         []string{"backlog"},
	}); err != nil {
		t.Fatalf("create backlog: %v", err)
	}

	limit := 10
	filtered, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:       PlanningTaskActionListTasks,
		CalendarName: "Work",
		Priority:     "high",
		Status:       "in_progress",
		Tags:         []string{"review", "planning"},
		Limit:        &limit,
	})
	if err != nil {
		t.Fatalf("list filtered: %v", err)
	}
	if filtered.Rejected || len(filtered.Tasks) != 1 || filtered.Tasks[0].Title != "Review" {
		t.Fatalf("filtered tasks = %#v, want Review only", filtered)
	}

	taskUpdateJSON := `{"action":"update_task","task_id":"` + task.Tasks[0].ID + `","priority":"medium","tags":null}`
	taskUpdate, err := DecodePlanningTaskRequest(bytes.NewBufferString(taskUpdateJSON))
	if err != nil {
		t.Fatalf("decode task update: %v", err)
	}
	updated, err := RunPlanningTask(ctx, options, taskUpdate)
	if err != nil {
		t.Fatalf("update task: %v", err)
	}
	if updated.Rejected || updated.Tasks[0].Priority != "medium" || len(updated.Tasks[0].Tags) != 0 {
		t.Fatalf("updated task = %#v, want priority medium and tags cleared", updated)
	}
}

func TestRunPlanningTaskTaskMetadataRejectionsBeforeDatabaseCreation(t *testing.T) {
	t.Parallel()

	databasePath := filepath.Join(t.TempDir(), "nested", "openplanner.db")
	tests := []struct {
		name    string
		request string
	}{
		{
			name:    "invalid priority",
			request: `{"action":"create_task","calendar_name":"Work","title":"Review","due_date":"2026-04-16","priority":"urgent"}`,
		},
		{
			name:    "invalid status",
			request: `{"action":"create_task","calendar_name":"Work","title":"Review","due_date":"2026-04-16","status":"blocked"}`,
		},
		{
			name:    "invalid tag",
			request: `{"action":"create_task","calendar_name":"Work","title":"Review","due_date":"2026-04-16","tags":["needs review"]}`,
		},
		{
			name:    "clear priority",
			request: `{"action":"update_task","task_id":"01ARZ3NDEKTSV4RRFFQ69G5FAV","priority":null}`,
		},
		{
			name:    "empty priority update",
			request: `{"action":"update_task","task_id":"01ARZ3NDEKTSV4RRFFQ69G5FAV","priority":""}`,
		},
		{
			name:    "empty status update",
			request: `{"action":"update_task","task_id":"01ARZ3NDEKTSV4RRFFQ69G5FAV","status":""}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request, err := DecodePlanningTaskRequest(bytes.NewBufferString(test.request))
			if err != nil {
				t.Fatalf("DecodePlanningTaskRequest(): %v", err)
			}
			result, err := RunPlanningTask(context.Background(), Options{DatabasePath: databasePath}, request)
			if err != nil {
				t.Fatalf("RunPlanningTask(): %v", err)
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

func TestRunPlanningTaskReminderCreateQueryDismissAndClear(t *testing.T) {
	t.Parallel()

	options := testOptions(t)
	ctx := context.Background()

	task, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:       PlanningTaskActionCreateTask,
		CalendarName: "Personal",
		Title:        "Take medicine",
		DueAt:        "2026-04-16T10:00:00Z",
		Reminders:    []ReminderRuleRequest{{BeforeMinutes: 60}},
	})
	if err != nil {
		t.Fatalf("create task reminder: %v", err)
	}
	if task.Rejected || len(task.Tasks) != 1 || len(task.Tasks[0].Reminders) != 1 || task.Tasks[0].Reminders[0].BeforeMinutes != 60 {
		t.Fatalf("task result = %#v, want stored reminder", task)
	}

	pending, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action: PlanningTaskActionListReminders,
		From:   "2026-04-16T08:00:00Z",
		To:     "2026-04-16T10:00:00Z",
	})
	if err != nil {
		t.Fatalf("list pending reminders: %v", err)
	}
	if pending.Rejected || len(pending.Reminders) != 1 {
		t.Fatalf("pending result = %#v, want one reminder", pending)
	}
	if pending.Reminders[0].Title != "Take medicine" || pending.Reminders[0].RemindAt != "2026-04-16T09:00:00Z" || pending.Reminders[0].DueAt != "2026-04-16T10:00:00Z" {
		t.Fatalf("pending reminder = %#v, want one hour before task due", pending.Reminders[0])
	}
	if pending.Reminders[0].ReminderOccurrenceID == "" || pending.Reminders[0].ReminderOccurrenceID != pending.Reminders[0].ID {
		t.Fatalf("pending reminder occurrence id = %#v, want documented occurrence id", pending.Reminders[0])
	}

	dismissed, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:               PlanningTaskActionDismissReminder,
		ReminderOccurrenceID: pending.Reminders[0].ReminderOccurrenceID,
	})
	if err != nil {
		t.Fatalf("dismiss reminder: %v", err)
	}
	if dismissed.Rejected || len(dismissed.Writes) != 1 || dismissed.Writes[0].Kind != "reminder_dismissal" || dismissed.Writes[0].Status != "dismissed" {
		t.Fatalf("dismissed result = %#v, want dismissed write", dismissed)
	}

	repeated, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:               PlanningTaskActionDismissReminder,
		ReminderOccurrenceID: pending.Reminders[0].ReminderOccurrenceID,
	})
	if err != nil {
		t.Fatalf("dismiss reminder again: %v", err)
	}
	if repeated.Rejected || repeated.Writes[0].Status != "already_dismissed" {
		t.Fatalf("repeat dismissal = %#v, want already_dismissed", repeated)
	}

	afterDismiss, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action: PlanningTaskActionListReminders,
		From:   "2026-04-16T08:00:00Z",
		To:     "2026-04-16T10:00:00Z",
	})
	if err != nil {
		t.Fatalf("list pending after dismiss: %v", err)
	}
	if afterDismiss.Rejected || len(afterDismiss.Reminders) != 0 {
		t.Fatalf("after dismiss = %#v, want no pending reminders", afterDismiss)
	}

	event, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:       PlanningTaskActionCreateEvent,
		CalendarName: "Work",
		Title:        "Standup",
		StartAt:      "2026-04-16T11:00:00Z",
		Reminders:    []ReminderRuleRequest{{BeforeMinutes: 30}},
	})
	if err != nil {
		t.Fatalf("create event reminder: %v", err)
	}
	if event.Rejected || len(event.Events[0].Reminders) != 1 {
		t.Fatalf("event result = %#v, want reminder", event)
	}

	eventUpdateJSON := `{"action":"update_event","event_id":"` + event.Events[0].ID + `","reminders":null}`
	eventUpdate, err := DecodePlanningTaskRequest(bytes.NewBufferString(eventUpdateJSON))
	if err != nil {
		t.Fatalf("decode event reminder clear: %v", err)
	}
	updated, err := RunPlanningTask(ctx, options, eventUpdate)
	if err != nil {
		t.Fatalf("clear event reminder: %v", err)
	}
	if updated.Rejected || len(updated.Events[0].Reminders) != 0 {
		t.Fatalf("updated event = %#v, want reminders cleared", updated)
	}
}

func TestRunPlanningTaskDirectReminderUpdates(t *testing.T) {
	t.Parallel()

	options := testOptions(t)
	ctx := context.Background()

	task, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:       PlanningTaskActionCreateTask,
		CalendarName: "Personal",
		Title:        "Call pharmacy",
		DueAt:        "2026-04-16T10:00:00Z",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if task.Rejected || len(task.Tasks) != 1 {
		t.Fatalf("task result = %#v, want stored task", task)
	}

	updatedTask, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:    PlanningTaskActionUpdateTask,
		TaskID:    task.Tasks[0].ID,
		Reminders: []ReminderRuleRequest{{BeforeMinutes: 45}},
	})
	if err != nil {
		t.Fatalf("update task reminders: %v", err)
	}
	if updatedTask.Rejected || len(updatedTask.Tasks) != 1 || len(updatedTask.Tasks[0].Reminders) != 1 || updatedTask.Tasks[0].Reminders[0].BeforeMinutes != 45 {
		t.Fatalf("updated task = %#v, want direct reminder update", updatedTask)
	}

	event, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:       PlanningTaskActionCreateEvent,
		CalendarName: "Work",
		Title:        "Check-in",
		StartAt:      "2026-04-16T11:00:00Z",
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	if event.Rejected || len(event.Events) != 1 {
		t.Fatalf("event result = %#v, want stored event", event)
	}

	updatedEvent, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:    PlanningTaskActionUpdateEvent,
		EventID:   event.Events[0].ID,
		Reminders: []ReminderRuleRequest{{BeforeMinutes: 15}},
	})
	if err != nil {
		t.Fatalf("update event reminders: %v", err)
	}
	if updatedEvent.Rejected || len(updatedEvent.Events) != 1 || len(updatedEvent.Events[0].Reminders) != 1 || updatedEvent.Events[0].Reminders[0].BeforeMinutes != 15 {
		t.Fatalf("updated event = %#v, want direct reminder update", updatedEvent)
	}
}

func TestRunPlanningTaskRecurringReminderPendingOccurrences(t *testing.T) {
	t.Parallel()

	options := testOptions(t)
	ctx := context.Background()
	count := int32(3)

	task, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:       PlanningTaskActionCreateTask,
		CalendarName: "Personal",
		Title:        "Daily review",
		DueDate:      "2026-04-16",
		Recurrence:   &RecurrenceRuleRequest{Frequency: "daily", Count: &count},
		Reminders:    []ReminderRuleRequest{{BeforeMinutes: 30}},
	})
	if err != nil {
		t.Fatalf("create recurring task reminder: %v", err)
	}
	if task.Rejected {
		t.Fatalf("task result = %#v, want success", task)
	}

	pending, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action: PlanningTaskActionListReminders,
		From:   "2026-04-15T23:00:00Z",
		To:     "2026-04-18T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("list recurring pending reminders: %v", err)
	}
	if pending.Rejected || len(pending.Reminders) != 3 {
		t.Fatalf("pending result = %#v, want three recurring reminders", pending)
	}
	if pending.Reminders[0].DueDate != "2026-04-16" || pending.Reminders[0].RemindAt != "2026-04-15T23:30:00Z" {
		t.Fatalf("first pending reminder = %#v, want UTC midnight minus offset", pending.Reminders[0])
	}
	if pending.Reminders[2].DueDate != "2026-04-18" {
		t.Fatalf("last pending reminder = %#v, want 2026-04-18 occurrence", pending.Reminders[2])
	}
}

func TestRunPlanningTaskEventTaskLinksCreateListDeleteAndExposeIDs(t *testing.T) {
	t.Parallel()

	options := testOptions(t)
	ctx := context.Background()

	event, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:       PlanningTaskActionCreateEvent,
		CalendarName: "Work",
		Title:        "Planning",
		StartDate:    "2026-04-16",
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	if event.Rejected || len(event.Events) != 1 {
		t.Fatalf("event result = %#v, want stored event", event)
	}

	task, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:       PlanningTaskActionCreateTask,
		CalendarName: "Work",
		Title:        "Prep notes",
		DueDate:      "2026-04-16",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if task.Rejected || len(task.Tasks) != 1 {
		t.Fatalf("task result = %#v, want stored task", task)
	}

	created, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:  PlanningTaskActionCreateEventTaskLink,
		EventID: event.Events[0].ID,
		TaskID:  task.Tasks[0].ID,
	})
	if err != nil {
		t.Fatalf("create event task link: %v", err)
	}
	if created.Rejected || len(created.EventTaskLinks) != 1 || created.EventTaskLinks[0].EventID != event.Events[0].ID || created.EventTaskLinks[0].TaskID != task.Tasks[0].ID {
		t.Fatalf("created link = %#v, want event-task link", created)
	}

	listed, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action: PlanningTaskActionListEventTaskLinks,
	})
	if err != nil {
		t.Fatalf("list links: %v", err)
	}
	if listed.Rejected || len(listed.EventTaskLinks) != 1 {
		t.Fatalf("listed links = %#v, want one link", listed)
	}
	filteredByEvent, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:  PlanningTaskActionListEventTaskLinks,
		EventID: event.Events[0].ID,
	})
	if err != nil {
		t.Fatalf("list links by event: %v", err)
	}
	if filteredByEvent.Rejected || len(filteredByEvent.EventTaskLinks) != 1 {
		t.Fatalf("event-filtered links = %#v, want one link", filteredByEvent)
	}
	filteredByTask, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action: PlanningTaskActionListEventTaskLinks,
		TaskID: task.Tasks[0].ID,
	})
	if err != nil {
		t.Fatalf("list links by task: %v", err)
	}
	if filteredByTask.Rejected || len(filteredByTask.EventTaskLinks) != 1 {
		t.Fatalf("task-filtered links = %#v, want one link", filteredByTask)
	}

	limit := 10
	events, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action: PlanningTaskActionListEvents,
		Limit:  &limit,
	})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if events.Rejected || len(events.Events) != 1 || !slices.Equal(events.Events[0].LinkedTaskIDs, []string{task.Tasks[0].ID}) {
		t.Fatalf("events = %#v, want linked task id", events)
	}
	tasks, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action: PlanningTaskActionListTasks,
		Limit:  &limit,
	})
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if tasks.Rejected || len(tasks.Tasks) != 1 || !slices.Equal(tasks.Tasks[0].LinkedEventIDs, []string{event.Events[0].ID}) {
		t.Fatalf("tasks = %#v, want linked event id", tasks)
	}

	agenda, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action: PlanningTaskActionListAgenda,
		From:   "2026-04-16T00:00:00Z",
		To:     "2026-04-17T00:00:00Z",
		Limit:  &limit,
	})
	if err != nil {
		t.Fatalf("list agenda: %v", err)
	}
	if agenda.Rejected || len(agenda.Agenda) != 2 {
		t.Fatalf("agenda = %#v, want two linked items", agenda)
	}
	for _, item := range agenda.Agenda {
		switch item.Kind {
		case "event":
			if !slices.Equal(item.LinkedTaskIDs, []string{task.Tasks[0].ID}) {
				t.Fatalf("event agenda linked tasks = %v, want %v", item.LinkedTaskIDs, []string{task.Tasks[0].ID})
			}
		case "task":
			if !slices.Equal(item.LinkedEventIDs, []string{event.Events[0].ID}) {
				t.Fatalf("task agenda linked events = %v, want %v", item.LinkedEventIDs, []string{event.Events[0].ID})
			}
		}
	}

	deleted, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:  PlanningTaskActionDeleteEventTaskLink,
		EventID: event.Events[0].ID,
		TaskID:  task.Tasks[0].ID,
	})
	if err != nil {
		t.Fatalf("delete link: %v", err)
	}
	if deleted.Rejected || len(deleted.Writes) != 1 || deleted.Writes[0].Kind != "event_task_link" || deleted.Writes[0].Status != "deleted" {
		t.Fatalf("deleted link = %#v, want deleted write", deleted)
	}

	afterDelete, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action: PlanningTaskActionListEventTaskLinks,
	})
	if err != nil {
		t.Fatalf("list links after delete: %v", err)
	}
	if afterDelete.Rejected || len(afterDelete.EventTaskLinks) != 0 {
		t.Fatalf("links after delete = %#v, want none", afterDelete)
	}
}

func TestRunPlanningTaskEventTaskLinkRejections(t *testing.T) {
	t.Parallel()

	databasePath := filepath.Join(t.TempDir(), "nested", "openplanner.db")
	invalidCases := []PlanningTaskRequest{
		{Action: PlanningTaskActionCreateEventTaskLink},
		{Action: PlanningTaskActionCreateEventTaskLink, EventID: "not-a-ulid", TaskID: "01ARZ3NDEKTSV4RRFFQ69G5FAV"},
		{Action: PlanningTaskActionCreateEventTaskLink, EventID: "01ARZ3NDEKTSV4RRFFQ69G5FAV", TaskID: "not-a-ulid"},
		{Action: PlanningTaskActionDeleteEventTaskLink, EventID: "01ARZ3NDEKTSV4RRFFQ69G5FAV"},
		{Action: PlanningTaskActionListEventTaskLinks, EventID: "not-a-ulid"},
	}
	for index, request := range invalidCases {
		result, err := RunPlanningTask(context.Background(), Options{DatabasePath: databasePath}, request)
		if err != nil {
			t.Fatalf("RunPlanningTask(invalid %d) error = %v", index, err)
		}
		if !result.Rejected || result.RejectionReason == "" {
			t.Fatalf("result %d = %#v, want rejection", index, result)
		}
		if _, err := os.Stat(filepath.Dir(databasePath)); !os.IsNotExist(err) {
			t.Fatalf("database directory exists after validation rejection: %v", err)
		}
	}

	options := testOptions(t)
	ctx := context.Background()
	task, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:       PlanningTaskActionCreateTask,
		CalendarName: "Work",
		Title:        "Prep",
		DueDate:      "2026-04-16",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	missingEvent := "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	missingResult, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:  PlanningTaskActionCreateEventTaskLink,
		EventID: missingEvent,
		TaskID:  task.Tasks[0].ID,
	})
	if err != nil {
		t.Fatalf("create link with missing event: %v", err)
	}
	if !missingResult.Rejected || missingResult.RejectionReason == "" {
		t.Fatalf("missing event result = %#v, want rejection", missingResult)
	}
	links, err := RunPlanningTask(ctx, options, PlanningTaskRequest{Action: PlanningTaskActionListEventTaskLinks})
	if err != nil {
		t.Fatalf("list links: %v", err)
	}
	if links.Rejected || len(links.EventTaskLinks) != 0 {
		t.Fatalf("links after missing event rejection = %#v, want none", links)
	}
}

func TestRunPlanningTaskReminderRejectionsBeforeDatabaseCreation(t *testing.T) {
	t.Parallel()

	databasePath := filepath.Join(t.TempDir(), "nested", "openplanner.db")
	tests := []struct {
		name    string
		request string
	}{
		{
			name:    "non-positive reminder",
			request: `{"action":"create_task","calendar_name":"Work","title":"Review","due_date":"2026-04-16","reminders":[{"before_minutes":0}]}`,
		},
		{
			name:    "duplicate reminder",
			request: `{"action":"create_task","calendar_name":"Work","title":"Review","due_date":"2026-04-16","reminders":[{"before_minutes":30},{"before_minutes":30}]}`,
		},
		{
			name:    "task reminder missing due",
			request: `{"action":"create_task","calendar_name":"Work","title":"Review","reminders":[{"before_minutes":30}]}`,
		},
		{
			name:    "missing dismissal id",
			request: `{"action":"dismiss_reminder"}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request, err := DecodePlanningTaskRequest(bytes.NewBufferString(test.request))
			if err != nil {
				t.Fatalf("DecodePlanningTaskRequest(): %v", err)
			}
			result, err := RunPlanningTask(context.Background(), Options{DatabasePath: databasePath}, request)
			if err != nil {
				t.Fatalf("RunPlanningTask(): %v", err)
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

func TestRunPlanningTaskAttendeeRejectionsBeforeDatabaseCreation(t *testing.T) {
	t.Parallel()

	databasePath := filepath.Join(t.TempDir(), "nested", "openplanner.db")
	tests := []struct {
		name    string
		request string
	}{
		{
			name:    "missing email",
			request: `{"action":"create_event","calendar_name":"Work","title":"Planning","start_date":"2026-04-16","attendees":[{"display_name":"No Email"}]}`,
		},
		{
			name:    "invalid email",
			request: `{"action":"create_event","calendar_name":"Work","title":"Planning","start_date":"2026-04-16","attendees":[{"email":"alex example.com"}]}`,
		},
		{
			name:    "duplicate email",
			request: `{"action":"create_event","calendar_name":"Work","title":"Planning","start_date":"2026-04-16","attendees":[{"email":"Alex@example.com"},{"email":"alex@example.com"}]}`,
		},
		{
			name:    "invalid role",
			request: `{"action":"create_event","calendar_name":"Work","title":"Planning","start_date":"2026-04-16","attendees":[{"email":"alex@example.com","role":"speaker"}]}`,
		},
		{
			name:    "invalid participation status",
			request: `{"action":"create_event","calendar_name":"Work","title":"Planning","start_date":"2026-04-16","attendees":[{"email":"alex@example.com","participation_status":"maybe"}]}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request, err := DecodePlanningTaskRequest(bytes.NewBufferString(test.request))
			if err != nil {
				t.Fatalf("DecodePlanningTaskRequest(): %v", err)
			}
			result, err := RunPlanningTask(context.Background(), Options{DatabasePath: databasePath}, request)
			if err != nil {
				t.Fatalf("RunPlanningTask(): %v", err)
			}
			if !result.Rejected || result.RejectionReason == "" {
				t.Fatalf("result = %#v, want rejection", result)
			}
			if _, err := os.Stat(filepath.Dir(databasePath)); !os.IsNotExist(err) {
				t.Fatalf("database directory exists after validation rejection: %v", err)
			}
		})
	}

	if _, err := DecodePlanningTaskRequest(bytes.NewBufferString(`{"action":"create_event","calendar_name":"Work","title":"Planning","start_date":"2026-04-16","attendees":[{"email":"alex@example.com","unknown":true}]}`)); err == nil {
		t.Fatal("DecodePlanningTaskRequest() error = nil, want unknown attendee field error")
	}
	if _, err := os.Stat(filepath.Dir(databasePath)); !os.IsNotExist(err) {
		t.Fatalf("database directory exists after decode rejection: %v", err)
	}
}

func TestRunPlanningTaskDeleteEventAndTask(t *testing.T) {
	t.Parallel()

	options := testOptions(t)
	ctx := context.Background()

	event, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:       PlanningTaskActionCreateEvent,
		CalendarName: "Work",
		Title:        "Old appointment",
		StartAt:      "2026-04-16T09:00:00Z",
		EndAt:        "2026-04-16T09:30:00Z",
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	task, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:       PlanningTaskActionCreateTask,
		CalendarName: "Personal",
		Title:        "Old note",
		DueDate:      "2026-04-16",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	deletedEvent, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:  PlanningTaskActionDeleteEvent,
		EventID: event.Events[0].ID,
	})
	if err != nil {
		t.Fatalf("delete event: %v", err)
	}
	if deletedEvent.Rejected || len(deletedEvent.Writes) != 1 || deletedEvent.Writes[0].Kind != "event" || deletedEvent.Writes[0].Status != "deleted" {
		t.Fatalf("deleted event result = %#v", deletedEvent)
	}

	deletedTask, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action: PlanningTaskActionDeleteTask,
		TaskID: task.Tasks[0].ID,
	})
	if err != nil {
		t.Fatalf("delete task: %v", err)
	}
	if deletedTask.Rejected || len(deletedTask.Writes) != 1 || deletedTask.Writes[0].Kind != "task" || deletedTask.Writes[0].Status != "deleted" {
		t.Fatalf("deleted task result = %#v", deletedTask)
	}

	limit := 100
	events, err := RunPlanningTask(ctx, options, PlanningTaskRequest{Action: PlanningTaskActionListEvents, Limit: &limit})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if events.Rejected || len(events.Events) != 0 {
		t.Fatalf("events after delete = %#v, want none", events)
	}
	tasks, err := RunPlanningTask(ctx, options, PlanningTaskRequest{Action: PlanningTaskActionListTasks, Limit: &limit})
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if tasks.Rejected || len(tasks.Tasks) != 0 {
		t.Fatalf("tasks after delete = %#v, want none", tasks)
	}
}

func TestRunPlanningTaskDeleteEmptyCalendar(t *testing.T) {
	t.Parallel()

	options := testOptions(t)
	ctx := context.Background()

	calendar, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:       PlanningTaskActionEnsureCalendar,
		CalendarName: "Archive",
	})
	if err != nil {
		t.Fatalf("ensure calendar: %v", err)
	}
	if calendar.Rejected || len(calendar.Calendars) != 1 {
		t.Fatalf("calendar result = %#v", calendar)
	}

	deleted, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:       PlanningTaskActionDeleteCalendar,
		CalendarName: "Archive",
	})
	if err != nil {
		t.Fatalf("delete calendar: %v", err)
	}
	if deleted.Rejected || len(deleted.Writes) != 1 || deleted.Writes[0].Kind != "calendar" || deleted.Writes[0].Status != "deleted" {
		t.Fatalf("deleted calendar result = %#v", deleted)
	}

	recreated, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:       PlanningTaskActionEnsureCalendar,
		CalendarName: "Archive",
	})
	if err != nil {
		t.Fatalf("ensure calendar after delete: %v", err)
	}
	if recreated.Rejected || len(recreated.Writes) != 1 || recreated.Writes[0].Status != "created" {
		t.Fatalf("recreated calendar result = %#v, want created", recreated)
	}
}

func TestRunPlanningTaskDeleteNonEmptyCalendarRejects(t *testing.T) {
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
	if _, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:     PlanningTaskActionCreateEvent,
		CalendarID: calendar.Calendars[0].ID,
		Title:      "Planning",
		StartDate:  "2026-04-16",
	}); err != nil {
		t.Fatalf("create event: %v", err)
	}

	deleted, err := RunPlanningTask(ctx, options, PlanningTaskRequest{
		Action:     PlanningTaskActionDeleteCalendar,
		CalendarID: calendar.Calendars[0].ID,
	})
	if err != nil {
		t.Fatalf("delete calendar: %v", err)
	}
	if !deleted.Rejected || deleted.RejectionReason == "" {
		t.Fatalf("deleted calendar result = %#v, want rejection", deleted)
	}

	limit := 100
	events, err := RunPlanningTask(ctx, options, PlanningTaskRequest{Action: PlanningTaskActionListEvents, CalendarName: "Work", Limit: &limit})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if events.Rejected || len(events.Events) != 1 || events.Events[0].Title != "Planning" {
		t.Fatalf("events after rejected calendar delete = %#v, want event preserved", events)
	}
}

func TestRunPlanningTaskUpdateRejections(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		request string
	}{
		{
			name:    "unknown field",
			request: `{"action":"update_task","task_id":"01ARZ3NDEKTSV4RRFFQ69G5FAV","unknown":true}`,
		},
		{
			name:    "invalid event id",
			request: `{"action":"update_event","event_id":"not-a-ulid","title":"Planning"}`,
		},
		{
			name:    "clear event title",
			request: `{"action":"update_event","event_id":"01ARZ3NDEKTSV4RRFFQ69G5FAV","title":null}`,
		},
		{
			name:    "clear calendar name",
			request: `{"action":"update_calendar","calendar_id":"01ARZ3NDEKTSV4RRFFQ69G5FAV","name":null}`,
		},
		{
			name:    "no update fields",
			request: `{"action":"update_task","task_id":"01ARZ3NDEKTSV4RRFFQ69G5FAV"}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request, err := DecodePlanningTaskRequest(bytes.NewBufferString(test.request))
			if test.name == "unknown field" {
				if err == nil {
					t.Fatal("DecodePlanningTaskRequest() error = nil, want unknown field error")
				}
				return
			}
			if err != nil {
				t.Fatalf("DecodePlanningTaskRequest(): %v", err)
			}
			result, err := RunPlanningTask(context.Background(), testOptions(t), request)
			if err != nil {
				t.Fatalf("RunPlanningTask(): %v", err)
			}
			if !result.Rejected || result.RejectionReason == "" {
				t.Fatalf("result = %#v, want rejection", result)
			}
		})
	}
}

func TestRunPlanningTaskDeleteRejectionsBeforeDatabaseCreation(t *testing.T) {
	t.Parallel()

	databasePath := filepath.Join(t.TempDir(), "nested", "openplanner.db")
	tests := []struct {
		name    string
		request PlanningTaskRequest
	}{
		{
			name:    "missing calendar identifier",
			request: PlanningTaskRequest{Action: PlanningTaskActionDeleteCalendar},
		},
		{
			name: "mixed calendar identifiers",
			request: PlanningTaskRequest{
				Action:       PlanningTaskActionDeleteCalendar,
				CalendarName: "Archive",
				CalendarID:   "01ARZ3NDEKTSV4RRFFQ69G5FAV",
			},
		},
		{
			name: "invalid calendar id",
			request: PlanningTaskRequest{
				Action:     PlanningTaskActionDeleteCalendar,
				CalendarID: "not-a-ulid",
			},
		},
		{
			name:    "missing event id",
			request: PlanningTaskRequest{Action: PlanningTaskActionDeleteEvent},
		},
		{
			name: "invalid event id",
			request: PlanningTaskRequest{
				Action:  PlanningTaskActionDeleteEvent,
				EventID: "not-a-ulid",
			},
		},
		{
			name:    "missing task id",
			request: PlanningTaskRequest{Action: PlanningTaskActionDeleteTask},
		},
		{
			name: "invalid task id",
			request: PlanningTaskRequest{
				Action: PlanningTaskActionDeleteTask,
				TaskID: "not-a-ulid",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := RunPlanningTask(context.Background(), Options{DatabasePath: databasePath}, test.request)
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

func TestDecodePlanningTaskRequestRejectsUnknownRecurrenceFields(t *testing.T) {
	t.Parallel()

	_, err := DecodePlanningTaskRequest(bytes.NewBufferString(`{"action":"create_task","calendar_name":"Work","title":"Review","due_date":"2026-04-16","recurrence":{"frequency":"daily","cnt":3}}`))
	if err == nil {
		t.Fatal("DecodePlanningTaskRequest() error = nil, want unknown recurrence field error")
	}
}

func TestRunPlanningTaskUpdatePatchDateRejectionsBeforeDatabaseCreation(t *testing.T) {
	t.Parallel()

	databasePath := filepath.Join(t.TempDir(), "nested", "openplanner.db")
	tests := []struct {
		name    string
		request string
	}{
		{
			name:    "event start date",
			request: `{"action":"update_event","event_id":"01ARZ3NDEKTSV4RRFFQ69G5FAV","start_date":"04/16"}`,
		},
		{
			name:    "task due date",
			request: `{"action":"update_task","task_id":"01ARZ3NDEKTSV4RRFFQ69G5FAV","due_date":"04/16"}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request, err := DecodePlanningTaskRequest(bytes.NewBufferString(test.request))
			if err != nil {
				t.Fatalf("DecodePlanningTaskRequest(): %v", err)
			}
			result, err := RunPlanningTask(context.Background(), Options{DatabasePath: databasePath}, request)
			if err != nil {
				t.Fatalf("RunPlanningTask(): %v", err)
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
			name: "invalid time zone",
			request: PlanningTaskRequest{
				Action:       PlanningTaskActionCreateEvent,
				CalendarName: "Work",
				Title:        "Planning",
				StartAt:      "2026-04-16T09:00:00Z",
				TimeZone:     stringPtr("Not/AZone"),
			},
		},
		{
			name: "time zone offset mismatch",
			request: PlanningTaskRequest{
				Action:       PlanningTaskActionCreateEvent,
				CalendarName: "Work",
				Title:        "Planning",
				StartAt:      "2026-03-10T09:00:00-05:00",
				TimeZone:     stringPtr("America/New_York"),
			},
		},
		{
			name: "all-day time zone",
			request: PlanningTaskRequest{
				Action:       PlanningTaskActionCreateEvent,
				CalendarName: "Work",
				Title:        "Planning",
				StartDate:    "2026-04-16",
				TimeZone:     stringPtr("UTC"),
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
			name: "duplicate recurrence weekday",
			request: PlanningTaskRequest{
				Action:       PlanningTaskActionCreateTask,
				CalendarName: "Work",
				Title:        "Review",
				DueDate:      "2026-04-16",
				Recurrence:   &RecurrenceRuleRequest{Frequency: "weekly", ByWeekday: []string{"MO", "MO"}},
			},
		},
		{
			name: "count with until recurrence",
			request: PlanningTaskRequest{
				Action:       PlanningTaskActionCreateTask,
				CalendarName: "Work",
				Title:        "Review",
				DueDate:      "2026-04-16",
				Recurrence:   &RecurrenceRuleRequest{Frequency: "daily", Count: int32Ptr(2), UntilDate: "2026-04-20"},
			},
		},
		{
			name: "occurrence action missing selector",
			request: PlanningTaskRequest{
				Action:  PlanningTaskActionCancelTaskOccurrence,
				TaskID:  "01ARZ3NDEKTSV4RRFFQ69G5FAV",
				DueDate: "2026-04-18",
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
			result, err := RunPlanningTask(context.Background(), Options{DatabasePath: databasePath}, test.request)
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

func testOptions(t *testing.T) Options {
	t.Helper()
	return Options{DatabasePath: filepath.Join(t.TempDir(), "openplanner.db")}
}

func intPtr(value int) *int {
	return &value
}

func int32Ptr(value int32) *int32 {
	return &value
}

func stringPtr(value string) *string {
	return &value
}
