package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/oklog/ulid/v2"

	"github.com/yazanabuashour/openplanner/internal/domain"
	internalservice "github.com/yazanabuashour/openplanner/internal/service"
)

const (
	maxPlanningRequestBytes                     = 4 << 20
	PlanningTaskActionEnsureCalendar            = "ensure_calendar"
	PlanningTaskActionCreateEvent               = "create_event"
	PlanningTaskActionCreateTask                = "create_task"
	PlanningTaskActionUpdateCalendar            = "update_calendar"
	PlanningTaskActionUpdateEvent               = "update_event"
	PlanningTaskActionUpdateTask                = "update_task"
	PlanningTaskActionDeleteCalendar            = "delete_calendar"
	PlanningTaskActionDeleteEvent               = "delete_event"
	PlanningTaskActionDeleteTask                = "delete_task"
	PlanningTaskActionCreateEventTaskLink       = "create_event_task_link"
	PlanningTaskActionDeleteEventTaskLink       = "delete_event_task_link"
	PlanningTaskActionListEventTaskLinks        = "list_event_task_links"
	PlanningTaskActionCancelEventOccurrence     = "cancel_event_occurrence"
	PlanningTaskActionRescheduleEventOccurrence = "reschedule_event_occurrence"
	PlanningTaskActionCancelTaskOccurrence      = "cancel_task_occurrence"
	PlanningTaskActionRescheduleTaskOccurrence  = "reschedule_task_occurrence"
	PlanningTaskActionListAgenda                = "list_agenda"
	PlanningTaskActionListEvents                = "list_events"
	PlanningTaskActionListTasks                 = "list_tasks"
	PlanningTaskActionCompleteTask              = "complete_task"
	PlanningTaskActionListReminders             = "list_pending_reminders"
	PlanningTaskActionDismissReminder           = "dismiss_reminder"
	PlanningTaskActionExportICalendar           = "export_icalendar"
	PlanningTaskActionImportICalendar           = "import_icalendar"
	PlanningTaskActionValidate                  = "validate"
)

var (
	colorPattern = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
	tagPattern   = regexp.MustCompile(`^[a-z0-9_-]+$`)
)

type PlanningTaskRequest struct {
	Action               string                 `json:"action"`
	CalendarName         string                 `json:"calendar_name,omitempty"`
	CalendarID           string                 `json:"calendar_id,omitempty"`
	EventID              string                 `json:"event_id,omitempty"`
	Name                 string                 `json:"name,omitempty"`
	Description          *string                `json:"description,omitempty"`
	Color                *string                `json:"color,omitempty"`
	Title                string                 `json:"title,omitempty"`
	Location             *string                `json:"location,omitempty"`
	StartAt              string                 `json:"start_at,omitempty"`
	EndAt                string                 `json:"end_at,omitempty"`
	TimeZone             *string                `json:"time_zone,omitempty"`
	StartDate            string                 `json:"start_date,omitempty"`
	EndDate              string                 `json:"end_date,omitempty"`
	DueAt                string                 `json:"due_at,omitempty"`
	DueDate              string                 `json:"due_date,omitempty"`
	Recurrence           *RecurrenceRuleRequest `json:"recurrence,omitempty"`
	Reminders            []ReminderRuleRequest  `json:"reminders,omitempty"`
	Attendees            []EventAttendeeRequest `json:"attendees,omitempty"`
	Priority             string                 `json:"priority,omitempty"`
	Status               string                 `json:"status,omitempty"`
	Tags                 []string               `json:"tags,omitempty"`
	TaskID               string                 `json:"task_id,omitempty"`
	ReminderOccurrenceID string                 `json:"reminder_occurrence_id,omitempty"`
	OccurrenceKey        string                 `json:"occurrence_key,omitempty"`
	OccurrenceAt         string                 `json:"occurrence_at,omitempty"`
	OccurrenceDate       string                 `json:"occurrence_date,omitempty"`
	From                 string                 `json:"from,omitempty"`
	To                   string                 `json:"to,omitempty"`
	Cursor               string                 `json:"cursor,omitempty"`
	Limit                *int                   `json:"limit,omitempty"`
	Content              string                 `json:"content,omitempty"`

	CalendarPatch domain.CalendarPatch `json:"-"`
	EventPatch    domain.EventPatch    `json:"-"`
	TaskPatch     domain.TaskPatch     `json:"-"`
}

type RecurrenceRuleRequest struct {
	Frequency  string   `json:"frequency,omitempty"`
	Interval   *int32   `json:"interval,omitempty"`
	Count      *int32   `json:"count,omitempty"`
	UntilAt    string   `json:"until_at,omitempty"`
	UntilDate  string   `json:"until_date,omitempty"`
	ByWeekday  []string `json:"by_weekday,omitempty"`
	ByMonthDay []int32  `json:"by_month_day,omitempty"`
}

type ReminderRuleRequest struct {
	BeforeMinutes int32 `json:"before_minutes"`
}

type EventAttendeeRequest struct {
	Email               string `json:"email"`
	DisplayName         string `json:"display_name,omitempty"`
	Role                string `json:"role,omitempty"`
	ParticipationStatus string `json:"participation_status,omitempty"`
	RSVP                bool   `json:"rsvp,omitempty"`
}

type PlanningTaskResult struct {
	Rejected        bool                  `json:"rejected"`
	RejectionReason string                `json:"rejection_reason,omitempty"`
	Writes          []PlanningWrite       `json:"writes,omitempty"`
	Calendars       []CalendarEntry       `json:"calendars,omitempty"`
	Events          []EventEntry          `json:"events,omitempty"`
	Tasks           []TaskEntry           `json:"tasks,omitempty"`
	Agenda          []AgendaEntry         `json:"agenda,omitempty"`
	Reminders       []ReminderEntry       `json:"reminders,omitempty"`
	EventTaskLinks  []EventTaskLinkEntry  `json:"event_task_links,omitempty"`
	ICalendar       *ICalendarEntry       `json:"icalendar,omitempty"`
	ICalendarImport *ICalendarImportEntry `json:"icalendar_import,omitempty"`
	NextCursor      string                `json:"next_cursor,omitempty"`
	Summary         string                `json:"summary"`
}

type PlanningWrite struct {
	Kind          string `json:"kind"`
	ID            string `json:"id,omitempty"`
	Status        string `json:"status"`
	Name          string `json:"name,omitempty"`
	Title         string `json:"title,omitempty"`
	OccurrenceKey string `json:"occurrence_key,omitempty"`
}

type CalendarEntry struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	Color       *string `json:"color,omitempty"`
}

type EventEntry struct {
	ID            string                `json:"id"`
	CalendarID    string                `json:"calendar_id"`
	Title         string                `json:"title"`
	Description   *string               `json:"description,omitempty"`
	Location      *string               `json:"location,omitempty"`
	StartAt       string                `json:"start_at,omitempty"`
	EndAt         string                `json:"end_at,omitempty"`
	TimeZone      *string               `json:"time_zone,omitempty"`
	StartDate     string                `json:"start_date,omitempty"`
	EndDate       string                `json:"end_date,omitempty"`
	Recurrence    *RecurrenceRuleResult `json:"recurrence,omitempty"`
	Reminders     []ReminderRuleEntry   `json:"reminders,omitempty"`
	Attendees     []EventAttendeeEntry  `json:"attendees,omitempty"`
	LinkedTaskIDs []string              `json:"linked_task_ids,omitempty"`
}

type TaskEntry struct {
	ID             string                `json:"id"`
	CalendarID     string                `json:"calendar_id"`
	Title          string                `json:"title"`
	Description    *string               `json:"description,omitempty"`
	DueAt          string                `json:"due_at,omitempty"`
	DueDate        string                `json:"due_date,omitempty"`
	Recurrence     *RecurrenceRuleResult `json:"recurrence,omitempty"`
	Reminders      []ReminderRuleEntry   `json:"reminders,omitempty"`
	Priority       string                `json:"priority"`
	Status         string                `json:"status"`
	Tags           []string              `json:"tags"`
	LinkedEventIDs []string              `json:"linked_event_ids,omitempty"`
	CompletedAt    string                `json:"completed_at,omitempty"`
}

type AgendaEntry struct {
	Kind           string               `json:"kind"`
	OccurrenceKey  string               `json:"occurrence_key"`
	CalendarID     string               `json:"calendar_id"`
	SourceID       string               `json:"source_id"`
	Title          string               `json:"title"`
	Description    *string              `json:"description,omitempty"`
	StartAt        string               `json:"start_at,omitempty"`
	EndAt          string               `json:"end_at,omitempty"`
	TimeZone       *string              `json:"time_zone,omitempty"`
	StartDate      string               `json:"start_date,omitempty"`
	EndDate        string               `json:"end_date,omitempty"`
	DueAt          string               `json:"due_at,omitempty"`
	DueDate        string               `json:"due_date,omitempty"`
	Priority       string               `json:"priority,omitempty"`
	Status         string               `json:"status,omitempty"`
	Tags           []string             `json:"tags,omitempty"`
	Attendees      []EventAttendeeEntry `json:"attendees,omitempty"`
	LinkedTaskIDs  []string             `json:"linked_task_ids,omitempty"`
	LinkedEventIDs []string             `json:"linked_event_ids,omitempty"`
	CompletedAt    string               `json:"completed_at,omitempty"`
}

type EventTaskLinkEntry struct {
	EventID string `json:"event_id"`
	TaskID  string `json:"task_id"`
}

type ICalendarEntry struct {
	ContentType  string `json:"content_type"`
	Filename     string `json:"filename"`
	CalendarID   string `json:"calendar_id,omitempty"`
	CalendarName string `json:"calendar_name,omitempty"`
	EventCount   int    `json:"event_count"`
	TaskCount    int    `json:"task_count"`
	Content      string `json:"content"`
}

type ICalendarImportEntry struct {
	CalendarCount int                        `json:"calendar_count"`
	EventCount    int                        `json:"event_count"`
	TaskCount     int                        `json:"task_count"`
	CreatedCount  int                        `json:"created_count"`
	UpdatedCount  int                        `json:"updated_count"`
	SkippedCount  int                        `json:"skipped_count"`
	Skips         []ICalendarImportSkipEntry `json:"skips,omitempty"`
}

type ICalendarImportSkipEntry struct {
	Kind   string `json:"kind"`
	UID    string `json:"uid,omitempty"`
	Reason string `json:"reason"`
}

type ReminderRuleEntry struct {
	ID            string `json:"id"`
	BeforeMinutes int32  `json:"before_minutes"`
}

type EventAttendeeEntry struct {
	Email               string  `json:"email"`
	DisplayName         *string `json:"display_name,omitempty"`
	Role                string  `json:"role"`
	ParticipationStatus string  `json:"participation_status"`
	RSVP                bool    `json:"rsvp"`
}

type ReminderEntry struct {
	ID                   string `json:"id"`
	ReminderOccurrenceID string `json:"reminder_occurrence_id"`
	ReminderID           string `json:"reminder_id"`
	OwnerKind            string `json:"owner_kind"`
	OwnerID              string `json:"owner_id"`
	CalendarID           string `json:"calendar_id"`
	Title                string `json:"title"`
	OccurrenceKey        string `json:"occurrence_key"`
	RemindAt             string `json:"remind_at"`
	BeforeMinutes        int32  `json:"before_minutes"`
	StartAt              string `json:"start_at,omitempty"`
	EndAt                string `json:"end_at,omitempty"`
	StartDate            string `json:"start_date,omitempty"`
	EndDate              string `json:"end_date,omitempty"`
	DueAt                string `json:"due_at,omitempty"`
	DueDate              string `json:"due_date,omitempty"`
}

type RecurrenceRuleResult struct {
	Frequency  string   `json:"frequency"`
	Interval   int32    `json:"interval,omitempty"`
	Count      *int32   `json:"count,omitempty"`
	UntilAt    string   `json:"until_at,omitempty"`
	UntilDate  string   `json:"until_date,omitempty"`
	ByWeekday  []string `json:"by_weekday,omitempty"`
	ByMonthDay []int32  `json:"by_month_day,omitempty"`
}

type normalizedPlanningTaskRequest struct {
	Action               string
	CalendarInput        domain.Calendar
	CalendarPatch        domain.CalendarPatch
	CalendarName         string
	CalendarID           string
	EventID              string
	EventInput           domain.Event
	EventPatch           domain.EventPatch
	TaskInput            domain.Task
	TaskPatch            domain.TaskPatch
	ListOptions          domain.PageParams
	TaskListOptions      domain.TaskListParams
	EventTaskLinkFilter  domain.EventTaskLinkFilter
	AgendaOptions        domain.AgendaParams
	ReminderOptions      domain.ReminderQueryParams
	ICalendarImport      domain.ICalendarImportRequest
	TaskID               string
	ReminderOccurrenceID string
	OccurrenceMutation   domain.OccurrenceMutationRequest
	Completion           domain.TaskCompletionRequest
}

