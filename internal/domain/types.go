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

type EventAttendeeRole string

const (
	EventAttendeeRoleRequired       EventAttendeeRole = "required"
	EventAttendeeRoleOptional       EventAttendeeRole = "optional"
	EventAttendeeRoleChair          EventAttendeeRole = "chair"
	EventAttendeeRoleNonParticipant EventAttendeeRole = "non_participant"
)

type EventParticipationStatus string

const (
	EventParticipationStatusNeedsAction EventParticipationStatus = "needs_action"
	EventParticipationStatusAccepted    EventParticipationStatus = "accepted"
	EventParticipationStatusDeclined    EventParticipationStatus = "declined"
	EventParticipationStatusTentative   EventParticipationStatus = "tentative"
	EventParticipationStatusDelegated   EventParticipationStatus = "delegated"
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
	ID            string
	CalendarID    string
	ICalendarUID  *string
	Title         string
	Description   *string
	Location      *string
	StartAt       *time.Time
	EndAt         *time.Time
	TimeZone      *string
	StartDate     *string
	EndDate       *string
	Recurrence    *RecurrenceRule
	Reminders     []ReminderRule
	Attendees     []EventAttendee
	LinkedTaskIDs []string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type EventPatch struct {
	Title       PatchField[string]
	Description PatchField[string]
	Location    PatchField[string]
	StartAt     PatchField[time.Time]
	EndAt       PatchField[time.Time]
	TimeZone    PatchField[string]
	StartDate   PatchField[string]
	EndDate     PatchField[string]
	Recurrence  PatchField[RecurrenceRule]
	Reminders   PatchField[[]ReminderRule]
	Attendees   PatchField[[]EventAttendee]
}

type EventAttendee struct {
	Email               string
	DisplayName         *string
	Role                EventAttendeeRole
	ParticipationStatus EventParticipationStatus
	RSVP                bool
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type Task struct {
	ID             string
	CalendarID     string
	ICalendarUID   *string
	Title          string
	Description    *string
	DueAt          *time.Time
	DueDate        *string
	Recurrence     *RecurrenceRule
	Reminders      []ReminderRule
	Priority       TaskPriority
	Status         TaskStatus
	Tags           []string
	LinkedEventIDs []string
	CompletedAt    *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
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
	OccurrenceKey  string
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

type OccurrenceOwnerKind string

const (
	OccurrenceOwnerKindEvent OccurrenceOwnerKind = "event"
	OccurrenceOwnerKindTask  OccurrenceOwnerKind = "task"
)

type OccurrenceMutationRequest struct {
	OccurrenceKey      string
	OccurrenceAt       *time.Time
	OccurrenceDate     *string
	ReplacementAt      *time.Time
	ReplacementEndAt   *time.Time
	ReplacementDate    *string
	ReplacementEndDate *string
}

type OccurrenceState struct {
	OwnerKind          OccurrenceOwnerKind
	OwnerID            string
	OccurrenceKey      string
	OccurrenceAt       *time.Time
	OccurrenceDate     *string
	Cancelled          bool
	ReplacementAt      *time.Time
	ReplacementEndAt   *time.Time
	ReplacementDate    *string
	ReplacementEndDate *string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type AgendaItemKind string

const (
	AgendaItemKindEvent AgendaItemKind = "event"
	AgendaItemKindTask  AgendaItemKind = "task"
)

type AgendaItem struct {
	Kind           AgendaItemKind
	OccurrenceKey  string
	CalendarID     string
	SourceID       string
	Title          string
	Description    *string
	StartAt        *time.Time
	EndAt          *time.Time
	TimeZone       *string
	StartDate      *string
	EndDate        *string
	DueAt          *time.Time
	DueDate        *string
	Priority       TaskPriority
	Status         TaskStatus
	Tags           []string
	Attendees      []EventAttendee
	LinkedTaskIDs  []string
	LinkedEventIDs []string
	CompletedAt    *time.Time
}

type EventTaskLink struct {
	EventID   string
	TaskID    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type EventTaskLinkFilter struct {
	EventID string
	TaskID  string
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

type ICalendarExport struct {
	ContentType  string
	Filename     string
	CalendarID   string
	CalendarName string
	EventCount   int
	TaskCount    int
	Content      string
}

type ICalendarImportRequest struct {
	Content      string
	CalendarID   string
	CalendarName string
}

type ICalendarImportSkip struct {
	Kind   string
	UID    string
	Reason string
}

type ICalendarImport struct {
	CalendarCount int
	EventCount    int
	TaskCount     int
	CreatedCount  int
	UpdatedCount  int
	SkippedCount  int
	Writes        []ICalendarImportWrite
	Skips         []ICalendarImportSkip
}

type ICalendarImportWrite struct {
	Kind          string
	ID            string
	Status        string
	Name          string
	Title         string
	OccurrenceKey string
}
