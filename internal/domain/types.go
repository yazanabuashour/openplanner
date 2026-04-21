package domain

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

type TaskPriority string

const (
	TaskPriorityLow    TaskPriority = "low"
	TaskPriorityMedium TaskPriority = "medium"
	TaskPriorityHigh   TaskPriority = "high"
)

type TaskStatus string

const (
	TaskStatusTodo       TaskStatus = "todo"
	TaskStatusInProgress TaskStatus = "in_progress"
	TaskStatusDone       TaskStatus = "done"
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

type Calendar struct {
	ID          string
	Name        string
	Description *string
	Color       *string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type PatchField[T any] struct {
	Present bool
	Clear   bool
	Value   T
}

func SetPatch[T any](value T) PatchField[T] {
	return PatchField[T]{Present: true, Value: value}
}

func ClearPatch[T any]() PatchField[T] {
	return PatchField[T]{Present: true, Clear: true}
}

type CalendarPatch struct {
	Name        PatchField[string]
	Description PatchField[string]
	Color       PatchField[string]
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
	Reminders   []ReminderRule
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type EventPatch struct {
	Title       PatchField[string]
	Description PatchField[string]
	Location    PatchField[string]
	StartAt     PatchField[time.Time]
	EndAt       PatchField[time.Time]
	StartDate   PatchField[string]
	EndDate     PatchField[string]
	Recurrence  PatchField[RecurrenceRule]
	Reminders   PatchField[[]ReminderRule]
}

type Task struct {
	ID          string
	CalendarID  string
	Title       string
	Description *string
	DueAt       *time.Time
	DueDate     *string
	Recurrence  *RecurrenceRule
	Reminders   []ReminderRule
	Priority    TaskPriority
	Status      TaskStatus
	Tags        []string
	CompletedAt *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type TaskPatch struct {
	Title       PatchField[string]
	Description PatchField[string]
	DueAt       PatchField[time.Time]
	DueDate     PatchField[string]
	Recurrence  PatchField[RecurrenceRule]
	Reminders   PatchField[[]ReminderRule]
	Priority    PatchField[TaskPriority]
	Status      PatchField[TaskStatus]
	Tags        PatchField[[]string]
}

type ReminderOwnerKind string

const (
	ReminderOwnerKindEvent ReminderOwnerKind = "event"
	ReminderOwnerKindTask  ReminderOwnerKind = "task"
)

type ReminderRule struct {
	ID            string
	BeforeMinutes int32
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type ReminderQueryParams struct {
	From       time.Time
	To         time.Time
	Cursor     string
	Limit      int
	CalendarID string
}

type PendingReminder struct {
	ID            string
	ReminderID    string
	OwnerKind     ReminderOwnerKind
	OwnerID       string
	CalendarID    string
	Title         string
	OccurrenceKey string
	RemindAt      time.Time
	BeforeMinutes int32
	StartAt       *time.Time
	EndAt         *time.Time
	StartDate     *string
	EndDate       *string
	DueAt         *time.Time
	DueDate       *string
}

type ReminderDismissal struct {
	ReminderID       string
	OccurrenceKey    string
	DismissedAt      time.Time
	AlreadyDismissed bool
}

type TaskCompletionRequest struct {
	OccurrenceAt   *time.Time
	OccurrenceDate *string
}

type TaskCompletion struct {
	TaskID         string
	OccurrenceKey  string
	OccurrenceAt   *time.Time
	OccurrenceDate *string
	CompletedAt    time.Time
}

type AgendaItemKind string

const (
	AgendaItemKindEvent AgendaItemKind = "event"
	AgendaItemKindTask  AgendaItemKind = "task"
)

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
	Priority      TaskPriority
	Status        TaskStatus
	Tags          []string
	CompletedAt   *time.Time
}

type PageParams struct {
	Cursor     string
	Limit      int
	CalendarID string
}

type TaskListParams struct {
	PageParams
	Priority TaskPriority
	Status   TaskStatus
	Tags     []string
}

type AgendaParams struct {
	From   time.Time
	To     time.Time
	Cursor string
	Limit  int
}

type Page[T any] struct {
	Items      []T
	NextCursor *string
}