func DecodePlanningTaskRequest(reader io.Reader) (PlanningTaskRequest, error) {
	content, err := readLimitedPlanningRequest(reader)
	if err != nil {
		return PlanningTaskRequest{}, err
	}

	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()

	var raw map[string]json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return PlanningTaskRequest{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return PlanningTaskRequest{}, errors.New("planning request must be a single JSON object")
	}

	for key := range raw {
		if !knownPlanningTaskFields[key] {
			return PlanningTaskRequest{}, fmt.Errorf("json: unknown field %q", key)
		}
	}

	var request PlanningTaskRequest
	content, err = json.Marshal(raw)
	if err != nil {
		return PlanningTaskRequest{}, err
	}
	if err := decodeStrictJSON(content, &request); err != nil {
		return PlanningTaskRequest{}, err
	}
	populatePatchFields(raw, &request)
	return request, nil
}

func readLimitedPlanningRequest(reader io.Reader) ([]byte, error) {
	content, err := io.ReadAll(io.LimitReader(reader, maxPlanningRequestBytes+1))
	if err != nil {
		return nil, err
	}
	if len(content) > maxPlanningRequestBytes {
		return nil, fmt.Errorf("planning request exceeds %d bytes", maxPlanningRequestBytes)
	}
	return content, nil
}

func decodeStrictJSON(content []byte, dest any) error {
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dest); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("planning request must be a single JSON object")
	}
	return nil
}

var knownPlanningTaskFields = map[string]bool{
	"action":                 true,
	"calendar_name":          true,
	"calendar_id":            true,
	"event_id":               true,
	"name":                   true,
	"description":            true,
	"color":                  true,
	"title":                  true,
	"location":               true,
	"start_at":               true,
	"end_at":                 true,
	"time_zone":              true,
	"start_date":             true,
	"end_date":               true,
	"due_at":                 true,
	"due_date":               true,
	"recurrence":             true,
	"reminders":              true,
	"attendees":              true,
	"priority":               true,
	"status":                 true,
	"tags":                   true,
	"task_id":                true,
	"reminder_occurrence_id": true,
	"occurrence_key":         true,
	"occurrence_at":          true,
	"occurrence_date":        true,
	"from":                   true,
	"to":                     true,
	"cursor":                 true,
	"limit":                  true,
	"content":                true,
}

func populatePatchFields(raw map[string]json.RawMessage, request *PlanningTaskRequest) {
	request.CalendarPatch.Name = jsonStringPatch(raw, "name")
	request.CalendarPatch.Description = jsonStringPatch(raw, "description")
	request.CalendarPatch.Color = jsonStringPatch(raw, "color")

	request.EventPatch.Title = jsonStringPatch(raw, "title")
	request.EventPatch.Description = jsonStringPatch(raw, "description")
	request.EventPatch.Location = jsonStringPatch(raw, "location")
	request.EventPatch.StartAt = jsonTimePatch(raw, "start_at")
	request.EventPatch.EndAt = jsonTimePatch(raw, "end_at")
	request.EventPatch.TimeZone = jsonStringPatch(raw, "time_zone")
	request.EventPatch.StartDate = jsonStringPatch(raw, "start_date")
	request.EventPatch.EndDate = jsonStringPatch(raw, "end_date")
	request.EventPatch.Recurrence = jsonRecurrencePatch(raw, "recurrence")
	request.EventPatch.Reminders = jsonRemindersPatch(raw, "reminders")
	request.EventPatch.Attendees = jsonAttendeesPatch(raw, "attendees")

	request.TaskPatch.Title = jsonStringPatch(raw, "title")
	request.TaskPatch.Description = jsonStringPatch(raw, "description")
	request.TaskPatch.DueAt = jsonTimePatch(raw, "due_at")
	request.TaskPatch.DueDate = jsonStringPatch(raw, "due_date")
	request.TaskPatch.Recurrence = jsonRecurrencePatch(raw, "recurrence")
	request.TaskPatch.Reminders = jsonRemindersPatch(raw, "reminders")
	request.TaskPatch.Priority = jsonTaskPriorityPatch(raw, "priority")
	request.TaskPatch.Status = jsonTaskStatusPatch(raw, "status")
	request.TaskPatch.Tags = jsonTagsPatch(raw, "tags")
}

func jsonStringPatch(raw map[string]json.RawMessage, key string) domain.PatchField[string] {
	value, ok := raw[key]
	if !ok {
		return domain.PatchField[string]{}
	}
	if isJSONNull(value) {
		return domain.ClearPatch[string]()
	}
	var decoded string
	if err := json.Unmarshal(value, &decoded); err != nil {
		return domain.PatchField[string]{}
	}
	return domain.SetPatch(decoded)
}

func jsonTimePatch(raw map[string]json.RawMessage, key string) domain.PatchField[time.Time] {
	value, ok := raw[key]
	if !ok {
		return domain.PatchField[time.Time]{}
	}
	if isJSONNull(value) {
		return domain.ClearPatch[time.Time]()
	}
	var decoded string
	if err := json.Unmarshal(value, &decoded); err != nil {
		return domain.PatchField[time.Time]{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(decoded))
	if err != nil {
		return domain.PatchField[time.Time]{}
	}
	return domain.SetPatch(parsed)
}

func jsonRecurrencePatch(raw map[string]json.RawMessage, key string) domain.PatchField[domain.RecurrenceRule] {
	value, ok := raw[key]
	if !ok {
		return domain.PatchField[domain.RecurrenceRule]{}
	}
	if isJSONNull(value) {
		return domain.ClearPatch[domain.RecurrenceRule]()
	}
	var request RecurrenceRuleRequest
	if err := json.Unmarshal(value, &request); err != nil {
		return domain.PatchField[domain.RecurrenceRule]{}
	}
	rule, rejection := normalizeRecurrence(&request, false)
	if rejection != "" || rule == nil {
		return domain.PatchField[domain.RecurrenceRule]{}
	}
	return domain.SetPatch(*rule)
}

func jsonRemindersPatch(raw map[string]json.RawMessage, key string) domain.PatchField[[]domain.ReminderRule] {
	value, ok := raw[key]
	if !ok {
		return domain.PatchField[[]domain.ReminderRule]{}
	}
	if isJSONNull(value) {
		return domain.ClearPatch[[]domain.ReminderRule]()
	}
	var request []ReminderRuleRequest
	if err := json.Unmarshal(value, &request); err != nil {
		return domain.PatchField[[]domain.ReminderRule]{}
	}
	reminders, rejection := normalizeReminders(request)
	if rejection != "" {
		return domain.PatchField[[]domain.ReminderRule]{Present: true}
	}
	return domain.SetPatch(reminders)
}

func jsonAttendeesPatch(raw map[string]json.RawMessage, key string) domain.PatchField[[]domain.EventAttendee] {
	value, ok := raw[key]
	if !ok {
		return domain.PatchField[[]domain.EventAttendee]{}
	}
	if isJSONNull(value) {
		return domain.ClearPatch[[]domain.EventAttendee]()
	}
	var request []EventAttendeeRequest
	if err := json.Unmarshal(value, &request); err != nil {
		return domain.PatchField[[]domain.EventAttendee]{}
	}
	attendees, rejection := normalizeAttendees(request)
	if rejection != "" {
		return domain.PatchField[[]domain.EventAttendee]{Present: true}
	}
	return domain.SetPatch(attendees)
}

func jsonTaskPriorityPatch(raw map[string]json.RawMessage, key string) domain.PatchField[domain.TaskPriority] {
	value, ok := raw[key]
	if !ok {
		return domain.PatchField[domain.TaskPriority]{}
	}
	if isJSONNull(value) {
		return domain.ClearPatch[domain.TaskPriority]()
	}
	var decoded string
	if err := json.Unmarshal(value, &decoded); err != nil {
		return domain.PatchField[domain.TaskPriority]{}
	}
	return domain.SetPatch(domain.TaskPriority(decoded))
}

func jsonTaskStatusPatch(raw map[string]json.RawMessage, key string) domain.PatchField[domain.TaskStatus] {
	value, ok := raw[key]
	if !ok {
		return domain.PatchField[domain.TaskStatus]{}
	}
	if isJSONNull(value) {
		return domain.ClearPatch[domain.TaskStatus]()
	}
	var decoded string
	if err := json.Unmarshal(value, &decoded); err != nil {
		return domain.PatchField[domain.TaskStatus]{}
	}
	return domain.SetPatch(domain.TaskStatus(decoded))
}

func jsonTagsPatch(raw map[string]json.RawMessage, key string) domain.PatchField[[]string] {
	value, ok := raw[key]
	if !ok {
		return domain.PatchField[[]string]{}
	}
	if isJSONNull(value) {
		return domain.ClearPatch[[]string]()
	}
	var decoded []string
	if err := json.Unmarshal(value, &decoded); err != nil {
		return domain.PatchField[[]string]{}
	}
	return domain.SetPatch(decoded)
}

func isJSONNull(value json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(value), []byte("null"))
}

func RunPlanningTask(ctx context.Context, options Options, request PlanningTaskRequest) (PlanningTaskResult, error) {
	normalized, rejection := normalizePlanningTaskRequest(request)
	if rejection != "" {
		return rejectedResult(rejection), nil
	}
	if normalized.Action == PlanningTaskActionValidate {
		return PlanningTaskResult{Summary: "valid"}, nil
	}

	api, err := openLocal(options)
	if err != nil {
		return PlanningTaskResult{}, err
	}
	defer func() {
		_ = api.Close()
	}()

	result, err := runPlanningTask(ctx, api, normalized)
	if err != nil {
		if rejection := expectedRejection(err); rejection != "" {
			return rejectedResult(rejection), nil
		}
		return PlanningTaskResult{}, err
	}
	return result, nil
}

func runPlanningTask(ctx context.Context, api *localRuntime, request normalizedPlanningTaskRequest) (PlanningTaskResult, error) {
	switch request.Action {
	case PlanningTaskActionEnsureCalendar:
		return runEnsureCalendar(ctx, api, request)
	case PlanningTaskActionCreateEvent:
		return runCreateEvent(ctx, api, request)
	case PlanningTaskActionCreateTask:
		return runCreateTask(ctx, api, request)
	case PlanningTaskActionUpdateCalendar:
		return runUpdateCalendar(ctx, api, request)
	case PlanningTaskActionUpdateEvent:
		return runUpdateEvent(ctx, api, request)
	case PlanningTaskActionUpdateTask:
		return runUpdateTask(ctx, api, request)
	case PlanningTaskActionDeleteCalendar:
		return runDeleteCalendar(ctx, api, request)
	case PlanningTaskActionDeleteEvent:
		return runDeleteEvent(ctx, api, request)
	case PlanningTaskActionDeleteTask:
		return runDeleteTask(ctx, api, request)
	case PlanningTaskActionCreateEventTaskLink:
		return runCreateEventTaskLink(ctx, api, request)
	case PlanningTaskActionDeleteEventTaskLink:
		return runDeleteEventTaskLink(ctx, api, request)
	case PlanningTaskActionListEventTaskLinks:
		return runListEventTaskLinks(ctx, api, request)
	case PlanningTaskActionCancelEventOccurrence:
		return runCancelEventOccurrence(ctx, api, request)
	case PlanningTaskActionRescheduleEventOccurrence:
		return runRescheduleEventOccurrence(ctx, api, request)
	case PlanningTaskActionCancelTaskOccurrence:
		return runCancelTaskOccurrence(ctx, api, request)
	case PlanningTaskActionRescheduleTaskOccurrence:
		return runRescheduleTaskOccurrence(ctx, api, request)
	case PlanningTaskActionListAgenda:
		return runListAgenda(ctx, api, request)
	case PlanningTaskActionListEvents:
		return runListEvents(ctx, api, request)
	case PlanningTaskActionListTasks:
		return runListTasks(ctx, api, request)
	case PlanningTaskActionCompleteTask:
		return runCompleteTask(ctx, api, request)
	case PlanningTaskActionListReminders:
		return runListPendingReminders(ctx, api, request)
	case PlanningTaskActionDismissReminder:
		return runDismissReminder(ctx, api, request)
	case PlanningTaskActionExportICalendar:
		return runExportICalendar(ctx, api, request)
	case PlanningTaskActionImportICalendar:
		return runImportICalendar(ctx, api, request)
	default:
		return rejectedResult(fmt.Sprintf("unsupported planning task action %q", request.Action)), nil
	}
}

func runEnsureCalendar(ctx context.Context, api *localRuntime, request normalizedPlanningTaskRequest) (PlanningTaskResult, error) {
	written, err := api.EnsureCalendar(ctx, request.CalendarInput)
	if err != nil {
		return PlanningTaskResult{}, err
	}
	calendar := calendarEntry(written.Calendar)
	return PlanningTaskResult{
		Writes: []PlanningWrite{{
			Kind:   "calendar",
			ID:     written.Calendar.ID,
			Status: string(written.Status),
			Name:   written.Calendar.Name,
		}},
		Calendars: []CalendarEntry{calendar},
		Summary:   fmt.Sprintf("calendar %s", written.Status),
	}, nil
}

