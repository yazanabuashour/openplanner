package icalendar

import (
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strconv"
	"strings"
	"time"

	ics "github.com/arran4/golang-ical"

	"github.com/yazanabuashour/openplanner/internal/domain"
)

const defaultImportCalendarName = "Imported Calendar"

var importTagPattern = regexp.MustCompile(`^[a-z0-9_-]+$`)

type ParsedImport struct {
	CalendarName  string
	CalendarColor *string
	Events        []ImportedEvent
	EventChanges  []ImportedEventChange
	Tasks         []ImportedTask
	TaskChanges   []ImportedTaskChange
	Skips         []domain.ICalendarImportSkip
}

type ImportedEvent struct {
	UID           string
	OpenPlannerID string
	CalendarName  string
	Event         domain.Event
	ExDates       []OccurrenceSelector
}

type ImportedEventChange struct {
	UID           string
	OpenPlannerID string
	CalendarName  string
	Recurrence    OccurrenceSelector
	Replacement   domain.Event
	Cancelled     bool
}

type ImportedTask struct {
	UID           string
	OpenPlannerID string
	CalendarName  string
	Task          domain.Task
	ExDates       []OccurrenceSelector
}

type ImportedTaskChange struct {
	UID           string
	OpenPlannerID string
	CalendarName  string
	Recurrence    OccurrenceSelector
	Replacement   domain.Task
	Cancelled     bool
	Completed     bool
	CompletedAt   *time.Time
}

type OccurrenceSelector struct {
	At   *time.Time
	Date *string
}

type propertyOwner interface {
	GetProperty(ics.ComponentProperty) *ics.IANAProperty
	GetProperties(ics.ComponentProperty) []*ics.IANAProperty
	Alarms() []*ics.VAlarm
}

func ParseImport(content string) (ParsedImport, error) {
	if strings.TrimSpace(content) == "" {
		return ParsedImport{}, fmt.Errorf("icalendar content is required")
	}

	calendar, err := ics.ParseCalendarWithOptions(strings.NewReader(content), ics.WithUnknownPropertyHandler(ics.AcceptUnknownPropertyHandler))
	if err != nil {
		return ParsedImport{}, fmt.Errorf("parse icalendar: %w", err)
	}

	parsed := ParsedImport{
		CalendarName: calendarProperty(calendar, string(ics.PropertyXWRCalName), defaultImportCalendarName),
	}
	parsed.CalendarColor = optionalCalendarProperty(calendar, string(ics.PropertyColor))

	for _, event := range calendar.Events() {
		if change, ok, skip := parseEventChange(event, parsed.CalendarName); skip.Reason != "" {
			parsed.Skips = append(parsed.Skips, skip)
		} else if ok {
			parsed.EventChanges = append(parsed.EventChanges, change)
		} else if imported, skip := parseEvent(event, parsed.CalendarName); skip.Reason != "" {
			parsed.Skips = append(parsed.Skips, skip)
		} else {
			parsed.Events = append(parsed.Events, imported)
		}
	}

	for _, todo := range calendar.Todos() {
		if change, ok, skip := parseTaskChange(todo, parsed.CalendarName); skip.Reason != "" {
			parsed.Skips = append(parsed.Skips, skip)
		} else if ok {
			parsed.TaskChanges = append(parsed.TaskChanges, change)
		} else if imported, skip := parseTask(todo, parsed.CalendarName); skip.Reason != "" {
			parsed.Skips = append(parsed.Skips, skip)
		} else {
			parsed.Tasks = append(parsed.Tasks, imported)
		}
	}

	return parsed, nil
}

