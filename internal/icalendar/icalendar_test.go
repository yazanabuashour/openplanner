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
	tests := []struct {
		name  string
		check func(*testing.T, ParsedImport)
	}{
		{
			name: "google.ics",
			check: func(t *testing.T, parsed ParsedImport) {
				t.Helper()
				if parsed.CalendarName != "Google Work" || parsed.CalendarColor == nil || *parsed.CalendarColor != "#2952A3" {
					t.Fatalf("calendar metadata = %#v, color %#v", parsed.CalendarName, parsed.CalendarColor)
				}
				if len(parsed.Events) != 1 || len(parsed.EventChanges) != 1 || len(parsed.Tasks) != 0 || len(parsed.TaskChanges) != 0 || len(parsed.Skips) != 0 {
					t.Fatalf("parsed = %#v, want one event, one event change, no tasks or skips", parsed)
				}
				event := parsed.Events[0]
				if event.UID != "google-weekly-sync@example.com" ||
					event.Event.Title != "Google weekly sync" ||
					event.Event.StartAt == nil ||
					event.Event.StartAt.Format(time.RFC3339) != "2026-04-20T09:00:00-05:00" ||
					event.Event.EndAt == nil ||
					event.Event.EndAt.Format(time.RFC3339) != "2026-04-20T09:30:00-05:00" ||
					event.Event.TimeZone == nil ||
					*event.Event.TimeZone != "America/Chicago" ||
					event.Event.Description == nil ||
					*event.Event.Description != "Imported from a synthesized Google Calendar export." ||
					event.Event.Location == nil ||
					*event.Event.Location != "Conference Room Blue" {
					t.Fatalf("google event = %#v", event.Event)
				}
				if event.Event.Recurrence == nil ||
					event.Event.Recurrence.Frequency != domain.RecurrenceFrequencyWeekly ||
					event.Event.Recurrence.Count == nil ||
					*event.Event.Recurrence.Count != 3 ||
					!slices.Equal(event.Event.Recurrence.ByWeekday, []domain.Weekday{domain.WeekdayMonday}) {
					t.Fatalf("google recurrence = %#v", event.Event.Recurrence)
				}
				if len(event.ExDates) != 1 || event.ExDates[0].At == nil || event.ExDates[0].At.Format(time.RFC3339) != "2026-04-27T09:00:00-05:00" {
					t.Fatalf("google exdates = %#v", event.ExDates)
				}
				if len(event.Event.Reminders) != 1 || event.Event.Reminders[0].BeforeMinutes != 10 {
					t.Fatalf("google reminders = %#v", event.Event.Reminders)
				}
				if len(event.Event.Attendees) != 2 ||
					event.Event.Attendees[0].Email != "alex@example.com" ||
					event.Event.Attendees[0].DisplayName == nil ||
					*event.Event.Attendees[0].DisplayName != "Alex Rivera" ||
					event.Event.Attendees[0].Role != domain.EventAttendeeRoleRequired ||
					event.Event.Attendees[0].ParticipationStatus != domain.EventParticipationStatusAccepted ||
					!event.Event.Attendees[0].RSVP ||
					event.Event.Attendees[1].Email != "sam@example.com" ||
					event.Event.Attendees[1].Role != domain.EventAttendeeRoleOptional ||
					event.Event.Attendees[1].ParticipationStatus != domain.EventParticipationStatusTentative {
					t.Fatalf("google attendees = %#v", event.Event.Attendees)
				}
				change := parsed.EventChanges[0]
				if change.UID != event.UID ||
					change.Recurrence.At == nil ||
					change.Recurrence.At.Format(time.RFC3339) != "2026-05-04T09:00:00-05:00" ||
					change.Replacement.StartAt == nil ||
					change.Replacement.StartAt.Format(time.RFC3339) != "2026-05-04T11:00:00-05:00" {
					t.Fatalf("google change = %#v", change)
				}
			},
		},
		{
			name: "apple.ics",
			check: func(t *testing.T, parsed ParsedImport) {
				t.Helper()
				if parsed.CalendarName != "Apple Personal" || parsed.CalendarColor == nil || *parsed.CalendarColor != "#63DA38" {
					t.Fatalf("calendar metadata = %#v, color %#v", parsed.CalendarName, parsed.CalendarColor)
				}
				if len(parsed.Events) != 2 || len(parsed.EventChanges) != 1 || len(parsed.Tasks) != 0 || len(parsed.TaskChanges) != 0 || len(parsed.Skips) != 0 {
					t.Fatalf("parsed = %#v, want two events, one event change, no tasks or skips", parsed)
				}
				retreat := parsed.Events[0].Event
				if retreat.Title != "Apple retreat" ||
					retreat.StartDate == nil ||
					*retreat.StartDate != "2026-04-21" ||
					retreat.EndDate == nil ||
					*retreat.EndDate != "2026-04-22" ||
					retreat.Location == nil ||
					*retreat.Location != "Lodge" {
					t.Fatalf("apple retreat = %#v", retreat)
				}
				focus := parsed.Events[1]
				if focus.UID != "apple-focus@example.com" ||
					focus.Event.Title != "Apple focus day" ||
					focus.Event.StartDate == nil ||
					*focus.Event.StartDate != "2026-05-01" ||
					focus.Event.Recurrence == nil ||
					focus.Event.Recurrence.Frequency != domain.RecurrenceFrequencyDaily ||
					focus.Event.Recurrence.Count == nil ||
					*focus.Event.Recurrence.Count != 3 {
					t.Fatalf("apple focus = %#v", focus.Event)
				}
				if len(focus.ExDates) != 1 || focus.ExDates[0].Date == nil || *focus.ExDates[0].Date != "2026-05-02" {
					t.Fatalf("apple exdates = %#v", focus.ExDates)
				}
				change := parsed.EventChanges[0]
				if change.UID != focus.UID ||
					change.Recurrence.Date == nil ||
					*change.Recurrence.Date != "2026-05-03" ||
					change.Replacement.StartDate == nil ||
					*change.Replacement.StartDate != "2026-05-04" {
					t.Fatalf("apple change = %#v", change)
				}
			},
		},
		{
			name: "microsoft.ics",
			check: func(t *testing.T, parsed ParsedImport) {
				t.Helper()
				if parsed.CalendarName != "Microsoft Migration" || parsed.CalendarColor == nil || *parsed.CalendarColor != "#D13438" {
					t.Fatalf("calendar metadata = %#v, color %#v", parsed.CalendarName, parsed.CalendarColor)
				}
				if len(parsed.Events) != 1 || len(parsed.EventChanges) != 0 || len(parsed.Tasks) != 1 || len(parsed.TaskChanges) != 0 || len(parsed.Skips) != 0 {
					t.Fatalf("parsed = %#v, want one event, one task, no changes or skips", parsed)
				}
				event := parsed.Events[0].Event
				if event.Title != "Outlook project review" ||
					event.StartAt == nil ||
					event.StartAt.Format(time.RFC3339) != "2026-04-22T15:00:00Z" ||
					event.EndAt == nil ||
					event.EndAt.Format(time.RFC3339) != "2026-04-22T16:00:00Z" ||
					event.Location == nil ||
					*event.Location != "Teams Meeting" ||
					len(event.Reminders) != 1 ||
					event.Reminders[0].BeforeMinutes != 15 {
					t.Fatalf("microsoft event = %#v", event)
				}
				task := parsed.Tasks[0].Task
				if task.Title != "Review Outlook notes" ||
					task.DueAt == nil ||
					task.DueAt.Format(time.RFC3339) != "2026-04-23T15:00:00Z" ||
					task.Priority != domain.TaskPriorityHigh ||
					task.Status != domain.TaskStatusInProgress ||
					!slices.Equal(task.Tags, []string{"planning", "review"}) ||
					len(task.Reminders) != 1 ||
					task.Reminders[0].BeforeMinutes != 60 {
					t.Fatalf("microsoft task = %#v", task)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			content, err := os.ReadFile(filepath.Join("testdata", "import", test.name))
			if err != nil {
				t.Fatalf("ReadFile(): %v", err)
			}
			parsed, err := ParseImport(string(content))
			if err != nil {
				t.Fatalf("ParseImport(): %v", err)
			}
			test.check(t, parsed)
		})
	}
}

func TestProviderFixtureFilesAreSafeToCommit(t *testing.T) {
	fixtureDir := filepath.Join("testdata", "import")
	forbidden := []string{"/Users/", "/home/", "/tmp/", "/var/folders/"}
	err := filepath.WalkDir(fixtureDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || (!strings.HasSuffix(path, ".ics") && !strings.HasSuffix(path, ".md")) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(data)
		for _, marker := range forbidden {
			if strings.Contains(text, marker) {
				t.Fatalf("%s contains machine-absolute path marker %q", path, marker)
			}
		}
		if strings.Contains(text, "@") && !strings.Contains(text, "example.com") {
			t.Fatalf("%s contains an email-like marker but no example.com address", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan provider fixtures: %v", err)
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
