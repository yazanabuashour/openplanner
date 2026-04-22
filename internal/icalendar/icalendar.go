package icalendar

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/yazanabuashour/openplanner/internal/domain"
)

const (
	ContentType = "text/calendar; charset=utf-8"
	productID   = "-//OpenPlanner//OpenPlanner//EN"
)

type Export struct {
	Calendars             []domain.Calendar
	Events                []domain.Event
	Tasks                 []domain.Task
	EventOccurrenceStates map[string]map[string]domain.OccurrenceState
	TaskOccurrenceStates  map[string]map[string]domain.OccurrenceState
	TaskCompletions       map[string]map[string]domain.TaskCompletion
	GeneratedAt           time.Time
	Name                  string
}

type Result struct {
	ContentType string
	Content     string
	EventCount  int
	TaskCount   int
}

func Build(input Export) Result {
	writer := newWriter()
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = "OpenPlanner"
	}
	generatedAt := input.GeneratedAt.UTC()
	if generatedAt.IsZero() {
		generatedAt = time.Now().UTC()
	}

	writer.property("BEGIN", "VCALENDAR")
	writer.property("VERSION", "2.0")
	writer.property("PRODID", productID)
	writer.property("CALSCALE", "GREGORIAN")
	writer.property("X-WR-CALNAME", escapeText(name))

	calendars := calendarIndex(input.Calendars)
	events := sortedEvents(input.Events)
	for _, event := range events {
		eventStates := input.EventOccurrenceStates[event.ID]
		writeEvent(writer, event, calendars[event.CalendarID], eventStates, generatedAt, nil)
		for _, state := range sortedOccurrenceStates(eventStates) {
			if state.Cancelled || !eventStateHasReplacement(state) {
				continue
			}
			writeEvent(writer, event, calendars[event.CalendarID], eventStates, generatedAt, &state)
		}
	}

	tasks := sortedTasks(input.Tasks)
	for _, task := range tasks {
		taskStates := input.TaskOccurrenceStates[task.ID]
		completions := input.TaskCompletions[task.ID]
		writeTask(writer, task, calendars[task.CalendarID], taskStates, completions, generatedAt, nil, nil)
		for _, state := range sortedOccurrenceStates(taskStates) {
			if state.Cancelled || !taskStateHasReplacement(state) {
				continue
			}
			completion, completed := completions[state.OccurrenceKey]
			if completed {
				writeTask(writer, task, calendars[task.CalendarID], taskStates, completions, generatedAt, &state, &completion)
				continue
			}
			writeTask(writer, task, calendars[task.CalendarID], taskStates, completions, generatedAt, &state, nil)
		}
		for _, completion := range sortedCompletions(completions) {
			if task.Recurrence == nil {
				continue
			}
			if state, ok := taskStates[completion.OccurrenceKey]; ok && taskStateHasReplacement(state) {
				continue
			}
			writeTask(writer, task, calendars[task.CalendarID], taskStates, completions, generatedAt, nil, &completion)
		}
	}

	writer.property("END", "VCALENDAR")
	return Result{
		ContentType: ContentType,
		Content:     writer.String(),
		EventCount:  len(events),
		TaskCount:   len(tasks),
	}
}

func writeEvent(writer *writer, event domain.Event, calendar *domain.Calendar, states map[string]domain.OccurrenceState, generatedAt time.Time, override *domain.OccurrenceState) {
	writer.property("BEGIN", "VEVENT")
	writer.property("UID", uid(event.ID, event.ICalendarUID))
	writer.property("DTSTAMP", formatUTC(generatedAt))
	writer.property("CREATED", formatUTC(event.CreatedAt))
	writer.property("LAST-MODIFIED", formatUTC(event.UpdatedAt))
	if override != nil {
		writeEventRecurrenceID(writer, event, *override)
	}
	writer.property("SUMMARY", escapeText(event.Title))
	if event.Description != nil {
		writer.property("DESCRIPTION", escapeText(*event.Description))
	}
	if event.Location != nil {
		writer.property("LOCATION", escapeText(*event.Location))
	}
	writeEventTiming(writer, event, override)
	if override == nil {
		writeRecurrence(writer, event.Recurrence)
		writeEventExdates(writer, event, states)
		for _, attendee := range event.Attendees {
			writeAttendee(writer, attendee)
		}
		for _, reminder := range event.Reminders {
			writeAlarm(writer, reminder, false)
		}
	}
	writeEventMetadata(writer, event, calendar, override)
	writer.property("END", "VEVENT")
}