func runCreateEvent(ctx context.Context, api *localRuntime, request normalizedPlanningTaskRequest) (PlanningTaskResult, error) {
	calendar, calendarWrite, err := resolveWriteCalendar(ctx, api, request)
	if err != nil {
		return PlanningTaskResult{}, err
	}
	eventInput := request.EventInput
	eventInput.CalendarID = calendar.ID
	event, err := api.CreateEvent(ctx, eventInput)
	if err != nil {
		return PlanningTaskResult{}, err
	}

	result := PlanningTaskResult{
		Calendars: []CalendarEntry{calendarEntry(calendar)},
		Events:    []EventEntry{eventEntry(event)},
		Writes: []PlanningWrite{{
			Kind:   "event",
			ID:     event.ID,
			Status: "created",
			Title:  event.Title,
		}},
		Summary: "created event",
	}
	if calendarWrite != nil {
		result.Writes = append([]PlanningWrite{*calendarWrite}, result.Writes...)
	}
	return result, nil
}

func runCreateTask(ctx context.Context, api *localRuntime, request normalizedPlanningTaskRequest) (PlanningTaskResult, error) {
	calendar, calendarWrite, err := resolveWriteCalendar(ctx, api, request)
	if err != nil {
		return PlanningTaskResult{}, err
	}
	taskInput := request.TaskInput
	taskInput.CalendarID = calendar.ID
	task, err := api.CreateTask(ctx, taskInput)
	if err != nil {
		return PlanningTaskResult{}, err
	}

	result := PlanningTaskResult{
		Calendars: []CalendarEntry{calendarEntry(calendar)},
		Tasks:     []TaskEntry{taskEntry(task)},
		Writes: []PlanningWrite{{
			Kind:   "task",
			ID:     task.ID,
			Status: "created",
			Title:  task.Title,
		}},
		Summary: "created task",
	}
	if calendarWrite != nil {
		result.Writes = append([]PlanningWrite{*calendarWrite}, result.Writes...)
	}
	return result, nil
}

func runUpdateCalendar(ctx context.Context, api *localRuntime, request normalizedPlanningTaskRequest) (PlanningTaskResult, error) {
	calendarID := request.CalendarID
	if calendarID == "" {
		calendar, found, err := findCalendarByName(ctx, api, request.CalendarName)
		if err != nil {
			return PlanningTaskResult{}, err
		}
		if !found {
			return rejectedResult(fmt.Sprintf("calendar %q was not found", request.CalendarName)), nil
		}
		calendarID = calendar.ID
	}
	updated, err := api.UpdateCalendar(ctx, calendarID, request.CalendarPatch)
	if err != nil {
		return PlanningTaskResult{}, err
	}
	return PlanningTaskResult{
		Writes: []PlanningWrite{{
			Kind:   "calendar",
			ID:     updated.ID,
			Status: "updated",
			Name:   updated.Name,
		}},
		Calendars: []CalendarEntry{calendarEntry(updated)},
		Summary:   "updated calendar",
	}, nil
}

func runUpdateEvent(ctx context.Context, api *localRuntime, request normalizedPlanningTaskRequest) (PlanningTaskResult, error) {
	updated, err := api.UpdateEvent(ctx, request.EventID, request.EventPatch)
	if err != nil {
		return PlanningTaskResult{}, err
	}
	return PlanningTaskResult{
		Writes: []PlanningWrite{{
			Kind:   "event",
			ID:     updated.ID,
			Status: "updated",
			Title:  updated.Title,
		}},
		Events:  []EventEntry{eventEntry(updated)},
		Summary: "updated event",
	}, nil
}

func runUpdateTask(ctx context.Context, api *localRuntime, request normalizedPlanningTaskRequest) (PlanningTaskResult, error) {
	updated, err := api.UpdateTask(ctx, request.TaskID, request.TaskPatch)
	if err != nil {
		return PlanningTaskResult{}, err
	}
	return PlanningTaskResult{
		Writes: []PlanningWrite{{
			Kind:   "task",
			ID:     updated.ID,
			Status: "updated",
			Title:  updated.Title,
		}},
		Tasks:   []TaskEntry{taskEntry(updated)},
		Summary: "updated task",
	}, nil
}

func runDeleteCalendar(ctx context.Context, api *localRuntime, request normalizedPlanningTaskRequest) (PlanningTaskResult, error) {
	calendarID := request.CalendarID
	calendarName := request.CalendarName
	if calendarID == "" {
		calendar, found, err := findCalendarByName(ctx, api, calendarName)
		if err != nil {
			return PlanningTaskResult{}, err
		}
		if !found {
			return rejectedResult(fmt.Sprintf("calendar %q was not found", calendarName)), nil
		}
		calendarID = calendar.ID
		calendarName = calendar.Name
	}
	if err := api.DeleteCalendar(ctx, calendarID); err != nil {
		return PlanningTaskResult{}, err
	}
	return PlanningTaskResult{
		Writes: []PlanningWrite{{
			Kind:   "calendar",
			ID:     calendarID,
			Status: "deleted",
			Name:   calendarName,
		}},
		Summary: "deleted calendar",
	}, nil
}

func runDeleteEvent(ctx context.Context, api *localRuntime, request normalizedPlanningTaskRequest) (PlanningTaskResult, error) {
	if err := api.DeleteEvent(ctx, request.EventID); err != nil {
		return PlanningTaskResult{}, err
	}
	return PlanningTaskResult{
		Writes: []PlanningWrite{{
			Kind:   "event",
			ID:     request.EventID,
			Status: "deleted",
		}},
		Summary: "deleted event",
	}, nil
}

func runDeleteTask(ctx context.Context, api *localRuntime, request normalizedPlanningTaskRequest) (PlanningTaskResult, error) {
	if err := api.DeleteTask(ctx, request.TaskID); err != nil {
		return PlanningTaskResult{}, err
	}
	return PlanningTaskResult{
		Writes: []PlanningWrite{{
			Kind:   "task",
			ID:     request.TaskID,
			Status: "deleted",
		}},
		Summary: "deleted task",
	}, nil
}

func runCreateEventTaskLink(ctx context.Context, api *localRuntime, request normalizedPlanningTaskRequest) (PlanningTaskResult, error) {
	link, err := api.CreateEventTaskLink(ctx, request.EventID, request.TaskID)
	if err != nil {
		return PlanningTaskResult{}, err
	}
	return PlanningTaskResult{
		Writes: []PlanningWrite{{
			Kind:   "event_task_link",
			ID:     link.EventID + ":" + link.TaskID,
			Status: "created",
		}},
		EventTaskLinks: []EventTaskLinkEntry{eventTaskLinkEntry(link)},
		Summary:        "created event task link",
	}, nil
}

func runDeleteEventTaskLink(ctx context.Context, api *localRuntime, request normalizedPlanningTaskRequest) (PlanningTaskResult, error) {
	if err := api.DeleteEventTaskLink(ctx, request.EventID, request.TaskID); err != nil {
		return PlanningTaskResult{}, err
	}
	return PlanningTaskResult{
		Writes: []PlanningWrite{{
			Kind:   "event_task_link",
			ID:     request.EventID + ":" + request.TaskID,
			Status: "deleted",
		}},
		Summary: "deleted event task link",
	}, nil
}

func runListEventTaskLinks(ctx context.Context, api *localRuntime, request normalizedPlanningTaskRequest) (PlanningTaskResult, error) {
	links, err := api.ListEventTaskLinks(ctx, request.EventTaskLinkFilter)
	if err != nil {
		return PlanningTaskResult{}, err
	}
	return PlanningTaskResult{
		EventTaskLinks: eventTaskLinkEntries(links),
		Summary:        fmt.Sprintf("returned %d event task links", len(links)),
	}, nil
}

func runCancelEventOccurrence(ctx context.Context, api *localRuntime, request normalizedPlanningTaskRequest) (PlanningTaskResult, error) {
	state, err := api.CancelEventOccurrence(ctx, request.EventID, request.OccurrenceMutation)
	if err != nil {
		return PlanningTaskResult{}, err
	}
	return occurrenceStateResult(state, "event_occurrence", "canceled", "canceled event occurrence"), nil
}

func runRescheduleEventOccurrence(ctx context.Context, api *localRuntime, request normalizedPlanningTaskRequest) (PlanningTaskResult, error) {
	state, err := api.RescheduleEventOccurrence(ctx, request.EventID, request.OccurrenceMutation)
	if err != nil {
		return PlanningTaskResult{}, err
	}
	return occurrenceStateResult(state, "event_occurrence", "rescheduled", "rescheduled event occurrence"), nil
}

func runCancelTaskOccurrence(ctx context.Context, api *localRuntime, request normalizedPlanningTaskRequest) (PlanningTaskResult, error) {
	state, err := api.CancelTaskOccurrence(ctx, request.TaskID, request.OccurrenceMutation)
	if err != nil {
		return PlanningTaskResult{}, err
	}
	return occurrenceStateResult(state, "task_occurrence", "canceled", "canceled task occurrence"), nil
}

func runRescheduleTaskOccurrence(ctx context.Context, api *localRuntime, request normalizedPlanningTaskRequest) (PlanningTaskResult, error) {
	state, err := api.RescheduleTaskOccurrence(ctx, request.TaskID, request.OccurrenceMutation)
	if err != nil {
		return PlanningTaskResult{}, err
	}
	return occurrenceStateResult(state, "task_occurrence", "rescheduled", "rescheduled task occurrence"), nil
}

func occurrenceStateResult(state domain.OccurrenceState, kind string, status string, summary string) PlanningTaskResult {
	return PlanningTaskResult{
		Writes: []PlanningWrite{{
			Kind:          kind,
			ID:            state.OwnerID,
			Status:        status,
			OccurrenceKey: state.OccurrenceKey,
		}},
		Summary: summary,
	}
}

func runListAgenda(ctx context.Context, api *localRuntime, request normalizedPlanningTaskRequest) (PlanningTaskResult, error) {
	page, err := api.ListAgenda(ctx, request.AgendaOptions)
	if err != nil {
		return PlanningTaskResult{}, err
	}
	result := PlanningTaskResult{
		Agenda:  agendaEntries(page.Items),
		Summary: fmt.Sprintf("returned %d agenda items", len(page.Items)),
	}
	if page.NextCursor != nil {
		result.NextCursor = *page.NextCursor
	}
	return result, nil
}

func runListEvents(ctx context.Context, api *localRuntime, request normalizedPlanningTaskRequest) (PlanningTaskResult, error) {
	options := request.ListOptions
	if request.CalendarName != "" {
		calendar, found, err := findCalendarByName(ctx, api, request.CalendarName)
		if err != nil {
			return PlanningTaskResult{}, err
		}
		if !found {
			return rejectedResult(fmt.Sprintf("calendar %q was not found", request.CalendarName)), nil
		}
		options.CalendarID = calendar.ID
	}
	page, err := api.ListEvents(ctx, options)
	if err != nil {
		return PlanningTaskResult{}, err
	}
	result := PlanningTaskResult{
		Events:  eventEntries(page.Items),
		Summary: fmt.Sprintf("returned %d events", len(page.Items)),
	}
	if page.NextCursor != nil {
		result.NextCursor = *page.NextCursor
	}
	return result, nil
}

func runListTasks(ctx context.Context, api *localRuntime, request normalizedPlanningTaskRequest) (PlanningTaskResult, error) {
	options := request.TaskListOptions
	if request.CalendarName != "" {
		calendar, found, err := findCalendarByName(ctx, api, request.CalendarName)
		if err != nil {
			return PlanningTaskResult{}, err
		}
		if !found {
			return rejectedResult(fmt.Sprintf("calendar %q was not found", request.CalendarName)), nil
		}
		options.CalendarID = calendar.ID
	}
	page, err := api.ListTasks(ctx, options)
	if err != nil {
		return PlanningTaskResult{}, err
	}
	result := PlanningTaskResult{
		Tasks:   taskEntries(page.Items),
		Summary: fmt.Sprintf("returned %d tasks", len(page.Items)),
	}
	if page.NextCursor != nil {
		result.NextCursor = *page.NextCursor
	}
	return result, nil
}

func runExportICalendar(ctx context.Context, api *localRuntime, request normalizedPlanningTaskRequest) (PlanningTaskResult, error) {
	calendarID := request.CalendarID
	if request.CalendarName != "" {
		calendar, found, err := findCalendarByName(ctx, api, request.CalendarName)
		if err != nil {
			return PlanningTaskResult{}, err
		}
		if !found {
			return rejectedResult(fmt.Sprintf("calendar %q was not found", request.CalendarName)), nil
		}
		calendarID = calendar.ID
	}
	export, err := api.ExportICalendar(ctx, calendarID)
	if err != nil {
		return PlanningTaskResult{}, err
	}
	entry := iCalendarEntry(export)
	return PlanningTaskResult{
		ICalendar: &entry,
		Summary:   exportICalendarSummary(export),
	}, nil
}

