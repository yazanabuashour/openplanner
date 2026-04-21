package service

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/yazanabuashour/openplanner/internal/domain"
	"github.com/yazanabuashour/openplanner/internal/recurrence"
	"github.com/yazanabuashour/openplanner/internal/store"
)

var (
	colorPattern = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
	tagPattern   = regexp.MustCompile(`^[a-z0-9_-]+$`)
)

type Service struct {
	store *store.Store
	now   func() time.Time
}

func New(repository *store.Store) *Service {
	return &Service{
		store: repository,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

func (service *Service) CreateCalendar(input domain.Calendar) (domain.Calendar, error) {
	name := strings.TrimSpace(input.Name)
	fieldErrors := []FieldError{}
	if name == "" {
		fieldErrors = append(fieldErrors, FieldError{Field: "name", Message: "name is required"})
	}

	color := sanitizeOptionalString(input.Color)
	if color != nil && !colorPattern.MatchString(*color) {
		fieldErrors = append(fieldErrors, FieldError{Field: "color", Message: "color must be a #RRGGBB hex string"})
	}

	if len(fieldErrors) > 0 {
		return domain.Calendar{}, &ValidationError{
			Message:     "calendar validation failed",
			FieldErrors: fieldErrors,
		}
	}

	now := service.now()
	calendar := domain.Calendar{
		ID:          ulid.Make().String(),
		Name:        name,
		Description: sanitizeOptionalString(input.Description),
		Color:       color,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := service.store.CreateCalendar(calendar); err != nil {
		if errors.Is(err, store.ErrConflict) {
			return domain.Calendar{}, &ConflictError{Message: "calendar name already exists"}
		}

		return domain.Calendar{}, err
	}

	return calendar, nil
}

func (service *Service) ListCalendars(params domain.PageParams) (domain.Page[domain.Calendar], error) {
	calendars, err := service.store.ListCalendars()
	if err != nil {
		return domain.Page[domain.Calendar]{}, err
	}

	return paginateByCreatedAt(calendars, params)
}

func (service *Service) GetCalendar(id string) (domain.Calendar, error) {
	if err := validateResourceID("calendarId", id); err != nil {
		return domain.Calendar{}, err
	}

	calendar, err := service.store.GetCalendar(id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return domain.Calendar{}, &NotFoundError{Resource: "calendar", ID: id, Message: "calendar not found"}
		}

		return domain.Calendar{}, err
	}

	return calendar, nil
}

func (service *Service) UpdateCalendar(id string, patch domain.CalendarPatch) (domain.Calendar, error) {
	calendar, err := service.GetCalendar(id)
	if err != nil {
		return domain.Calendar{}, err
	}

	if patch.Name.Present {
		if patch.Name.Clear {
			calendar.Name = ""
		} else {
			calendar.Name = strings.TrimSpace(patch.Name.Value)
		}
	}
	if patch.Description.Present {
		if patch.Description.Clear {
			calendar.Description = nil
		} else {
			calendar.Description = sanitizeOptionalString(&patch.Description.Value)
		}
	}
	if patch.Color.Present {
		if patch.Color.Clear {
			calendar.Color = nil
		} else {
			calendar.Color = sanitizeOptionalString(&patch.Color.Value)
		}
	}

	fieldErrors := []FieldError{}
	if calendar.Name == "" {
		fieldErrors = append(fieldErrors, FieldError{Field: "name", Message: "name is required"})
	}
	if calendar.Color != nil && !colorPattern.MatchString(*calendar.Color) {
		fieldErrors = append(fieldErrors, FieldError{Field: "color", Message: "color must be a #RRGGBB hex string"})
	}
	if len(fieldErrors) > 0 {
		return domain.Calendar{}, &ValidationError{
			Message:     "calendar validation failed",
			FieldErrors: fieldErrors,
		}
	}

	calendar.UpdatedAt = service.now()
	if err := service.store.UpdateCalendar(calendar); err != nil {
		if errors.Is(err, store.ErrConflict) {
			return domain.Calendar{}, &ConflictError{Message: "calendar name already exists"}
		}
		if errors.Is(err, store.ErrNotFound) {
			return domain.Calendar{}, &NotFoundError{Resource: "calendar", ID: id, Message: "calendar not found"}
		}

		return domain.Calendar{}, err
	}

	return calendar, nil
}

func (service *Service) DeleteCalendar(id string) error {
	if err := validateResourceID("calendarId", id); err != nil {
		return err
	}

	if err := service.store.DeleteCalendar(id); err != nil {
		if errors.Is(err, store.ErrConflict) {
			return &ConflictError{Message: "calendar must be empty before deletion"}
		}
		if errors.Is(err, store.ErrNotFound) {
			return &NotFoundError{Resource: "calendar", ID: id, Message: "calendar not found"}
		}

		return err
	}

	return nil
}

func (service *Service) CreateEvent(input domain.Event) (domain.Event, error) {
	if err := validateResourceID("calendarId", input.CalendarID); err != nil {
		return domain.Event{}, err
	}
	if _, err := service.GetCalendar(input.CalendarID); err != nil {
		return domain.Event{}, err
	}

	event := domain.Event{
		ID:          ulid.Make().String(),
		CalendarID:  input.CalendarID,
		Title:       strings.TrimSpace(input.Title),
		Description: sanitizeOptionalString(input.Description),
		Location:    sanitizeOptionalString(input.Location),
		StartAt:     cloneTimePtr(input.StartAt),
		EndAt:       cloneTimePtr(input.EndAt),
		StartDate:   sanitizeOptionalString(input.StartDate),
		EndDate:     sanitizeOptionalString(input.EndDate),
		Recurrence:  cloneRule(input.Recurrence),
		CreatedAt:   service.now(),
		UpdatedAt:   service.now(),
	}

	if err := validateEvent(event); err != nil {
		return domain.Event{}, err
	}

	if err := service.store.CreateEvent(event); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return domain.Event{}, &NotFoundError{Resource: "calendar", ID: input.CalendarID, Message: "calendar not found"}
		}

		return domain.Event{}, err
	}

	return event, nil
}