func writeTask(writer *writer, task domain.Task, calendar *domain.Calendar, states map[string]domain.OccurrenceState, completions map[string]domain.TaskCompletion, generatedAt time.Time, override *domain.OccurrenceState, completion *domain.TaskCompletion) {
	writer.property("BEGIN", "VTODO")
	writer.property("UID", uid(task.ID, task.ICalendarUID))
	writer.property("DTSTAMP", formatUTC(generatedAt))
	writer.property("CREATED", formatUTC(task.CreatedAt))
	writer.property("LAST-MODIFIED", formatUTC(task.UpdatedAt))
	if override != nil {
		writeTaskRecurrenceID(writer, task, *override)
	}
	if override == nil && completion != nil {
		writeTaskCompletionRecurrenceID(writer, task, *completion)
	}
	writer.property("SUMMARY", escapeText(task.Title))
	if task.Description != nil {
		writer.property("DESCRIPTION", escapeText(*task.Description))
	}
	writeTaskDue(writer, task, override, completion)
	if override == nil && completion == nil {
		writeRecurrence(writer, task.Recurrence)
		writeTaskExdates(writer, task, states)
		for _, reminder := range task.Reminders {
			writeAlarm(writer, reminder, true)
		}
	}
	writeTaskStatus(writer, task, completion)
	writeTaskPriority(writer, task.Priority)
	writeTaskCategories(writer, task.Tags)
	writeTaskMetadata(writer, task, calendar, override, completion)
	writer.property("END", "VTODO")
}

func writeEventTiming(writer *writer, event domain.Event, override *domain.OccurrenceState) {
	switch {
	case event.StartAt != nil:
		startAt := *event.StartAt
		endAt := event.EndAt
		if override != nil && override.ReplacementAt != nil {
			startAt = *override.ReplacementAt
			endAt = override.ReplacementEndAt
			if endAt == nil && event.EndAt != nil {
				duration := event.EndAt.Sub(*event.StartAt)
				derived := startAt.Add(duration)
				endAt = &derived
			}
		}
		writeDateTimeProperty(writer, "DTSTART", startAt, event.TimeZone)
		if endAt != nil {
			writeDateTimeProperty(writer, "DTEND", *endAt, event.TimeZone)
		}
	case event.StartDate != nil:
		startDate := *event.StartDate
		endDate := event.EndDate
		if override != nil && override.ReplacementDate != nil {
			startDate = *override.ReplacementDate
			endDate = override.ReplacementEndDate
			if endDate == nil && event.EndDate != nil {
				derived := addDays(startDate, inclusiveDateSpan(*event.StartDate, *event.EndDate)-1)
				endDate = &derived
			}
		}
		writer.propertyWithParams("DTSTART", []param{{Name: "VALUE", Value: "DATE"}}, formatDateValue(startDate))
		if endDate != nil {
			writer.propertyWithParams("DTEND", []param{{Name: "VALUE", Value: "DATE"}}, formatDateValue(addDays(*endDate, 1)))
		}
	}
}

func writeTaskDue(writer *writer, task domain.Task, override *domain.OccurrenceState, completion *domain.TaskCompletion) {
	switch {
	case task.DueAt != nil:
		dueAt := *task.DueAt
		if override != nil && override.ReplacementAt != nil {
			dueAt = *override.ReplacementAt
		}
		if override == nil && completion != nil && completion.OccurrenceAt != nil {
			dueAt = *completion.OccurrenceAt
		}
		writeDateTimeProperty(writer, "DUE", dueAt, nil)
	case task.DueDate != nil:
		dueDate := *task.DueDate
		if override != nil && override.ReplacementDate != nil {
			dueDate = *override.ReplacementDate
		}
		if override == nil && completion != nil && completion.OccurrenceDate != nil {
			dueDate = *completion.OccurrenceDate
		}
		writer.propertyWithParams("DUE", []param{{Name: "VALUE", Value: "DATE"}}, formatDateValue(dueDate))
	}
}

func writeTaskCategories(writer *writer, tags []string) {
	if len(tags) == 0 {
		return
	}
	values := make([]string, 0, len(tags))
	for _, tag := range tags {
		values = append(values, escapeText(tag))
	}
	writer.property("CATEGORIES", strings.Join(values, ","))
}

