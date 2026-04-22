package icalendar

import (
	"strings"
	"testing"
	"time"

	"github.com/yazanabuashour/openplanner/internal/domain"
)

func TestBuildEscapesFoldsAndUsesCRLF(t *testing.T) {
	longTitle := strings.Repeat("Planning, sync; review\\notes ", 4)
	event := domain.Event{
		ID:         "event-1",
		CalendarID: "calendar-1",
		Title:      longTitle,
		StartAt:    timePtr(time.Date(2026, 4, 16, 9, 0, 0, 0, time.UTC)),
		CreatedAt:  fixedTime(),
		UpdatedAt:  fixedTime(),
	}

	result := Build(Export{
		Calendars:   []domain.Calendar{{ID: "calendar-1", Name: "Work"}},
		Events:      []domain.Event{event},
		GeneratedAt: fixedTime(),
		Name:        "Work",
	})

	if strings.Contains(result.Content, "\n") && !strings.Contains(result.Content, "\r\n") {
		t.Fatalf("content uses bare LF: %q", result.Content)
	}
	if !strings.Contains(result.Content, "\r\n ") {
		t.Fatalf("content = %q, want folded line", result.Content)
	}
	unfolded := unfold(result.Content)
	if !strings.Contains(unfolded, `SUMMARY:Planning\, sync\; review\\notes`) {
		t.Fatalf("unfolded content = %q, want escaped summary", unfolded)
	}
}

func TestBuildEventTaskRecurrenceExceptionsRemindersAndAttendees(t *testing.T) {
	count := int32(3)
	timeZone := "America/New_York"
	eventStart := time.Date(2026, 3, 3, 9, 0, 0, 0, fixedZone(-5*60*60))
	eventEnd := time.Date(2026, 3, 3, 10, 0, 0, 0, fixedZone(-5*60*60))
	event := domain.Event{
		ID:          "event-1",
		CalendarID:  "calendar-1",
		Title:       "Weekly sync",
		Description: stringPtr("Discuss launch"),
		Location:    stringPtr("Room 1"),
		StartAt:     &eventStart,
		EndAt:       &eventEnd,
		TimeZone:    &timeZone,
		Recurrence: &domain.RecurrenceRule{
			Frequency: domain.RecurrenceFrequencyWeekly,
			Count:     &count,
			ByWeekday: []domain.Weekday{domain.WeekdayTuesday},
		},
		Reminders: []domain.ReminderRule{{ID: "reminder-1", BeforeMinutes: 30}},
		Attendees: []domain.EventAttendee{{
			Email:               "alex@example.com",
			DisplayName:         stringPtr("Alex Rivera"),
			Role:                domain.EventAttendeeRoleOptional,
			ParticipationStatus: domain.EventParticipationStatusAccepted,
			RSVP:                true,
		}},
		CreatedAt: fixedTime(),
		UpdatedAt: fixedTime(),
	}
	cancelledAt := time.Date(2026, 3, 10, 9, 0, 0, 0, fixedZone(-4*60*60))
	movedAt := time.Date(2026, 3, 17, 11, 0, 0, 0, fixedZone(-4*60*60))
	eventStates := map[string]map[string]domain.OccurrenceState{
		event.ID: {
			"event-1:20260310T090000": {
				OwnerKind:     domain.OccurrenceOwnerKindEvent,
				OwnerID:       event.ID,
				OccurrenceKey: "event-1:20260310T090000",
				OccurrenceAt:  &cancelledAt,
				Cancelled:     true,
			},
			"event-1:20260317T090000": {
				OwnerKind:     domain.OccurrenceOwnerKindEvent,
				OwnerID:       event.ID,
				OccurrenceKey: "event-1:20260317T090000",
				OccurrenceAt:  timePtr(time.Date(2026, 3, 17, 9, 0, 0, 0, fixedZone(-4*60*60))),
				ReplacementAt: &movedAt,
			},
		},
	}

	task := domain.Task{
		ID:         "task-1",
		CalendarID: "calendar-1",
		Title:      "Review notes",
		DueDate:    stringPtr("2026-04-16"),
		Recurrence: &domain.RecurrenceRule{
			Frequency:  domain.RecurrenceFrequencyMonthly,
			Interval:   1,
			ByMonthDay: []int32{16},
			Count:      &count,
		},
		Reminders: []domain.ReminderRule{{ID: "reminder-2", BeforeMinutes: 60}},
		Priority:  domain.TaskPriorityHigh,
		Status:    domain.TaskStatusInProgress,
		Tags:      []string{"planning", "review"},
		CreatedAt: fixedTime(),
		UpdatedAt: fixedTime(),
	}
	completion := domain.TaskCompletion{
		TaskID:         task.ID,
		OccurrenceKey:  "task-1:20260416",
		OccurrenceDate: stringPtr("2026-04-16"),
		CompletedAt:    fixedTime(),
	}
	movedTaskState := domain.OccurrenceState{
		OwnerKind:       domain.OccurrenceOwnerKindTask,
		OwnerID:         task.ID,
		OccurrenceKey:   "task-1:20260516",
		OccurrenceDate:  stringPtr("2026-05-16"),
		ReplacementDate: stringPtr("2026-05-17"),
	}
	movedCompletion := domain.TaskCompletion{
		TaskID:         task.ID,
		OccurrenceKey:  movedTaskState.OccurrenceKey,
		OccurrenceDate: stringPtr("2026-05-16"),
		CompletedAt:    fixedTime(),
	}

	result := Build(Export{
		Calendars:             []domain.Calendar{{ID: "calendar-1", Name: "Work"}},
		Events:                []domain.Event{event},
		Tasks:                 []domain.Task{task},
		EventOccurrenceStates: eventStates,
		TaskOccurrenceStates:  map[string]map[string]domain.OccurrenceState{task.ID: {movedTaskState.OccurrenceKey: movedTaskState}},
		TaskCompletions: map[string]map[string]domain.TaskCompletion{task.ID: {
			completion.OccurrenceKey:      completion,
			movedCompletion.OccurrenceKey: movedCompletion,
		}},
		GeneratedAt: fixedTime(),
		Name:        "Work",
	})
	unfolded := unfold(result.Content)

	assertContains(t, unfolded, "BEGIN:VEVENT\r\nUID:event-1@openplanner.local")
	assertContains(t, unfolded, "DTSTART;TZID=America/New_York:20260303T090000")
	assertContains(t, unfolded, "DTEND;TZID=America/New_York:20260303T100000")
	assertContains(t, unfolded, "RRULE:FREQ=WEEKLY;COUNT=3;BYDAY=TU")
	assertContains(t, unfolded, "EXDATE;TZID=America/New_York:20260310T090000")
	assertContains(t, unfolded, "RECURRENCE-ID;TZID=America/New_York:20260317T090000")
	assertContains(t, unfolded, "ATTENDEE;ROLE=OPT-PARTICIPANT;PARTSTAT=ACCEPTED;RSVP=TRUE;CN=\"Alex Rivera\":mailto:alex@example.com")
	assertContains(t, unfolded, "TRIGGER:-PT30M")
	assertContains(t, unfolded, "BEGIN:VTODO")
	assertContains(t, unfolded, "DUE;VALUE=DATE:20260416")
	assertContains(t, unfolded, "RRULE:FREQ=MONTHLY;INTERVAL=1;COUNT=3;BYMONTHDAY=16")
	assertContains(t, unfolded, "STATUS:IN-PROCESS")
	assertContains(t, unfolded, "PRIORITY:1")
	assertContains(t, unfolded, "CATEGORIES:planning,review")
	assertContains(t, unfolded, "TRIGGER;RELATED=END:-PT60M")
	assertContains(t, unfolded, "RECURRENCE-ID;VALUE=DATE:20260416")
	assertContains(t, unfolded, "STATUS:COMPLETED")
	assertContains(t, unfolded, "COMPLETED:20260416T120000Z")
	assertContains(t, unfolded, "EXDATE;VALUE=DATE:20260516")
	assertContains(t, unfolded, "RECURRENCE-ID;VALUE=DATE:20260516\r\nSUMMARY:Review notes\r\nDUE;VALUE=DATE:20260517\r\nSTATUS:COMPLETED")
}