func (service *Service) ListEvents(params domain.PageParams) (domain.Page[domain.Event], error) {
	if params.CalendarID != "" {
		if err := validateResourceID("calendarId", params.CalendarID); err != nil {
			return domain.Page[domain.Event]{}, err
		}
	}

	events, err := service.store.ListEvents(params.CalendarID)
	if err != nil {
		return domain.Page[domain.Event]{}, err
	}

	return paginateByCreatedAt(events, params)
}

func (service *Service) GetEvent(id string) (domain.Event, error) {
	if err := validateResourceID("eventId", id); err != nil {
		return domain.Event{}, err
	}

	event, err := service.store.GetEvent(id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return domain.Event{}, &NotFoundError{Resource: "event", ID: id, Message: "event not found"}
		}

		return domain.Event{}, err
	}

	return event, nil
}

func (service *Service) UpdateEvent(id string, patch domain.EventPatch) (domain.Event, error) {
	event, err := service.GetEvent(id)
	if err != nil {
		return domain.Event{}, err
	}

	if patch.Title.Present {
		if patch.Title.Clear {
			event.Title = ""
		} else {
			event.Title = strings.TrimSpace(patch.Title.Value)
		}
	}
	if patch.Description.Present {
		if patch.Description.Clear {
			event.Description = nil
		} else {
			event.Description = sanitizeOptionalString(&patch.Description.Value)
		}
	}
	if patch.Location.Present {
		if patch.Location.Clear {
			event.Location = nil
		} else {
			event.Location = sanitizeOptionalString(&patch.Location.Value)
		}
	}
	if patch.StartAt.Present {
		if patch.StartAt.Clear {
			event.StartAt = nil
		} else {
			event.StartAt = cloneTimePtr(&patch.StartAt.Value)
		}
	}
	if patch.EndAt.Present {
		if patch.EndAt.Clear {
			event.EndAt = nil
		} else {
			event.EndAt = cloneTimePtr(&patch.EndAt.Value)
		}
	}
	if patch.StartDate.Present {
		if patch.StartDate.Clear {
			event.StartDate = nil
		} else {
			event.StartDate = sanitizeOptionalString(&patch.StartDate.Value)
		}
	}
	if patch.EndDate.Present {
		if patch.EndDate.Clear {
			event.EndDate = nil
		} else {
			event.EndDate = sanitizeOptionalString(&patch.EndDate.Value)
		}
	}
	if patch.Recurrence.Present {
		if patch.Recurrence.Clear {
			event.Recurrence = nil
		} else {
			event.Recurrence = cloneRule(&patch.Recurrence.Value)
		}
	}
	event.UpdatedAt = service.now()

	if err := validateEvent(event); err != nil {
		return domain.Event{}, err
	}

	if err := service.store.UpdateEvent(event); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return domain.Event{}, &NotFoundError{Resource: "event", ID: id, Message: "event not found"}
		}

		return domain.Event{}, err
	}

	return event, nil
}

func (service *Service) DeleteEvent(id string) error {
	if err := validateResourceID("eventId", id); err != nil {
		return err
	}

	if err := service.store.DeleteEvent(id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return &NotFoundError{Resource: "event", ID: id, Message: "event not found"}
		}

		return err
	}

	return nil
}

func (service *Service) CreateTask(input domain.Task) (domain.Task, error) {
	if err := validateResourceID("calendarId", input.CalendarID); err != nil {
		return domain.Task{}, err
	}
	if _, err := service.GetCalendar(input.CalendarID); err != nil {
		return domain.Task{}, err
	}

	task := domain.Task{
		ID:          ulid.Make().String(),
		CalendarID:  input.CalendarID,
		Title:       strings.TrimSpace(input.Title),
		Description: sanitizeOptionalString(input.Description),
		DueAt:       cloneTimePtr(input.DueAt),
		DueDate:     sanitizeOptionalString(input.DueDate),
		Recurrence:  cloneRule(input.Recurrence),
		Priority:    defaultTaskPriority(input.Priority),
		Status:      defaultTaskStatus(input.Status),
		Tags:        normalizeTags(input.Tags),
		CreatedAt:   service.now(),
		UpdatedAt:   service.now(),
	}
	if task.Status == domain.TaskStatusDone {
		if task.Recurrence != nil {
			return domain.Task{}, &ValidationError{
				Message: "task validation failed",
				FieldErrors: []FieldError{
					{Field: "status", Message: "recurring tasks must be completed with complete_task and an occurrence selector"},
				},
			}
		}
		completedAt := service.now()
		task.CompletedAt = &completedAt
	}

	if err := validateTask(task); err != nil {
		return domain.Task{}, err
	}

	if err := service.store.CreateTask(task); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return domain.Task{}, &NotFoundError{Resource: "calendar", ID: input.CalendarID, Message: "calendar not found"}
		}

		return domain.Task{}, err
	}

	return task, nil
}