func parseEvent(event *ics.VEvent, fallbackCalendarName string) (ImportedEvent, domain.ICalendarImportSkip) {
	uid := trimmedProperty(event, ics.ComponentPropertyUniqueId)
	if uid == "" {
		return ImportedEvent{}, importSkip("event", "", "UID is required")
	}
	title := trimmedProperty(event, ics.ComponentPropertySummary)
	if title == "" {
		return ImportedEvent{}, importSkip("event", uid, "SUMMARY is required")
	}
	timing, skipReason := parseEventTiming(event)
	if skipReason != "" {
		return ImportedEvent{}, importSkip("event", uid, skipReason)
	}
	recurrence, skipReason := parseImportRecurrence(event)
	if skipReason != "" {
		return ImportedEvent{}, importSkip("event", uid, skipReason)
	}
	reminders, skipReason := parseAlarms(event)
	if skipReason != "" {
		return ImportedEvent{}, importSkip("event", uid, skipReason)
	}
	attendees, skipReason := parseAttendees(event)
	if skipReason != "" {
		return ImportedEvent{}, importSkip("event", uid, skipReason)
	}
	exDates, skipReason := parseExDates(event, timing.AllDay)
	if skipReason != "" {
		return ImportedEvent{}, importSkip("event", uid, skipReason)
	}

	eventInput := domain.Event{
		ICalendarUID: importStringPtr(uid),
		Title:        title,
		Description:  optionalProperty(event, ics.ComponentPropertyDescription),
		Location:     optionalProperty(event, ics.ComponentPropertyLocation),
		StartAt:      timing.StartAt,
		EndAt:        timing.EndAt,
		TimeZone:     timing.TimeZone,
		StartDate:    timing.StartDate,
		EndDate:      timing.EndDate,
		Recurrence:   recurrence,
		Reminders:    reminders,
		Attendees:    attendees,
	}

	return ImportedEvent{
		UID:           uid,
		OpenPlannerID: trimmedExtendedProperty(event, "X-OPENPLANNER-ID"),
		CalendarName:  componentCalendarName(event, fallbackCalendarName),
		Event:         eventInput,
		ExDates:       exDates,
	}, domain.ICalendarImportSkip{}
}

func parseEventChange(event *ics.VEvent, fallbackCalendarName string) (ImportedEventChange, bool, domain.ICalendarImportSkip) {
	recurrenceProp := event.GetProperty(ics.ComponentPropertyRecurrenceId)
	if recurrenceProp == nil {
		return ImportedEventChange{}, false, domain.ICalendarImportSkip{}
	}
	uid := trimmedProperty(event, ics.ComponentPropertyUniqueId)
	if uid == "" {
		return ImportedEventChange{}, true, importSkip("event_occurrence", "", "UID is required")
	}
	recurrence, allDay, err := parseSelectorProperty(recurrenceProp)
	if err != nil {
		return ImportedEventChange{}, true, importSkip("event_occurrence", uid, err.Error())
	}
	timing, skipReason := parseEventTiming(event)
	if skipReason != "" && !eventCancelled(event) {
		return ImportedEventChange{}, true, importSkip("event_occurrence", uid, skipReason)
	}
	if recurrence.Date != nil && !allDay {
		return ImportedEventChange{}, true, importSkip("event_occurrence", uid, "RECURRENCE-ID date value is required for all-day override")
	}
	return ImportedEventChange{
		UID:           uid,
		OpenPlannerID: trimmedExtendedProperty(event, "X-OPENPLANNER-ID"),
		CalendarName:  componentCalendarName(event, fallbackCalendarName),
		Recurrence:    recurrence,
		Replacement: domain.Event{
			StartAt:   timing.StartAt,
			EndAt:     timing.EndAt,
			StartDate: timing.StartDate,
			EndDate:   timing.EndDate,
		},
		Cancelled: eventCancelled(event),
	}, true, domain.ICalendarImportSkip{}
}

func parseTask(todo *ics.VTodo, fallbackCalendarName string) (ImportedTask, domain.ICalendarImportSkip) {
	uid := trimmedProperty(todo, ics.ComponentPropertyUniqueId)
	if uid == "" {
		return ImportedTask{}, importSkip("task", "", "UID is required")
	}
	title := trimmedProperty(todo, ics.ComponentPropertySummary)
	if title == "" {
		return ImportedTask{}, importSkip("task", uid, "SUMMARY is required")
	}
	timing, skipReason := parseTaskTiming(todo)
	if skipReason != "" {
		return ImportedTask{}, importSkip("task", uid, skipReason)
	}
	recurrence, skipReason := parseImportRecurrence(todo)
	if skipReason != "" {
		return ImportedTask{}, importSkip("task", uid, skipReason)
	}
	reminders, skipReason := parseAlarms(todo)
	if skipReason != "" {
		return ImportedTask{}, importSkip("task", uid, skipReason)
	}
	tags := parseCategories(todo)
	exDates, skipReason := parseExDates(todo, timing.AllDay)
	if skipReason != "" {
		return ImportedTask{}, importSkip("task", uid, skipReason)
	}

	task := domain.Task{
		ICalendarUID: importStringPtr(uid),
		Title:        title,
		Description:  optionalProperty(todo, ics.ComponentPropertyDescription),
		DueAt:        timing.DueAt,
		DueDate:      timing.DueDate,
		Recurrence:   recurrence,
		Reminders:    reminders,
		Priority:     taskPriority(todo),
		Status:       taskStatus(todo),
		Tags:         tags,
		CompletedAt:  completedAt(todo),
	}

	return ImportedTask{
		UID:           uid,
		OpenPlannerID: trimmedExtendedProperty(todo, "X-OPENPLANNER-ID"),
		CalendarName:  componentCalendarName(todo, fallbackCalendarName),
		Task:          task,
		ExDates:       exDates,
	}, domain.ICalendarImportSkip{}
}