func runImportICalendar(ctx context.Context, api *localRuntime, request normalizedPlanningTaskRequest) (PlanningTaskResult, error) {
	imported, err := api.ImportICalendar(ctx, request.ICalendarImport)
	if err != nil {
		return PlanningTaskResult{}, err
	}
	entry := iCalendarImportEntry(imported)
	return PlanningTaskResult{
		Writes:          importWrites(imported.Writes),
		ICalendarImport: &entry,
		Summary:         importICalendarSummary(imported),
	}, nil
}

func runCompleteTask(ctx context.Context, api *localRuntime, request normalizedPlanningTaskRequest) (PlanningTaskResult, error) {
	completion, err := api.CompleteTask(ctx, request.TaskID, request.Completion)
	if err != nil {
		return PlanningTaskResult{}, err
	}
	return PlanningTaskResult{
		Writes: []PlanningWrite{{
			Kind:          "task_completion",
			ID:            completion.TaskID,
			Status:        "completed",
			OccurrenceKey: completion.OccurrenceKey,
		}},
		Summary: "completed task",
	}, nil
}

func runListPendingReminders(ctx context.Context, api *localRuntime, request normalizedPlanningTaskRequest) (PlanningTaskResult, error) {
	options := request.ReminderOptions
	if request.CalendarName != "" {
		calendar, found, err := findCalendarByName(ctx, api, request.CalendarName)
		if err != nil {
			return PlanningTaskResult{}, err
		}
		if !found {
			return rejectedResult(fmt.Sprintf("calendar %q was not found", request.CalendarName)), nil
		}
		options.CalendarID = calendar.ID
	}
	page, err := api.ListPendingReminders(ctx, options)
	if err != nil {
		return PlanningTaskResult{}, err
	}
	result := PlanningTaskResult{
		Reminders: reminderEntries(page.Items),
		Summary:   fmt.Sprintf("returned %d pending reminders", len(page.Items)),
	}
	if page.NextCursor != nil {
		result.NextCursor = *page.NextCursor
	}
	return result, nil
}

func runDismissReminder(ctx context.Context, api *localRuntime, request normalizedPlanningTaskRequest) (PlanningTaskResult, error) {
	dismissal, err := api.DismissReminderOccurrence(ctx, request.ReminderOccurrenceID)
	if err != nil {
		return PlanningTaskResult{}, err
	}
	status := "dismissed"
	if dismissal.AlreadyDismissed {
		status = "already_dismissed"
	}
	return PlanningTaskResult{
		Writes: []PlanningWrite{{
			Kind:   "reminder_dismissal",
			ID:     dismissal.ReminderID,
			Status: status,
		}},
		Summary: status + " reminder",
	}, nil
}

func resolveWriteCalendar(ctx context.Context, api *localRuntime, request normalizedPlanningTaskRequest) (domain.Calendar, *PlanningWrite, error) {
	if request.CalendarName == "" {
		calendar, found, err := findCalendarByID(ctx, api, request.CalendarID)
		if err != nil {
			return domain.Calendar{}, nil, err
		}
		if !found {
			return domain.Calendar{}, nil, &internalservice.NotFoundError{Resource: "calendar", ID: request.CalendarID, Message: "calendar not found"}
		}
		return calendar, nil, nil
	}
	written, err := api.EnsureCalendar(ctx, domain.Calendar{Name: request.CalendarName})
	if err != nil {
		return domain.Calendar{}, nil, err
	}
	write := PlanningWrite{
		Kind:   "calendar",
		ID:     written.Calendar.ID,
		Status: string(written.Status),
		Name:   written.Calendar.Name,
	}
	return written.Calendar, &write, nil
}

func normalizePlanningTaskRequest(request PlanningTaskRequest) (normalizedPlanningTaskRequest, string) {
	action := strings.TrimSpace(request.Action)
	if action == "" {
		return normalizedPlanningTaskRequest{}, "action is required"
	}

	limit, rejection := normalizeLimit(request.Limit)
	if rejection != "" {
		return normalizedPlanningTaskRequest{}, rejection
	}
	normalized := normalizedPlanningTaskRequest{
		Action: action,
		ListOptions: domain.PageParams{
			Cursor: strings.TrimSpace(request.Cursor),
			Limit:  limit,
		},
		TaskListOptions: domain.TaskListParams{
			PageParams: domain.PageParams{
				Cursor: strings.TrimSpace(request.Cursor),
				Limit:  limit,
			},
		},
	}

	switch action {
	case PlanningTaskActionValidate:
		return normalized, ""
	case PlanningTaskActionEnsureCalendar:
		input, rejection := normalizeCalendarInput(request)
		if rejection != "" {
			return normalizedPlanningTaskRequest{}, rejection
		}
		normalized.CalendarInput = input
		return normalized, ""
	case PlanningTaskActionCreateEvent:
		if rejection := normalizeCalendarRef(request, &normalized); rejection != "" {
			return normalizedPlanningTaskRequest{}, rejection
		}
		input, rejection := normalizeEventInput(request)
		if rejection != "" {
			return normalizedPlanningTaskRequest{}, rejection
		}
		normalized.EventInput = input
		return normalized, ""
	case PlanningTaskActionCreateTask:
		if rejection := normalizeCalendarRef(request, &normalized); rejection != "" {
			return normalizedPlanningTaskRequest{}, rejection
		}
		input, rejection := normalizeTaskInput(request)
		if rejection != "" {
			return normalizedPlanningTaskRequest{}, rejection
		}
		normalized.TaskInput = input
		return normalized, ""
	case PlanningTaskActionUpdateCalendar:
		identifier, patch, rejection := normalizeCalendarPatchInput(request)
		if rejection != "" {
			return normalizedPlanningTaskRequest{}, rejection
		}
		normalized.CalendarName = identifier.CalendarName
		normalized.CalendarID = identifier.CalendarID
		normalized.CalendarPatch = patch
		return normalized, ""
	case PlanningTaskActionUpdateEvent:
		eventID := strings.TrimSpace(request.EventID)
		if eventID == "" {
			return normalizedPlanningTaskRequest{}, "event_id is required"
		}
		if _, err := ulid.ParseStrict(eventID); err != nil {
			return normalizedPlanningTaskRequest{}, "event_id must be a valid ULID"
		}
		patch, rejection := normalizeEventPatchInput(request)
		if rejection != "" {
			return normalizedPlanningTaskRequest{}, rejection
		}
		normalized.EventID = eventID
		normalized.EventPatch = patch
		return normalized, ""
	case PlanningTaskActionUpdateTask:
		taskID := strings.TrimSpace(request.TaskID)
		if taskID == "" {
			return normalizedPlanningTaskRequest{}, "task_id is required"
		}
		if _, err := ulid.ParseStrict(taskID); err != nil {
			return normalizedPlanningTaskRequest{}, "task_id must be a valid ULID"
		}
		patch, rejection := normalizeTaskPatchInput(request)
		if rejection != "" {
			return normalizedPlanningTaskRequest{}, rejection
		}
		normalized.TaskID = taskID
		normalized.TaskPatch = patch
		return normalized, ""
	case PlanningTaskActionDeleteCalendar:
		identifier, rejection := normalizeCalendarDeleteInput(request)
		if rejection != "" {
			return normalizedPlanningTaskRequest{}, rejection
		}
		normalized.CalendarName = identifier.CalendarName
		normalized.CalendarID = identifier.CalendarID
		return normalized, ""
	case PlanningTaskActionDeleteEvent:
		eventID := strings.TrimSpace(request.EventID)
		if eventID == "" {
			return normalizedPlanningTaskRequest{}, "event_id is required"
		}
		if _, err := ulid.ParseStrict(eventID); err != nil {
			return normalizedPlanningTaskRequest{}, "event_id must be a valid ULID"
		}
		normalized.EventID = eventID
		return normalized, ""
	case PlanningTaskActionDeleteTask:
		taskID := strings.TrimSpace(request.TaskID)
		if taskID == "" {
			return normalizedPlanningTaskRequest{}, "task_id is required"
		}
		if _, err := ulid.ParseStrict(taskID); err != nil {
			return normalizedPlanningTaskRequest{}, "task_id must be a valid ULID"
		}
		normalized.TaskID = taskID
		return normalized, ""
	case PlanningTaskActionCreateEventTaskLink:
		if rejection := normalizeEventTaskLinkRef(request, &normalized, true); rejection != "" {
			return normalizedPlanningTaskRequest{}, rejection
		}
		return normalized, ""
	case PlanningTaskActionDeleteEventTaskLink:
		if rejection := normalizeEventTaskLinkRef(request, &normalized, true); rejection != "" {
			return normalizedPlanningTaskRequest{}, rejection
		}
		return normalized, ""
	case PlanningTaskActionListEventTaskLinks:
		if rejection := normalizeEventTaskLinkRef(request, &normalized, false); rejection != "" {
			return normalizedPlanningTaskRequest{}, rejection
		}
		normalized.EventTaskLinkFilter = domain.EventTaskLinkFilter{
			EventID: normalized.EventID,
			TaskID:  normalized.TaskID,
		}
		return normalized, ""
	case PlanningTaskActionCancelEventOccurrence:
		eventID := strings.TrimSpace(request.EventID)
		if eventID == "" {
			return normalizedPlanningTaskRequest{}, "event_id is required"
		}
		if _, err := ulid.ParseStrict(eventID); err != nil {
			return normalizedPlanningTaskRequest{}, "event_id must be a valid ULID"
		}
		mutation, rejection := normalizeOccurrenceMutationInput(request, false, true)
		if rejection != "" {
			return normalizedPlanningTaskRequest{}, rejection
		}
		normalized.EventID = eventID
		normalized.OccurrenceMutation = mutation
		return normalized, ""
	case PlanningTaskActionRescheduleEventOccurrence:
		eventID := strings.TrimSpace(request.EventID)
		if eventID == "" {
			return normalizedPlanningTaskRequest{}, "event_id is required"
		}
		if _, err := ulid.ParseStrict(eventID); err != nil {
			return normalizedPlanningTaskRequest{}, "event_id must be a valid ULID"
		}
		mutation, rejection := normalizeOccurrenceMutationInput(request, true, true)
		if rejection != "" {
			return normalizedPlanningTaskRequest{}, rejection
		}
		normalized.EventID = eventID
		normalized.OccurrenceMutation = mutation
		return normalized, ""
	case PlanningTaskActionCancelTaskOccurrence:
		taskID := strings.TrimSpace(request.TaskID)
		if taskID == "" {
			return normalizedPlanningTaskRequest{}, "task_id is required"
		}
		if _, err := ulid.ParseStrict(taskID); err != nil {
			return normalizedPlanningTaskRequest{}, "task_id must be a valid ULID"
		}
		mutation, rejection := normalizeOccurrenceMutationInput(request, false, false)
		if rejection != "" {
			return normalizedPlanningTaskRequest{}, rejection
		}
		normalized.TaskID = taskID
		normalized.OccurrenceMutation = mutation
		return normalized, ""
	case PlanningTaskActionRescheduleTaskOccurrence:
		taskID := strings.TrimSpace(request.TaskID)
		if taskID == "" {
			return normalizedPlanningTaskRequest{}, "task_id is required"
		}
		if _, err := ulid.ParseStrict(taskID); err != nil {
			return normalizedPlanningTaskRequest{}, "task_id must be a valid ULID"
		}
		mutation, rejection := normalizeOccurrenceMutationInput(request, true, false)
		if rejection != "" {
			return normalizedPlanningTaskRequest{}, rejection
		}
		normalized.TaskID = taskID
		normalized.OccurrenceMutation = mutation
		return normalized, ""
	case PlanningTaskActionListAgenda:
		from, rejection := parseRequiredTime("from", request.From)
		if rejection != "" {
			return normalizedPlanningTaskRequest{}, rejection
		}
		to, rejection := parseRequiredTime("to", request.To)
		if rejection != "" {
			return normalizedPlanningTaskRequest{}, rejection
		}
		if !to.After(from) {
			return normalizedPlanningTaskRequest{}, "to must be after from"
		}
		normalized.AgendaOptions = domain.AgendaParams{
			From:   from,
			To:     to,
			Cursor: strings.TrimSpace(request.Cursor),
			Limit:  limit,
		}
		return normalized, ""
	case PlanningTaskActionListEvents:
		if rejection := normalizeOptionalCalendarFilter(request, &normalized); rejection != "" {
			return normalizedPlanningTaskRequest{}, rejection
		}
		return normalized, ""
	case PlanningTaskActionListTasks:
		if rejection := normalizeOptionalTaskFilter(request, &normalized); rejection != "" {
			return normalizedPlanningTaskRequest{}, rejection
		}
		return normalized, ""
	case PlanningTaskActionCompleteTask:
		taskID := strings.TrimSpace(request.TaskID)
		if taskID == "" {
			return normalizedPlanningTaskRequest{}, "task_id is required"
		}
		if _, err := ulid.ParseStrict(taskID); err != nil {
			return normalizedPlanningTaskRequest{}, "task_id must be a valid ULID"
		}
		completion, rejection := normalizeCompletionInput(request)
		if rejection != "" {
			return normalizedPlanningTaskRequest{}, rejection
		}
		normalized.TaskID = taskID
		normalized.Completion = completion
		return normalized, ""
	case PlanningTaskActionListReminders:
		from, rejection := parseRequiredTime("from", request.From)
		if rejection != "" {
			return normalizedPlanningTaskRequest{}, rejection
		}
		to, rejection := parseRequiredTime("to", request.To)
		if rejection != "" {
			return normalizedPlanningTaskRequest{}, rejection
		}
		if !to.After(from) {
			return normalizedPlanningTaskRequest{}, "to must be after from"
		}
		if rejection := normalizeOptionalCalendarFilter(request, &normalized); rejection != "" {
			return normalizedPlanningTaskRequest{}, rejection
		}
		normalized.ReminderOptions = domain.ReminderQueryParams{
			From:       from,
			To:         to,
			Cursor:     strings.TrimSpace(request.Cursor),
			Limit:      limit,
			CalendarID: normalized.ListOptions.CalendarID,
		}
		return normalized, ""
	case PlanningTaskActionDismissReminder:
		reminderOccurrenceID := strings.TrimSpace(request.ReminderOccurrenceID)
		if reminderOccurrenceID == "" {
			return normalizedPlanningTaskRequest{}, "reminder_occurrence_id is required"
		}
		normalized.ReminderOccurrenceID = reminderOccurrenceID
		return normalized, ""
	case PlanningTaskActionExportICalendar:
		if strings.TrimSpace(request.From) != "" || strings.TrimSpace(request.To) != "" || strings.TrimSpace(request.Cursor) != "" || request.Limit != nil {
			return normalizedPlanningTaskRequest{}, "export_icalendar does not accept from, to, cursor, or limit"
		}
		if rejection := normalizeOptionalCalendarFilter(request, &normalized); rejection != "" {
			return normalizedPlanningTaskRequest{}, rejection
		}
		return normalized, ""
	case PlanningTaskActionImportICalendar:
		if strings.TrimSpace(request.From) != "" || strings.TrimSpace(request.To) != "" || strings.TrimSpace(request.Cursor) != "" || request.Limit != nil {
			return normalizedPlanningTaskRequest{}, "import_icalendar does not accept from, to, cursor, or limit"
		}
		if strings.TrimSpace(request.Content) == "" {
			return normalizedPlanningTaskRequest{}, "content is required"
		}
		if rejection := normalizeOptionalCalendarFilter(request, &normalized); rejection != "" {
			return normalizedPlanningTaskRequest{}, rejection
		}
		normalized.ICalendarImport = domain.ICalendarImportRequest{
			Content:      request.Content,
			CalendarID:   normalized.CalendarID,
			CalendarName: normalized.CalendarName,
		}
		return normalized, ""
	default:
		return normalizedPlanningTaskRequest{}, fmt.Sprintf("unsupported planning task action %q", action)
	}
}