func (service *Service) ListTasks(params domain.TaskListParams) (domain.Page[domain.Task], error) {
	if params.CalendarID != "" {
		if err := validateResourceID("calendarId", params.CalendarID); err != nil {
			return domain.Page[domain.Task]{}, err
		}
	}
	if err := validateTaskPriority(params.Priority); err != nil {
		return domain.Page[domain.Task]{}, &ValidationError{
			Message:     "task filter validation failed",
			FieldErrors: []FieldError{*err},
		}
	}
	if err := validateTaskStatus(params.Status); err != nil {
		return domain.Page[domain.Task]{}, &ValidationError{
			Message:     "task filter validation failed",
			FieldErrors: []FieldError{*err},
		}
	}
	tags := normalizeTags(params.Tags)
	if fieldErrors := validateTags(tags); len(fieldErrors) > 0 {
		return domain.Page[domain.Task]{}, &ValidationError{
			Message:     "task filter validation failed",
			FieldErrors: fieldErrors,
		}
	}
	params.Tags = tags

	tasks, err := service.store.ListTasks(params)
	if err != nil {
		return domain.Page[domain.Task]{}, err
	}

	return paginateByCreatedAt(tasks, params.PageParams)
}

func (service *Service) GetTask(id string) (domain.Task, error) {
	if err := validateResourceID("taskId", id); err != nil {
		return domain.Task{}, err
	}

	task, err := service.store.GetTask(id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return domain.Task{}, &NotFoundError{Resource: "task", ID: id, Message: "task not found"}
		}

		return domain.Task{}, err
	}

	return task, nil
}

func (service *Service) UpdateTask(id string, patch domain.TaskPatch) (domain.Task, error) {
	task, err := service.GetTask(id)
	if err != nil {
		return domain.Task{}, err
	}

	if patch.Title.Present {
		if patch.Title.Clear {
			task.Title = ""
		} else {
			task.Title = strings.TrimSpace(patch.Title.Value)
		}
	}
	if patch.Description.Present {
		if patch.Description.Clear {
			task.Description = nil
		} else {
			task.Description = sanitizeOptionalString(&patch.Description.Value)
		}
	}
	if patch.DueAt.Present {
		if patch.DueAt.Clear {
			task.DueAt = nil
		} else {
			task.DueAt = cloneTimePtr(&patch.DueAt.Value)
		}
	}
	if patch.DueDate.Present {
		if patch.DueDate.Clear {
			task.DueDate = nil
		} else {
			task.DueDate = sanitizeOptionalString(&patch.DueDate.Value)
		}
	}
	if patch.Recurrence.Present {
		if patch.Recurrence.Clear {
			task.Recurrence = nil
		} else {
			task.Recurrence = cloneRule(&patch.Recurrence.Value)
		}
	}
	if patch.Priority.Present {
		if patch.Priority.Clear {
			return domain.Task{}, &ValidationError{
				Message:     "task validation failed",
				FieldErrors: []FieldError{{Field: "priority", Message: "priority cannot be cleared"}},
			}
		}
		if patch.Priority.Value == "" {
			return domain.Task{}, &ValidationError{
				Message:     "task validation failed",
				FieldErrors: []FieldError{{Field: "priority", Message: "priority must be low, medium, or high"}},
			}
		}
		task.Priority = patch.Priority.Value
	}
	if patch.Status.Present {
		if patch.Status.Clear {
			return domain.Task{}, &ValidationError{
				Message:     "task validation failed",
				FieldErrors: []FieldError{{Field: "status", Message: "status cannot be cleared"}},
			}
		}
		nextStatus := patch.Status.Value
		if nextStatus == "" {
			return domain.Task{}, &ValidationError{
				Message:     "task validation failed",
				FieldErrors: []FieldError{{Field: "status", Message: "status must be todo, in_progress, or done"}},
			}
		}
		if nextStatus == domain.TaskStatusDone {
			if task.Recurrence != nil {
				return domain.Task{}, &ValidationError{
					Message: "task validation failed",
					FieldErrors: []FieldError{
						{Field: "status", Message: "recurring tasks must be completed with complete_task and an occurrence selector"},
					},
				}
			}
			if task.CompletedAt == nil {
				completedAt := service.now()
				task.CompletedAt = &completedAt
			}
		} else if task.CompletedAt != nil {
			return domain.Task{}, &ValidationError{
				Message: "task validation failed",
				FieldErrors: []FieldError{
					{Field: "status", Message: "completed tasks cannot be reopened by status update"},
				},
			}
		}
		task.Status = nextStatus
	}
	if patch.Tags.Present {
		if patch.Tags.Clear {
			task.Tags = []string{}
		} else {
			task.Tags = normalizeTags(patch.Tags.Value)
		}
	}
	task.UpdatedAt = service.now()

	if err := validateTask(task); err != nil {
		return domain.Task{}, err
	}

	if err := service.store.UpdateTask(task); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return domain.Task{}, &NotFoundError{Resource: "task", ID: id, Message: "task not found"}
		}

		return domain.Task{}, err
	}

	return task, nil
}