func parseTaskChange(todo *ics.VTodo, fallbackCalendarName string) (ImportedTaskChange, bool, domain.ICalendarImportSkip) {
	recurrenceProp := todo.GetProperty(ics.ComponentPropertyRecurrenceId)
	if recurrenceProp == nil {
		return ImportedTaskChange{}, false, domain.ICalendarImportSkip{}
	}
	uid := trimmedProperty(todo, ics.ComponentPropertyUniqueId)
	if uid == "" {
		return ImportedTaskChange{}, true, importSkip("task_occurrence", "", "UID is required")
	}
	recurrence, _, err := parseSelectorProperty(recurrenceProp)
	if err != nil {
		return ImportedTaskChange{}, true, importSkip("task_occurrence", uid, err.Error())
	}
	timing, skipReason := parseTaskTiming(todo)
	if skipReason != "" && !taskCancelled(todo) && !taskCompleted(todo) {
		return ImportedTaskChange{}, true, importSkip("task_occurrence", uid, skipReason)
	}
	return ImportedTaskChange{
		UID:           uid,
		OpenPlannerID: trimmedExtendedProperty(todo, "X-OPENPLANNER-ID"),
		CalendarName:  componentCalendarName(todo, fallbackCalendarName),
		Recurrence:    recurrence,
		Replacement: domain.Task{
			DueAt:   timing.DueAt,
			DueDate: timing.DueDate,
		},
		Cancelled:   taskCancelled(todo),
		Completed:   taskCompleted(todo),
		CompletedAt: completedAt(todo),
	}, true, domain.ICalendarImportSkip{}
}

type eventTiming struct {
	AllDay    bool
	StartAt   *time.Time
	EndAt     *time.Time
	TimeZone  *string
	StartDate *string
	EndDate   *string
}

func parseEventTiming(owner propertyOwner) (eventTiming, string) {
	startProp := owner.GetProperty(ics.ComponentPropertyDtStart)
	if startProp == nil {
		return eventTiming{}, "DTSTART is required"
	}
	start, allDay, tzid, err := parseTimeProperty(startProp)
	if err != nil {
		return eventTiming{}, err.Error()
	}
	endProp := owner.GetProperty(ics.ComponentPropertyDtEnd)
	durationProp := owner.GetProperty(ics.ComponentPropertyDuration)

	if allDay {
		startDate := start.Format(time.DateOnly)
		var endDate *string
		if endProp != nil {
			end, endAllDay, _, err := parseTimeProperty(endProp)
			if err != nil {
				return eventTiming{}, err.Error()
			}
			if !endAllDay {
				return eventTiming{}, "DTEND must be a date for all-day events"
			}
			inclusiveEnd := end.AddDate(0, 0, -1).Format(time.DateOnly)
			if inclusiveEnd < startDate {
				inclusiveEnd = startDate
			}
			endDate = &inclusiveEnd
		} else if durationProp != nil {
			duration, err := parseICSDuration(durationProp.Value)
			if err != nil {
				return eventTiming{}, "DURATION is unsupported"
			}
			days := int(duration.Hours() / 24)
			if days > 0 {
				value := start.AddDate(0, 0, days-1).Format(time.DateOnly)
				endDate = &value
			}
		}
		return eventTiming{AllDay: true, StartDate: &startDate, EndDate: endDate}, ""
	}

	timing := eventTiming{StartAt: &start, TimeZone: tzid}
	if endProp != nil {
		end, endAllDay, _, err := parseTimeProperty(endProp)
		if err != nil {
			return eventTiming{}, err.Error()
		}
		if endAllDay {
			return eventTiming{}, "DTEND must be a date-time for timed events"
		}
		timing.EndAt = &end
	} else if durationProp != nil {
		duration, err := parseICSDuration(durationProp.Value)
		if err != nil {
			return eventTiming{}, "DURATION is unsupported"
		}
		end := start.Add(duration)
		timing.EndAt = &end
	}
	return timing, ""
}

