package icalendar

import (
	"os"
	"path/filepath"
	"slices"
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

func TestParseImportMapsSupportedEventTaskAndExceptions(t *testing.T) {
	content := strings.Join([]string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"X-WR-CALNAME:Work",
		"BEGIN:VEVENT",
		"UID:event-1@example.com",
		"SUMMARY:Weekly sync",
		"DESCRIPTION:Discuss launch",
		"LOCATION:Room 1",
		"DTSTART;TZID=America/New_York:20260303T090000",
		"DTEND;TZID=America/New_York:20260303T100000",
		"RRULE:FREQ=WEEKLY;COUNT=3;BYDAY=TU",
		"EXDATE;TZID=America/New_York:20260310T090000",
		"ATTENDEE;ROLE=OPT-PARTICIPANT;PARTSTAT=ACCEPTED;RSVP=TRUE;CN=Alex Rivera:mailto:alex@example.com",
		"BEGIN:VALARM",
		"ACTION:DISPLAY",
		"TRIGGER:-PT30M",
		"END:VALARM",
		"END:VEVENT",
		"BEGIN:VEVENT",
		"UID:event-1@example.com",
		"RECURRENCE-ID;TZID=America/New_York:20260317T090000",
		"SUMMARY:Weekly sync",
		"DTSTART;TZID=America/New_York:20260317T110000",
		"DTEND;TZID=America/New_York:20260317T120000",
		"END:VEVENT",
		"BEGIN:VTODO",
		"UID:task-1@example.com",
		"SUMMARY:Review notes",
		"DUE;VALUE=DATE:20260416",
		"RRULE:FREQ=MONTHLY;COUNT=3;BYMONTHDAY=16",
		"STATUS:IN-PROCESS",
		"PRIORITY:1",
		"CATEGORIES:planning,review",
		"BEGIN:VALARM",
		"ACTION:DISPLAY",
		"TRIGGER;RELATED=END:-PT60M",
		"END:VALARM",
		"END:VTODO",
		"BEGIN:VTODO",
		"UID:task-1@example.com",
		"RECURRENCE-ID;VALUE=DATE:20260516",
		"SUMMARY:Review notes",
		"DUE;VALUE=DATE:20260517",
		"STATUS:COMPLETED",
		"COMPLETED:20260517T120000Z",
		"END:VTODO",
		"END:VCALENDAR",
	}, "\r\n")

	parsed, err := ParseImport(content)
	if err != nil {
		t.Fatalf("ParseImport(): %v", err)
	}
	if len(parsed.Skips) != 0 {
		t.Fatalf("skips = %#v, want none", parsed.Skips)
	}
	if parsed.CalendarName != "Work" || len(parsed.Events) != 1 || len(parsed.EventChanges) != 1 || len(parsed.Tasks) != 1 || len(parsed.TaskChanges) != 1 {
		t.Fatalf("parsed = %#v, want imported event/task and changes", parsed)
	}
	event := parsed.Events[0].Event
	if event.TimeZone == nil || *event.TimeZone != "America/New_York" || event.Recurrence == nil || len(event.Reminders) != 1 || len(event.Attendees) != 1 {
		t.Fatalf("event = %#v, want timezone recurrence reminder attendee", event)
	}
	if len(parsed.Events[0].ExDates) != 1 || parsed.Events[0].ExDates[0].At == nil {
		t.Fatalf("event exdates = %#v, want timed EXDATE", parsed.Events[0].ExDates)
	}
	task := parsed.Tasks[0].Task
	if task.DueDate == nil || *task.DueDate != "2026-04-16" || task.Priority != domain.TaskPriorityHigh || task.Status != domain.TaskStatusInProgress || !slices.Equal(task.Tags, []string{"planning", "review"}) {
		t.Fatalf("task = %#v, want mapped VTODO metadata", task)
	}
	if parsed.TaskChanges[0].CompletedAt == nil || parsed.TaskChanges[0].Replacement.DueDate == nil || *parsed.TaskChanges[0].Replacement.DueDate != "2026-05-17" {
		t.Fatalf("task change = %#v, want completion and replacement due date", parsed.TaskChanges[0])
	}
}

func TestParseImportRejectsMalformedAndSkipsUnsupportedComponents(t *testing.T) {
	if _, err := ParseImport("not an ics file"); err == nil {
		t.Fatal("ParseImport(malformed) error = nil, want error")
	}

	content := strings.Join([]string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"BEGIN:VEVENT",
		"UID:event-unsupported@example.com",
		"SUMMARY:Unsupported",
		"DTSTART:20260416T090000",
		"RRULE:FREQ=HOURLY;COUNT=2",
		"END:VEVENT",
		"BEGIN:VEVENT",
		"UID:event-allday@example.com",
		"SUMMARY:Retreat",
		"DTSTART;VALUE=DATE:20260416",
		"DTEND;VALUE=DATE:20260418",
		"END:VEVENT",
		"END:VCALENDAR",
	}, "\r\n")

	parsed, err := ParseImport(content)
	if err != nil {
		t.Fatalf("ParseImport(): %v", err)
	}
	if len(parsed.Events) != 1 || parsed.Events[0].Event.EndDate == nil || *parsed.Events[0].Event.EndDate != "2026-04-17" {
		t.Fatalf("events = %#v, want imported all-day event with inclusive end date", parsed.Events)
	}
	if len(parsed.Skips) != 1 || parsed.Skips[0].UID != "event-unsupported@example.com" {
		t.Fatalf("skips = %#v, want unsupported event skip", parsed.Skips)
	}
}

func TestParseImportProviderFixtures(t *testing.T) {
	for _, name := range []string{"google.ics", "apple.ics", "microsoft.ics"} {
		t.Run(name, func(t *testing.T) {
			content, err := os.ReadFile(filepath.Join("testdata", "import", name))
			if err != nil {
				t.Fatalf("ReadFile(): %v", err)
			}
			parsed, err := ParseImport(string(content))
			if err != nil {
				t.Fatalf("ParseImport(): %v", err)
			}
			if len(parsed.Events)+len(parsed.Tasks) == 0 {
				t.Fatalf("parsed = %#v, want at least one imported component", parsed)
			}
			if len(parsed.Skips) != 0 {
				t.Fatalf("skips = %#v, want provider fixture without skips", parsed.Skips)
			}
		})
	}
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