func (service *Service) DeleteTask(id string) error {
	if err := validateResourceID("taskId", id); err != nil {
		return err
	}

	if err := service.store.DeleteTask(id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return &NotFoundError{Resource: "task", ID: id, Message: "task not found"}
		}

		return err
	}

	return nil
}

func (service *Service) CompleteTask(id string, request domain.TaskCompletionRequest) (domain.TaskCompletion, error) {
	task, err := service.GetTask(id)
	if err != nil {
		return domain.TaskCompletion{}, err
	}

	if task.Recurrence == nil {
		if request.OccurrenceAt != nil || request.OccurrenceDate != nil {
			return domain.TaskCompletion{}, &ValidationError{
				Message: "task completion validation failed",
				FieldErrors: []FieldError{
					{Field: "occurrence", Message: "non-recurring tasks do not accept an occurrence selector"},
				},
			}
		}
		if task.CompletedAt != nil {
			return domain.TaskCompletion{}, &ConflictError{Message: "task is already completed"}
		}

		completedAt := service.now()
		if err := service.store.MarkTaskCompleted(task.ID, completedAt); err != nil {
			if errors.Is(err, store.ErrConflict) {
				return domain.TaskCompletion{}, &ConflictError{Message: "task is already completed"}
			}

			return domain.TaskCompletion{}, err
		}

		return domain.TaskCompletion{
			TaskID:      task.ID,
			CompletedAt: completedAt,
		}, nil
	}

	if task.DueAt != nil {
		if request.OccurrenceAt == nil || request.OccurrenceDate != nil {
			return domain.TaskCompletion{}, &ValidationError{
				Message: "task completion validation failed",
				FieldErrors: []FieldError{
					{Field: "occurrenceAt", Message: "recurring timed tasks require occurrenceAt"},
				},
			}
		}
		if !recurrence.IncludesTimed(*task.DueAt, task.Recurrence, *request.OccurrenceAt) {
			return domain.TaskCompletion{}, &ValidationError{
				Message: "task completion validation failed",
				FieldErrors: []FieldError{
					{Field: "occurrenceAt", Message: "occurrenceAt does not match the recurring task schedule"},
				},
			}
		}

		completion := domain.TaskCompletion{
			TaskID:        task.ID,
			OccurrenceKey: occurrenceKey(task.ID, request.OccurrenceAt, nil),
			OccurrenceAt:  cloneTimePtr(request.OccurrenceAt),
			CompletedAt:   service.now(),
		}
		if err := service.store.CreateTaskCompletion(completion); err != nil {
			if errors.Is(err, store.ErrConflict) {
				return domain.TaskCompletion{}, &ConflictError{Message: "task occurrence is already completed"}
			}

			return domain.TaskCompletion{}, err
		}

		return completion, nil
	}

	if request.OccurrenceDate == nil || request.OccurrenceAt != nil {
		return domain.TaskCompletion{}, &ValidationError{
			Message: "task completion validation failed",
			FieldErrors: []FieldError{
				{Field: "occurrenceDate", Message: "recurring date-based tasks require occurrenceDate"},
			},
		}
	}
	if !recurrence.IncludesDate(*task.DueDate, task.Recurrence, *request.OccurrenceDate) {
		return domain.TaskCompletion{}, &ValidationError{
			Message: "task completion validation failed",
			FieldErrors: []FieldError{
				{Field: "occurrenceDate", Message: "occurrenceDate does not match the recurring task schedule"},
			},
		}
	}

	completion := domain.TaskCompletion{
		TaskID:         task.ID,
		OccurrenceKey:  occurrenceKey(task.ID, nil, request.OccurrenceDate),
		OccurrenceDate: sanitizeOptionalString(request.OccurrenceDate),
		CompletedAt:    service.now(),
	}
	if err := service.store.CreateTaskCompletion(completion); err != nil {
		if errors.Is(err, store.ErrConflict) {
			return domain.TaskCompletion{}, &ConflictError{Message: "task occurrence is already completed"}
		}

		return domain.TaskCompletion{}, err
	}

	return completion, nil
}

func (service *Service) ListAgenda(params domain.AgendaParams) (domain.Page[domain.AgendaItem], error) {
	if params.From.IsZero() || params.To.IsZero() || !params.From.Before(params.To) {
		return domain.Page[domain.AgendaItem]{}, &ValidationError{
			Message: "agenda validation failed",
			FieldErrors: []FieldError{
				{Field: "range", Message: "from must be before to"},
			},
		}
	}

	events, err := service.store.ListEvents("")
	if err != nil {
		return domain.Page[domain.AgendaItem]{}, err
	}
	tasks, err := service.store.ListTasks(domain.TaskListParams{})
	if err != nil {
		return domain.Page[domain.AgendaItem]{}, err
	}

	taskIDs := make([]string, 0, len(tasks))
	for _, task := range tasks {
		if task.Recurrence != nil {
			taskIDs = append(taskIDs, task.ID)
		}
	}
	completions, err := service.store.ListTaskCompletions(taskIDs)
	if err != nil {
		return domain.Page[domain.AgendaItem]{}, err
	}

	items := make([]domain.AgendaItem, 0, len(events)+len(tasks))
	for _, event := range events {
		items = append(items, agendaItemsForEvent(event, params.From, params.To)...)
	}
	for _, task := range tasks {
		items = append(items, agendaItemsForTask(task, completions[task.ID], params.From, params.To)...)
	}

	slices.SortFunc(items, compareAgendaItems)

	return paginateAgenda(items, params)
}