func normalizeEventTaskLinkRef(request PlanningTaskRequest, normalized *normalizedPlanningTaskRequest, requireBoth bool) string {
	eventID := strings.TrimSpace(request.EventID)
	taskID := strings.TrimSpace(request.TaskID)
	if requireBoth && eventID == "" {
		return "event_id is required"
	}
	if requireBoth && taskID == "" {
		return "task_id is required"
	}
	if eventID != "" {
		if _, err := ulid.ParseStrict(eventID); err != nil {
			return "event_id must be a valid ULID"
		}
		normalized.EventID = eventID
	}
	if taskID != "" {
		if _, err := ulid.ParseStrict(taskID); err != nil {
			return "task_id must be a valid ULID"
		}
		normalized.TaskID = taskID
	}

	return ""
}

func normalizeCalendarInput(request PlanningTaskRequest) (domain.Calendar, string) {
	name := calendarName(request)
	if name == "" {
		return domain.Calendar{}, "calendar_name is required"
	}
	if request.Color != nil {
		color := strings.TrimSpace(*request.Color)
		if color != "" && !colorPattern.MatchString(color) {
			return domain.Calendar{}, "color must be a #RRGGBB hex string"
		}
	}
	return domain.Calendar{
		Name:        name,
		Description: request.Description,
		Color:       request.Color,
	}, ""
}

type calendarIdentifier struct {
	CalendarName string
	CalendarID   string
}

func normalizeCalendarPatchInput(request PlanningTaskRequest) (calendarIdentifier, domain.CalendarPatch, string) {
	name := strings.TrimSpace(request.CalendarName)
	id := strings.TrimSpace(request.CalendarID)
	if name == "" && id == "" {
		return calendarIdentifier{}, domain.CalendarPatch{}, "calendar_name or calendar_id is required"
	}
	if name != "" && id != "" {
		return calendarIdentifier{}, domain.CalendarPatch{}, "use calendar_name or calendar_id, not both"
	}
	if id != "" {
		if _, err := ulid.ParseStrict(id); err != nil {
			return calendarIdentifier{}, domain.CalendarPatch{}, "calendar_id must be a valid ULID"
		}
	}

	patch := request.CalendarPatch
	if !patch.Name.Present && strings.TrimSpace(request.Name) != "" {
		patch.Name = domain.SetPatch(request.Name)
	}
	if !patch.Description.Present && request.Description != nil {
		patch.Description = domain.SetPatch(*request.Description)
	}
	if !patch.Color.Present && request.Color != nil {
		patch.Color = domain.SetPatch(*request.Color)
	}
	if patch.Name.Clear {
		return calendarIdentifier{}, domain.CalendarPatch{}, "name cannot be cleared"
	}
	if patch.Color.Present && !patch.Color.Clear {
		color := strings.TrimSpace(patch.Color.Value)
		if color != "" && !colorPattern.MatchString(color) {
			return calendarIdentifier{}, domain.CalendarPatch{}, "color must be a #RRGGBB hex string"
		}
	}
	if !calendarPatchHasUpdate(patch) {
		return calendarIdentifier{}, domain.CalendarPatch{}, "at least one update field is required"
	}
	return calendarIdentifier{CalendarName: name, CalendarID: id}, patch, ""
}

func normalizeCalendarDeleteInput(request PlanningTaskRequest) (calendarIdentifier, string) {
	name := strings.TrimSpace(request.CalendarName)
	id := strings.TrimSpace(request.CalendarID)
	if name == "" && id == "" {
		return calendarIdentifier{}, "calendar_name or calendar_id is required"
	}
	if name != "" && id != "" {
		return calendarIdentifier{}, "use calendar_name or calendar_id, not both"
	}
	if id != "" {
		if _, err := ulid.ParseStrict(id); err != nil {
			return calendarIdentifier{}, "calendar_id must be a valid ULID"
		}
	}
	return calendarIdentifier{CalendarName: name, CalendarID: id}, ""
}

func normalizeCalendarRef(request PlanningTaskRequest, normalized *normalizedPlanningTaskRequest) string {
	name := calendarName(request)
	id := strings.TrimSpace(request.CalendarID)
	if name == "" && id == "" {
		return "calendar_name or calendar_id is required"
	}
	if name != "" && id != "" {
		return "use calendar_name or calendar_id, not both"
	}
	if id != "" {
		if _, err := ulid.ParseStrict(id); err != nil {
			return "calendar_id must be a valid ULID"
		}
		normalized.CalendarID = id
		normalized.ListOptions.CalendarID = id
		return ""
	}
	normalized.CalendarName = name
	return ""
}

func normalizeEventPatchInput(request PlanningTaskRequest) (domain.EventPatch, string) {
	patch := request.EventPatch
	if !patch.Title.Present && strings.TrimSpace(request.Title) != "" {
		patch.Title = domain.SetPatch(request.Title)
	}
	if !patch.Description.Present && request.Description != nil {
		patch.Description = domain.SetPatch(*request.Description)
	}
	if !patch.Location.Present && request.Location != nil {
		patch.Location = domain.SetPatch(*request.Location)
	}
	if !patch.StartAt.Present && strings.TrimSpace(request.StartAt) != "" {
		startAt, rejection := parseRequiredTime("start_at", request.StartAt)
		if rejection != "" {
			return domain.EventPatch{}, rejection
		}
		patch.StartAt = domain.SetPatch(startAt)
	}
	if !patch.EndAt.Present && strings.TrimSpace(request.EndAt) != "" {
		endAt, rejection := parseRequiredTime("end_at", request.EndAt)
		if rejection != "" {
			return domain.EventPatch{}, rejection
		}
		patch.EndAt = domain.SetPatch(endAt)
	}
	if !patch.TimeZone.Present && request.TimeZone != nil {
		timeZone, rejection := normalizeRequiredTimeZone("time_zone", *request.TimeZone)
		if rejection != "" {
			return domain.EventPatch{}, rejection
		}
		patch.TimeZone = domain.SetPatch(timeZone)
	}
	if !patch.StartDate.Present && strings.TrimSpace(request.StartDate) != "" {
		startDate, rejection := parseRequiredDate("start_date", request.StartDate)
		if rejection != "" {
			return domain.EventPatch{}, rejection
		}
		patch.StartDate = domain.SetPatch(startDate)
	}
	if !patch.EndDate.Present && strings.TrimSpace(request.EndDate) != "" {
		endDate, rejection := parseRequiredDate("end_date", request.EndDate)
		if rejection != "" {
			return domain.EventPatch{}, rejection
		}
		patch.EndDate = domain.SetPatch(endDate)
	}
	startDate, rejection := normalizeDatePatch("start_date", patch.StartDate)
	if rejection != "" {
		return domain.EventPatch{}, rejection
	}
	patch.StartDate = startDate
	endDate, rejection := normalizeDatePatch("end_date", patch.EndDate)
	if rejection != "" {
		return domain.EventPatch{}, rejection
	}
	patch.EndDate = endDate
	if patch.TimeZone.Present && !patch.TimeZone.Clear {
		timeZone, rejection := normalizeRequiredTimeZone("time_zone", patch.TimeZone.Value)
		if rejection != "" {
			return domain.EventPatch{}, rejection
		}
		patch.TimeZone = domain.SetPatch(timeZone)
		if (patch.StartDate.Present && !patch.StartDate.Clear) || (patch.EndDate.Present && !patch.EndDate.Clear) {
			return domain.EventPatch{}, "time_zone is only supported for timed events"
		}
		if patch.StartAt.Present && !patch.StartAt.Clear {
			if rejection := validateTimeOffset("start_at", patch.StartAt.Value, timeZone); rejection != "" {
				return domain.EventPatch{}, rejection
			}
		}
		if patch.EndAt.Present && !patch.EndAt.Clear {
			if rejection := validateTimeOffset("end_at", patch.EndAt.Value, timeZone); rejection != "" {
				return domain.EventPatch{}, rejection
			}
		}
	}
	if !patch.Recurrence.Present && request.Recurrence != nil {
		recurrence, rejection := normalizeRecurrence(request.Recurrence, false)
		if rejection != "" {
			return domain.EventPatch{}, rejection
		}
		if recurrence != nil {
			patch.Recurrence = domain.SetPatch(*recurrence)
		}
	}
	if !patch.Reminders.Present && request.Reminders != nil {
		reminders, rejection := normalizeReminders(request.Reminders)
		if rejection != "" {
			return domain.EventPatch{}, rejection
		}
		patch.Reminders = domain.SetPatch(reminders)
	}
	if patch.Reminders.Present && !patch.Reminders.Clear && request.Reminders != nil {
		reminders, rejection := normalizeReminders(request.Reminders)
		if rejection != "" {
			return domain.EventPatch{}, rejection
		}
		patch.Reminders = domain.SetPatch(reminders)
	}
	if !patch.Attendees.Present && request.Attendees != nil {
		attendees, rejection := normalizeAttendees(request.Attendees)
		if rejection != "" {
			return domain.EventPatch{}, rejection
		}
		patch.Attendees = domain.SetPatch(attendees)
	}
	if patch.Attendees.Present && !patch.Attendees.Clear && request.Attendees != nil {
		attendees, rejection := normalizeAttendees(request.Attendees)
		if rejection != "" {
			return domain.EventPatch{}, rejection
		}
		patch.Attendees = domain.SetPatch(attendees)
	}
	if patch.Title.Clear {
		return domain.EventPatch{}, "title cannot be cleared"
	}
	if !eventPatchHasUpdate(patch) {
		return domain.EventPatch{}, "at least one update field is required"
	}
	return patch, ""
}