type taskTiming struct {
	AllDay  bool
	DueAt   *time.Time
	DueDate *string
}

func parseTaskTiming(owner propertyOwner) (taskTiming, string) {
	dueProp := owner.GetProperty(ics.ComponentPropertyDue)
	if dueProp == nil {
		return taskTiming{}, ""
	}
	due, allDay, _, err := parseTimeProperty(dueProp)
	if err != nil {
		return taskTiming{}, err.Error()
	}
	if allDay {
		dueDate := due.Format(time.DateOnly)
		return taskTiming{AllDay: true, DueDate: &dueDate}, ""
	}
	return taskTiming{DueAt: &due}, ""
}

func parseImportRecurrence(owner interface {
	GetRRules() ([]*ics.RecurrenceRule, error)
	GetProperties(ics.ComponentProperty) []*ics.IANAProperty
}) (*domain.RecurrenceRule, string) {
	if len(owner.GetProperties(ics.ComponentPropertyRdate)) > 0 || len(owner.GetProperties(ics.ComponentPropertyExrule)) > 0 {
		return nil, "RDATE and EXRULE are not supported"
	}
	rules, err := owner.GetRRules()
	if err != nil {
		return nil, "RRULE is malformed"
	}
	if len(rules) == 0 {
		return nil, ""
	}
	if len(rules) > 1 {
		return nil, "multiple RRULE values are not supported"
	}
	rule := rules[0]
	out := domain.RecurrenceRule{Interval: int32(rule.Interval)}
	if out.Interval == 0 {
		out.Interval = 1
	}
	switch rule.Freq {
	case ics.FrequencyDaily:
		out.Frequency = domain.RecurrenceFrequencyDaily
	case ics.FrequencyWeekly:
		out.Frequency = domain.RecurrenceFrequencyWeekly
	case ics.FrequencyMonthly:
		out.Frequency = domain.RecurrenceFrequencyMonthly
	default:
		return nil, "unsupported recurrence frequency"
	}
	if rule.Count > 0 {
		count := int32(rule.Count)
		out.Count = &count
	}
	if !rule.Until.IsZero() {
		if rule.UntilDateOnly {
			untilDate := rule.Until.Format(time.DateOnly)
			out.UntilDate = &untilDate
		} else {
			until := rule.Until
			out.UntilAt = &until
		}
	}
	if len(rule.BySecond) > 0 || len(rule.ByMinute) > 0 || len(rule.ByHour) > 0 ||
		len(rule.ByYearDay) > 0 || len(rule.ByWeekNo) > 0 || len(rule.ByMonth) > 0 ||
		len(rule.BySetPos) > 0 {
		return nil, "unsupported recurrence rule part"
	}
	if len(rule.ByDay) > 0 {
		if out.Frequency != domain.RecurrenceFrequencyWeekly {
			return nil, "BYDAY is only supported for weekly recurrence"
		}
		for _, day := range rule.ByDay {
			if day.OrdWeek != 0 {
				return nil, "ordinal BYDAY is not supported"
			}
			weekday, ok := importWeekday(day.Day)
			if !ok {
				return nil, "unsupported BYDAY value"
			}
			out.ByWeekday = append(out.ByWeekday, weekday)
		}
	}
	if len(rule.ByMonthDay) > 0 {
		if out.Frequency != domain.RecurrenceFrequencyMonthly {
			return nil, "BYMONTHDAY is only supported for monthly recurrence"
		}
		for _, day := range rule.ByMonthDay {
			if day < 1 || day > 31 {
				return nil, "BYMONTHDAY values must be between 1 and 31"
			}
			out.ByMonthDay = append(out.ByMonthDay, int32(day))
		}
	}
	return &out, ""
}