func validateEvent(event domain.Event) error {
	fieldErrors := []FieldError{}
	if event.Title == "" {
		fieldErrors = append(fieldErrors, FieldError{Field: "title", Message: "title is required"})
	}

	timed := event.StartAt != nil || event.EndAt != nil
	dated := event.StartDate != nil || event.EndDate != nil
	if timed && dated {
		fieldErrors = append(fieldErrors, FieldError{Field: "timing", Message: "use either timed or all-day fields, not both"})
	}
	if !timed && !dated {
		fieldErrors = append(fieldErrors, FieldError{Field: "timing", Message: "startAt or startDate is required"})
	}
	if event.EndAt != nil && event.StartAt == nil {
		fieldErrors = append(fieldErrors, FieldError{Field: "startAt", Message: "startAt is required when endAt is provided"})
	}
	if event.StartAt != nil && event.EndAt != nil && !event.EndAt.After(*event.StartAt) {
		fieldErrors = append(fieldErrors, FieldError{Field: "endAt", Message: "endAt must be after startAt"})
	}
	if event.EndDate != nil && event.StartDate == nil {
		fieldErrors = append(fieldErrors, FieldError{Field: "startDate", Message: "startDate is required when endDate is provided"})
	}
	if event.StartDate != nil {
		startDate, err := time.Parse(time.DateOnly, *event.StartDate)
		if err != nil {
			fieldErrors = append(fieldErrors, FieldError{Field: "startDate", Message: "startDate must be YYYY-MM-DD"})
		}
		if event.EndDate != nil {
			endDate, endErr := time.Parse(time.DateOnly, *event.EndDate)
			if endErr != nil {
				fieldErrors = append(fieldErrors, FieldError{Field: "endDate", Message: "endDate must be YYYY-MM-DD"})
			} else if endDate.Before(startDate) {
				fieldErrors = append(fieldErrors, FieldError{Field: "endDate", Message: "endDate must not be before startDate"})
			}
		}
	}
	fieldErrors = append(fieldErrors, validateRecurrence(event.Recurrence, timed || event.StartAt != nil)...)

	if len(fieldErrors) > 0 {
		return &ValidationError{
			Message:     "event validation failed",
			FieldErrors: fieldErrors,
		}
	}

	return nil
}

func validateTask(task domain.Task) error {
	fieldErrors := []FieldError{}
	if task.Title == "" {
		fieldErrors = append(fieldErrors, FieldError{Field: "title", Message: "title is required"})
	}
	if err := validateTaskPriority(task.Priority); err != nil {
		fieldErrors = append(fieldErrors, *err)
	}
	if err := validateTaskStatus(task.Status); err != nil {
		fieldErrors = append(fieldErrors, *err)
	}
	fieldErrors = append(fieldErrors, validateTags(task.Tags)...)
	if task.Status == domain.TaskStatusDone && task.Recurrence != nil {
		fieldErrors = append(fieldErrors, FieldError{Field: "status", Message: "recurring tasks must be completed with complete_task and an occurrence selector"})
	}
	if task.Status == domain.TaskStatusDone && task.CompletedAt == nil {
		fieldErrors = append(fieldErrors, FieldError{Field: "completedAt", Message: "done tasks require completedAt"})
	}
	if task.DueAt != nil && task.DueDate != nil {
		fieldErrors = append(fieldErrors, FieldError{Field: "due", Message: "use either dueAt or dueDate, not both"})
	}
	if task.DueDate != nil {
		if _, err := time.Parse(time.DateOnly, *task.DueDate); err != nil {
			fieldErrors = append(fieldErrors, FieldError{Field: "dueDate", Message: "dueDate must be YYYY-MM-DD"})
		}
	}
	if task.Recurrence != nil && task.DueAt == nil && task.DueDate == nil {
		fieldErrors = append(fieldErrors, FieldError{Field: "recurrence", Message: "recurring tasks require dueAt or dueDate"})
	}
	fieldErrors = append(fieldErrors, validateRecurrence(task.Recurrence, task.DueAt != nil)...)

	if len(fieldErrors) > 0 {
		return &ValidationError{
			Message:     "task validation failed",
			FieldErrors: fieldErrors,
		}
	}

	return nil
}

func defaultTaskPriority(priority domain.TaskPriority) domain.TaskPriority {
	if priority == "" {
		return domain.TaskPriorityMedium
	}
	return priority
}

func defaultTaskStatus(status domain.TaskStatus) domain.TaskStatus {
	if status == "" {
		return domain.TaskStatusTodo
	}
	return status
}

func validateTaskPriority(priority domain.TaskPriority) *FieldError {
	switch priority {
	case "", domain.TaskPriorityLow, domain.TaskPriorityMedium, domain.TaskPriorityHigh:
		return nil
	default:
		return &FieldError{Field: "priority", Message: "priority must be low, medium, or high"}
	}
}