func normalizeTaskPatchInput(request PlanningTaskRequest) (domain.TaskPatch, string) {
	patch := request.TaskPatch
	if !patch.Title.Present && strings.TrimSpace(request.Title) != "" {
		patch.Title = domain.SetPatch(request.Title)
	}
	if !patch.Description.Present && request.Description != nil {
		patch.Description = domain.SetPatch(*request.Description)
	}
	if !patch.DueAt.Present && strings.TrimSpace(request.DueAt) != "" {
		dueAt, rejection := parseRequiredTime("due_at", request.DueAt)
		if rejection != "" {
			return domain.TaskPatch{}, rejection
		}
		patch.DueAt = domain.SetPatch(dueAt)
	}
	if !patch.DueDate.Present && strings.TrimSpace(request.DueDate) != "" {
		dueDate, rejection := parseRequiredDate("due_date", request.DueDate)
		if rejection != "" {
			return domain.TaskPatch{}, rejection
		}
		patch.DueDate = domain.SetPatch(dueDate)
	}
	dueDate, rejection := normalizeDatePatch("due_date", patch.DueDate)
	if rejection != "" {
		return domain.TaskPatch{}, rejection
	}
	patch.DueDate = dueDate
	if !patch.Recurrence.Present && request.Recurrence != nil {
		recurrence, rejection := normalizeRecurrence(request.Recurrence, false)
		if rejection != "" {
			return domain.TaskPatch{}, rejection
		}
		if recurrence != nil {
			patch.Recurrence = domain.SetPatch(*recurrence)
		}
	}
	if !patch.Reminders.Present && request.Reminders != nil {
		reminders, rejection := normalizeReminders(request.Reminders)
		if rejection != "" {
			return domain.TaskPatch{}, rejection
		}
		patch.Reminders = domain.SetPatch(reminders)
	}
	if patch.Reminders.Present && !patch.Reminders.Clear && request.Reminders != nil {
		reminders, rejection := normalizeReminders(request.Reminders)
		if rejection != "" {
			return domain.TaskPatch{}, rejection
		}
		patch.Reminders = domain.SetPatch(reminders)
	}
	if !patch.Priority.Present && strings.TrimSpace(request.Priority) != "" {
		priority, rejection := normalizeTaskPriority(request.Priority)
		if rejection != "" {
			return domain.TaskPatch{}, rejection
		}
		patch.Priority = domain.SetPatch(priority)
	}
	if !patch.Status.Present && strings.TrimSpace(request.Status) != "" {
		status, rejection := normalizeTaskStatus(request.Status)
		if rejection != "" {
			return domain.TaskPatch{}, rejection
		}
		patch.Status = domain.SetPatch(status)
	}
	if !patch.Tags.Present && request.Tags != nil {
		tags, rejection := normalizeTags(request.Tags)
		if rejection != "" {
			return domain.TaskPatch{}, rejection
		}
		patch.Tags = domain.SetPatch(tags)
	}
	if patch.Title.Clear {
		return domain.TaskPatch{}, "title cannot be cleared"
	}
	if patch.Priority.Clear {
		return domain.TaskPatch{}, "priority cannot be cleared"
	}
	if patch.Status.Clear {
		return domain.TaskPatch{}, "status cannot be cleared"
	}
	if patch.Priority.Present && !patch.Priority.Clear {
		if strings.TrimSpace(string(patch.Priority.Value)) == "" {
			return domain.TaskPatch{}, "priority must be low, medium, or high"
		}
		priority, rejection := normalizeTaskPriority(string(patch.Priority.Value))
		if rejection != "" {
			return domain.TaskPatch{}, rejection
		}
		patch.Priority = domain.SetPatch(priority)
	}
	if patch.Status.Present && !patch.Status.Clear {
		if strings.TrimSpace(string(patch.Status.Value)) == "" {
			return domain.TaskPatch{}, "status must be todo, in_progress, or done"
		}
		status, rejection := normalizeTaskStatus(string(patch.Status.Value))
		if rejection != "" {
			return domain.TaskPatch{}, rejection
		}
		patch.Status = domain.SetPatch(status)
	}
	if patch.Tags.Present && !patch.Tags.Clear {
		tags, rejection := normalizeTags(patch.Tags.Value)
		if rejection != "" {
			return domain.TaskPatch{}, rejection
		}
		patch.Tags = domain.SetPatch(tags)
	}
	if !taskPatchHasUpdate(patch) {
		return domain.TaskPatch{}, "at least one update field is required"
	}
	return patch, ""
}

func normalizeOptionalCalendarFilter(request PlanningTaskRequest, normalized *normalizedPlanningTaskRequest) string {
	name := calendarName(request)
	id := strings.TrimSpace(request.CalendarID)
	if name != "" && id != "" {
		return "use calendar_name or calendar_id, not both"
	}
	if id != "" {
		if _, err := ulid.ParseStrict(id); err != nil {
			return "calendar_id must be a valid ULID"
		}
		normalized.CalendarID = id
		normalized.ListOptions.CalendarID = id
	}
	if name != "" {
		normalized.CalendarName = name
	}
	return ""
}

func normalizeOptionalTaskFilter(request PlanningTaskRequest, normalized *normalizedPlanningTaskRequest) string {
	if rejection := normalizeOptionalCalendarFilter(request, normalized); rejection != "" {
		return rejection
	}
	normalized.TaskListOptions.PageParams = normalized.ListOptions
	normalized.TaskListOptions.CalendarID = normalized.ListOptions.CalendarID
	if strings.TrimSpace(request.Priority) != "" {
		priority, rejection := normalizeTaskPriority(request.Priority)
		if rejection != "" {
			return rejection
		}
		normalized.TaskListOptions.Priority = priority
	}
	if strings.TrimSpace(request.Status) != "" {
		status, rejection := normalizeTaskStatus(request.Status)
		if rejection != "" {
			return rejection
		}
		normalized.TaskListOptions.Status = status
	}
	if request.Tags != nil {
		tags, rejection := normalizeTags(request.Tags)
		if rejection != "" {
			return rejection
		}
		normalized.TaskListOptions.Tags = tags
	}
	return ""
}

func normalizeEventInput(request PlanningTaskRequest) (domain.Event, string) {
	title := strings.TrimSpace(request.Title)
	if title == "" {
		return domain.Event{}, "title is required"
	}
	startAt, rejection := parseOptionalTime("start_at", request.StartAt)
	if rejection != "" {
		return domain.Event{}, rejection
	}
	endAt, rejection := parseOptionalTime("end_at", request.EndAt)
	if rejection != "" {
		return domain.Event{}, rejection
	}
	startDate, rejection := parseOptionalDate("start_date", request.StartDate)
	if rejection != "" {
		return domain.Event{}, rejection
	}
	endDate, rejection := parseOptionalDate("end_date", request.EndDate)
	if rejection != "" {
		return domain.Event{}, rejection
	}

	timed := startAt != nil || endAt != nil
	dated := startDate != nil || endDate != nil
	if timed && dated {
		return domain.Event{}, "use either timed or all-day fields, not both"
	}
	if !timed && !dated {
		return domain.Event{}, "start_at or start_date is required"
	}
	timeZone, rejection := normalizeOptionalTimeZone(request.TimeZone)
	if rejection != "" {
		return domain.Event{}, rejection
	}
	if timeZone != nil && dated {
		return domain.Event{}, "time_zone is only supported for timed events"
	}
	if timeZone != nil {
		if startAt != nil {
			if rejection := validateTimeOffset("start_at", *startAt, *timeZone); rejection != "" {
				return domain.Event{}, rejection
			}
		}
		if endAt != nil {
			if rejection := validateTimeOffset("end_at", *endAt, *timeZone); rejection != "" {
				return domain.Event{}, rejection
			}
		}
	}
	if endAt != nil && startAt == nil {
		return domain.Event{}, "start_at is required when end_at is provided"
	}
	if startAt != nil && endAt != nil && !endAt.After(*startAt) {
		return domain.Event{}, "end_at must be after start_at"
	}
	if endDate != nil && startDate == nil {
		return domain.Event{}, "start_date is required when end_date is provided"
	}
	if startDate != nil && endDate != nil && *endDate < *startDate {
		return domain.Event{}, "end_date must not be before start_date"
	}
	recurrence, rejection := normalizeRecurrence(request.Recurrence, timed)
	if rejection != "" {
		return domain.Event{}, rejection
	}
	reminders, rejection := normalizeReminders(request.Reminders)
	if rejection != "" {
		return domain.Event{}, rejection
	}
	attendees, rejection := normalizeAttendees(request.Attendees)
	if rejection != "" {
		return domain.Event{}, rejection
	}
	return domain.Event{
		Title:       title,
		Description: request.Description,
		Location:    request.Location,
		StartAt:     startAt,
		EndAt:       endAt,
		TimeZone:    timeZone,
		StartDate:   startDate,
		EndDate:     endDate,
		Recurrence:  recurrence,
		Reminders:   reminders,
		Attendees:   attendees,
	}, ""
}

func normalizeTaskInput(request PlanningTaskRequest) (domain.Task, string) {
	title := strings.TrimSpace(request.Title)
	if title == "" {
		return domain.Task{}, "title is required"
	}
	if request.TaskPatch.Priority.Clear {
		return domain.Task{}, "priority cannot be cleared"
	}
	if request.TaskPatch.Status.Clear {
		return domain.Task{}, "status cannot be cleared"
	}
	dueAt, rejection := parseOptionalTime("due_at", request.DueAt)
	if rejection != "" {
		return domain.Task{}, rejection
	}
	dueDate, rejection := parseOptionalDate("due_date", request.DueDate)
	if rejection != "" {
		return domain.Task{}, rejection
	}
	if dueAt != nil && dueDate != nil {
		return domain.Task{}, "use either due_at or due_date, not both"
	}
	recurrence, rejection := normalizeRecurrence(request.Recurrence, dueAt != nil)
	if rejection != "" {
		return domain.Task{}, rejection
	}
	if recurrence != nil && dueAt == nil && dueDate == nil {
		return domain.Task{}, "recurring tasks require due_at or due_date"
	}
	priority, rejection := normalizeTaskPriority(request.Priority)
	if rejection != "" {
		return domain.Task{}, rejection
	}
	status, rejection := normalizeTaskStatus(request.Status)
	if rejection != "" {
		return domain.Task{}, rejection
	}
	tags, rejection := normalizeTags(request.Tags)
	if rejection != "" {
		return domain.Task{}, rejection
	}
	reminders, rejection := normalizeReminders(request.Reminders)
	if rejection != "" {
		return domain.Task{}, rejection
	}
	if len(reminders) > 0 && dueAt == nil && dueDate == nil {
		return domain.Task{}, "task reminders require due_at or due_date"
	}
	return domain.Task{
		Title:       title,
		Description: request.Description,
		DueAt:       dueAt,
		DueDate:     dueDate,
		Recurrence:  recurrence,
		Reminders:   reminders,
		Priority:    priority,
		Status:      status,
		Tags:        tags,
	}, ""
}

func normalizeCompletionInput(request PlanningTaskRequest) (domain.TaskCompletionRequest, string) {
	occurrenceKey := strings.TrimSpace(request.OccurrenceKey)
	occurrenceAt, rejection := parseOptionalTime("occurrence_at", request.OccurrenceAt)
	if rejection != "" {
		return domain.TaskCompletionRequest{}, rejection
	}
	occurrenceDate, rejection := parseOptionalDate("occurrence_date", request.OccurrenceDate)
	if rejection != "" {
		return domain.TaskCompletionRequest{}, rejection
	}
	if occurrenceAt != nil && occurrenceDate != nil {
		return domain.TaskCompletionRequest{}, "use occurrence_at or occurrence_date, not both"
	}
	if occurrenceKey != "" && (occurrenceAt != nil || occurrenceDate != nil) {
		return domain.TaskCompletionRequest{}, "use occurrence_key, occurrence_at, or occurrence_date, not more than one"
	}
	return domain.TaskCompletionRequest{OccurrenceKey: occurrenceKey, OccurrenceAt: occurrenceAt, OccurrenceDate: occurrenceDate}, ""
}