func writeRecurrence(writer *writer, rule *domain.RecurrenceRule) {
	if rule == nil {
		return
	}
	parts := []string{"FREQ=" + strings.ToUpper(string(rule.Frequency))}
	if rule.Interval > 0 {
		parts = append(parts, "INTERVAL="+strconv.FormatInt(int64(rule.Interval), 10))
	}
	if rule.Count != nil {
		parts = append(parts, "COUNT="+strconv.FormatInt(int64(*rule.Count), 10))
	}
	if rule.UntilAt != nil {
		parts = append(parts, "UNTIL="+formatUTC(*rule.UntilAt))
	}
	if rule.UntilDate != nil {
		parts = append(parts, "UNTIL="+formatDateValue(*rule.UntilDate))
	}
	if len(rule.ByWeekday) > 0 {
		values := make([]string, 0, len(rule.ByWeekday))
		for _, weekday := range rule.ByWeekday {
			values = append(values, string(weekday))
		}
		parts = append(parts, "BYDAY="+strings.Join(values, ","))
	}
	if len(rule.ByMonthDay) > 0 {
		values := make([]string, 0, len(rule.ByMonthDay))
		for _, monthDay := range rule.ByMonthDay {
			values = append(values, strconv.FormatInt(int64(monthDay), 10))
		}
		parts = append(parts, "BYMONTHDAY="+strings.Join(values, ","))
	}
	writer.property("RRULE", strings.Join(parts, ";"))
}

func writeEventExdates(writer *writer, event domain.Event, states map[string]domain.OccurrenceState) {
	if len(states) == 0 {
		return
	}
	for _, state := range sortedOccurrenceStates(states) {
		if !state.Cancelled && !eventStateHasReplacement(state) {
			continue
		}
		switch {
		case state.OccurrenceAt != nil:
			writeDateTimeProperty(writer, "EXDATE", *state.OccurrenceAt, event.TimeZone)
		case state.OccurrenceDate != nil:
			writer.propertyWithParams("EXDATE", []param{{Name: "VALUE", Value: "DATE"}}, formatDateValue(*state.OccurrenceDate))
		}
	}
}

func writeTaskExdates(writer *writer, task domain.Task, states map[string]domain.OccurrenceState) {
	if len(states) == 0 {
		return
	}
	for _, state := range sortedOccurrenceStates(states) {
		if !state.Cancelled && !taskStateHasReplacement(state) {
			continue
		}
		switch {
		case state.OccurrenceAt != nil:
			writeDateTimeProperty(writer, "EXDATE", *state.OccurrenceAt, nil)
		case state.OccurrenceDate != nil:
			writer.propertyWithParams("EXDATE", []param{{Name: "VALUE", Value: "DATE"}}, formatDateValue(*state.OccurrenceDate))
		}
	}
}

func writeEventRecurrenceID(writer *writer, event domain.Event, state domain.OccurrenceState) {
	switch {
	case state.OccurrenceAt != nil:
		writeDateTimeProperty(writer, "RECURRENCE-ID", *state.OccurrenceAt, event.TimeZone)
	case state.OccurrenceDate != nil:
		writer.propertyWithParams("RECURRENCE-ID", []param{{Name: "VALUE", Value: "DATE"}}, formatDateValue(*state.OccurrenceDate))
	}
}

func writeTaskRecurrenceID(writer *writer, task domain.Task, state domain.OccurrenceState) {
	switch {
	case state.OccurrenceAt != nil:
		writeDateTimeProperty(writer, "RECURRENCE-ID", *state.OccurrenceAt, nil)
	case state.OccurrenceDate != nil:
		writer.propertyWithParams("RECURRENCE-ID", []param{{Name: "VALUE", Value: "DATE"}}, formatDateValue(*state.OccurrenceDate))
	default:
		_ = task
	}
}

func writeTaskCompletionRecurrenceID(writer *writer, task domain.Task, completion domain.TaskCompletion) {
	switch {
	case completion.OccurrenceAt != nil:
		writeDateTimeProperty(writer, "RECURRENCE-ID", *completion.OccurrenceAt, nil)
	case completion.OccurrenceDate != nil:
		writer.propertyWithParams("RECURRENCE-ID", []param{{Name: "VALUE", Value: "DATE"}}, formatDateValue(*completion.OccurrenceDate))
	default:
		_ = task
	}
}

func writeDateTimeProperty(writer *writer, name string, value time.Time, timeZone *string) {
	if timeZone != nil && strings.TrimSpace(*timeZone) != "" {
		writer.propertyWithParams(name, []param{{Name: "TZID", Value: strings.TrimSpace(*timeZone)}}, formatFloatingLocal(value))
		return
	}
	writer.property(name, formatUTC(value))
}