func validateTaskStatus(status domain.TaskStatus) *FieldError {
	switch status {
	case "", domain.TaskStatusTodo, domain.TaskStatusInProgress, domain.TaskStatusDone:
		return nil
	default:
		return &FieldError{Field: "status", Message: "status must be todo, in_progress, or done"}
	}
}

func normalizeTags(tags []string) []string {
	if tags == nil {
		return []string{}
	}
	normalized := make([]string, 0, len(tags))
	for _, tag := range tags {
		normalized = append(normalized, strings.ToLower(strings.TrimSpace(tag)))
	}
	return normalized
}

func validateTags(tags []string) []FieldError {
	fieldErrors := []FieldError{}
	seen := map[string]bool{}
	for _, tag := range tags {
		switch {
		case tag == "":
			fieldErrors = append(fieldErrors, FieldError{Field: "tags", Message: "tags cannot contain empty values"})
		case !tagPattern.MatchString(tag):
			fieldErrors = append(fieldErrors, FieldError{Field: "tags", Message: "tags must contain only lowercase letters, digits, underscores, or hyphens"})
		case seen[tag]:
			fieldErrors = append(fieldErrors, FieldError{Field: "tags", Message: "tags cannot contain duplicates"})
		}
		seen[tag] = true
	}
	return fieldErrors
}

func validateRecurrence(rule *domain.RecurrenceRule, timed bool) []FieldError {
	if rule == nil {
		return nil
	}

	fieldErrors := []FieldError{}
	switch rule.Frequency {
	case domain.RecurrenceFrequencyDaily, domain.RecurrenceFrequencyWeekly, domain.RecurrenceFrequencyMonthly:
	default:
		fieldErrors = append(fieldErrors, FieldError{Field: "recurrence.frequency", Message: "unsupported recurrence frequency"})
	}

	if rule.Interval < 0 {
		fieldErrors = append(fieldErrors, FieldError{Field: "recurrence.interval", Message: "interval must be greater than 0"})
	}
	if rule.Count != nil && *rule.Count < 1 {
		fieldErrors = append(fieldErrors, FieldError{Field: "recurrence.count", Message: "count must be greater than 0"})
	}
	if rule.UntilAt != nil && rule.UntilDate != nil {
		fieldErrors = append(fieldErrors, FieldError{Field: "recurrence", Message: "use untilAt or untilDate, not both"})
	}
	if rule.UntilDate != nil {
		if _, err := time.Parse(time.DateOnly, *rule.UntilDate); err != nil {
			fieldErrors = append(fieldErrors, FieldError{Field: "recurrence.untilDate", Message: "untilDate must be YYYY-MM-DD"})
		}
	}
	if len(rule.ByWeekday) > 0 && rule.Frequency != domain.RecurrenceFrequencyWeekly {
		fieldErrors = append(fieldErrors, FieldError{Field: "recurrence.byWeekday", Message: "byWeekday is only supported for weekly recurrence"})
	}
	if len(rule.ByMonthDay) > 0 && rule.Frequency != domain.RecurrenceFrequencyMonthly {
		fieldErrors = append(fieldErrors, FieldError{Field: "recurrence.byMonthDay", Message: "byMonthDay is only supported for monthly recurrence"})
	}
	if timed && rule.UntilDate == nil && rule.UntilAt == nil && rule.Count == nil {
		return fieldErrors
	}

	return fieldErrors
}

func validateResourceID(field, value string) error {
	if value == "" {
		return &ValidationError{
			Message:     "identifier validation failed",
			FieldErrors: []FieldError{{Field: field, Message: field + " is required"}},
		}
	}
	if _, err := ulid.ParseStrict(value); err != nil {
		return &ValidationError{
			Message:     "identifier validation failed",
			FieldErrors: []FieldError{{Field: field, Message: field + " must be a valid ULID"}},
		}
	}

	return nil
}

type createdCursor struct {
	ID        string `json:"id"`
	CreatedAt string `json:"createdAt"`
}

type agendaCursor struct {
	OccurrenceKey string `json:"occurrenceKey"`
	SortKey       string `json:"sortKey"`
}

func paginateByCreatedAt[T any](items []T, params domain.PageParams) (domain.Page[T], error) {
	limit := normalizeLimit(params.Limit)
	start := 0
	if params.Cursor != "" {
		var cursor createdCursor
		if err := decodeCursor(params.Cursor, &cursor); err != nil {
			return domain.Page[T]{}, invalidCursorError()
		}

		matched := false
		for index, item := range items {
			id, createdAt := entityMetadata(item)
			if id == cursor.ID && createdAt.Format(time.RFC3339Nano) == cursor.CreatedAt {
				start = index + 1
				matched = true
				break
			}
		}
		if !matched {
			return domain.Page[T]{}, invalidCursorError()
		}
	}
	if len(items) == 0 {
		return domain.Page[T]{Items: []T{}}, nil
	}

	end := start + limit
	if end > len(items) {
		end = len(items)
	}

	page := domain.Page[T]{Items: slices.Clone(items[start:end])}
	if end < len(items) {
		id, createdAt := entityMetadata(items[end-1])
		nextCursor := encodeCursor(createdCursor{
			ID:        id,
			CreatedAt: createdAt.Format(time.RFC3339Nano),
		})
		page.NextCursor = &nextCursor
	}

	return page, nil
}