func normalizeOccurrenceMutationInput(request PlanningTaskRequest, requireReplacement bool, event bool) (domain.OccurrenceMutationRequest, string) {
	occurrenceAt, rejection := parseOptionalTime("occurrence_at", request.OccurrenceAt)
	if rejection != "" {
		return domain.OccurrenceMutationRequest{}, rejection
	}
	occurrenceDate, rejection := parseOptionalDate("occurrence_date", request.OccurrenceDate)
	if rejection != "" {
		return domain.OccurrenceMutationRequest{}, rejection
	}
	if occurrenceAt == nil && occurrenceDate == nil {
		return domain.OccurrenceMutationRequest{}, "occurrence_at or occurrence_date is required"
	}
	if occurrenceAt != nil && occurrenceDate != nil {
		return domain.OccurrenceMutationRequest{}, "use occurrence_at or occurrence_date, not both"
	}

	mutation := domain.OccurrenceMutationRequest{
		OccurrenceAt:   occurrenceAt,
		OccurrenceDate: occurrenceDate,
	}
	if !requireReplacement {
		if strings.TrimSpace(request.StartAt) != "" || strings.TrimSpace(request.EndAt) != "" ||
			strings.TrimSpace(request.StartDate) != "" || strings.TrimSpace(request.EndDate) != "" ||
			strings.TrimSpace(request.DueAt) != "" || strings.TrimSpace(request.DueDate) != "" {
			return domain.OccurrenceMutationRequest{}, "canceled occurrences cannot include replacement timing"
		}
		return mutation, ""
	}

	if event {
		startAt, rejection := parseOptionalTime("start_at", request.StartAt)
		if rejection != "" {
			return domain.OccurrenceMutationRequest{}, rejection
		}
		endAt, rejection := parseOptionalTime("end_at", request.EndAt)
		if rejection != "" {
			return domain.OccurrenceMutationRequest{}, rejection
		}
		startDate, rejection := parseOptionalDate("start_date", request.StartDate)
		if rejection != "" {
			return domain.OccurrenceMutationRequest{}, rejection
		}
		endDate, rejection := parseOptionalDate("end_date", request.EndDate)
		if rejection != "" {
			return domain.OccurrenceMutationRequest{}, rejection
		}
		if startAt != nil && startDate != nil {
			return domain.OccurrenceMutationRequest{}, "use start_at or start_date, not both"
		}
		if startAt == nil && startDate == nil {
			return domain.OccurrenceMutationRequest{}, "start_at or start_date is required"
		}
		if endAt != nil && startAt == nil {
			return domain.OccurrenceMutationRequest{}, "start_at is required when end_at is provided"
		}
		if startAt != nil && endAt != nil && !endAt.After(*startAt) {
			return domain.OccurrenceMutationRequest{}, "end_at must be after start_at"
		}
		if endDate != nil && startDate == nil {
			return domain.OccurrenceMutationRequest{}, "start_date is required when end_date is provided"
		}
		if startDate != nil && endDate != nil && *endDate < *startDate {
			return domain.OccurrenceMutationRequest{}, "end_date must not be before start_date"
		}
		if strings.TrimSpace(request.DueAt) != "" || strings.TrimSpace(request.DueDate) != "" {
			return domain.OccurrenceMutationRequest{}, "event occurrence reschedules use start_at or start_date"
		}
		mutation.ReplacementAt = startAt
		mutation.ReplacementEndAt = endAt
		mutation.ReplacementDate = startDate
		mutation.ReplacementEndDate = endDate
		return mutation, ""
	}

	dueAt, rejection := parseOptionalTime("due_at", request.DueAt)
	if rejection != "" {
		return domain.OccurrenceMutationRequest{}, rejection
	}
	dueDate, rejection := parseOptionalDate("due_date", request.DueDate)
	if rejection != "" {
		return domain.OccurrenceMutationRequest{}, rejection
	}
	if dueAt == nil && dueDate == nil {
		return domain.OccurrenceMutationRequest{}, "due_at or due_date is required"
	}
	if dueAt != nil && dueDate != nil {
		return domain.OccurrenceMutationRequest{}, "use due_at or due_date, not both"
	}
	if strings.TrimSpace(request.StartAt) != "" || strings.TrimSpace(request.EndAt) != "" ||
		strings.TrimSpace(request.StartDate) != "" || strings.TrimSpace(request.EndDate) != "" {
		return domain.OccurrenceMutationRequest{}, "task occurrence reschedules use due_at or due_date"
	}
	mutation.ReplacementAt = dueAt
	mutation.ReplacementDate = dueDate
	return mutation, ""
}

func normalizeRecurrence(input *RecurrenceRuleRequest, timed bool) (*domain.RecurrenceRule, string) {
	if input == nil {
		return nil, ""
	}
	frequency := strings.TrimSpace(input.Frequency)
	switch frequency {
	case string(domain.RecurrenceFrequencyDaily), string(domain.RecurrenceFrequencyWeekly), string(domain.RecurrenceFrequencyMonthly):
	case "":
		return nil, "recurrence.frequency is required"
	default:
		return nil, "unsupported recurrence frequency"
	}
	rule := domain.RecurrenceRule{Frequency: domain.RecurrenceFrequency(frequency)}
	if input.Interval != nil {
		if *input.Interval <= 0 {
			return nil, "recurrence.interval must be greater than 0"
		}
		rule.Interval = *input.Interval
	}
	if input.Count != nil {
		if *input.Count <= 0 {
			return nil, "recurrence.count must be greater than 0"
		}
		rule.Count = cloneInt32(input.Count)
	}
	if input.UntilAt != "" {
		untilAt, rejection := parseRequiredTime("recurrence.until_at", input.UntilAt)
		if rejection != "" {
			return nil, rejection
		}
		rule.UntilAt = &untilAt
	}
	if input.UntilDate != "" {
		untilDate, rejection := parseOptionalDate("recurrence.until_date", input.UntilDate)
		if rejection != "" {
			return nil, rejection
		}
		rule.UntilDate = untilDate
	}
	if rule.UntilAt != nil && rule.UntilDate != nil {
		return nil, "use recurrence.until_at or recurrence.until_date, not both"
	}
	if rule.Count != nil && (rule.UntilAt != nil || rule.UntilDate != nil) {
		return nil, "use recurrence.count or recurrence.until, not both"
	}
	if len(input.ByWeekday) > 0 {
		if rule.Frequency != domain.RecurrenceFrequencyWeekly {
			return nil, "recurrence.by_weekday is only supported for weekly recurrence"
		}
		weekdays, rejection := normalizeWeekdays(input.ByWeekday)
		if rejection != "" {
			return nil, rejection
		}
		rule.ByWeekday = weekdays
	}
	if len(input.ByMonthDay) > 0 {
		if rule.Frequency != domain.RecurrenceFrequencyMonthly {
			return nil, "recurrence.by_month_day is only supported for monthly recurrence"
		}
		seen := map[int32]bool{}
		for _, day := range input.ByMonthDay {
			if day < 1 || day > 31 {
				return nil, "recurrence.by_month_day values must be between 1 and 31"
			}
			if seen[day] {
				return nil, "recurrence.by_month_day cannot contain duplicates"
			}
			seen[day] = true
		}
		rule.ByMonthDay = append([]int32(nil), input.ByMonthDay...)
	}
	_ = timed
	return &rule, ""
}

func normalizeWeekdays(values []string) ([]domain.Weekday, string) {
	out := make([]domain.Weekday, 0, len(values))
	seen := map[domain.Weekday]bool{}
	for _, value := range values {
		weekday := domain.Weekday(strings.TrimSpace(value))
		switch weekday {
		case domain.WeekdayMonday, domain.WeekdayTuesday, domain.WeekdayWednesday, domain.WeekdayThursday, domain.WeekdayFriday, domain.WeekdaySaturday, domain.WeekdaySunday:
			if seen[weekday] {
				return nil, "recurrence.by_weekday cannot contain duplicates"
			}
			seen[weekday] = true
			out = append(out, weekday)
		default:
			return nil, "recurrence.by_weekday values must be MO, TU, WE, TH, FR, SA, or SU"
		}
	}
	return out, ""
}

func normalizeLimit(limit *int) (int, string) {
	if limit == nil {
		return 0, ""
	}
	if *limit <= 0 {
		return 0, "limit must be greater than 0"
	}
	if *limit > 200 {
		return 0, "limit must be less than or equal to 200"
	}
	return *limit, ""
}

func normalizeTaskPriority(value string) (domain.TaskPriority, string) {
	value = strings.TrimSpace(value)
	switch domain.TaskPriority(value) {
	case "":
		return "", ""
	case domain.TaskPriorityLow, domain.TaskPriorityMedium, domain.TaskPriorityHigh:
		return domain.TaskPriority(value), ""
	default:
		return "", "priority must be low, medium, or high"
	}
}

func normalizeTaskStatus(value string) (domain.TaskStatus, string) {
	value = strings.TrimSpace(value)
	switch domain.TaskStatus(value) {
	case "":
		return "", ""
	case domain.TaskStatusTodo, domain.TaskStatusInProgress, domain.TaskStatusDone:
		return domain.TaskStatus(value), ""
	default:
		return "", "status must be todo, in_progress, or done"
	}
}

func normalizeTags(values []string) ([]string, string) {
	if values == nil {
		return []string{}, ""
	}
	tags := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, raw := range values {
		tag := strings.ToLower(strings.TrimSpace(raw))
		switch {
		case tag == "":
			return nil, "tags cannot contain empty values"
		case !tagPattern.MatchString(tag):
			return nil, "tags must contain only lowercase letters, digits, underscores, or hyphens"
		case seen[tag]:
			return nil, "tags cannot contain duplicates"
		}
		seen[tag] = true
		tags = append(tags, tag)
	}
	return tags, ""
}

func normalizeReminders(values []ReminderRuleRequest) ([]domain.ReminderRule, string) {
	if values == nil {
		return []domain.ReminderRule{}, ""
	}
	reminders := make([]domain.ReminderRule, 0, len(values))
	seen := map[int32]bool{}
	for _, value := range values {
		switch {
		case value.BeforeMinutes <= 0:
			return nil, "reminders.before_minutes must be greater than 0"
		case seen[value.BeforeMinutes]:
			return nil, "reminders cannot contain duplicate before_minutes values"
		}
		seen[value.BeforeMinutes] = true
		reminders = append(reminders, domain.ReminderRule{BeforeMinutes: value.BeforeMinutes})
	}
	return reminders, ""
}

func normalizeAttendees(values []EventAttendeeRequest) ([]domain.EventAttendee, string) {
	if values == nil {
		return []domain.EventAttendee{}, ""
	}
	attendees := make([]domain.EventAttendee, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		email := strings.TrimSpace(value.Email)
		if !validAttendeeEmail(email) {
			return nil, "attendees.email must be a valid email address"
		}
		emailKey := strings.ToLower(email)
		if seen[emailKey] {
			return nil, "attendees cannot contain duplicate email values"
		}
		seen[emailKey] = true

		role, rejection := normalizeAttendeeRole(value.Role)
		if rejection != "" {
			return nil, rejection
		}
		status, rejection := normalizeParticipationStatus(value.ParticipationStatus)
		if rejection != "" {
			return nil, rejection
		}
		attendees = append(attendees, domain.EventAttendee{
			Email:               email,
			DisplayName:         sanitizeOptionalString(value.DisplayName),
			Role:                role,
			ParticipationStatus: status,
			RSVP:                value.RSVP,
		})
	}
	return attendees, ""
}

func normalizeAttendeeRole(value string) (domain.EventAttendeeRole, string) {
	value = strings.TrimSpace(value)
	switch domain.EventAttendeeRole(value) {
	case "":
		return domain.EventAttendeeRoleRequired, ""
	case domain.EventAttendeeRoleRequired, domain.EventAttendeeRoleOptional, domain.EventAttendeeRoleChair, domain.EventAttendeeRoleNonParticipant:
		return domain.EventAttendeeRole(value), ""
	default:
		return "", "attendees.role must be required, optional, chair, or non_participant"
	}
}

func normalizeParticipationStatus(value string) (domain.EventParticipationStatus, string) {
	value = strings.TrimSpace(value)
	switch domain.EventParticipationStatus(value) {
	case "":
		return domain.EventParticipationStatusNeedsAction, ""
	case domain.EventParticipationStatusNeedsAction, domain.EventParticipationStatusAccepted, domain.EventParticipationStatusDeclined, domain.EventParticipationStatusTentative, domain.EventParticipationStatusDelegated:
		return domain.EventParticipationStatus(value), ""
	default:
		return "", "attendees.participation_status must be needs_action, accepted, declined, tentative, or delegated"
	}
}

func validAttendeeEmail(value string) bool {
	if strings.Count(value, "@") != 1 {
		return false
	}
	parts := strings.Split(value, "@")
	if parts[0] == "" || parts[1] == "" {
		return false
	}
	return !strings.ContainsFunc(value, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r)
	})
}

func sanitizeOptionalString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func calendarName(request PlanningTaskRequest) string {
	if name := strings.TrimSpace(request.CalendarName); name != "" {
		return name
	}
	return strings.TrimSpace(request.Name)
}

func parseOptionalDate(field string, value string) (*string, string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, ""
	}
	parsed, err := time.Parse(time.DateOnly, value)
	if err != nil || parsed.Format(time.DateOnly) != value {
		return nil, field + " must be YYYY-MM-DD"
	}
	return &value, ""
}

func parseRequiredDate(field string, value string) (string, string) {
	parsed, rejection := parseOptionalDate(field, value)
	if rejection != "" {
		return "", rejection
	}
	if parsed == nil {
		return "", field + " is required"
	}
	return *parsed, ""
}

func normalizeDatePatch(field string, patch domain.PatchField[string]) (domain.PatchField[string], string) {
	if !patch.Present || patch.Clear {
		return patch, ""
	}
	value, rejection := parseRequiredDate(field, patch.Value)
	if rejection != "" {
		return domain.PatchField[string]{}, rejection
	}
	return domain.SetPatch(value), ""
}

func parseOptionalTime(field string, value string) (*time.Time, string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, ""
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return nil, field + " must be RFC3339"
	}
	return &parsed, ""
}

func normalizeOptionalTimeZone(value *string) (*string, string) {
	if value == nil {
		return nil, ""
	}
	normalized, rejection := normalizeRequiredTimeZone("time_zone", *value)
	if rejection != "" {
		return nil, rejection
	}
	return &normalized, ""
}

func normalizeRequiredTimeZone(field string, value string) (string, string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", field + " must be a valid IANA timezone"
	}
	if value == "Local" {
		return "", field + " must not be Local"
	}
	if _, err := time.LoadLocation(value); err != nil {
		return "", field + " must be a valid IANA timezone"
	}
	return value, ""
}

