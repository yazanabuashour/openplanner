package agentops

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"

	internalservice "github.com/yazanabuashour/openplanner/internal/service"
	"github.com/yazanabuashour/openplanner/sdk"
)

const (
	PlanningTaskActionEnsureCalendar = "ensure_calendar"
	PlanningTaskActionCreateEvent    = "create_event"
	PlanningTaskActionCreateTask     = "create_task"
	PlanningTaskActionListAgenda     = "list_agenda"
	PlanningTaskActionListEvents     = "list_events"
	PlanningTaskActionListTasks      = "list_tasks"
	PlanningTaskActionCompleteTask   = "complete_task"
	PlanningTaskActionValidate       = "validate"
)

var colorPattern = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

type PlanningTaskRequest struct {
	Action         string                 `json:"action"`
	CalendarName   string                 `json:"calendar_name,omitempty"`
	CalendarID     string                 `json:"calendar_id,omitempty"`
	Name           string                 `json:"name,omitempty"`
	Description    *string                `json:"description,omitempty"`
	Color          *string                `json:"color,omitempty"`
	Title          string                 `json:"title,omitempty"`
	Location       *string                `json:"location,omitempty"`
	StartAt        string                 `json:"start_at,omitempty"`
	EndAt          string                 `json:"end_at,omitempty"`
	StartDate      string                 `json:"start_date,omitempty"`
	EndDate        string                 `json:"end_date,omitempty"`
	DueAt          string                 `json:"due_at,omitempty"`
	DueDate        string                 `json:"due_date,omitempty"`
	Recurrence     *RecurrenceRuleRequest `json:"recurrence,omitempty"`
	TaskID         string                 `json:"task_id,omitempty"`
	OccurrenceAt   string                 `json:"occurrence_at,omitempty"`
	OccurrenceDate string                 `json:"occurrence_date,omitempty"`
	From           string                 `json:"from,omitempty"`
	To             string                 `json:"to,omitempty"`
	Cursor         string                 `json:"cursor,omitempty"`
	Limit          *int                   `json:"limit,omitempty"`
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

type PlanningTaskResult struct {
	Rejected        bool            `json:"rejected"`
	RejectionReason string          `json:"rejection_reason,omitempty"`
	Writes          []PlanningWrite `json:"writes,omitempty"`
	Calendars       []CalendarEntry `json:"calendars,omitempty"`
	Events          []EventEntry    `json:"events,omitempty"`
	Tasks           []TaskEntry     `json:"tasks,omitempty"`
	Agenda          []AgendaEntry   `json:"agenda,omitempty"`
	NextCursor      string          `json:"next_cursor,omitempty"`
	Summary         string          `json:"summary"`
}

type PlanningWrite struct {
	Kind   string `json:"kind"`
	ID     string `json:"id,omitempty"`
	Status string `json:"status"`
	Name   string `json:"name,omitempty"`
	Title  string `json:"title,omitempty"`
}

type CalendarEntry struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	Color       *string `json:"color,omitempty"`
}

type EventEntry struct {
	ID          string                `json:"id"`
	CalendarID  string                `json:"calendar_id"`
	Title       string                `json:"title"`
	Description *string               `json:"description,omitempty"`
	Location    *string               `json:"location,omitempty"`
	StartAt     string                `json:"start_at,omitempty"`
	EndAt       string                `json:"end_at,omitempty"`
	StartDate   string                `json:"start_date,omitempty"`
	EndDate     string                `json:"end_date,omitempty"`
	Recurrence  *RecurrenceRuleResult `json:"recurrence,omitempty"`
}

type TaskEntry struct {
	ID          string                `json:"id"`
	CalendarID  string                `json:"calendar_id"`
	Title       string                `json:"title"`
	Description *string               `json:"description,omitempty"`
	DueAt       string                `json:"due_at,omitempty"`
	DueDate     string                `json:"due_date,omitempty"`
	Recurrence  *RecurrenceRuleResult `json:"recurrence,omitempty"`
	CompletedAt string                `json:"completed_at,omitempty"`
}

