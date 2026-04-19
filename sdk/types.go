package sdk

import "time"

type RecurrenceFrequency string

const (
	RecurrenceFrequencyDaily   RecurrenceFrequency = "daily"
	RecurrenceFrequencyWeekly  RecurrenceFrequency = "weekly"
	RecurrenceFrequencyMonthly RecurrenceFrequency = "monthly"
)

type Weekday string

const (
	WeekdayMonday    Weekday = "MO"
	WeekdayTuesday   Weekday = "TU"
	WeekdayWednesday Weekday = "WE"
	WeekdayThursday  Weekday = "TH"
	WeekdayFriday    Weekday = "FR"
	WeekdaySaturday  Weekday = "SA"
	WeekdaySunday    Weekday = "SU"
)

type AgendaItemKind string

const (
	AgendaItemKindEvent AgendaItemKind = "event"
	AgendaItemKindTask  AgendaItemKind = "task"
)

type CalendarWriteStatus string

const (
	CalendarWriteStatusCreated       CalendarWriteStatus = "created"
	CalendarWriteStatusAlreadyExists CalendarWriteStatus = "already_exists"
	CalendarWriteStatusUpdated       CalendarWriteStatus = "updated"
)

type RecurrenceRule struct {
	Frequency  RecurrenceFrequency
	Interval   int32
	Count      *int32
	UntilAt    *time.Time
	UntilDate  *string
	ByWeekday  []Weekday
	ByMonthDay []int32
}

type CalendarInput struct {
	Name        string
	Description *string
	Color       *string
}

type PatchField[T any] struct {
	set   bool
	clear bool
	value T
}

func SetPatch[T any](value T) PatchField[T] {
	return PatchField[T]{set: true, value: value}
}

func ClearPatch[T any]() PatchField[T] {
	return PatchField[T]{set: true, clear: true}
}

func (field PatchField[T]) IsSet() bool {
	return field.set
}

func (field PatchField[T]) IsClear() bool {
	return field.set && field.clear
}

func (field PatchField[T]) Value() T {
	return field.value
}

type CalendarPatchInput struct {
	Name        PatchField[string]
	Description PatchField[string]
	Color       PatchField[string]
}

type EventInput struct {
	CalendarID  string
	Title       string
	Description *string
	Location    *string
	StartAt     *time.Time
	EndAt       *time.Time
	StartDate   *string
	EndDate     *string
	Recurrence  *RecurrenceRule
}

type EventPatchInput struct {
	Title       PatchField[string]
	Description PatchField[string]
	Location    PatchField[string]
	StartAt     PatchField[time.Time]
	EndAt       PatchField[time.Time]
	StartDate   PatchField[string]
	EndDate     PatchField[string]
	Recurrence  PatchField[RecurrenceRule]
}

type TaskInput struct {
	CalendarID  string
	Title       string
	Description *string
	DueAt       *time.Time
	DueDate     *string
	Recurrence  *RecurrenceRule
}

type TaskPatchInput struct {
	Title       PatchField[string]
	Description PatchField[string]
	DueAt       PatchField[time.Time]
	DueDate     PatchField[string]
	Recurrence  PatchField[RecurrenceRule]
}

type TaskCompletionInput struct {
	OccurrenceAt   *time.Time
	OccurrenceDate *string
}

type ListOptions struct {
	Cursor     string
	Limit      int
	CalendarID string
}

type AgendaOptions struct {
	From   time.Time
	To     time.Time
	Cursor string
	Limit  int
}

type Page[T any] struct {
	Items      []T
	NextCursor *string
}

type CalendarWriteResult struct {
	Calendar Calendar
	Status   CalendarWriteStatus
}

type Calendar struct {
	ID          string
	Name        string
	Description *string
	Color       *string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Event struct {
	ID          string
	CalendarID  string
	Title       string
	Description *string
	Location    *string
	StartAt     *time.Time
	EndAt       *time.Time
	StartDate   *string
	EndDate     *string
	Recurrence  *RecurrenceRule
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Task struct {
	ID          string
	CalendarID  string
	Title       string
	Description *string
	DueAt       *time.Time
	DueDate     *string
	Recurrence  *RecurrenceRule
	CompletedAt *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type TaskCompletion struct {
	TaskID         string
	OccurrenceKey  string
	OccurrenceAt   *time.Time
	OccurrenceDate *string
	CompletedAt    time.Time
}

type AgendaItem struct {
	Kind          AgendaItemKind
	OccurrenceKey string
	CalendarID    string
	SourceID      string
	Title         string
	Description   *string
	StartAt       *time.Time
	EndAt         *time.Time
	StartDate     *string
	EndDate       *string
	DueAt         *time.Time
	DueDate       *string
	CompletedAt   *time.Time
}