func parseExDates(owner propertyOwner, allDay bool) ([]OccurrenceSelector, string) {
	var out []OccurrenceSelector
	for _, prop := range owner.GetProperties(ics.ComponentPropertyExdate) {
		values := strings.Split(prop.Value, ",")
		for _, value := range values {
			clone := *prop
			clone.Value = strings.TrimSpace(value)
			selector, selectorAllDay, err := parseSelectorProperty(&clone)
			if err != nil {
				return nil, err.Error()
			}
			if allDay != selectorAllDay {
				return nil, "EXDATE type must match component timing"
			}
			out = append(out, selector)
		}
	}
	return out, ""
}

func parseSelectorProperty(prop *ics.IANAProperty) (OccurrenceSelector, bool, error) {
	value, allDay, _, err := parseTimeProperty(prop)
	if err != nil {
		return OccurrenceSelector{}, false, err
	}
	if allDay {
		date := value.Format(time.DateOnly)
		return OccurrenceSelector{Date: &date}, true, nil
	}
	return OccurrenceSelector{At: &value}, false, nil
}

func parseTimeProperty(prop *ics.IANAProperty) (time.Time, bool, *string, error) {
	value := strings.TrimSpace(prop.Value)
	valueType := strings.ToUpper(firstParam(prop, string(ics.ParameterValue)))
	tzid := strings.TrimSpace(firstParam(prop, string(ics.ParameterTzid)))
	allDay := valueType == string(ics.ValueDataTypeDate) || isICalDate(value)
	if allDay {
		parsed, err := time.ParseInLocation("20060102", value, time.UTC)
		if err != nil {
			return time.Time{}, false, nil, fmt.Errorf("date value must be YYYYMMDD")
		}
		return parsed, true, nil, nil
	}
	if strings.HasSuffix(value, "Z") {
		parsed, err := time.Parse("20060102T150405Z", value)
		if err != nil {
			return time.Time{}, false, nil, fmt.Errorf("date-time value is malformed")
		}
		return parsed, false, nil, nil
	}
	if tzid == "" {
		return time.Time{}, false, nil, fmt.Errorf("floating date-times require TZID")
	}
	location, err := time.LoadLocation(tzid)
	if err != nil {
		return time.Time{}, false, nil, fmt.Errorf("TZID must be a valid IANA timezone")
	}
	parsed, err := time.ParseInLocation("20060102T150405", value, location)
	if err != nil {
		return time.Time{}, false, nil, fmt.Errorf("date-time value is malformed")
	}
	return parsed, false, &tzid, nil
}

func parseAlarms(owner propertyOwner) ([]domain.ReminderRule, string) {
	seen := map[int32]bool{}
	var reminders []domain.ReminderRule
	for _, alarm := range owner.Alarms() {
		trigger := alarm.GetProperty(ics.ComponentPropertyTrigger)
		if trigger == nil {
			continue
		}
		duration, err := parseICSDuration(trigger.Value)
		if err != nil || duration >= 0 {
			continue
		}
		minutes := int32((-duration + time.Minute - 1) / time.Minute)
		if minutes <= 0 {
			continue
		}
		if seen[minutes] {
			return nil, "duplicate reminder offsets are not supported"
		}
		seen[minutes] = true
		reminders = append(reminders, domain.ReminderRule{BeforeMinutes: minutes})
	}
	return reminders, ""
}

func parseAttendees(owner interface {
	Attendees() []*ics.Attendee
}) ([]domain.EventAttendee, string) {
	var attendees []domain.EventAttendee
	for _, attendee := range owner.Attendees() {
		email := strings.TrimSpace(strings.TrimPrefix(attendee.Email(), "mailto:"))
		if _, err := mail.ParseAddress(email); err != nil {
			return nil, "ATTENDEE email is invalid"
		}
		attendees = append(attendees, domain.EventAttendee{
			Email:               email,
			DisplayName:         optionalParam(&attendee.IANAProperty, string(ics.ParameterCn)),
			Role:                attendeeRoleFromICal(firstParam(&attendee.IANAProperty, string(ics.ParameterRole))),
			ParticipationStatus: attendeeStatusFromICal(firstParam(&attendee.IANAProperty, string(ics.ParameterParticipationStatus))),
			RSVP:                strings.EqualFold(firstParam(&attendee.IANAProperty, string(ics.ParameterRsvp)), "TRUE"),
		})
	}
	return attendees, ""
}