type AgendaEntry struct {
	Kind          string  `json:"kind"`
	OccurrenceKey string  `json:"occurrence_key"`
	CalendarID    string  `json:"calendar_id"`
	SourceID      string  `json:"source_id"`
	Title         string  `json:"title"`
	Description   *string `json:"description,omitempty"`
	StartAt       string  `json:"start_at,omitempty"`
	EndAt         string  `json:"end_at,omitempty"`
	StartDate     string  `json:"start_date,omitempty"`
	EndDate       string  `json:"end_date,omitempty"`
	DueAt         string  `json:"due_at,omitempty"`
	DueDate       string  `json:"due_date,omitempty"`
	CompletedAt   string  `json:"completed_at,omitempty"`
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
	Action        string
	CalendarInput sdk.CalendarInput
	CalendarName  string
	CalendarID    string
	EventInput    sdk.EventInput
	TaskInput     sdk.TaskInput
	ListOptions   sdk.ListOptions
	AgendaOptions sdk.AgendaOptions
	TaskID        string
	Completion    sdk.TaskCompletionInput
}

func RunPlanningTask(ctx context.Context, options sdk.Options, request PlanningTaskRequest) (PlanningTaskResult, error) {
	normalized, rejection := normalizePlanningTaskRequest(request)
	if rejection != "" {
		return rejectedResult(rejection), nil
	}
	if normalized.Action == PlanningTaskActionValidate {
		return PlanningTaskResult{Summary: "valid"}, nil
	}

	api, err := sdk.OpenLocal(options)
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

func runPlanningTask(ctx context.Context, api *sdk.Client, request normalizedPlanningTaskRequest) (PlanningTaskResult, error) {
	switch request.Action {
	case PlanningTaskActionEnsureCalendar:
		return runEnsureCalendar(ctx, api, request)
	case PlanningTaskActionCreateEvent:
		return runCreateEvent(ctx, api, request)
	case PlanningTaskActionCreateTask:
		return runCreateTask(ctx, api, request)
	case PlanningTaskActionListAgenda:
		return runListAgenda(ctx, api, request)
	case PlanningTaskActionListEvents:
		return runListEvents(ctx, api, request)
	case PlanningTaskActionListTasks:
		return runListTasks(ctx, api, request)
	case PlanningTaskActionCompleteTask:
		return runCompleteTask(ctx, api, request)
	default:
		return rejectedResult(fmt.Sprintf("unsupported planning task action %q", request.Action)), nil
	}
}

func runEnsureCalendar(ctx context.Context, api *sdk.Client, request normalizedPlanningTaskRequest) (PlanningTaskResult, error) {
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

func runCreateEvent(ctx context.Context, api *sdk.Client, request normalizedPlanningTaskRequest) (PlanningTaskResult, error) {
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

func runCreateTask(ctx context.Context, api *sdk.Client, request normalizedPlanningTaskRequest) (PlanningTaskResult, error) {
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

func runListAgenda(ctx context.Context, api *sdk.Client, request normalizedPlanningTaskRequest) (PlanningTaskResult, error) {
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

func runListEvents(ctx context.Context, api *sdk.Client, request normalizedPlanningTaskRequest) (PlanningTaskResult, error) {
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

func runListTasks(ctx context.Context, api *sdk.Client, request normalizedPlanningTaskRequest) (PlanningTaskResult, error) {
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

func runCompleteTask(ctx context.Context, api *sdk.Client, request normalizedPlanningTaskRequest) (PlanningTaskResult, error) {
	completion, err := api.CompleteTask(ctx, request.TaskID, request.Completion)
	if err != nil {
		return PlanningTaskResult{}, err
	}
	return PlanningTaskResult{
		Writes: []PlanningWrite{{
			Kind:   "task_completion",
			ID:     completion.TaskID,
			Status: "completed",
		}},
		Summary: "completed task",
	}, nil
}

func resolveWriteCalendar(ctx context.Context, api *sdk.Client, request normalizedPlanningTaskRequest) (sdk.Calendar, *PlanningWrite, error) {
	if request.CalendarName == "" {
		calendar, found, err := findCalendarByID(ctx, api, request.CalendarID)
		if err != nil {
			return sdk.Calendar{}, nil, err
		}
		if !found {
			return sdk.Calendar{}, nil, &internalservice.NotFoundError{Resource: "calendar", ID: request.CalendarID, Message: "calendar not found"}
		}
		return calendar, nil, nil
	}
	written, err := api.EnsureCalendar(ctx, sdk.CalendarInput{Name: request.CalendarName})
	if err != nil {
		return sdk.Calendar{}, nil, err
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
		ListOptions: sdk.ListOptions{
			Cursor: strings.TrimSpace(request.Cursor),
			Limit:  limit,
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
		normalized.AgendaOptions = sdk.AgendaOptions{
			From:   from,
			To:     to,
			Cursor: strings.TrimSpace(request.Cursor),
			Limit:  limit,
		}
		return normalized, ""
	case PlanningTaskActionListEvents, PlanningTaskActionListTasks:
		if rejection := normalizeOptionalCalendarFilter(request, &normalized); rejection != "" {
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
	default:
		return normalizedPlanningTaskRequest{}, fmt.Sprintf("unsupported planning task action %q", action)
	}
}

func normalizeCalendarInput(request PlanningTaskRequest) (sdk.CalendarInput, string) {
	name := calendarName(request)
	if name == "" {
		return sdk.CalendarInput{}, "calendar_name is required"
	}
	if request.Color != nil {
		color := strings.TrimSpace(*request.Color)
		if color != "" && !colorPattern.MatchString(color) {
			return sdk.CalendarInput{}, "color must be a #RRGGBB hex string"
		}
	}
	return sdk.CalendarInput{
		Name:        name,
		Description: request.Description,
		Color:       request.Color,
	}, ""
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

func normalizeEventInput(request PlanningTaskRequest) (sdk.EventInput, string) {
	title := strings.TrimSpace(request.Title)
	if title == "" {
		return sdk.EventInput{}, "title is required"
	}
	startAt, rejection := parseOptionalTime("start_at", request.StartAt)
	if rejection != "" {
		return sdk.EventInput{}, rejection
	}
	endAt, rejection := parseOptionalTime("end_at", request.EndAt)
	if rejection != "" {
		return sdk.EventInput{}, rejection
	}
	startDate, rejection := parseOptionalDate("start_date", request.StartDate)
	if rejection != "" {
		return sdk.EventInput{}, rejection
	}
	endDate, rejection := parseOptionalDate("end_date", request.EndDate)
	if rejection != "" {
		return sdk.EventInput{}, rejection
	}

	timed := startAt != nil || endAt != nil
	dated := startDate != nil || endDate != nil
	if timed && dated {
		return sdk.EventInput{}, "use either timed or all-day fields, not both"
	}
	if !timed && !dated {
		return sdk.EventInput{}, "start_at or start_date is required"
	}
	if endAt != nil && startAt == nil {
		return sdk.EventInput{}, "start_at is required when end_at is provided"
	}
	if startAt != nil && endAt != nil && !endAt.After(*startAt) {
		return sdk.EventInput{}, "end_at must be after start_at"
	}
	if endDate != nil && startDate == nil {
		return sdk.EventInput{}, "start_date is required when end_date is provided"
	}
	if startDate != nil && endDate != nil && *endDate < *startDate {
		return sdk.EventInput{}, "end_date must not be before start_date"
	}
	recurrence, rejection := normalizeRecurrence(request.Recurrence, timed)
	if rejection != "" {
		return sdk.EventInput{}, rejection
	}
	return sdk.EventInput{
		Title:       title,
		Description: request.Description,
		Location:    request.Location,
		StartAt:     startAt,
		EndAt:       endAt,
		StartDate:   startDate,
		EndDate:     endDate,
		Recurrence:  recurrence,
	}, ""
}

func normalizeTaskInput(request PlanningTaskRequest) (sdk.TaskInput, string) {
	title := strings.TrimSpace(request.Title)
	if title == "" {
		return sdk.TaskInput{}, "title is required"
	}
	dueAt, rejection := parseOptionalTime("due_at", request.DueAt)
	if rejection != "" {
		return sdk.TaskInput{}, rejection
	}
	dueDate, rejection := parseOptionalDate("due_date", request.DueDate)
	if rejection != "" {
		return sdk.TaskInput{}, rejection
	}
	if dueAt != nil && dueDate != nil {
		return sdk.TaskInput{}, "use either due_at or due_date, not both"
	}
	recurrence, rejection := normalizeRecurrence(request.Recurrence, dueAt != nil)
	if rejection != "" {
		return sdk.TaskInput{}, rejection
	}
	if recurrence != nil && dueAt == nil && dueDate == nil {
		return sdk.TaskInput{}, "recurring tasks require due_at or due_date"
	}
	return sdk.TaskInput{
		Title:       title,
		Description: request.Description,
		DueAt:       dueAt,
		DueDate:     dueDate,
		Recurrence:  recurrence,
	}, ""
}

func normalizeCompletionInput(request PlanningTaskRequest) (sdk.TaskCompletionInput, string) {
	occurrenceAt, rejection := parseOptionalTime("occurrence_at", request.OccurrenceAt)
	if rejection != "" {
		return sdk.TaskCompletionInput{}, rejection
	}
	occurrenceDate, rejection := parseOptionalDate("occurrence_date", request.OccurrenceDate)
	if rejection != "" {
		return sdk.TaskCompletionInput{}, rejection
	}
	if occurrenceAt != nil && occurrenceDate != nil {
		return sdk.TaskCompletionInput{}, "use occurrence_at or occurrence_date, not both"
	}
	return sdk.TaskCompletionInput{OccurrenceAt: occurrenceAt, OccurrenceDate: occurrenceDate}, ""
}

func normalizeRecurrence(input *RecurrenceRuleRequest, timed bool) (*sdk.RecurrenceRule, string) {
	if input == nil {
		return nil, ""
	}
	frequency := strings.TrimSpace(input.Frequency)
	switch frequency {
	case string(sdk.RecurrenceFrequencyDaily), string(sdk.RecurrenceFrequencyWeekly), string(sdk.RecurrenceFrequencyMonthly):
	case "":
		return nil, "recurrence.frequency is required"
	default:
		return nil, "unsupported recurrence frequency"
	}
	rule := sdk.RecurrenceRule{Frequency: sdk.RecurrenceFrequency(frequency)}
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
	if len(input.ByWeekday) > 0 {
		if rule.Frequency != sdk.RecurrenceFrequencyWeekly {
			return nil, "recurrence.by_weekday is only supported for weekly recurrence"
		}
		weekdays, rejection := normalizeWeekdays(input.ByWeekday)
		if rejection != "" {
			return nil, rejection
		}
		rule.ByWeekday = weekdays
	}
	if len(input.ByMonthDay) > 0 {
		if rule.Frequency != sdk.RecurrenceFrequencyMonthly {
			return nil, "recurrence.by_month_day is only supported for monthly recurrence"
		}
		for _, day := range input.ByMonthDay {
			if day < 1 || day > 31 {
				return nil, "recurrence.by_month_day values must be between 1 and 31"
			}
		}
		rule.ByMonthDay = append([]int32(nil), input.ByMonthDay...)
	}
	_ = timed
	return &rule, ""
}

func normalizeWeekdays(values []string) ([]sdk.Weekday, string) {
	out := make([]sdk.Weekday, 0, len(values))
	for _, value := range values {
		switch sdk.Weekday(strings.TrimSpace(value)) {
		case sdk.WeekdayMonday, sdk.WeekdayTuesday, sdk.WeekdayWednesday, sdk.WeekdayThursday, sdk.WeekdayFriday, sdk.WeekdaySaturday, sdk.WeekdaySunday:
			out = append(out, sdk.Weekday(strings.TrimSpace(value)))
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

func findCalendarByName(ctx context.Context, api *sdk.Client, name string) (sdk.Calendar, bool, error) {
	cursor := ""
	for {
		page, err := api.ListCalendars(ctx, sdk.ListOptions{Cursor: cursor, Limit: 200})
		if err != nil {
			return sdk.Calendar{}, false, err
		}
		for _, calendar := range page.Items {
			if calendar.Name == name {
				return calendar, true, nil
			}
		}
		if page.NextCursor == nil {
			return sdk.Calendar{}, false, nil
		}
		cursor = *page.NextCursor
	}
}

func findCalendarByID(ctx context.Context, api *sdk.Client, id string) (sdk.Calendar, bool, error) {
	cursor := ""
	for {
		page, err := api.ListCalendars(ctx, sdk.ListOptions{Cursor: cursor, Limit: 200})
		if err != nil {
			return sdk.Calendar{}, false, err
		}
		for _, calendar := range page.Items {
			if calendar.ID == id {
				return calendar, true, nil
			}
		}
		if page.NextCursor == nil {
			return sdk.Calendar{}, false, nil
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

func calendarEntry(calendar sdk.Calendar) CalendarEntry {
	return CalendarEntry{
		ID:          calendar.ID,
		Name:        calendar.Name,
		Description: cloneString(calendar.Description),
		Color:       cloneString(calendar.Color),
	}
}

func eventEntries(events []sdk.Event) []EventEntry {
	out := make([]EventEntry, 0, len(events))
	for _, event := range events {
		out = append(out, eventEntry(event))
	}
	return out
}

func eventEntry(event sdk.Event) EventEntry {
	out := EventEntry{
		ID:          event.ID,
		CalendarID:  event.CalendarID,
		Title:       event.Title,
		Description: cloneString(event.Description),
		Location:    cloneString(event.Location),
		StartDate:   stringValue(event.StartDate),
		EndDate:     stringValue(event.EndDate),
		Recurrence:  recurrenceResult(event.Recurrence),
	}
	if event.StartAt != nil {
		out.StartAt = formatJSONTime(*event.StartAt)
	}
	if event.EndAt != nil {
		out.EndAt = formatJSONTime(*event.EndAt)
	}
	return out
}

func taskEntries(tasks []sdk.Task) []TaskEntry {
	out := make([]TaskEntry, 0, len(tasks))
	for _, task := range tasks {
		out = append(out, taskEntry(task))
	}
	return out
}

func taskEntry(task sdk.Task) TaskEntry {
	out := TaskEntry{
		ID:          task.ID,
		CalendarID:  task.CalendarID,
		Title:       task.Title,
		Description: cloneString(task.Description),
		DueDate:     stringValue(task.DueDate),
		Recurrence:  recurrenceResult(task.Recurrence),
	}
	if task.DueAt != nil {
		out.DueAt = formatJSONTime(*task.DueAt)
	}
	if task.CompletedAt != nil {
		out.CompletedAt = formatJSONTime(*task.CompletedAt)
	}
	return out
}

func agendaEntries(items []sdk.AgendaItem) []AgendaEntry {
	out := make([]AgendaEntry, 0, len(items))
	for _, item := range items {
		entry := AgendaEntry{
			Kind:          string(item.Kind),
			OccurrenceKey: item.OccurrenceKey,
			CalendarID:    item.CalendarID,
			SourceID:      item.SourceID,
			Title:         item.Title,
			Description:   cloneString(item.Description),
			StartDate:     stringValue(item.StartDate),
			EndDate:       stringValue(item.EndDate),
			DueDate:       stringValue(item.DueDate),
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

func recurrenceResult(rule *sdk.RecurrenceRule) *RecurrenceRuleResult {
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