func writeAlarm(writer *writer, reminder domain.ReminderRule, task bool) {
	writer.property("BEGIN", "VALARM")
	writer.property("ACTION", "DISPLAY")
	writer.property("DESCRIPTION", "Reminder")
	params := []param{}
	if task {
		params = append(params, param{Name: "RELATED", Value: "END"})
	}
	writer.propertyWithParams("TRIGGER", params, fmt.Sprintf("-PT%dM", reminder.BeforeMinutes))
	writer.property("END", "VALARM")
}

func writeAttendee(writer *writer, attendee domain.EventAttendee) {
	params := []param{
		{Name: "ROLE", Value: attendeeRole(attendee.Role)},
		{Name: "PARTSTAT", Value: participationStatus(attendee.ParticipationStatus)},
		{Name: "RSVP", Value: boolValue(attendee.RSVP)},
	}
	if attendee.DisplayName != nil {
		params = append(params, param{Name: "CN", Value: *attendee.DisplayName})
	}
	writer.propertyWithParams("ATTENDEE", params, "mailto:"+attendee.Email)
}

func writeTaskStatus(writer *writer, task domain.Task, completion *domain.TaskCompletion) {
	if completion != nil || task.CompletedAt != nil || task.Status == domain.TaskStatusDone {
		writer.property("STATUS", "COMPLETED")
		if completion != nil {
			writer.property("COMPLETED", formatUTC(completion.CompletedAt))
			return
		}
		if task.CompletedAt != nil {
			writer.property("COMPLETED", formatUTC(*task.CompletedAt))
		}
		return
	}
	switch task.Status {
	case domain.TaskStatusInProgress:
		writer.property("STATUS", "IN-PROCESS")
	default:
		writer.property("STATUS", "NEEDS-ACTION")
	}
}

func writeTaskPriority(writer *writer, priority domain.TaskPriority) {
	switch priority {
	case domain.TaskPriorityHigh:
		writer.property("PRIORITY", "1")
	case domain.TaskPriorityLow:
		writer.property("PRIORITY", "9")
	default:
		writer.property("PRIORITY", "5")
	}
}

func writeEventMetadata(writer *writer, event domain.Event, calendar *domain.Calendar, override *domain.OccurrenceState) {
	writer.property("X-OPENPLANNER-KIND", "event")
	writer.property("X-OPENPLANNER-ID", event.ID)
	writer.property("X-OPENPLANNER-CALENDAR-ID", event.CalendarID)
	if calendar != nil {
		writer.property("X-OPENPLANNER-CALENDAR-NAME", escapeText(calendar.Name))
	}
	if event.TimeZone != nil {
		writer.property("X-OPENPLANNER-TIME-ZONE", escapeText(*event.TimeZone))
	}
	if len(event.LinkedTaskIDs) > 0 {
		writer.property("X-OPENPLANNER-LINKED-TASK-IDS", escapeText(strings.Join(event.LinkedTaskIDs, ",")))
	}
	if override != nil {
		writer.property("X-OPENPLANNER-OCCURRENCE-KEY", escapeText(override.OccurrenceKey))
	}
}

func writeTaskMetadata(writer *writer, task domain.Task, calendar *domain.Calendar, override *domain.OccurrenceState, completion *domain.TaskCompletion) {
	writer.property("X-OPENPLANNER-KIND", "task")
	writer.property("X-OPENPLANNER-ID", task.ID)
	writer.property("X-OPENPLANNER-CALENDAR-ID", task.CalendarID)
	if calendar != nil {
		writer.property("X-OPENPLANNER-CALENDAR-NAME", escapeText(calendar.Name))
	}
	if len(task.LinkedEventIDs) > 0 {
		writer.property("X-OPENPLANNER-LINKED-EVENT-IDS", escapeText(strings.Join(task.LinkedEventIDs, ",")))
	}
	switch {
	case override != nil:
		writer.property("X-OPENPLANNER-OCCURRENCE-KEY", escapeText(override.OccurrenceKey))
	case completion != nil:
		writer.property("X-OPENPLANNER-OCCURRENCE-KEY", escapeText(completion.OccurrenceKey))
	}
}

func uid(id string, icalendarUID *string) string {
	if icalendarUID != nil {
		if value := strings.TrimSpace(*icalendarUID); value != "" {
			return value
		}
	}
	return id + "@openplanner.local"
}

func attendeeRole(role domain.EventAttendeeRole) string {
	switch role {
	case domain.EventAttendeeRoleOptional:
		return "OPT-PARTICIPANT"
	case domain.EventAttendeeRoleChair:
		return "CHAIR"
	case domain.EventAttendeeRoleNonParticipant:
		return "NON-PARTICIPANT"
	default:
		return "REQ-PARTICIPANT"
	}
}