func parseCategories(owner interface {
	GetProperties(ics.ComponentProperty) []*ics.IANAProperty
}) []string {
	seen := map[string]bool{}
	var tags []string
	for _, prop := range owner.GetProperties(ics.ComponentPropertyCategories) {
		for _, part := range strings.Split(prop.Value, ",") {
			tag := strings.ToLower(strings.TrimSpace(part))
			tag = strings.ReplaceAll(tag, " ", "-")
			if tag == "" || !importTagPattern.MatchString(tag) || seen[tag] {
				continue
			}
			seen[tag] = true
			tags = append(tags, tag)
		}
	}
	return tags
}

func taskPriority(owner interface {
	GetProperty(ics.ComponentProperty) *ics.IANAProperty
}) domain.TaskPriority {
	value, err := strconv.Atoi(strings.TrimSpace(trimmedProperty(owner, ics.ComponentPropertyPriority)))
	if err != nil {
		return domain.TaskPriorityMedium
	}
	switch {
	case value >= 1 && value <= 4:
		return domain.TaskPriorityHigh
	case value >= 6 && value <= 9:
		return domain.TaskPriorityLow
	default:
		return domain.TaskPriorityMedium
	}
}

func taskStatus(owner interface {
	GetProperty(ics.ComponentProperty) *ics.IANAProperty
}) domain.TaskStatus {
	switch strings.ToUpper(trimmedProperty(owner, ics.ComponentPropertyStatus)) {
	case "COMPLETED":
		return domain.TaskStatusDone
	case "IN-PROCESS":
		return domain.TaskStatusInProgress
	default:
		return domain.TaskStatusTodo
	}
}

func completedAt(owner interface {
	GetProperty(ics.ComponentProperty) *ics.IANAProperty
}) *time.Time {
	prop := owner.GetProperty(ics.ComponentPropertyCompleted)
	if prop == nil {
		return nil
	}
	value, allDay, _, err := parseTimeProperty(prop)
	if err != nil {
		return nil
	}
	if allDay {
		value = time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
	}
	return &value
}

func taskCompleted(owner interface {
	GetProperty(ics.ComponentProperty) *ics.IANAProperty
}) bool {
	return taskStatus(owner) == domain.TaskStatusDone || completedAt(owner) != nil
}

func eventCancelled(owner interface {
	GetProperty(ics.ComponentProperty) *ics.IANAProperty
}) bool {
	return strings.EqualFold(trimmedProperty(owner, ics.ComponentPropertyStatus), "CANCELLED")
}

func taskCancelled(owner interface {
	GetProperty(ics.ComponentProperty) *ics.IANAProperty
}) bool {
	return strings.EqualFold(trimmedProperty(owner, ics.ComponentPropertyStatus), "CANCELLED")
}

func componentCalendarName(owner interface {
	GetProperty(ics.ComponentProperty) *ics.IANAProperty
}, fallback string) string {
	if value := strings.TrimSpace(trimmedExtendedProperty(owner, "X-OPENPLANNER-CALENDAR-NAME")); value != "" {
		return value
	}
	return fallback
}

func calendarProperty(calendar *ics.Calendar, name string, fallback string) string {
	if value := optionalCalendarProperty(calendar, name); value != nil {
		return *value
	}
	return fallback
}

func optionalCalendarProperty(calendar *ics.Calendar, name string) *string {
	for _, prop := range calendar.CalendarProperties {
		if strings.EqualFold(prop.IANAToken, name) {
			return importStringPtr(strings.TrimSpace(prop.Value))
		}
	}
	return nil
}

func optionalProperty(owner interface {
	GetProperty(ics.ComponentProperty) *ics.IANAProperty
}, prop ics.ComponentProperty) *string {
	value := trimmedProperty(owner, prop)
	if value == "" {
		return nil
	}
	return &value
}

func trimmedProperty(owner interface {
	GetProperty(ics.ComponentProperty) *ics.IANAProperty
}, prop ics.ComponentProperty) string {
	value := owner.GetProperty(prop)
	if value == nil {
		return ""
	}
	return strings.TrimSpace(value.Value)
}