func TestBuildAllDayEventUsesExclusiveDTEND(t *testing.T) {
	event := domain.Event{
		ID:         "event-1",
		CalendarID: "calendar-1",
		Title:      "Retreat",
		StartDate:  stringPtr("2026-04-16"),
		EndDate:    stringPtr("2026-04-17"),
		CreatedAt:  fixedTime(),
		UpdatedAt:  fixedTime(),
	}

	result := Build(Export{
		Calendars:   []domain.Calendar{{ID: "calendar-1", Name: "Work"}},
		Events:      []domain.Event{event},
		GeneratedAt: fixedTime(),
	})
	unfolded := unfold(result.Content)

	assertContains(t, unfolded, "DTSTART;VALUE=DATE:20260416")
	assertContains(t, unfolded, "DTEND;VALUE=DATE:20260418")
}

func unfold(content string) string {
	return strings.ReplaceAll(content, "\r\n ", "")
}

func assertContains(t *testing.T, content string, expected string) {
	t.Helper()
	if !strings.Contains(content, expected) {
		t.Fatalf("content missing %q:\n%s", expected, content)
	}
}

func fixedTime() time.Time {
	return time.Date(2026, 4, 16, 12, 0, 0, 0, time.UTC)
}

func fixedZone(offset int) *time.Location {
	return time.FixedZone("", offset)
}

func timePtr(value time.Time) *time.Time {
	return &value
}

func stringPtr(value string) *string {
	return &value
}