func paginateAgenda(items []domain.AgendaItem, params domain.AgendaParams) (domain.Page[domain.AgendaItem], error) {
	limit := normalizeLimit(params.Limit)
	start := 0
	if params.Cursor != "" {
		var cursor agendaCursor
		if err := decodeCursor(params.Cursor, &cursor); err != nil {
			return domain.Page[domain.AgendaItem]{}, invalidCursorError()
		}

		matched := false
		for index, item := range items {
			if item.OccurrenceKey == cursor.OccurrenceKey && agendaSortKey(item) == cursor.SortKey {
				start = index + 1
				matched = true
				break
			}
		}
		if !matched {
			return domain.Page[domain.AgendaItem]{}, invalidCursorError()
		}
	}
	if len(items) == 0 {
		return domain.Page[domain.AgendaItem]{Items: []domain.AgendaItem{}}, nil
	}

	end := start + limit
	if end > len(items) {
		end = len(items)
	}

	page := domain.Page[domain.AgendaItem]{Items: slices.Clone(items[start:end])}
	if end < len(items) {
		last := items[end-1]
		nextCursor := encodeCursor(agendaCursor{
			OccurrenceKey: last.OccurrenceKey,
			SortKey:       agendaSortKey(last),
		})
		page.NextCursor = &nextCursor
	}

	return page, nil
}

func agendaItemsForEvent(event domain.Event, from, to time.Time) []domain.AgendaItem {
	items := []domain.AgendaItem{}

	switch {
	case event.StartAt != nil:
		duration := time.Duration(0)
		if event.EndAt != nil {
			duration = event.EndAt.Sub(*event.StartAt)
		}

		var occurrences []time.Time
		if event.Recurrence != nil {
			occurrences = recurrence.ExpandTimed(*event.StartAt, event.Recurrence, from.Add(-duration), to)
		} else {
			occurrences = []time.Time{*event.StartAt}
		}

		for _, occurrence := range occurrences {
			endAt := occurrence
			if event.EndAt != nil {
				endAt = occurrence.Add(duration)
			}
			if !occursInTimedRange(occurrence, endAt, from, to) {
				continue
			}

			item := domain.AgendaItem{
				Kind:          domain.AgendaItemKindEvent,
				OccurrenceKey: occurrenceKey(event.ID, &occurrence, nil),
				CalendarID:    event.CalendarID,
				SourceID:      event.ID,
				Title:         event.Title,
				Description:   cloneStringPtr(event.Description),
				StartAt:       cloneTimePtr(&occurrence),
				EndAt:         cloneTimePtr(&endAt),
			}
			if event.EndAt == nil {
				item.EndAt = nil
			}
			items = append(items, item)
		}
	case event.StartDate != nil:
		spanDays := 1
		if event.EndDate != nil {
			spanDays = inclusiveDateSpan(*event.StartDate, *event.EndDate)
		}

		var occurrences []string
		if event.Recurrence != nil {
			occurrences = recurrence.ExpandDate(*event.StartDate, event.Recurrence, from.AddDate(0, 0, -spanDays), to)
		} else {
			occurrences = []string{*event.StartDate}
		}

		for _, occurrence := range occurrences {
			startDate := occurrence
			endDate := startDate
			if spanDays > 1 {
				endDate = dateAddDays(startDate, spanDays-1)
			}

			if !occursInDateRange(startDate, endDate, from, to) {
				continue
			}

			item := domain.AgendaItem{
				Kind:          domain.AgendaItemKindEvent,
				OccurrenceKey: occurrenceKey(event.ID, nil, &startDate),
				CalendarID:    event.CalendarID,
				SourceID:      event.ID,
				Title:         event.Title,
				Description:   cloneStringPtr(event.Description),
				StartDate:     &startDate,
				EndDate:       &endDate,
			}
			if spanDays == 1 {
				item.EndDate = nil
			}
			items = append(items, item)
		}
	}

	return items
}

