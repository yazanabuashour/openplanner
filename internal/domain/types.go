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

type CalendarPatch struct {
	Name        *string
	Description *string
	Color       *string
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

type EventPatch struct {
	Title       *string
	Description *string
	Location    *string
	StartAt     *time.Time
	EndAt       *time.Time
	StartDate   *string
	EndDate     *string
	Recurrence  *RecurrenceRule
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

type TaskPatch struct {
	Title       *string
	Description *string
	DueAt       *time.Time
	DueDate     *string
	Recurrence  *RecurrenceRule
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
	CompletedAt   *time.Time
}

type PageParams struct {
	Cursor     string
	Limit      int
	CalendarID string
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