func trimmedExtendedProperty(owner interface {
	GetProperty(ics.ComponentProperty) *ics.IANAProperty
}, name string) string {
	return trimmedProperty(owner, ics.ComponentPropertyExtended(name))
}

func firstParam(prop *ics.IANAProperty, key string) string {
	if prop == nil {
		return ""
	}
	values := prop.ICalParameters[key]
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0])
}

func optionalParam(prop *ics.IANAProperty, key string) *string {
	value := firstParam(prop, key)
	if value == "" {
		return nil
	}
	return &value
}

func parseICSDuration(value string) (time.Duration, error) {
	value = strings.TrimSpace(strings.ToUpper(value))
	if value == "" {
		return 0, errors.New("duration is empty")
	}
	sign := time.Duration(1)
	if strings.HasPrefix(value, "-") {
		sign = -1
		value = strings.TrimPrefix(value, "-")
	} else {
		value = strings.TrimPrefix(value, "+")
	}
	if !strings.HasPrefix(value, "P") {
		return 0, errors.New("duration must start with P")
	}
	value = strings.TrimPrefix(value, "P")
	inTime := false
	var digits strings.Builder
	var duration time.Duration
	for _, char := range value {
		if char == 'T' {
			inTime = true
			continue
		}
		if char >= '0' && char <= '9' {
			digits.WriteRune(char)
			continue
		}
		if digits.Len() == 0 {
			return 0, errors.New("duration value is missing")
		}
		number, err := strconv.Atoi(digits.String())
		if err != nil {
			return 0, err
		}
		digits.Reset()
		switch char {
		case 'W':
			duration += time.Duration(number) * 7 * 24 * time.Hour
		case 'D':
			duration += time.Duration(number) * 24 * time.Hour
		case 'H':
			if !inTime {
				return 0, errors.New("hours must be in time duration")
			}
			duration += time.Duration(number) * time.Hour
		case 'M':
			if !inTime {
				return 0, errors.New("months are not supported in durations")
			}
			duration += time.Duration(number) * time.Minute
		case 'S':
			if !inTime {
				return 0, errors.New("seconds must be in time duration")
			}
			duration += time.Duration(number) * time.Second
		default:
			return 0, errors.New("unsupported duration part")
		}
	}
	if digits.Len() > 0 || duration == 0 {
		return 0, errors.New("duration is malformed")
	}
	return sign * duration, nil
}

func importWeekday(value ics.Weekday) (domain.Weekday, bool) {
	switch value {
	case ics.WeekdayMonday:
		return domain.WeekdayMonday, true
	case ics.WeekdayTuesday:
		return domain.WeekdayTuesday, true
	case ics.WeekdayWednesday:
		return domain.WeekdayWednesday, true
	case ics.WeekdayThursday:
		return domain.WeekdayThursday, true
	case ics.WeekdayFriday:
		return domain.WeekdayFriday, true
	case ics.WeekdaySaturday:
		return domain.WeekdaySaturday, true
	case ics.WeekdaySunday:
		return domain.WeekdaySunday, true
	default:
		return "", false
	}
}

func attendeeRoleFromICal(value string) domain.EventAttendeeRole {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "OPT-PARTICIPANT":
		return domain.EventAttendeeRoleOptional
	case "CHAIR":
		return domain.EventAttendeeRoleChair
	case "NON-PARTICIPANT":
		return domain.EventAttendeeRoleNonParticipant
	default:
		return domain.EventAttendeeRoleRequired
	}
}

func attendeeStatusFromICal(value string) domain.EventParticipationStatus {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "ACCEPTED":
		return domain.EventParticipationStatusAccepted
	case "DECLINED":
		return domain.EventParticipationStatusDeclined
	case "TENTATIVE":
		return domain.EventParticipationStatusTentative
	case "DELEGATED":
		return domain.EventParticipationStatusDelegated
	default:
		return domain.EventParticipationStatusNeedsAction
	}
}

func importSkip(kind string, uid string, reason string) domain.ICalendarImportSkip {
	return domain.ICalendarImportSkip{Kind: kind, UID: uid, Reason: reason}
}

func isICalDate(value string) bool {
	if len(value) != len("20060102") {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func importStringPtr(value string) *string {
	return &value
}