func agendaItemsForTask(task domain.Task, completions map[string]domain.TaskCompletion, from, to time.Time) []domain.AgendaItem {
	items := []domain.AgendaItem{}

	switch {
	case task.DueAt != nil:
		var occurrences []time.Time
		if task.Recurrence != nil {
			occurrences = recurrence.ExpandTimed(*task.DueAt, task.Recurrence, from, to)
		} else {
			occurrences = []time.Time{*task.DueAt}
		}

		for _, occurrence := range occurrences {
			if occurrence.Before(from) || !occurrence.Before(to) {
				continue
			}

			key := occurrenceKey(task.ID, &occurrence, nil)
			item := domain.AgendaItem{
				Kind:          domain.AgendaItemKindTask,
				OccurrenceKey: key,
				CalendarID:    task.CalendarID,
				SourceID:      task.ID,
				Title:         task.Title,
				Description:   cloneStringPtr(task.Description),
				DueAt:         cloneTimePtr(&occurrence),
				Priority:      task.Priority,
				Status:        task.Status,
				Tags:          slices.Clone(task.Tags),
			}
			if task.Recurrence == nil {
				item.CompletedAt = cloneTimePtr(task.CompletedAt)
			} else if completion, ok := completions[key]; ok {
				item.CompletedAt = cloneTimePtr(&completion.CompletedAt)
			}
			items = append(items, item)
		}
	case task.DueDate != nil:
		var occurrences []string
		if task.Recurrence != nil {
			occurrences = recurrence.ExpandDate(*task.DueDate, task.Recurrence, from, to)
		} else {
			occurrences = []string{*task.DueDate}
		}

		for _, occurrence := range occurrences {
			if !occursInDateRange(occurrence, occurrence, from, to) {
				continue
			}

			key := occurrenceKey(task.ID, nil, &occurrence)
			item := domain.AgendaItem{
				Kind:          domain.AgendaItemKindTask,
				OccurrenceKey: key,
				CalendarID:    task.CalendarID,
				SourceID:      task.ID,
				Title:         task.Title,
				Description:   cloneStringPtr(task.Description),
				DueDate:       &occurrence,
				Priority:      task.Priority,
				Status:        task.Status,
				Tags:          slices.Clone(task.Tags),
			}
			if task.Recurrence == nil {
				item.CompletedAt = cloneTimePtr(task.CompletedAt)
			} else if completion, ok := completions[key]; ok {
				item.CompletedAt = cloneTimePtr(&completion.CompletedAt)
			}
			items = append(items, item)
		}
	}

	return items
}

func compareAgendaItems(left, right domain.AgendaItem) int {
	if cmp := strings.Compare(agendaSortKey(left), agendaSortKey(right)); cmp != 0 {
		return cmp
	}
	if cmp := strings.Compare(string(left.Kind), string(right.Kind)); cmp != 0 {
		return cmp
	}
	if cmp := strings.Compare(left.SourceID, right.SourceID); cmp != 0 {
		return cmp
	}

	return strings.Compare(left.OccurrenceKey, right.OccurrenceKey)
}

func agendaSortKey(item domain.AgendaItem) string {
	switch {
	case item.StartAt != nil:
		return item.StartAt.Format(time.RFC3339Nano)
	case item.DueAt != nil:
		return item.DueAt.Format(time.RFC3339Nano)
	case item.StartDate != nil:
		return *item.StartDate
	case item.DueDate != nil:
		return *item.DueDate
	default:
		return ""
	}
}

func occurrenceKey(id string, at *time.Time, date *string) string {
	if at != nil {
		return id + "@" + at.Format(time.RFC3339Nano)
	}
	if date != nil {
		return id + "@" + *date
	}

	return id + "@single"
}

func encodeCursor(value any) string {
	payload, _ := json.Marshal(value)
	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodeCursor(token string, dest any) error {
	payload, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return err
	}

	return json.Unmarshal(payload, dest)
}

func normalizeLimit(limit int) int {
	switch {
	case limit <= 0:
		return 50
	case limit > 200:
		return 200
	default:
		return limit
	}
}

func invalidCursorError() error {
	return &ValidationError{
		Message:     "query validation failed",
		FieldErrors: []FieldError{{Field: "cursor", Message: "cursor must be a valid cursor returned by a previous list response"}},
	}
}

func entityMetadata[T any](item T) (string, time.Time) {
	switch value := any(item).(type) {
	case domain.Calendar:
		return value.ID, value.CreatedAt
	case domain.Event:
		return value.ID, value.CreatedAt
	case domain.Task:
		return value.ID, value.CreatedAt
	default:
		return "", time.Time{}
	}
}

func sanitizeOptionalString(value *string) *string {
	if value == nil {
		return nil
	}

	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}

	return &trimmed
}

func cloneStringPtr(value *string) *string {
	if value == nil {
		return nil
	}

	result := *value
	return &result
}

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}

	result := *value
	return &result
}

func cloneRule(value *domain.RecurrenceRule) *domain.RecurrenceRule {
	if value == nil {
		return nil
	}

	clone := *value
	if value.Count != nil {
		count := *value.Count
		clone.Count = &count
	}
	if value.UntilAt != nil {
		untilAt := *value.UntilAt
		clone.UntilAt = &untilAt
	}
	if value.UntilDate != nil {
		untilDate := *value.UntilDate
		clone.UntilDate = &untilDate
	}
	clone.ByWeekday = slices.Clone(value.ByWeekday)
	clone.ByMonthDay = slices.Clone(value.ByMonthDay)
	if clone.Interval == 0 {
		clone.Interval = 1
	}

	return &clone
}

func occursInTimedRange(start, end, from, to time.Time) bool {
	return start.Before(to) && !end.Before(from)
}

func occursInDateRange(startDate, endDate string, from, to time.Time) bool {
	start := mustParseDate(startDate)
	endExclusive := mustParseDate(endDate).AddDate(0, 0, 1)
	return start.Before(to) && endExclusive.After(from)
}

func inclusiveDateSpan(startDate, endDate string) int {
	start := mustParseDate(startDate)
	end := mustParseDate(endDate)
	if end.Before(start) {
		return 1
	}

	return int(end.Sub(start).Hours()/24) + 1
}

func dateAddDays(value string, days int) string {
	return mustParseDate(value).AddDate(0, 0, days).Format(time.DateOnly)
}

func mustParseDate(value string) time.Time {
	parsed, err := time.Parse(time.DateOnly, value)
	if err != nil {
		return time.Time{}
	}

	return parsed
}