func validateTimeOffset(field string, value time.Time, timeZone string) string {
	location, err := time.LoadLocation(timeZone)
	if err != nil {
		return field + " time_zone is invalid"
	}
	_, inputOffset := value.Zone()
	_, zoneOffset := value.In(location).Zone()
	if inputOffset != zoneOffset {
		return field + " offset must match time_zone"
	}
	return ""
}

func calendarPatchHasUpdate(patch domain.CalendarPatch) bool {
	return patch.Name.Present || patch.Description.Present || patch.Color.Present
}

func eventPatchHasUpdate(patch domain.EventPatch) bool {
	return patch.Title.Present ||
		patch.Description.Present ||
		patch.Location.Present ||
		patch.StartAt.Present ||
		patch.EndAt.Present ||
		patch.TimeZone.Present ||
		patch.StartDate.Present ||
		patch.EndDate.Present ||
		patch.Recurrence.Present ||
		patch.Reminders.Present ||
		patch.Attendees.Present
}

func taskPatchHasUpdate(patch domain.TaskPatch) bool {
	return patch.Title.Present ||
		patch.Description.Present ||
		patch.DueAt.Present ||
		patch.DueDate.Present ||
		patch.Recurrence.Present ||
		patch.Reminders.Present ||
		patch.Priority.Present ||
		patch.Status.Present ||
		patch.Tags.Present
}

func parseRequiredTime(field string, value string) (time.Time, string) {
	parsed, rejection := parseOptionalTime(field, value)
	if rejection != "" {
		return time.Time{}, rejection
	}
	if parsed == nil {
		return time.Time{}, field + " is required"
	}
	return *parsed, ""
}

func findCalendarByName(ctx context.Context, api *localRuntime, name string) (domain.Calendar, bool, error) {
	cursor := ""
	for {
		page, err := api.ListCalendars(ctx, domain.PageParams{Cursor: cursor, Limit: 200})
		if err != nil {
			return domain.Calendar{}, false, err
		}
		for _, calendar := range page.Items {
			if calendar.Name == name {
				return calendar, true, nil
			}
		}
		if page.NextCursor == nil {
			return domain.Calendar{}, false, nil
		}
		cursor = *page.NextCursor
	}
}

func findCalendarByID(ctx context.Context, api *localRuntime, id string) (domain.Calendar, bool, error) {
	cursor := ""
	for {
		page, err := api.ListCalendars(ctx, domain.PageParams{Cursor: cursor, Limit: 200})
		if err != nil {
			return domain.Calendar{}, false, err
		}
		for _, calendar := range page.Items {
			if calendar.ID == id {
				return calendar, true, nil
			}
		}
		if page.NextCursor == nil {
			return domain.Calendar{}, false, nil
		}
		cursor = *page.NextCursor
	}
}

func expectedRejection(err error) string {
	var validationErr *internalservice.ValidationError
	if errors.As(err, &validationErr) {
		return validationErr.Error()
	}
	var conflictErr *internalservice.ConflictError
	if errors.As(err, &conflictErr) {
		return conflictErr.Error()
	}
	var notFoundErr *internalservice.NotFoundError
	if errors.As(err, &notFoundErr) {
		return notFoundErr.Error()
	}
	return ""
}

func rejectedResult(reason string) PlanningTaskResult {
	return PlanningTaskResult{
		Rejected:        true,
		RejectionReason: reason,
		Summary:         reason,
	}
}

func calendarEntry(calendar domain.Calendar) CalendarEntry {
	return CalendarEntry{
		ID:          calendar.ID,
		Name:        calendar.Name,
		Description: cloneString(calendar.Description),
		Color:       cloneString(calendar.Color),
	}
}

func eventEntries(events []domain.Event) []EventEntry {
	out := make([]EventEntry, 0, len(events))
	for _, event := range events {
		out = append(out, eventEntry(event))
	}
	return out
}

func eventEntry(event domain.Event) EventEntry {
	out := EventEntry{
		ID:            event.ID,
		CalendarID:    event.CalendarID,
		Title:         event.Title,
		Description:   cloneString(event.Description),
		Location:      cloneString(event.Location),
		TimeZone:      cloneString(event.TimeZone),
		StartDate:     stringValue(event.StartDate),
		EndDate:       stringValue(event.EndDate),
		Recurrence:    recurrenceResult(event.Recurrence),
		Reminders:     reminderRuleEntries(event.Reminders),
		Attendees:     attendeeEntries(event.Attendees),
		LinkedTaskIDs: slices.Clone(event.LinkedTaskIDs),
	}
	if event.StartAt != nil {
		out.StartAt = formatJSONTime(*event.StartAt)
	}
	if event.EndAt != nil {
		out.EndAt = formatJSONTime(*event.EndAt)
	}
	return out
}

func taskEntries(tasks []domain.Task) []TaskEntry {
	out := make([]TaskEntry, 0, len(tasks))
	for _, task := range tasks {
		out = append(out, taskEntry(task))
	}
	return out
}

func taskEntry(task domain.Task) TaskEntry {
	out := TaskEntry{
		ID:             task.ID,
		CalendarID:     task.CalendarID,
		Title:          task.Title,
		Description:    cloneString(task.Description),
		DueDate:        stringValue(task.DueDate),
		Recurrence:     recurrenceResult(task.Recurrence),
		Reminders:      reminderRuleEntries(task.Reminders),
		Priority:       string(task.Priority),
		Status:         string(task.Status),
		Tags:           slices.Clone(task.Tags),
		LinkedEventIDs: slices.Clone(task.LinkedEventIDs),
	}
	if task.DueAt != nil {
		out.DueAt = formatJSONTime(*task.DueAt)
	}
	if task.CompletedAt != nil {
		out.CompletedAt = formatJSONTime(*task.CompletedAt)
	}
	return out
}

func reminderRuleEntries(reminders []domain.ReminderRule) []ReminderRuleEntry {
	if len(reminders) == 0 {
		return nil
	}
	out := make([]ReminderRuleEntry, 0, len(reminders))
	for _, reminder := range reminders {
		out = append(out, ReminderRuleEntry{
			ID:            reminder.ID,
			BeforeMinutes: reminder.BeforeMinutes,
		})
	}
	return out
}

func attendeeEntries(attendees []domain.EventAttendee) []EventAttendeeEntry {
	if len(attendees) == 0 {
		return nil
	}
	out := make([]EventAttendeeEntry, 0, len(attendees))
	for _, attendee := range attendees {
		out = append(out, EventAttendeeEntry{
			Email:               attendee.Email,
			DisplayName:         cloneString(attendee.DisplayName),
			Role:                string(attendee.Role),
			ParticipationStatus: string(attendee.ParticipationStatus),
			RSVP:                attendee.RSVP,
		})
	}
	return out
}

func agendaEntries(items []domain.AgendaItem) []AgendaEntry {
	out := make([]AgendaEntry, 0, len(items))
	for _, item := range items {
		entry := AgendaEntry{
			Kind:           string(item.Kind),
			OccurrenceKey:  item.OccurrenceKey,
			CalendarID:     item.CalendarID,
			SourceID:       item.SourceID,
			Title:          item.Title,
			Description:    cloneString(item.Description),
			TimeZone:       cloneString(item.TimeZone),
			StartDate:      stringValue(item.StartDate),
			EndDate:        stringValue(item.EndDate),
			DueDate:        stringValue(item.DueDate),
			Priority:       string(item.Priority),
			Status:         string(item.Status),
			Tags:           slices.Clone(item.Tags),
			Attendees:      attendeeEntries(item.Attendees),
			LinkedTaskIDs:  slices.Clone(item.LinkedTaskIDs),
			LinkedEventIDs: slices.Clone(item.LinkedEventIDs),
		}
		if item.StartAt != nil {
			entry.StartAt = formatJSONTime(*item.StartAt)
		}
		if item.EndAt != nil {
			entry.EndAt = formatJSONTime(*item.EndAt)
		}
		if item.DueAt != nil {
			entry.DueAt = formatJSONTime(*item.DueAt)
		}
		if item.CompletedAt != nil {
			entry.CompletedAt = formatJSONTime(*item.CompletedAt)
		}
		out = append(out, entry)
	}
	return out
}

func eventTaskLinkEntries(links []domain.EventTaskLink) []EventTaskLinkEntry {
	out := make([]EventTaskLinkEntry, 0, len(links))
	for _, link := range links {
		out = append(out, eventTaskLinkEntry(link))
	}
	return out
}

func eventTaskLinkEntry(link domain.EventTaskLink) EventTaskLinkEntry {
	return EventTaskLinkEntry{
		EventID: link.EventID,
		TaskID:  link.TaskID,
	}
}

func iCalendarEntry(export domain.ICalendarExport) ICalendarEntry {
	return ICalendarEntry{
		ContentType:  export.ContentType,
		Filename:     export.Filename,
		CalendarID:   export.CalendarID,
		CalendarName: export.CalendarName,
		EventCount:   export.EventCount,
		TaskCount:    export.TaskCount,
		Content:      export.Content,
	}
}

func iCalendarImportEntry(imported domain.ICalendarImport) ICalendarImportEntry {
	return ICalendarImportEntry{
		CalendarCount: imported.CalendarCount,
		EventCount:    imported.EventCount,
		TaskCount:     imported.TaskCount,
		CreatedCount:  imported.CreatedCount,
		UpdatedCount:  imported.UpdatedCount,
		SkippedCount:  imported.SkippedCount,
		Skips:         iCalendarImportSkipEntries(imported.Skips),
	}
}

func iCalendarImportSkipEntries(skips []domain.ICalendarImportSkip) []ICalendarImportSkipEntry {
	if len(skips) == 0 {
		return nil
	}
	out := make([]ICalendarImportSkipEntry, 0, len(skips))
	for _, skip := range skips {
		out = append(out, ICalendarImportSkipEntry{
			Kind:   skip.Kind,
			UID:    skip.UID,
			Reason: skip.Reason,
		})
	}
	return out
}

func importWrites(writes []domain.ICalendarImportWrite) []PlanningWrite {
	if len(writes) == 0 {
		return nil
	}
	out := make([]PlanningWrite, 0, len(writes))
	for _, write := range writes {
		out = append(out, PlanningWrite{
			Kind:          write.Kind,
			ID:            write.ID,
			Status:        write.Status,
			Name:          write.Name,
			Title:         write.Title,
			OccurrenceKey: write.OccurrenceKey,
		})
	}
	return out
}

func exportICalendarSummary(export domain.ICalendarExport) string {
	if export.CalendarID != "" {
		return "exported 1 calendar to iCalendar"
	}
	return fmt.Sprintf("exported %d events and %d tasks", export.EventCount, export.TaskCount)
}

func importICalendarSummary(imported domain.ICalendarImport) string {
	return fmt.Sprintf("imported %d events and %d tasks (%d created, %d updated, %d skipped)", imported.EventCount, imported.TaskCount, imported.CreatedCount, imported.UpdatedCount, imported.SkippedCount)
}

func reminderEntries(items []domain.PendingReminder) []ReminderEntry {
	out := make([]ReminderEntry, 0, len(items))
	for _, item := range items {
		entry := ReminderEntry{
			ID:                   item.ID,
			ReminderOccurrenceID: item.ID,
			ReminderID:           item.ReminderID,
			OwnerKind:            string(item.OwnerKind),
			OwnerID:              item.OwnerID,
			CalendarID:           item.CalendarID,
			Title:                item.Title,
			OccurrenceKey:        item.OccurrenceKey,
			RemindAt:             formatJSONTime(item.RemindAt),
			BeforeMinutes:        item.BeforeMinutes,
			StartDate:            stringValue(item.StartDate),
			EndDate:              stringValue(item.EndDate),
			DueDate:              stringValue(item.DueDate),
		}
		if item.StartAt != nil {
			entry.StartAt = formatJSONTime(*item.StartAt)
		}
		if item.EndAt != nil {
			entry.EndAt = formatJSONTime(*item.EndAt)
		}
		if item.DueAt != nil {
			entry.DueAt = formatJSONTime(*item.DueAt)
		}
		out = append(out, entry)
	}
	return out
}

func recurrenceResult(rule *domain.RecurrenceRule) *RecurrenceRuleResult {
	if rule == nil {
		return nil
	}
	out := &RecurrenceRuleResult{
		Frequency:  string(rule.Frequency),
		Interval:   rule.Interval,
		Count:      cloneInt32(rule.Count),
		UntilDate:  stringValue(rule.UntilDate),
		ByMonthDay: append([]int32(nil), rule.ByMonthDay...),
	}
	if rule.UntilAt != nil {
		out.UntilAt = formatJSONTime(*rule.UntilAt)
	}
	if len(rule.ByWeekday) > 0 {
		out.ByWeekday = make([]string, 0, len(rule.ByWeekday))
		for _, weekday := range rule.ByWeekday {
			out.ByWeekday = append(out.ByWeekday, string(weekday))
		}
	}
	return out
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func formatJSONTime(value time.Time) string {
	return value.Format(time.RFC3339Nano)
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneInt32(value *int32) *int32 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