func participationStatus(status domain.EventParticipationStatus) string {
	switch status {
	case domain.EventParticipationStatusAccepted:
		return "ACCEPTED"
	case domain.EventParticipationStatusDeclined:
		return "DECLINED"
	case domain.EventParticipationStatusTentative:
		return "TENTATIVE"
	case domain.EventParticipationStatusDelegated:
		return "DELEGATED"
	default:
		return "NEEDS-ACTION"
	}
}

func boolValue(value bool) string {
	if value {
		return "TRUE"
	}
	return "FALSE"
}

func eventStateHasReplacement(state domain.OccurrenceState) bool {
	return state.ReplacementAt != nil || state.ReplacementDate != nil
}

func taskStateHasReplacement(state domain.OccurrenceState) bool {
	return state.ReplacementAt != nil || state.ReplacementDate != nil
}

func calendarIndex(calendars []domain.Calendar) map[string]*domain.Calendar {
	index := make(map[string]*domain.Calendar, len(calendars))
	for i := range calendars {
		index[calendars[i].ID] = &calendars[i]
	}
	return index
}

func sortedEvents(events []domain.Event) []domain.Event {
	out := append([]domain.Event(nil), events...)
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}

func sortedTasks(tasks []domain.Task) []domain.Task {
	out := append([]domain.Task(nil), tasks...)
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}

func sortedOccurrenceStates(states map[string]domain.OccurrenceState) []domain.OccurrenceState {
	out := make([]domain.OccurrenceState, 0, len(states))
	for _, state := range states {
		out = append(out, state)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].OccurrenceKey < out[j].OccurrenceKey
	})
	return out
}

func sortedCompletions(completions map[string]domain.TaskCompletion) []domain.TaskCompletion {
	out := make([]domain.TaskCompletion, 0, len(completions))
	for _, completion := range completions {
		out = append(out, completion)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].OccurrenceKey < out[j].OccurrenceKey
	})
	return out
}

func formatUTC(value time.Time) string {
	return value.UTC().Format("20060102T150405Z")
}

func formatFloatingLocal(value time.Time) string {
	return value.Format("20060102T150405")
}

func formatDateValue(value string) string {
	parsed, err := time.Parse(time.DateOnly, value)
	if err != nil {
		return strings.ReplaceAll(value, "-", "")
	}
	return parsed.Format("20060102")
}

func addDays(value string, days int) string {
	parsed, err := time.Parse(time.DateOnly, value)
	if err != nil {
		return value
	}
	return parsed.AddDate(0, 0, days).Format(time.DateOnly)
}

func inclusiveDateSpan(start string, end string) int {
	startDate, startErr := time.Parse(time.DateOnly, start)
	endDate, endErr := time.Parse(time.DateOnly, end)
	if startErr != nil || endErr != nil || endDate.Before(startDate) {
		return 1
	}
	return int(endDate.Sub(startDate).Hours()/24) + 1
}

type writer struct {
	builder strings.Builder
}

type param struct {
	Name  string
	Value string
}

func newWriter() *writer {
	return &writer{}
}

func (writer *writer) property(name string, value string) {
	writer.propertyWithParams(name, nil, value)
}

func (writer *writer) propertyWithParams(name string, params []param, value string) {
	line := name
	for _, param := range params {
		line += ";" + param.Name + "=" + escapeParam(param.Value)
	}
	line += ":" + value
	writer.writeFolded(line)
}

func (writer *writer) writeFolded(line string) {
	count := 0
	for _, value := range line {
		size := len(string(value))
		if count > 0 && count+size > 75 {
			writer.builder.WriteString("\r\n ")
			count = 1
		}
		writer.builder.WriteRune(value)
		count += size
	}
	writer.builder.WriteString("\r\n")
}

func (writer *writer) String() string {
	return writer.builder.String()
}

func escapeText(value string) string {
	var builder strings.Builder
	for _, char := range value {
		switch char {
		case '\\':
			builder.WriteString(`\\`)
		case ';':
			builder.WriteString(`\;`)
		case ',':
			builder.WriteString(`\,`)
		case '\r':
			continue
		case '\n':
			builder.WriteString(`\n`)
		default:
			if unicode.IsControl(char) {
				continue
			}
			builder.WriteRune(char)
		}
	}
	return builder.String()
}

func escapeParam(value string) string {
	escaped := escapeText(value)
	if strings.ContainsAny(escaped, `":;,`) || strings.ContainsFunc(escaped, unicode.IsSpace) {
		escaped = strings.ReplaceAll(escaped, `"`, `\"`)
		return `"` + escaped + `"`
	}
	return escaped
}
