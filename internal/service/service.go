package service

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/oklog/ulid/v2"

	"github.com/yazanabuashour/openplanner/internal/domain"
	"github.com/yazanabuashour/openplanner/internal/icalendar"
	"github.com/yazanabuashour/openplanner/internal/recurrence"
	"github.com/yazanabuashour/openplanner/internal/store"
)

var (
	colorPattern = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
	tagPattern   = regexp.MustCompile(`^[a-z0-9_-]+$`)
)

const (
	maxICalendarImportBytes      = 2 << 20
	maxICalendarImportComponents = 2000
)

type Service struct {
	store *store.Store
	now   func() time.Time
}

type calendarWriteStatus string

const (
	calendarWriteStatusCreated       calendarWriteStatus = "created"
	calendarWriteStatusAlreadyExists calendarWriteStatus = "already_exists"
	calendarWriteStatusUpdated       calendarWriteStatus = "updated"
)

type calendarWriteResult struct {
	Calendar domain.Calendar
	Status   calendarWriteStatus
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

	now := service.now()
	event := domain.Event{
		ID:           ulid.Make().String(),
		CalendarID:   input.CalendarID,
		ICalendarUID: sanitizeOptionalString(input.ICalendarUID),
		Title:        strings.TrimSpace(input.Title),
		Description:  sanitizeOptionalString(input.Description),
		Location:     sanitizeOptionalString(input.Location),
		StartAt:      cloneTimePtr(input.StartAt),
		EndAt:        cloneTimePtr(input.EndAt),
		TimeZone:     sanitizeOptionalString(input.TimeZone),
		StartDate:    sanitizeOptionalString(input.StartDate),
		EndDate:      sanitizeOptionalString(input.EndDate),
		Recurrence:   cloneRule(input.Recurrence),
		Reminders:    buildReminderRules(input.Reminders, now),
		Attendees:    buildEventAttendees(input.Attendees, now),
		CreatedAt:    now,
		UpdatedAt:    now,
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
	if patch.TimeZone.Present {
		if patch.TimeZone.Clear {
			event.TimeZone = nil
		} else {
			event.TimeZone = sanitizeOptionalString(&patch.TimeZone.Value)
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
	if patch.Reminders.Present {
		if patch.Reminders.Clear {
			event.Reminders = []domain.ReminderRule{}
		} else {
			event.Reminders = buildReminderRules(patch.Reminders.Value, service.now())
		}
	}
	if patch.Attendees.Present {
		if patch.Attendees.Clear {
			event.Attendees = []domain.EventAttendee{}
		} else {
			event.Attendees = buildEventAttendees(patch.Attendees.Value, service.now())
		}
	}
	if event.StartDate != nil || event.EndDate != nil {
		if patch.TimeZone.Present && !patch.TimeZone.Clear {
			return domain.Event{}, &ValidationError{
				Message:     "event validation failed",
				FieldErrors: []FieldError{{Field: "timeZone", Message: "timeZone is only supported for timed events"}},
			}
		}
		event.TimeZone = nil
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
		ID:           ulid.Make().String(),
		CalendarID:   input.CalendarID,
		ICalendarUID: sanitizeOptionalString(input.ICalendarUID),
		Title:        strings.TrimSpace(input.Title),
		Description:  sanitizeOptionalString(input.Description),
		DueAt:        cloneTimePtr(input.DueAt),
		DueDate:      sanitizeOptionalString(input.DueDate),
		Recurrence:   cloneRule(input.Recurrence),
		Reminders:    buildReminderRules(input.Reminders, service.now()),
		Priority:     defaultTaskPriority(input.Priority),
		Status:       defaultTaskStatus(input.Status),
		Tags:         normalizeTags(input.Tags),
		CompletedAt:  cloneTimePtr(input.CompletedAt),
		CreatedAt:    service.now(),
		UpdatedAt:    service.now(),
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
		if task.CompletedAt == nil {
			completedAt := service.now()
			task.CompletedAt = &completedAt
		}
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

func (service *Service) ExportICalendar(calendarID string) (domain.ICalendarExport, error) {
	if calendarID != "" {
		if err := validateResourceID("calendarId", calendarID); err != nil {
			return domain.ICalendarExport{}, err
		}
	}

	calendars, err := service.store.ListCalendars()
	if err != nil {
		return domain.ICalendarExport{}, err
	}
	exportCalendars := calendars
	calendarName := ""
	filename := "openplanner.ics"
	if calendarID != "" {
		found := false
		for _, calendar := range calendars {
			if calendar.ID == calendarID {
				exportCalendars = []domain.Calendar{calendar}
				calendarName = calendar.Name
				filename = sanitizeICalendarFilename(calendar.Name)
				found = true
				break
			}
		}
		if !found {
			return domain.ICalendarExport{}, &NotFoundError{Resource: "calendar", ID: calendarID, Message: "calendar not found"}
		}
	}

	events, err := service.store.ListEvents(calendarID)
	if err != nil {
		return domain.ICalendarExport{}, err
	}
	tasks, err := service.store.ListTasks(domain.TaskListParams{PageParams: domain.PageParams{CalendarID: calendarID}})
	if err != nil {
		return domain.ICalendarExport{}, err
	}

	eventIDs := recurringEventIDs(events)
	taskIDs := recurringTaskIDs(tasks)
	eventStates, err := service.store.ListOccurrenceStates(domain.OccurrenceOwnerKindEvent, eventIDs)
	if err != nil {
		return domain.ICalendarExport{}, err
	}
	taskStates, err := service.store.ListOccurrenceStates(domain.OccurrenceOwnerKindTask, taskIDs)
	if err != nil {
		return domain.ICalendarExport{}, err
	}
	taskCompletions, err := service.store.ListTaskCompletions(taskIDs)
	if err != nil {
		return domain.ICalendarExport{}, err
	}

	result := icalendar.Build(icalendar.Export{
		Calendars:             exportCalendars,
		Events:                events,
		Tasks:                 tasks,
		EventOccurrenceStates: eventStates,
		TaskOccurrenceStates:  taskStates,
		TaskCompletions:       taskCompletions,
		GeneratedAt:           service.now(),
		Name:                  calendarExportName(calendarName),
	})

	return domain.ICalendarExport{
		ContentType:  result.ContentType,
		Filename:     filename,
		CalendarID:   calendarID,
		CalendarName: calendarName,
		EventCount:   result.EventCount,
		TaskCount:    result.TaskCount,
		Content:      result.Content,
	}, nil
}

func (service *Service) ImportICalendar(request domain.ICalendarImportRequest) (domain.ICalendarImport, error) {
	if len(request.Content) > maxICalendarImportBytes {
		return domain.ICalendarImport{}, icalendarImportValidationError("content", fmt.Sprintf("content must be less than or equal to %d bytes", maxICalendarImportBytes))
	}

	content := strings.TrimSpace(request.Content)
	if content == "" {
		return domain.ICalendarImport{}, icalendarImportValidationError("content", "content is required")
	}
	if strings.TrimSpace(request.CalendarID) != "" && strings.TrimSpace(request.CalendarName) != "" {
		return domain.ICalendarImport{}, icalendarImportValidationError("calendar", "use calendarId or calendarName, not both")
	}

	parsed, err := icalendar.ParseImport(content)
	if err != nil {
		return domain.ICalendarImport{}, icalendarImportValidationError("content", err.Error())
	}
	if componentCount := iCalendarImportComponentCount(parsed); componentCount > maxICalendarImportComponents {
		return domain.ICalendarImport{}, icalendarImportValidationError("content", fmt.Sprintf("iCalendar import contains %d components; maximum is %d", componentCount, maxICalendarImportComponents))
	}

	result := domain.ICalendarImport{Skips: append([]domain.ICalendarImportSkip(nil), parsed.Skips...)}
	calendarCache := map[string]domain.Calendar{}
	if request.CalendarID != "" {
		if err := validateResourceID("calendarId", request.CalendarID); err != nil {
			return domain.ICalendarImport{}, err
		}
		calendar, err := service.GetCalendar(request.CalendarID)
		if err != nil {
			return domain.ICalendarImport{}, err
		}
		calendarCache[calendar.Name] = calendar
	}

	eventByUID := map[string]domain.Event{}
	taskByUID := map[string]domain.Task{}
	calendarIDs := map[string]bool{}

	for _, imported := range parsed.Events {
		calendar, write, err := service.resolveImportCalendar(request, imported.CalendarName, parsed.CalendarColor, calendarCache)
		if err != nil {
			return domain.ICalendarImport{}, err
		}
		if write != nil {
			result.Writes = append(result.Writes, *write)
			countImportWrite(&result, write.Status)
		}
		calendarIDs[calendar.ID] = true

		eventInput := imported.Event
		eventInput.CalendarID = calendar.ID
		event, status, err := service.importEvent(imported, eventInput)
		if err != nil {
			service.addImportSkip(&result, "event", imported.UID, err.Error())
			continue
		}
		eventByUID[imported.UID] = event
		result.EventCount++
		result.Writes = append(result.Writes, domain.ICalendarImportWrite{Kind: "event", ID: event.ID, Status: status, Title: event.Title})
		countImportWrite(&result, status)

		for _, exDate := range imported.ExDates {
			if event.Recurrence == nil || eventChangeHasSelector(parsed.EventChanges, imported.UID, exDate) {
				continue
			}
			state, err := service.importCancelEventOccurrence(event, exDate)
			if err != nil {
				service.addImportSkip(&result, "event_occurrence", imported.UID, err.Error())
				continue
			}
			result.Writes = append(result.Writes, domain.ICalendarImportWrite{
				Kind:          "event_occurrence",
				ID:            event.ID,
				Status:        "updated",
				OccurrenceKey: state.OccurrenceKey,
			})
			countImportWrite(&result, "updated")
		}
	}

	for _, imported := range parsed.Tasks {
		calendar, write, err := service.resolveImportCalendar(request, imported.CalendarName, parsed.CalendarColor, calendarCache)
		if err != nil {
			return domain.ICalendarImport{}, err
		}
		if write != nil {
			result.Writes = append(result.Writes, *write)
			countImportWrite(&result, write.Status)
		}
		calendarIDs[calendar.ID] = true

		taskInput := imported.Task
		taskInput.CalendarID = calendar.ID
		task, status, err := service.importTask(imported, taskInput)
		if err != nil {
			service.addImportSkip(&result, "task", imported.UID, err.Error())
			continue
		}
		taskByUID[imported.UID] = task
		result.TaskCount++
		result.Writes = append(result.Writes, domain.ICalendarImportWrite{Kind: "task", ID: task.ID, Status: status, Title: task.Title})
		countImportWrite(&result, status)

		for _, exDate := range imported.ExDates {
			if task.Recurrence == nil || taskChangeHasSelector(parsed.TaskChanges, imported.UID, exDate) {
				continue
			}
			state, err := service.importCancelTaskOccurrence(task, exDate)
			if err != nil {
				service.addImportSkip(&result, "task_occurrence", imported.UID, err.Error())
				continue
			}
			result.Writes = append(result.Writes, domain.ICalendarImportWrite{
				Kind:          "task_occurrence",
				ID:            task.ID,
				Status:        "updated",
				OccurrenceKey: state.OccurrenceKey,
			})
			countImportWrite(&result, "updated")
		}
	}

	for _, change := range parsed.EventChanges {
		event, ok := eventByUID[change.UID]
		if !ok {
			service.addImportSkip(&result, "event_occurrence", change.UID, "base event was not imported")
			continue
		}
		state, err := service.importEventChange(event, change)
		if err != nil {
			service.addImportSkip(&result, "event_occurrence", change.UID, err.Error())
			continue
		}
		result.Writes = append(result.Writes, domain.ICalendarImportWrite{
			Kind:          "event_occurrence",
			ID:            event.ID,
			Status:        "updated",
			OccurrenceKey: state.OccurrenceKey,
		})
		countImportWrite(&result, "updated")
	}

	for _, change := range parsed.TaskChanges {
		task, ok := taskByUID[change.UID]
		if !ok {
			service.addImportSkip(&result, "task_occurrence", change.UID, "base task was not imported")
			continue
		}
		if change.Cancelled {
			state, err := service.importCancelTaskOccurrence(task, change.Recurrence)
			if err != nil {
				service.addImportSkip(&result, "task_occurrence", change.UID, err.Error())
				continue
			}
			result.Writes = append(result.Writes, domain.ICalendarImportWrite{Kind: "task_occurrence", ID: task.ID, Status: "updated", OccurrenceKey: state.OccurrenceKey})
			countImportWrite(&result, "updated")
			continue
		}
		if change.Replacement.DueAt != nil || change.Replacement.DueDate != nil {
			state, err := service.importRescheduleTaskOccurrence(task, change)
			if err != nil {
				service.addImportSkip(&result, "task_occurrence", change.UID, err.Error())
				continue
			}
			result.Writes = append(result.Writes, domain.ICalendarImportWrite{Kind: "task_occurrence", ID: task.ID, Status: "updated", OccurrenceKey: state.OccurrenceKey})
			countImportWrite(&result, "updated")
		}
		if change.Completed {
			completion, status, err := service.importCompleteTaskOccurrence(task, change.Recurrence, change.CompletedAt)
			if err != nil {
				service.addImportSkip(&result, "task_completion", change.UID, err.Error())
				continue
			}
			result.Writes = append(result.Writes, domain.ICalendarImportWrite{
				Kind:          "task_completion",
				ID:            task.ID,
				Status:        status,
				OccurrenceKey: completion.OccurrenceKey,
			})
			countImportWrite(&result, status)
		}
	}

	result.CalendarCount = len(calendarIDs)
	result.SkippedCount += len(result.Skips)
	return result, nil
}

func icalendarImportValidationError(field string, message string) error {
	return &ValidationError{
		Message:     "icalendar import validation failed",
		FieldErrors: []FieldError{{Field: field, Message: message}},
	}
}

func iCalendarImportComponentCount(parsed icalendar.ParsedImport) int {
	return len(parsed.Events) + len(parsed.EventChanges) + len(parsed.Tasks) + len(parsed.TaskChanges)
}

func (service *Service) resolveImportCalendar(request domain.ICalendarImportRequest, componentName string, color *string, cache map[string]domain.Calendar) (domain.Calendar, *domain.ICalendarImportWrite, error) {
	if request.CalendarID != "" {
		for _, calendar := range cache {
			if calendar.ID == request.CalendarID {
				return calendar, nil, nil
			}
		}
		calendar, err := service.GetCalendar(request.CalendarID)
		if err != nil {
			return domain.Calendar{}, nil, err
		}
		cache[calendar.Name] = calendar
		return calendar, nil, nil
	}

	name := strings.TrimSpace(request.CalendarName)
	if name == "" {
		name = strings.TrimSpace(componentName)
	}
	if name == "" {
		name = "Imported Calendar"
	}
	if calendar, ok := cache[name]; ok {
		return calendar, nil, nil
	}

	written, err := service.ensureImportCalendar(domain.Calendar{Name: name, Color: color})
	if err != nil {
		return domain.Calendar{}, nil, err
	}
	cache[written.Calendar.Name] = written.Calendar
	if written.Status == calendarWriteStatusAlreadyExists {
		return written.Calendar, nil, nil
	}
	return written.Calendar, &domain.ICalendarImportWrite{
		Kind:   "calendar",
		ID:     written.Calendar.ID,
		Status: string(written.Status),
		Name:   written.Calendar.Name,
	}, nil
}

func (service *Service) ensureImportCalendar(input domain.Calendar) (calendarWriteResult, error) {
	name := strings.TrimSpace(input.Name)
	calendars, err := service.store.ListCalendars()
	if err != nil {
		return calendarWriteResult{}, err
	}
	for _, calendar := range calendars {
		if calendar.Name != name {
			continue
		}
		if input.Color != nil && !stringPtrEqual(calendar.Color, sanitizeOptionalString(input.Color)) {
			updated, err := service.UpdateCalendar(calendar.ID, domain.CalendarPatch{Color: domain.SetPatch(*input.Color)})
			if err != nil {
				return calendarWriteResult{}, err
			}
			return calendarWriteResult{Calendar: updated, Status: calendarWriteStatusUpdated}, nil
		}
		return calendarWriteResult{Calendar: calendar, Status: calendarWriteStatusAlreadyExists}, nil
	}
	calendar, err := service.CreateCalendar(domain.Calendar{Name: name, Color: input.Color})
	if err != nil {
		return calendarWriteResult{}, err
	}
	return calendarWriteResult{Calendar: calendar, Status: calendarWriteStatusCreated}, nil
}

func (service *Service) importEvent(imported icalendar.ImportedEvent, eventInput domain.Event) (domain.Event, string, error) {
	if id := strings.TrimSpace(imported.OpenPlannerID); id != "" {
		if _, err := ulid.ParseStrict(id); err == nil {
			existing, err := service.GetEvent(id)
			if err == nil {
				if existing.CalendarID == eventInput.CalendarID {
					return service.importUpdateEvent(existing, eventInput)
				}
			} else {
				var notFound *NotFoundError
				if !errors.As(err, &notFound) {
					return domain.Event{}, "", err
				}
			}
		}
	}
	if imported.UID != "" {
		existing, err := service.store.GetEventByICalendarUID(eventInput.CalendarID, imported.UID)
		if err == nil {
			return service.importUpdateEvent(existing, eventInput)
		}
		if !errors.Is(err, store.ErrNotFound) {
			return domain.Event{}, "", err
		}
	}
	created, err := service.CreateEvent(eventInput)
	if err != nil {
		return domain.Event{}, "", err
	}
	return created, "created", nil
}

func (service *Service) importUpdateEvent(existing domain.Event, input domain.Event) (domain.Event, string, error) {
	existing.ICalendarUID = sanitizeOptionalString(input.ICalendarUID)
	existing.Title = strings.TrimSpace(input.Title)
	existing.Description = sanitizeOptionalString(input.Description)
	existing.Location = sanitizeOptionalString(input.Location)
	existing.StartAt = cloneTimePtr(input.StartAt)
	existing.EndAt = cloneTimePtr(input.EndAt)
	existing.TimeZone = sanitizeOptionalString(input.TimeZone)
	existing.StartDate = sanitizeOptionalString(input.StartDate)
	existing.EndDate = sanitizeOptionalString(input.EndDate)
	existing.Recurrence = cloneRule(input.Recurrence)
	existing.Reminders = buildReminderRules(input.Reminders, service.now())
	existing.Attendees = buildEventAttendees(input.Attendees, service.now())
	existing.UpdatedAt = service.now()
	if err := validateEvent(existing); err != nil {
		return domain.Event{}, "", err
	}
	if err := service.store.UpdateEvent(existing); err != nil {
		if errors.Is(err, store.ErrConflict) {
			return domain.Event{}, "", &ConflictError{Message: "event iCalendar UID already exists"}
		}
		return domain.Event{}, "", err
	}
	if err := service.store.DeleteOccurrenceStates(domain.OccurrenceOwnerKindEvent, existing.ID); err != nil {
		return domain.Event{}, "", err
	}
	return existing, "updated", nil
}

func (service *Service) importTask(imported icalendar.ImportedTask, taskInput domain.Task) (domain.Task, string, error) {
	if id := strings.TrimSpace(imported.OpenPlannerID); id != "" {
		if _, err := ulid.ParseStrict(id); err == nil {
			existing, err := service.GetTask(id)
			if err == nil {
				if existing.CalendarID == taskInput.CalendarID {
					return service.importUpdateTask(existing, taskInput)
				}
			} else {
				var notFound *NotFoundError
				if !errors.As(err, &notFound) {
					return domain.Task{}, "", err
				}
			}
		}
	}
	if imported.UID != "" {
		existing, err := service.store.GetTaskByICalendarUID(taskInput.CalendarID, imported.UID)
		if err == nil {
			return service.importUpdateTask(existing, taskInput)
		}
		if !errors.Is(err, store.ErrNotFound) {
			return domain.Task{}, "", err
		}
	}
	created, err := service.CreateTask(taskInput)
	if err != nil {
		return domain.Task{}, "", err
	}
	return created, "created", nil
}

func (service *Service) importUpdateTask(existing domain.Task, input domain.Task) (domain.Task, string, error) {
	existing.ICalendarUID = sanitizeOptionalString(input.ICalendarUID)
	existing.Title = strings.TrimSpace(input.Title)
	existing.Description = sanitizeOptionalString(input.Description)
	existing.DueAt = cloneTimePtr(input.DueAt)
	existing.DueDate = sanitizeOptionalString(input.DueDate)
	existing.Recurrence = cloneRule(input.Recurrence)
	existing.Reminders = buildReminderRules(input.Reminders, service.now())
	existing.Priority = defaultTaskPriority(input.Priority)
	existing.Status = defaultTaskStatus(input.Status)
	existing.Tags = normalizeTags(input.Tags)
	existing.CompletedAt = cloneTimePtr(input.CompletedAt)
	if existing.Status == domain.TaskStatusDone && existing.CompletedAt == nil {
		completedAt := service.now()
		existing.CompletedAt = &completedAt
	}
	existing.UpdatedAt = service.now()
	if err := validateTask(existing); err != nil {
		return domain.Task{}, "", err
	}
	if err := service.store.UpdateTask(existing); err != nil {
		if errors.Is(err, store.ErrConflict) {
			return domain.Task{}, "", &ConflictError{Message: "task iCalendar UID already exists"}
		}
		return domain.Task{}, "", err
	}
	if err := service.store.DeleteOccurrenceStates(domain.OccurrenceOwnerKindTask, existing.ID); err != nil {
		return domain.Task{}, "", err
	}
	if err := service.store.DeleteTaskCompletions(existing.ID); err != nil {
		return domain.Task{}, "", err
	}
	return existing, "updated", nil
}

func (service *Service) importEventChange(event domain.Event, change icalendar.ImportedEventChange) (domain.OccurrenceState, error) {
	if change.Cancelled {
		return service.importCancelEventOccurrence(event, change.Recurrence)
	}
	mutation := domain.OccurrenceMutationRequest{
		OccurrenceAt:       cloneTimePtr(change.Recurrence.At),
		OccurrenceDate:     cloneStringPtr(change.Recurrence.Date),
		ReplacementAt:      cloneTimePtr(change.Replacement.StartAt),
		ReplacementEndAt:   cloneTimePtr(change.Replacement.EndAt),
		ReplacementDate:    cloneStringPtr(change.Replacement.StartDate),
		ReplacementEndDate: cloneStringPtr(change.Replacement.EndDate),
	}
	return service.RescheduleEventOccurrence(event.ID, mutation)
}

func (service *Service) importCancelEventOccurrence(event domain.Event, selector icalendar.OccurrenceSelector) (domain.OccurrenceState, error) {
	return service.CancelEventOccurrence(event.ID, domain.OccurrenceMutationRequest{
		OccurrenceAt:   cloneTimePtr(selector.At),
		OccurrenceDate: cloneStringPtr(selector.Date),
	})
}

func (service *Service) importCancelTaskOccurrence(task domain.Task, selector icalendar.OccurrenceSelector) (domain.OccurrenceState, error) {
	return service.CancelTaskOccurrence(task.ID, domain.OccurrenceMutationRequest{
		OccurrenceAt:   cloneTimePtr(selector.At),
		OccurrenceDate: cloneStringPtr(selector.Date),
	})
}

func (service *Service) importRescheduleTaskOccurrence(task domain.Task, change icalendar.ImportedTaskChange) (domain.OccurrenceState, error) {
	return service.RescheduleTaskOccurrence(task.ID, domain.OccurrenceMutationRequest{
		OccurrenceAt:    cloneTimePtr(change.Recurrence.At),
		OccurrenceDate:  cloneStringPtr(change.Recurrence.Date),
		ReplacementAt:   cloneTimePtr(change.Replacement.DueAt),
		ReplacementDate: cloneStringPtr(change.Replacement.DueDate),
	})
}

func (service *Service) importCompleteTaskOccurrence(task domain.Task, selector icalendar.OccurrenceSelector, completedAt *time.Time) (domain.TaskCompletion, string, error) {
	if task.Recurrence == nil {
		return domain.TaskCompletion{}, "", &ValidationError{
			Message:     "task completion validation failed",
			FieldErrors: []FieldError{{Field: "recurrence", Message: "task is not recurring"}},
		}
	}
	completed := service.now()
	if completedAt != nil {
		completed = *completedAt
	}
	completion := domain.TaskCompletion{TaskID: task.ID, CompletedAt: completed}
	switch {
	case task.DueAt != nil && selector.At != nil:
		if !recurrence.IncludesTimed(*task.DueAt, task.Recurrence, *selector.At) {
			return domain.TaskCompletion{}, "", occurrenceValidationError("occurrenceAt", "occurrenceAt does not match the recurring task schedule")
		}
		key := occurrenceKey(task.ID, selector.At, nil)
		if service.taskOccurrenceCancelled(task.ID, key) {
			return domain.TaskCompletion{}, "", &ConflictError{Message: "task occurrence is canceled"}
		}
		completion.OccurrenceKey = key
		completion.OccurrenceAt = cloneTimePtr(selector.At)
	case task.DueDate != nil && selector.Date != nil:
		if !recurrence.IncludesDate(*task.DueDate, task.Recurrence, *selector.Date) {
			return domain.TaskCompletion{}, "", occurrenceValidationError("occurrenceDate", "occurrenceDate does not match the recurring task schedule")
		}
		key := occurrenceKey(task.ID, nil, selector.Date)
		if service.taskOccurrenceCancelled(task.ID, key) {
			return domain.TaskCompletion{}, "", &ConflictError{Message: "task occurrence is canceled"}
		}
		completion.OccurrenceKey = key
		completion.OccurrenceDate = cloneStringPtr(selector.Date)
	default:
		return domain.TaskCompletion{}, "", &ValidationError{
			Message:     "task completion validation failed",
			FieldErrors: []FieldError{{Field: "occurrence", Message: "occurrence does not match task timing"}},
		}
	}
	if err := service.store.CreateTaskCompletion(completion); err != nil {
		if errors.Is(err, store.ErrConflict) {
			return completion, "skipped", nil
		}
		return domain.TaskCompletion{}, "", err
	}
	return completion, "created", nil
}

func eventChangeHasSelector(changes []icalendar.ImportedEventChange, uid string, selector icalendar.OccurrenceSelector) bool {
	for _, change := range changes {
		if change.UID == uid && occurrenceSelectorEqual(change.Recurrence, selector) {
			return true
		}
	}
	return false
}

func taskChangeHasSelector(changes []icalendar.ImportedTaskChange, uid string, selector icalendar.OccurrenceSelector) bool {
	for _, change := range changes {
		if change.UID == uid && occurrenceSelectorEqual(change.Recurrence, selector) {
			return true
		}
	}
	return false
}

func occurrenceSelectorEqual(left icalendar.OccurrenceSelector, right icalendar.OccurrenceSelector) bool {
	switch {
	case left.At != nil && right.At != nil:
		return left.At.Equal(*right.At)
	case left.Date != nil && right.Date != nil:
		return *left.Date == *right.Date
	default:
		return false
	}
}

func (service *Service) addImportSkip(result *domain.ICalendarImport, kind string, uid string, reason string) {
	result.Skips = append(result.Skips, domain.ICalendarImportSkip{Kind: kind, UID: uid, Reason: reason})
}

func countImportWrite(result *domain.ICalendarImport, status string) {
	switch status {
	case "created":
		result.CreatedCount++
	case "updated":
		result.UpdatedCount++
	case "skipped":
		result.SkippedCount++
	}
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
	if patch.Reminders.Present {
		if patch.Reminders.Clear {
			task.Reminders = []domain.ReminderRule{}
		} else {
			task.Reminders = buildReminderRules(patch.Reminders.Value, service.now())
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

func (service *Service) CreateEventTaskLink(eventID string, taskID string) (domain.EventTaskLink, error) {
	if err := validateResourceID("eventId", eventID); err != nil {
		return domain.EventTaskLink{}, err
	}
	if err := validateResourceID("taskId", taskID); err != nil {
		return domain.EventTaskLink{}, err
	}
	if _, err := service.GetEvent(eventID); err != nil {
		return domain.EventTaskLink{}, err
	}
	if _, err := service.GetTask(taskID); err != nil {
		return domain.EventTaskLink{}, err
	}

	now := service.now()
	link := domain.EventTaskLink{
		EventID:   eventID,
		TaskID:    taskID,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := service.store.CreateEventTaskLink(link); err != nil {
		if errors.Is(err, store.ErrConflict) {
			return domain.EventTaskLink{}, &ConflictError{Message: "event task link already exists"}
		}
		if errors.Is(err, store.ErrNotFound) {
			return domain.EventTaskLink{}, &NotFoundError{Message: "event or task not found"}
		}

		return domain.EventTaskLink{}, err
	}

	return link, nil
}

func (service *Service) DeleteEventTaskLink(eventID string, taskID string) error {
	if err := validateResourceID("eventId", eventID); err != nil {
		return err
	}
	if err := validateResourceID("taskId", taskID); err != nil {
		return err
	}
	if _, err := service.GetEvent(eventID); err != nil {
		return err
	}
	if _, err := service.GetTask(taskID); err != nil {
		return err
	}

	if err := service.store.DeleteEventTaskLink(eventID, taskID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return &NotFoundError{Message: "event task link not found"}
		}

		return err
	}

	return nil
}

func (service *Service) ListEventTaskLinks(filter domain.EventTaskLinkFilter) ([]domain.EventTaskLink, error) {
	if filter.EventID != "" {
		if err := validateResourceID("eventId", filter.EventID); err != nil {
			return nil, err
		}
	}
	if filter.TaskID != "" {
		if err := validateResourceID("taskId", filter.TaskID); err != nil {
			return nil, err
		}
	}

	return service.store.ListEventTaskLinks(filter)
}

func (service *Service) CompleteTask(id string, request domain.TaskCompletionRequest) (domain.TaskCompletion, error) {
	task, err := service.GetTask(id)
	if err != nil {
		return domain.TaskCompletion{}, err
	}

	if task.Recurrence == nil {
		if request.OccurrenceKey != "" || request.OccurrenceAt != nil || request.OccurrenceDate != nil {
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
	if request.OccurrenceKey != "" && (request.OccurrenceAt != nil || request.OccurrenceDate != nil) {
		return domain.TaskCompletion{}, &ValidationError{
			Message: "task completion validation failed",
			FieldErrors: []FieldError{
				{Field: "occurrence", Message: "use occurrenceKey, occurrenceAt, or occurrenceDate, not more than one"},
			},
		}
	}

	if task.DueAt != nil {
		if request.OccurrenceKey != "" {
			return service.completeRecurringTaskByOccurrenceKey(task, request.OccurrenceKey)
		}
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
		if service.taskOccurrenceCancelled(task.ID, occurrenceKey(task.ID, request.OccurrenceAt, nil)) {
			return domain.TaskCompletion{}, &ConflictError{Message: "task occurrence is canceled"}
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

	if request.OccurrenceKey != "" {
		return service.completeRecurringTaskByOccurrenceKey(task, request.OccurrenceKey)
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
	if service.taskOccurrenceCancelled(task.ID, occurrenceKey(task.ID, nil, request.OccurrenceDate)) {
		return domain.TaskCompletion{}, &ConflictError{Message: "task occurrence is canceled"}
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

func (service *Service) CancelEventOccurrence(id string, request domain.OccurrenceMutationRequest) (domain.OccurrenceState, error) {
	event, err := service.GetEvent(id)
	if err != nil {
		return domain.OccurrenceState{}, err
	}
	state, err := service.validateEventOccurrenceMutation(event, request, true)
	if err != nil {
		return domain.OccurrenceState{}, err
	}
	return service.upsertOccurrenceState(state)
}

func (service *Service) RescheduleEventOccurrence(id string, request domain.OccurrenceMutationRequest) (domain.OccurrenceState, error) {
	event, err := service.GetEvent(id)
	if err != nil {
		return domain.OccurrenceState{}, err
	}
	state, err := service.validateEventOccurrenceMutation(event, request, false)
	if err != nil {
		return domain.OccurrenceState{}, err
	}
	return service.upsertOccurrenceState(state)
}

func (service *Service) CancelTaskOccurrence(id string, request domain.OccurrenceMutationRequest) (domain.OccurrenceState, error) {
	task, err := service.GetTask(id)
	if err != nil {
		return domain.OccurrenceState{}, err
	}
	state, err := service.validateTaskOccurrenceMutation(task, request, true)
	if err != nil {
		return domain.OccurrenceState{}, err
	}
	return service.upsertOccurrenceState(state)
}

func (service *Service) RescheduleTaskOccurrence(id string, request domain.OccurrenceMutationRequest) (domain.OccurrenceState, error) {
	task, err := service.GetTask(id)
	if err != nil {
		return domain.OccurrenceState{}, err
	}
	state, err := service.validateTaskOccurrenceMutation(task, request, false)
	if err != nil {
		return domain.OccurrenceState{}, err
	}
	return service.upsertOccurrenceState(state)
}

func (service *Service) completeRecurringTaskByOccurrenceKey(task domain.Task, key string) (domain.TaskCompletion, error) {
	occurrenceAt, occurrenceDate, err := parseOccurrenceKey(task.ID, strings.TrimSpace(key))
	if err != nil {
		return domain.TaskCompletion{}, &ValidationError{
			Message: "task completion validation failed",
			FieldErrors: []FieldError{
				{Field: "occurrenceKey", Message: "occurrenceKey must match a recurring task occurrence"},
			},
		}
	}
	if task.DueAt != nil {
		if occurrenceAt == nil || occurrenceDate != nil || !recurrence.IncludesTimed(*task.DueAt, task.Recurrence, *occurrenceAt) {
			return domain.TaskCompletion{}, &ValidationError{
				Message: "task completion validation failed",
				FieldErrors: []FieldError{
					{Field: "occurrenceKey", Message: "occurrenceKey does not match the recurring task schedule"},
				},
			}
		}
		if service.taskOccurrenceCancelled(task.ID, key) {
			return domain.TaskCompletion{}, &ConflictError{Message: "task occurrence is canceled"}
		}
		completion := domain.TaskCompletion{
			TaskID:        task.ID,
			OccurrenceKey: key,
			OccurrenceAt:  cloneTimePtr(occurrenceAt),
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

	if occurrenceDate == nil || occurrenceAt != nil || !recurrence.IncludesDate(*task.DueDate, task.Recurrence, *occurrenceDate) {
		return domain.TaskCompletion{}, &ValidationError{
			Message: "task completion validation failed",
			FieldErrors: []FieldError{
				{Field: "occurrenceKey", Message: "occurrenceKey does not match the recurring task schedule"},
			},
		}
	}
	if service.taskOccurrenceCancelled(task.ID, key) {
		return domain.TaskCompletion{}, &ConflictError{Message: "task occurrence is canceled"}
	}
	completion := domain.TaskCompletion{
		TaskID:         task.ID,
		OccurrenceKey:  key,
		OccurrenceDate: sanitizeOptionalString(occurrenceDate),
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

func (service *Service) taskOccurrenceCancelled(taskID string, key string) bool {
	states, err := service.store.ListOccurrenceStates(domain.OccurrenceOwnerKindTask, []string{taskID})
	if err != nil {
		return false
	}
	return states[taskID][key].Cancelled
}

func (service *Service) validateEventOccurrenceMutation(event domain.Event, request domain.OccurrenceMutationRequest, cancel bool) (domain.OccurrenceState, error) {
	if event.Recurrence == nil {
		return domain.OccurrenceState{}, &ValidationError{
			Message:     "event occurrence validation failed",
			FieldErrors: []FieldError{{Field: "recurrence", Message: "event is not recurring"}},
		}
	}
	now := service.now()
	state := domain.OccurrenceState{
		OwnerKind: domain.OccurrenceOwnerKindEvent,
		OwnerID:   event.ID,
		Cancelled: cancel,
		CreatedAt: now,
		UpdatedAt: now,
	}

	switch {
	case event.StartAt != nil:
		if request.OccurrenceAt == nil || request.OccurrenceDate != nil {
			return domain.OccurrenceState{}, occurrenceValidationError("occurrenceAt", "recurring timed events require occurrenceAt")
		}
		location := eventLocation(event)
		if location != nil && !timeOffsetMatchesLocation(*request.OccurrenceAt, location) {
			return domain.OccurrenceState{}, occurrenceValidationError("occurrenceAt", "occurrenceAt offset must match event timeZone")
		}
		if !eventIncludesTimedOccurrence(event, *request.OccurrenceAt) {
			return domain.OccurrenceState{}, occurrenceValidationError("occurrenceAt", "occurrenceAt does not match the recurring event schedule")
		}
		state.OccurrenceKey = occurrenceKey(event.ID, request.OccurrenceAt, nil)
		state.OccurrenceAt = cloneTimePtr(request.OccurrenceAt)
		if cancel {
			if request.ReplacementAt != nil || request.ReplacementEndAt != nil || request.ReplacementDate != nil || request.ReplacementEndDate != nil {
				return domain.OccurrenceState{}, occurrenceValidationError("replacement", "canceled occurrences cannot include replacement timing")
			}
			return state, nil
		}
		if request.ReplacementAt == nil || request.ReplacementDate != nil || request.ReplacementEndDate != nil {
			return domain.OccurrenceState{}, occurrenceValidationError("startAt", "rescheduled timed events require startAt")
		}
		if location != nil && !timeOffsetMatchesLocation(*request.ReplacementAt, location) {
			return domain.OccurrenceState{}, occurrenceValidationError("startAt", "startAt offset must match event timeZone")
		}
		if location != nil && request.ReplacementEndAt != nil && !timeOffsetMatchesLocation(*request.ReplacementEndAt, location) {
			return domain.OccurrenceState{}, occurrenceValidationError("endAt", "endAt offset must match event timeZone")
		}
		if request.ReplacementEndAt != nil && !request.ReplacementEndAt.After(*request.ReplacementAt) {
			return domain.OccurrenceState{}, occurrenceValidationError("endAt", "endAt must be after startAt")
		}
		state.ReplacementAt = cloneTimePtr(request.ReplacementAt)
		state.ReplacementEndAt = cloneTimePtr(request.ReplacementEndAt)
		return state, nil
	case event.StartDate != nil:
		if request.OccurrenceDate == nil || request.OccurrenceAt != nil {
			return domain.OccurrenceState{}, occurrenceValidationError("occurrenceDate", "recurring all-day events require occurrenceDate")
		}
		if !recurrence.IncludesDate(*event.StartDate, event.Recurrence, *request.OccurrenceDate) {
			return domain.OccurrenceState{}, occurrenceValidationError("occurrenceDate", "occurrenceDate does not match the recurring event schedule")
		}
		state.OccurrenceKey = occurrenceKey(event.ID, nil, request.OccurrenceDate)
		state.OccurrenceDate = sanitizeOptionalString(request.OccurrenceDate)
		if cancel {
			if request.ReplacementAt != nil || request.ReplacementEndAt != nil || request.ReplacementDate != nil || request.ReplacementEndDate != nil {
				return domain.OccurrenceState{}, occurrenceValidationError("replacement", "canceled occurrences cannot include replacement timing")
			}
			return state, nil
		}
		if request.ReplacementDate == nil || request.ReplacementAt != nil || request.ReplacementEndAt != nil {
			return domain.OccurrenceState{}, occurrenceValidationError("startDate", "rescheduled all-day events require startDate")
		}
		if request.ReplacementEndDate != nil && *request.ReplacementEndDate < *request.ReplacementDate {
			return domain.OccurrenceState{}, occurrenceValidationError("endDate", "endDate must not be before startDate")
		}
		state.ReplacementDate = sanitizeOptionalString(request.ReplacementDate)
		state.ReplacementEndDate = sanitizeOptionalString(request.ReplacementEndDate)
		return state, nil
	default:
		return domain.OccurrenceState{}, occurrenceValidationError("timing", "event start timing is required")
	}
}

func (service *Service) validateTaskOccurrenceMutation(task domain.Task, request domain.OccurrenceMutationRequest, cancel bool) (domain.OccurrenceState, error) {
	if task.Recurrence == nil {
		return domain.OccurrenceState{}, &ValidationError{
			Message:     "task occurrence validation failed",
			FieldErrors: []FieldError{{Field: "recurrence", Message: "task is not recurring"}},
		}
	}
	now := service.now()
	state := domain.OccurrenceState{
		OwnerKind: domain.OccurrenceOwnerKindTask,
		OwnerID:   task.ID,
		Cancelled: cancel,
		CreatedAt: now,
		UpdatedAt: now,
	}

	switch {
	case task.DueAt != nil:
		if request.OccurrenceAt == nil || request.OccurrenceDate != nil {
			return domain.OccurrenceState{}, occurrenceValidationError("occurrenceAt", "recurring timed tasks require occurrenceAt")
		}
		if !recurrence.IncludesTimed(*task.DueAt, task.Recurrence, *request.OccurrenceAt) {
			return domain.OccurrenceState{}, occurrenceValidationError("occurrenceAt", "occurrenceAt does not match the recurring task schedule")
		}
		state.OccurrenceKey = occurrenceKey(task.ID, request.OccurrenceAt, nil)
		state.OccurrenceAt = cloneTimePtr(request.OccurrenceAt)
		if cancel {
			if request.ReplacementAt != nil || request.ReplacementEndAt != nil || request.ReplacementDate != nil || request.ReplacementEndDate != nil {
				return domain.OccurrenceState{}, occurrenceValidationError("replacement", "canceled occurrences cannot include replacement timing")
			}
			return state, nil
		}
		if request.ReplacementAt == nil || request.ReplacementEndAt != nil || request.ReplacementDate != nil || request.ReplacementEndDate != nil {
			return domain.OccurrenceState{}, occurrenceValidationError("dueAt", "rescheduled timed tasks require dueAt")
		}
		state.ReplacementAt = cloneTimePtr(request.ReplacementAt)
		return state, nil
	case task.DueDate != nil:
		if request.OccurrenceDate == nil || request.OccurrenceAt != nil {
			return domain.OccurrenceState{}, occurrenceValidationError("occurrenceDate", "recurring date-based tasks require occurrenceDate")
		}
		if !recurrence.IncludesDate(*task.DueDate, task.Recurrence, *request.OccurrenceDate) {
			return domain.OccurrenceState{}, occurrenceValidationError("occurrenceDate", "occurrenceDate does not match the recurring task schedule")
		}
		state.OccurrenceKey = occurrenceKey(task.ID, nil, request.OccurrenceDate)
		state.OccurrenceDate = sanitizeOptionalString(request.OccurrenceDate)
		if cancel {
			if request.ReplacementAt != nil || request.ReplacementEndAt != nil || request.ReplacementDate != nil || request.ReplacementEndDate != nil {
				return domain.OccurrenceState{}, occurrenceValidationError("replacement", "canceled occurrences cannot include replacement timing")
			}
			return state, nil
		}
		if request.ReplacementDate == nil || request.ReplacementAt != nil || request.ReplacementEndAt != nil || request.ReplacementEndDate != nil {
			return domain.OccurrenceState{}, occurrenceValidationError("dueDate", "rescheduled date-based tasks require dueDate")
		}
		state.ReplacementDate = sanitizeOptionalString(request.ReplacementDate)
		return state, nil
	default:
		return domain.OccurrenceState{}, occurrenceValidationError("due", "task due timing is required")
	}
}

func (service *Service) upsertOccurrenceState(state domain.OccurrenceState) (domain.OccurrenceState, error) {
	if err := service.store.UpsertOccurrenceState(state); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return domain.OccurrenceState{}, &NotFoundError{Message: "recurring owner not found"}
		}
		return domain.OccurrenceState{}, err
	}
	return state, nil
}

func occurrenceValidationError(field string, message string) error {
	return &ValidationError{
		Message:     "occurrence validation failed",
		FieldErrors: []FieldError{{Field: field, Message: message}},
	}
}

func loadEventLocation(name string) (*time.Location, error) {
	name = strings.TrimSpace(name)
	if name == "" || name == "Local" {
		return nil, fmt.Errorf("invalid event timezone")
	}

	return time.LoadLocation(name)
}

func timeOffsetMatchesLocation(value time.Time, location *time.Location) bool {
	_, inputOffset := value.Zone()
	_, locationOffset := value.In(location).Zone()
	return inputOffset == locationOffset
}

func eventLocation(event domain.Event) *time.Location {
	if event.TimeZone == nil {
		return nil
	}
	location, err := loadEventLocation(*event.TimeZone)
	if err != nil {
		return nil
	}
	return location
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
	eventIDs := make([]string, 0, len(events))
	for _, event := range events {
		if event.Recurrence != nil {
			eventIDs = append(eventIDs, event.ID)
		}
	}
	for _, task := range tasks {
		if task.Recurrence != nil {
			taskIDs = append(taskIDs, task.ID)
		}
	}
	completions, err := service.store.ListTaskCompletions(taskIDs)
	if err != nil {
		return domain.Page[domain.AgendaItem]{}, err
	}
	eventOccurrenceStates, err := service.store.ListOccurrenceStates(domain.OccurrenceOwnerKindEvent, eventIDs)
	if err != nil {
		return domain.Page[domain.AgendaItem]{}, err
	}
	taskOccurrenceStates, err := service.store.ListOccurrenceStates(domain.OccurrenceOwnerKindTask, taskIDs)
	if err != nil {
		return domain.Page[domain.AgendaItem]{}, err
	}

	items := make([]domain.AgendaItem, 0, len(events)+len(tasks))
	for _, event := range events {
		items = append(items, agendaItemsForEvent(event, eventOccurrenceStates[event.ID], params.From, params.To)...)
	}
	for _, task := range tasks {
		items = append(items, agendaItemsForTask(task, completions[task.ID], taskOccurrenceStates[task.ID], params.From, params.To)...)
	}

	slices.SortFunc(items, compareAgendaItems)

	return paginateAgenda(items, params)
}

func (service *Service) ListPendingReminders(params domain.ReminderQueryParams) (domain.Page[domain.PendingReminder], error) {
	if params.From.IsZero() || params.To.IsZero() || !params.From.Before(params.To) {
		return domain.Page[domain.PendingReminder]{}, &ValidationError{
			Message: "reminder query validation failed",
			FieldErrors: []FieldError{
				{Field: "range", Message: "from must be before to"},
			},
		}
	}
	if params.CalendarID != "" {
		if err := validateResourceID("calendarId", params.CalendarID); err != nil {
			return domain.Page[domain.PendingReminder]{}, err
		}
	}

	events, err := service.store.ListEvents(params.CalendarID)
	if err != nil {
		return domain.Page[domain.PendingReminder]{}, err
	}
	tasks, err := service.store.ListTasks(domain.TaskListParams{PageParams: domain.PageParams{CalendarID: params.CalendarID}})
	if err != nil {
		return domain.Page[domain.PendingReminder]{}, err
	}

	reminderIDs := reminderIDsFor(events, tasks)
	dismissals, err := service.store.ListReminderDismissals(reminderIDs)
	if err != nil {
		return domain.Page[domain.PendingReminder]{}, err
	}
	eventIDs := make([]string, 0, len(events))
	for _, event := range events {
		if event.Recurrence != nil {
			eventIDs = append(eventIDs, event.ID)
		}
	}
	taskIDs := make([]string, 0, len(tasks))
	for _, task := range tasks {
		if task.Recurrence != nil {
			taskIDs = append(taskIDs, task.ID)
		}
	}
	eventOccurrenceStates, err := service.store.ListOccurrenceStates(domain.OccurrenceOwnerKindEvent, eventIDs)
	if err != nil {
		return domain.Page[domain.PendingReminder]{}, err
	}
	taskOccurrenceStates, err := service.store.ListOccurrenceStates(domain.OccurrenceOwnerKindTask, taskIDs)
	if err != nil {
		return domain.Page[domain.PendingReminder]{}, err
	}

	items := []domain.PendingReminder{}
	for _, event := range events {
		items = append(items, pendingRemindersForEvent(event, eventOccurrenceStates[event.ID], dismissals, params.From, params.To)...)
	}
	for _, task := range tasks {
		items = append(items, pendingRemindersForTask(task, taskOccurrenceStates[task.ID], dismissals, params.From, params.To)...)
	}
	slices.SortFunc(items, comparePendingReminders)

	return paginatePendingReminders(items, params)
}

func (service *Service) DismissReminderOccurrence(reminderOccurrenceID string) (domain.ReminderDismissal, error) {
	token, err := decodeReminderOccurrenceID(strings.TrimSpace(reminderOccurrenceID))
	if err != nil || token.ReminderID == "" || token.OccurrenceKey == "" {
		return domain.ReminderDismissal{}, &ValidationError{
			Message: "reminder dismissal validation failed",
			FieldErrors: []FieldError{
				{Field: "reminderOccurrenceId", Message: "reminderOccurrenceId must be a valid reminder occurrence id"},
			},
		}
	}
	if err := validateResourceID("reminderId", token.ReminderID); err != nil {
		return domain.ReminderDismissal{}, err
	}
	if _, err := service.store.GetReminder(token.ReminderID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return domain.ReminderDismissal{}, &NotFoundError{Resource: "reminder", ID: token.ReminderID, Message: "reminder not found"}
		}
		return domain.ReminderDismissal{}, err
	}

	dismissedAt := service.now()
	alreadyDismissed, err := service.store.DismissReminderOccurrence(token.ReminderID, token.OccurrenceKey, dismissedAt)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return domain.ReminderDismissal{}, &NotFoundError{Resource: "reminder", ID: token.ReminderID, Message: "reminder not found"}
		}
		return domain.ReminderDismissal{}, err
	}

	return domain.ReminderDismissal{
		ReminderID:       token.ReminderID,
		OccurrenceKey:    token.OccurrenceKey,
		DismissedAt:      dismissedAt,
		AlreadyDismissed: alreadyDismissed,
	}, nil
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
	if event.TimeZone != nil {
		location, err := loadEventLocation(*event.TimeZone)
		switch {
		case err != nil:
			fieldErrors = append(fieldErrors, FieldError{Field: "timeZone", Message: "timeZone must be a valid IANA timezone"})
		case !timed || dated:
			fieldErrors = append(fieldErrors, FieldError{Field: "timeZone", Message: "timeZone is only supported for timed events"})
		default:
			if event.StartAt != nil && !timeOffsetMatchesLocation(*event.StartAt, location) {
				fieldErrors = append(fieldErrors, FieldError{Field: "startAt", Message: "startAt offset must match timeZone"})
			}
			if event.EndAt != nil && !timeOffsetMatchesLocation(*event.EndAt, location) {
				fieldErrors = append(fieldErrors, FieldError{Field: "endAt", Message: "endAt offset must match timeZone"})
			}
		}
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
	fieldErrors = append(fieldErrors, validateReminders(event.Reminders)...)
	fieldErrors = append(fieldErrors, validateEventAttendees(event.Attendees)...)

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
	if len(task.Reminders) > 0 && task.DueAt == nil && task.DueDate == nil {
		fieldErrors = append(fieldErrors, FieldError{Field: "reminders", Message: "task reminders require dueAt or dueDate"})
	}
	fieldErrors = append(fieldErrors, validateRecurrence(task.Recurrence, task.DueAt != nil)...)
	fieldErrors = append(fieldErrors, validateReminders(task.Reminders)...)

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

func validateReminders(reminders []domain.ReminderRule) []FieldError {
	fieldErrors := []FieldError{}
	seen := map[int32]bool{}
	for _, reminder := range reminders {
		switch {
		case reminder.BeforeMinutes <= 0:
			fieldErrors = append(fieldErrors, FieldError{Field: "reminders.beforeMinutes", Message: "beforeMinutes must be greater than 0"})
		case seen[reminder.BeforeMinutes]:
			fieldErrors = append(fieldErrors, FieldError{Field: "reminders", Message: "reminders cannot contain duplicate beforeMinutes values"})
		}
		seen[reminder.BeforeMinutes] = true
	}
	return fieldErrors
}

func validateEventAttendees(attendees []domain.EventAttendee) []FieldError {
	fieldErrors := []FieldError{}
	seen := map[string]bool{}
	for _, attendee := range attendees {
		email := strings.TrimSpace(attendee.Email)
		emailKey := strings.ToLower(email)
		switch {
		case !validAttendeeEmail(email):
			fieldErrors = append(fieldErrors, FieldError{Field: "attendees.email", Message: "email must be a valid email address"})
		case seen[emailKey]:
			fieldErrors = append(fieldErrors, FieldError{Field: "attendees", Message: "attendees cannot contain duplicate email values"})
		}
		seen[emailKey] = true
		if err := validateEventAttendeeRole(attendee.Role); err != nil {
			fieldErrors = append(fieldErrors, *err)
		}
		if err := validateEventParticipationStatus(attendee.ParticipationStatus); err != nil {
			fieldErrors = append(fieldErrors, *err)
		}
	}
	return fieldErrors
}

func validateEventAttendeeRole(role domain.EventAttendeeRole) *FieldError {
	switch role {
	case domain.EventAttendeeRoleRequired, domain.EventAttendeeRoleOptional, domain.EventAttendeeRoleChair, domain.EventAttendeeRoleNonParticipant:
		return nil
	default:
		return &FieldError{Field: "attendees.role", Message: "role must be required, optional, chair, or non_participant"}
	}
}

func validateEventParticipationStatus(status domain.EventParticipationStatus) *FieldError {
	switch status {
	case domain.EventParticipationStatusNeedsAction, domain.EventParticipationStatusAccepted, domain.EventParticipationStatusDeclined, domain.EventParticipationStatusTentative, domain.EventParticipationStatusDelegated:
		return nil
	default:
		return &FieldError{Field: "attendees.participationStatus", Message: "participationStatus must be needs_action, accepted, declined, tentative, or delegated"}
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

func buildReminderRules(input []domain.ReminderRule, now time.Time) []domain.ReminderRule {
	if len(input) == 0 {
		return []domain.ReminderRule{}
	}
	reminders := make([]domain.ReminderRule, 0, len(input))
	for _, reminder := range input {
		built := domain.ReminderRule{
			ID:            reminder.ID,
			BeforeMinutes: reminder.BeforeMinutes,
			CreatedAt:     reminder.CreatedAt,
			UpdatedAt:     reminder.UpdatedAt,
		}
		if built.ID == "" {
			built.ID = ulid.Make().String()
		}
		if built.CreatedAt.IsZero() {
			built.CreatedAt = now
		}
		if built.UpdatedAt.IsZero() {
			built.UpdatedAt = now
		}
		reminders = append(reminders, built)
	}
	return reminders
}

func buildEventAttendees(input []domain.EventAttendee, now time.Time) []domain.EventAttendee {
	if len(input) == 0 {
		return []domain.EventAttendee{}
	}
	attendees := make([]domain.EventAttendee, 0, len(input))
	for _, attendee := range input {
		built := domain.EventAttendee{
			Email:               strings.TrimSpace(attendee.Email),
			DisplayName:         sanitizeOptionalString(attendee.DisplayName),
			Role:                defaultEventAttendeeRole(attendee.Role),
			ParticipationStatus: defaultEventParticipationStatus(attendee.ParticipationStatus),
			RSVP:                attendee.RSVP,
			CreatedAt:           attendee.CreatedAt,
			UpdatedAt:           attendee.UpdatedAt,
		}
		if built.CreatedAt.IsZero() {
			built.CreatedAt = now
		}
		if built.UpdatedAt.IsZero() {
			built.UpdatedAt = now
		}
		attendees = append(attendees, built)
	}
	return attendees
}

func defaultEventAttendeeRole(role domain.EventAttendeeRole) domain.EventAttendeeRole {
	if role == "" {
		return domain.EventAttendeeRoleRequired
	}
	return role
}

func defaultEventParticipationStatus(status domain.EventParticipationStatus) domain.EventParticipationStatus {
	if status == "" {
		return domain.EventParticipationStatusNeedsAction
	}
	return status
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

	if rule.Interval <= 0 {
		fieldErrors = append(fieldErrors, FieldError{Field: "recurrence.interval", Message: "interval must be greater than 0"})
	}
	if rule.Count != nil && *rule.Count < 1 {
		fieldErrors = append(fieldErrors, FieldError{Field: "recurrence.count", Message: "count must be greater than 0"})
	}
	if rule.Count != nil && (rule.UntilAt != nil || rule.UntilDate != nil) {
		fieldErrors = append(fieldErrors, FieldError{Field: "recurrence", Message: "use count or until, not both"})
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
	for _, weekday := range rule.ByWeekday {
		if !validWeekday(weekday) {
			fieldErrors = append(fieldErrors, FieldError{Field: "recurrence.byWeekday", Message: "byWeekday values must be MO, TU, WE, TH, FR, SA, or SU"})
			break
		}
	}
	if hasDuplicateWeekdays(rule.ByWeekday) {
		fieldErrors = append(fieldErrors, FieldError{Field: "recurrence.byWeekday", Message: "byWeekday cannot contain duplicates"})
	}
	if len(rule.ByMonthDay) > 0 && rule.Frequency != domain.RecurrenceFrequencyMonthly {
		fieldErrors = append(fieldErrors, FieldError{Field: "recurrence.byMonthDay", Message: "byMonthDay is only supported for monthly recurrence"})
	}
	for _, monthDay := range rule.ByMonthDay {
		if monthDay < 1 || monthDay > 31 {
			fieldErrors = append(fieldErrors, FieldError{Field: "recurrence.byMonthDay", Message: "byMonthDay values must be between 1 and 31"})
			break
		}
	}
	if hasDuplicateMonthDays(rule.ByMonthDay) {
		fieldErrors = append(fieldErrors, FieldError{Field: "recurrence.byMonthDay", Message: "byMonthDay cannot contain duplicates"})
	}
	if timed && rule.UntilDate == nil && rule.UntilAt == nil && rule.Count == nil {
		return fieldErrors
	}

	return fieldErrors
}

func validWeekday(value domain.Weekday) bool {
	switch value {
	case domain.WeekdayMonday, domain.WeekdayTuesday, domain.WeekdayWednesday, domain.WeekdayThursday, domain.WeekdayFriday, domain.WeekdaySaturday, domain.WeekdaySunday:
		return true
	default:
		return false
	}
}

func hasDuplicateWeekdays(values []domain.Weekday) bool {
	seen := map[domain.Weekday]bool{}
	for _, value := range values {
		if seen[value] {
			return true
		}
		seen[value] = true
	}
	return false
}

func hasDuplicateMonthDays(values []int32) bool {
	seen := map[int32]bool{}
	for _, value := range values {
		if seen[value] {
			return true
		}
		seen[value] = true
	}
	return false
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

type reminderCursor struct {
	ID       string `json:"id"`
	RemindAt string `json:"remindAt"`
}

type reminderOccurrenceToken struct {
	ReminderID    string `json:"reminder_id"`
	OccurrenceKey string `json:"occurrence_key"`
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

func paginatePendingReminders(items []domain.PendingReminder, params domain.ReminderQueryParams) (domain.Page[domain.PendingReminder], error) {
	limit := normalizeLimit(params.Limit)
	start := 0
	if params.Cursor != "" {
		var cursor reminderCursor
		if err := decodeCursor(params.Cursor, &cursor); err != nil {
			return domain.Page[domain.PendingReminder]{}, invalidCursorError()
		}

		matched := false
		for index, item := range items {
			if item.ID == cursor.ID && item.RemindAt.Format(time.RFC3339Nano) == cursor.RemindAt {
				start = index + 1
				matched = true
				break
			}
		}
		if !matched {
			return domain.Page[domain.PendingReminder]{}, invalidCursorError()
		}
	}
	if len(items) == 0 {
		return domain.Page[domain.PendingReminder]{Items: []domain.PendingReminder{}}, nil
	}

	end := start + limit
	if end > len(items) {
		end = len(items)
	}

	page := domain.Page[domain.PendingReminder]{Items: slices.Clone(items[start:end])}
	if end < len(items) {
		last := items[end-1]
		nextCursor := encodeCursor(reminderCursor{
			ID:       last.ID,
			RemindAt: last.RemindAt.Format(time.RFC3339Nano),
		})
		page.NextCursor = &nextCursor
	}

	return page, nil
}

func agendaItemsForEvent(event domain.Event, states map[string]domain.OccurrenceState, from, to time.Time) []domain.AgendaItem {
	items := []domain.AgendaItem{}

	switch {
	case event.StartAt != nil:
		location := eventLocation(event)
		startAt := timeInEventLocation(*event.StartAt, location)
		duration := time.Duration(0)
		if event.EndAt != nil {
			duration = event.EndAt.Sub(*event.StartAt)
		}

		var occurrences []time.Time
		if event.Recurrence != nil {
			occurrences = expandTimedEventOccurrences(event, from.Add(-duration), to, location)
		} else {
			occurrences = []time.Time{startAt}
		}

		for _, occurrence := range occurrences {
			key := occurrenceKey(event.ID, &occurrence, nil)
			if suppressBaseOccurrence(states, key) {
				continue
			}
			endAt := occurrence
			if event.EndAt != nil {
				endAt = occurrence.Add(duration)
			}
			if !occursInTimedRange(occurrence, endAt, from, to) {
				continue
			}

			item := domain.AgendaItem{
				Kind:          domain.AgendaItemKindEvent,
				OccurrenceKey: key,
				CalendarID:    event.CalendarID,
				SourceID:      event.ID,
				Title:         event.Title,
				Description:   cloneStringPtr(event.Description),
				StartAt:       cloneTimePtr(&occurrence),
				EndAt:         cloneTimePtr(&endAt),
				TimeZone:      cloneStringPtr(event.TimeZone),
				Attendees:     cloneEventAttendees(event.Attendees),
				LinkedTaskIDs: slices.Clone(event.LinkedTaskIDs),
			}
			if event.EndAt == nil {
				item.EndAt = nil
			}
			items = append(items, item)
		}
		items = append(items, rescheduledTimedEventAgendaItems(event, states, duration, from, to)...)
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
			key := occurrenceKey(event.ID, nil, &occurrence)
			if suppressBaseOccurrence(states, key) {
				continue
			}
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
				OccurrenceKey: key,
				CalendarID:    event.CalendarID,
				SourceID:      event.ID,
				Title:         event.Title,
				Description:   cloneStringPtr(event.Description),
				StartDate:     &startDate,
				EndDate:       &endDate,
				Attendees:     cloneEventAttendees(event.Attendees),
				LinkedTaskIDs: slices.Clone(event.LinkedTaskIDs),
			}
			if spanDays == 1 {
				item.EndDate = nil
			}
			items = append(items, item)
		}
		items = append(items, rescheduledDateEventAgendaItems(event, states, spanDays, from, to)...)
	}

	return items
}

func agendaItemsForTask(task domain.Task, completions map[string]domain.TaskCompletion, states map[string]domain.OccurrenceState, from, to time.Time) []domain.AgendaItem {
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
			key := occurrenceKey(task.ID, &occurrence, nil)
			if suppressBaseOccurrence(states, key) {
				continue
			}
			if occurrence.Before(from) || !occurrence.Before(to) {
				continue
			}

			item := domain.AgendaItem{
				Kind:           domain.AgendaItemKindTask,
				OccurrenceKey:  key,
				CalendarID:     task.CalendarID,
				SourceID:       task.ID,
				Title:          task.Title,
				Description:    cloneStringPtr(task.Description),
				DueAt:          cloneTimePtr(&occurrence),
				Priority:       task.Priority,
				Status:         task.Status,
				Tags:           slices.Clone(task.Tags),
				LinkedEventIDs: slices.Clone(task.LinkedEventIDs),
			}
			if task.Recurrence == nil {
				item.CompletedAt = cloneTimePtr(task.CompletedAt)
			} else if completion, ok := completions[key]; ok {
				item.CompletedAt = cloneTimePtr(&completion.CompletedAt)
			}
			items = append(items, item)
		}
		items = append(items, rescheduledTimedTaskAgendaItems(task, completions, states, from, to)...)
	case task.DueDate != nil:
		var occurrences []string
		if task.Recurrence != nil {
			occurrences = recurrence.ExpandDate(*task.DueDate, task.Recurrence, from, to)
		} else {
			occurrences = []string{*task.DueDate}
		}

		for _, occurrence := range occurrences {
			key := occurrenceKey(task.ID, nil, &occurrence)
			if suppressBaseOccurrence(states, key) {
				continue
			}
			if !occursInDateRange(occurrence, occurrence, from, to) {
				continue
			}

			item := domain.AgendaItem{
				Kind:           domain.AgendaItemKindTask,
				OccurrenceKey:  key,
				CalendarID:     task.CalendarID,
				SourceID:       task.ID,
				Title:          task.Title,
				Description:    cloneStringPtr(task.Description),
				DueDate:        &occurrence,
				Priority:       task.Priority,
				Status:         task.Status,
				Tags:           slices.Clone(task.Tags),
				LinkedEventIDs: slices.Clone(task.LinkedEventIDs),
			}
			if task.Recurrence == nil {
				item.CompletedAt = cloneTimePtr(task.CompletedAt)
			} else if completion, ok := completions[key]; ok {
				item.CompletedAt = cloneTimePtr(&completion.CompletedAt)
			}
			items = append(items, item)
		}
		items = append(items, rescheduledDateTaskAgendaItems(task, completions, states, from, to)...)
	}

	return items
}

func suppressBaseOccurrence(states map[string]domain.OccurrenceState, key string) bool {
	state, ok := states[key]
	return ok && (state.Cancelled || occurrenceStateHasReplacement(state))
}

func occurrenceStateHasReplacement(state domain.OccurrenceState) bool {
	return state.ReplacementAt != nil || state.ReplacementDate != nil
}

func eventOccurrenceStateMatchesSchedule(event domain.Event, state domain.OccurrenceState) bool {
	if event.Recurrence == nil {
		return false
	}

	switch {
	case event.StartAt != nil:
		return state.OccurrenceAt != nil &&
			state.OccurrenceDate == nil &&
			eventIncludesTimedOccurrence(event, *state.OccurrenceAt)
	case event.StartDate != nil:
		return state.OccurrenceDate != nil &&
			state.OccurrenceAt == nil &&
			recurrence.IncludesDate(*event.StartDate, event.Recurrence, *state.OccurrenceDate)
	default:
		return false
	}
}

func eventIncludesTimedOccurrence(event domain.Event, target time.Time) bool {
	if event.StartAt == nil {
		return false
	}
	if location := eventLocation(event); location != nil {
		return recurrence.IncludesTimedInLocation(*event.StartAt, event.Recurrence, target, location)
	}
	return recurrence.IncludesTimed(*event.StartAt, event.Recurrence, target)
}

func expandTimedEventOccurrences(event domain.Event, from, to time.Time, location *time.Location) []time.Time {
	if event.StartAt == nil {
		return nil
	}
	if location != nil {
		return recurrence.ExpandTimedInLocation(*event.StartAt, event.Recurrence, from, to, location)
	}
	return recurrence.ExpandTimed(*event.StartAt, event.Recurrence, from, to)
}

func timeInEventLocation(value time.Time, location *time.Location) time.Time {
	if location == nil {
		return value
	}
	return value.In(location)
}

func taskOccurrenceStateMatchesSchedule(task domain.Task, state domain.OccurrenceState) bool {
	if task.Recurrence == nil {
		return false
	}

	switch {
	case task.DueAt != nil:
		return state.OccurrenceAt != nil &&
			state.OccurrenceDate == nil &&
			recurrence.IncludesTimed(*task.DueAt, task.Recurrence, *state.OccurrenceAt)
	case task.DueDate != nil:
		return state.OccurrenceDate != nil &&
			state.OccurrenceAt == nil &&
			recurrence.IncludesDate(*task.DueDate, task.Recurrence, *state.OccurrenceDate)
	default:
		return false
	}
}

func rescheduledTimedEventAgendaItems(event domain.Event, states map[string]domain.OccurrenceState, duration time.Duration, from, to time.Time) []domain.AgendaItem {
	items := []domain.AgendaItem{}
	location := eventLocation(event)
	for _, state := range states {
		if state.Cancelled || state.ReplacementAt == nil || !eventOccurrenceStateMatchesSchedule(event, state) {
			continue
		}
		startAt := timeInEventLocation(*state.ReplacementAt, location)
		endAt := startAt
		if state.ReplacementEndAt != nil {
			endAt = timeInEventLocation(*state.ReplacementEndAt, location)
		} else if event.EndAt != nil {
			endAt = startAt.Add(duration)
		}
		if !occursInTimedRange(startAt, endAt, from, to) {
			continue
		}
		item := domain.AgendaItem{
			Kind:          domain.AgendaItemKindEvent,
			OccurrenceKey: state.OccurrenceKey,
			CalendarID:    event.CalendarID,
			SourceID:      event.ID,
			Title:         event.Title,
			Description:   cloneStringPtr(event.Description),
			StartAt:       cloneTimePtr(&startAt),
			EndAt:         cloneTimePtr(&endAt),
			TimeZone:      cloneStringPtr(event.TimeZone),
			Attendees:     cloneEventAttendees(event.Attendees),
			LinkedTaskIDs: slices.Clone(event.LinkedTaskIDs),
		}
		if event.EndAt == nil && state.ReplacementEndAt == nil {
			item.EndAt = nil
		}
		items = append(items, item)
	}
	return items
}

func rescheduledDateEventAgendaItems(event domain.Event, states map[string]domain.OccurrenceState, spanDays int, from, to time.Time) []domain.AgendaItem {
	items := []domain.AgendaItem{}
	for _, state := range states {
		if state.Cancelled || state.ReplacementDate == nil || !eventOccurrenceStateMatchesSchedule(event, state) {
			continue
		}
		startDate := *state.ReplacementDate
		endDate := startDate
		if state.ReplacementEndDate != nil {
			endDate = *state.ReplacementEndDate
		} else if spanDays > 1 {
			endDate = dateAddDays(startDate, spanDays-1)
		}
		if !occursInDateRange(startDate, endDate, from, to) {
			continue
		}
		item := domain.AgendaItem{
			Kind:          domain.AgendaItemKindEvent,
			OccurrenceKey: state.OccurrenceKey,
			CalendarID:    event.CalendarID,
			SourceID:      event.ID,
			Title:         event.Title,
			Description:   cloneStringPtr(event.Description),
			StartDate:     &startDate,
			EndDate:       &endDate,
			Attendees:     cloneEventAttendees(event.Attendees),
			LinkedTaskIDs: slices.Clone(event.LinkedTaskIDs),
		}
		if spanDays == 1 && state.ReplacementEndDate == nil {
			item.EndDate = nil
		}
		items = append(items, item)
	}
	return items
}

func rescheduledTimedTaskAgendaItems(task domain.Task, completions map[string]domain.TaskCompletion, states map[string]domain.OccurrenceState, from, to time.Time) []domain.AgendaItem {
	items := []domain.AgendaItem{}
	for _, state := range states {
		if state.Cancelled || state.ReplacementAt == nil || !taskOccurrenceStateMatchesSchedule(task, state) {
			continue
		}
		dueAt := *state.ReplacementAt
		if dueAt.Before(from) || !dueAt.Before(to) {
			continue
		}
		item := domain.AgendaItem{
			Kind:           domain.AgendaItemKindTask,
			OccurrenceKey:  state.OccurrenceKey,
			CalendarID:     task.CalendarID,
			SourceID:       task.ID,
			Title:          task.Title,
			Description:    cloneStringPtr(task.Description),
			DueAt:          cloneTimePtr(&dueAt),
			Priority:       task.Priority,
			Status:         task.Status,
			Tags:           slices.Clone(task.Tags),
			LinkedEventIDs: slices.Clone(task.LinkedEventIDs),
		}
		if completion, ok := completions[state.OccurrenceKey]; ok {
			item.CompletedAt = cloneTimePtr(&completion.CompletedAt)
		}
		items = append(items, item)
	}
	return items
}

func rescheduledDateTaskAgendaItems(task domain.Task, completions map[string]domain.TaskCompletion, states map[string]domain.OccurrenceState, from, to time.Time) []domain.AgendaItem {
	items := []domain.AgendaItem{}
	for _, state := range states {
		if state.Cancelled || state.ReplacementDate == nil || !taskOccurrenceStateMatchesSchedule(task, state) {
			continue
		}
		dueDate := *state.ReplacementDate
		if !occursInDateRange(dueDate, dueDate, from, to) {
			continue
		}
		item := domain.AgendaItem{
			Kind:           domain.AgendaItemKindTask,
			OccurrenceKey:  state.OccurrenceKey,
			CalendarID:     task.CalendarID,
			SourceID:       task.ID,
			Title:          task.Title,
			Description:    cloneStringPtr(task.Description),
			DueDate:        &dueDate,
			Priority:       task.Priority,
			Status:         task.Status,
			Tags:           slices.Clone(task.Tags),
			LinkedEventIDs: slices.Clone(task.LinkedEventIDs),
		}
		if completion, ok := completions[state.OccurrenceKey]; ok {
			item.CompletedAt = cloneTimePtr(&completion.CompletedAt)
		}
		items = append(items, item)
	}
	return items
}

func pendingRemindersForEvent(event domain.Event, states map[string]domain.OccurrenceState, dismissals map[string]map[string]time.Time, from, to time.Time) []domain.PendingReminder {
	items := []domain.PendingReminder{}
	if len(event.Reminders) == 0 {
		return items
	}

	switch {
	case event.StartAt != nil:
		location := eventLocation(event)
		duration := time.Duration(0)
		if event.EndAt != nil {
			duration = event.EndAt.Sub(*event.StartAt)
		}
		for _, reminder := range event.Reminders {
			offset := time.Duration(reminder.BeforeMinutes) * time.Minute
			var occurrences []time.Time
			if event.Recurrence != nil {
				occurrences = expandTimedEventOccurrences(event, from.Add(offset), to.Add(offset), location)
			} else {
				occurrences = []time.Time{timeInEventLocation(*event.StartAt, location)}
			}
			for _, occurrence := range occurrences {
				key := occurrenceKey(event.ID, &occurrence, nil)
				if suppressBaseOccurrence(states, key) {
					continue
				}
				remindAt := occurrence.Add(-offset)
				if remindAt.Before(from) || !remindAt.Before(to) {
					continue
				}
				if reminderDismissed(dismissals, reminder.ID, key) {
					continue
				}
				endAt := occurrence
				if event.EndAt != nil {
					endAt = occurrence.Add(duration)
				}
				item := domain.PendingReminder{
					ID:            reminderOccurrenceID(reminder.ID, key),
					ReminderID:    reminder.ID,
					OwnerKind:     domain.ReminderOwnerKindEvent,
					OwnerID:       event.ID,
					CalendarID:    event.CalendarID,
					Title:         event.Title,
					OccurrenceKey: key,
					RemindAt:      remindAt,
					BeforeMinutes: reminder.BeforeMinutes,
					StartAt:       cloneTimePtr(&occurrence),
				}
				if event.EndAt != nil {
					item.EndAt = cloneTimePtr(&endAt)
				}
				items = append(items, item)
			}
			items = append(items, rescheduledTimedEventPendingReminders(event, states, dismissals, reminder, offset, duration, from, to)...)
		}
	case event.StartDate != nil:
		spanDays := 1
		if event.EndDate != nil {
			spanDays = inclusiveDateSpan(*event.StartDate, *event.EndDate)
		}
		for _, reminder := range event.Reminders {
			offset := time.Duration(reminder.BeforeMinutes) * time.Minute
			var occurrences []string
			if event.Recurrence != nil {
				occurrences = recurrence.ExpandDate(*event.StartDate, event.Recurrence, from.Add(offset), to.Add(offset))
			} else {
				occurrences = []string{*event.StartDate}
			}
			for _, occurrence := range occurrences {
				key := occurrenceKey(event.ID, nil, &occurrence)
				if suppressBaseOccurrence(states, key) {
					continue
				}
				remindAt := mustParseDate(occurrence).Add(-offset)
				if remindAt.Before(from) || !remindAt.Before(to) {
					continue
				}
				if reminderDismissed(dismissals, reminder.ID, key) {
					continue
				}
				endDate := occurrence
				if spanDays > 1 {
					endDate = dateAddDays(occurrence, spanDays-1)
				}
				item := domain.PendingReminder{
					ID:            reminderOccurrenceID(reminder.ID, key),
					ReminderID:    reminder.ID,
					OwnerKind:     domain.ReminderOwnerKindEvent,
					OwnerID:       event.ID,
					CalendarID:    event.CalendarID,
					Title:         event.Title,
					OccurrenceKey: key,
					RemindAt:      remindAt,
					BeforeMinutes: reminder.BeforeMinutes,
					StartDate:     &occurrence,
				}
				if spanDays > 1 {
					item.EndDate = &endDate
				}
				items = append(items, item)
			}
			items = append(items, rescheduledDateEventPendingReminders(event, states, dismissals, reminder, offset, spanDays, from, to)...)
		}
	}

	return items
}

func pendingRemindersForTask(task domain.Task, states map[string]domain.OccurrenceState, dismissals map[string]map[string]time.Time, from, to time.Time) []domain.PendingReminder {
	items := []domain.PendingReminder{}
	if len(task.Reminders) == 0 {
		return items
	}

	switch {
	case task.DueAt != nil:
		for _, reminder := range task.Reminders {
			offset := time.Duration(reminder.BeforeMinutes) * time.Minute
			var occurrences []time.Time
			if task.Recurrence != nil {
				occurrences = recurrence.ExpandTimed(*task.DueAt, task.Recurrence, from.Add(offset), to.Add(offset))
			} else {
				occurrences = []time.Time{*task.DueAt}
			}
			for _, occurrence := range occurrences {
				key := occurrenceKey(task.ID, &occurrence, nil)
				if suppressBaseOccurrence(states, key) {
					continue
				}
				remindAt := occurrence.Add(-offset)
				if remindAt.Before(from) || !remindAt.Before(to) {
					continue
				}
				if reminderDismissed(dismissals, reminder.ID, key) {
					continue
				}
				items = append(items, domain.PendingReminder{
					ID:            reminderOccurrenceID(reminder.ID, key),
					ReminderID:    reminder.ID,
					OwnerKind:     domain.ReminderOwnerKindTask,
					OwnerID:       task.ID,
					CalendarID:    task.CalendarID,
					Title:         task.Title,
					OccurrenceKey: key,
					RemindAt:      remindAt,
					BeforeMinutes: reminder.BeforeMinutes,
					DueAt:         cloneTimePtr(&occurrence),
				})
			}
			items = append(items, rescheduledTimedTaskPendingReminders(task, states, dismissals, reminder, offset, from, to)...)
		}
	case task.DueDate != nil:
		for _, reminder := range task.Reminders {
			offset := time.Duration(reminder.BeforeMinutes) * time.Minute
			var occurrences []string
			if task.Recurrence != nil {
				occurrences = recurrence.ExpandDate(*task.DueDate, task.Recurrence, from.Add(offset), to.Add(offset))
			} else {
				occurrences = []string{*task.DueDate}
			}
			for _, occurrence := range occurrences {
				key := occurrenceKey(task.ID, nil, &occurrence)
				if suppressBaseOccurrence(states, key) {
					continue
				}
				remindAt := mustParseDate(occurrence).Add(-offset)
				if remindAt.Before(from) || !remindAt.Before(to) {
					continue
				}
				if reminderDismissed(dismissals, reminder.ID, key) {
					continue
				}
				items = append(items, domain.PendingReminder{
					ID:            reminderOccurrenceID(reminder.ID, key),
					ReminderID:    reminder.ID,
					OwnerKind:     domain.ReminderOwnerKindTask,
					OwnerID:       task.ID,
					CalendarID:    task.CalendarID,
					Title:         task.Title,
					OccurrenceKey: key,
					RemindAt:      remindAt,
					BeforeMinutes: reminder.BeforeMinutes,
					DueDate:       &occurrence,
				})
			}
			items = append(items, rescheduledDateTaskPendingReminders(task, states, dismissals, reminder, offset, from, to)...)
		}
	}

	return items
}

func rescheduledTimedEventPendingReminders(event domain.Event, states map[string]domain.OccurrenceState, dismissals map[string]map[string]time.Time, reminder domain.ReminderRule, offset time.Duration, duration time.Duration, from, to time.Time) []domain.PendingReminder {
	items := []domain.PendingReminder{}
	location := eventLocation(event)
	for _, state := range states {
		if state.Cancelled || state.ReplacementAt == nil || !eventOccurrenceStateMatchesSchedule(event, state) {
			continue
		}
		occurrence := timeInEventLocation(*state.ReplacementAt, location)
		remindAt := occurrence.Add(-offset)
		if remindAt.Before(from) || !remindAt.Before(to) || reminderDismissed(dismissals, reminder.ID, state.OccurrenceKey) {
			continue
		}
		endAt := occurrence
		if state.ReplacementEndAt != nil {
			endAt = timeInEventLocation(*state.ReplacementEndAt, location)
		} else if event.EndAt != nil {
			endAt = occurrence.Add(duration)
		}
		item := domain.PendingReminder{
			ID:            reminderOccurrenceID(reminder.ID, state.OccurrenceKey),
			ReminderID:    reminder.ID,
			OwnerKind:     domain.ReminderOwnerKindEvent,
			OwnerID:       event.ID,
			CalendarID:    event.CalendarID,
			Title:         event.Title,
			OccurrenceKey: state.OccurrenceKey,
			RemindAt:      remindAt,
			BeforeMinutes: reminder.BeforeMinutes,
			StartAt:       cloneTimePtr(&occurrence),
		}
		if event.EndAt != nil || state.ReplacementEndAt != nil {
			item.EndAt = cloneTimePtr(&endAt)
		}
		items = append(items, item)
	}
	return items
}

func rescheduledDateEventPendingReminders(event domain.Event, states map[string]domain.OccurrenceState, dismissals map[string]map[string]time.Time, reminder domain.ReminderRule, offset time.Duration, spanDays int, from, to time.Time) []domain.PendingReminder {
	items := []domain.PendingReminder{}
	for _, state := range states {
		if state.Cancelled || state.ReplacementDate == nil || !eventOccurrenceStateMatchesSchedule(event, state) {
			continue
		}
		occurrence := *state.ReplacementDate
		remindAt := mustParseDate(occurrence).Add(-offset)
		if remindAt.Before(from) || !remindAt.Before(to) || reminderDismissed(dismissals, reminder.ID, state.OccurrenceKey) {
			continue
		}
		endDate := occurrence
		if state.ReplacementEndDate != nil {
			endDate = *state.ReplacementEndDate
		} else if spanDays > 1 {
			endDate = dateAddDays(occurrence, spanDays-1)
		}
		item := domain.PendingReminder{
			ID:            reminderOccurrenceID(reminder.ID, state.OccurrenceKey),
			ReminderID:    reminder.ID,
			OwnerKind:     domain.ReminderOwnerKindEvent,
			OwnerID:       event.ID,
			CalendarID:    event.CalendarID,
			Title:         event.Title,
			OccurrenceKey: state.OccurrenceKey,
			RemindAt:      remindAt,
			BeforeMinutes: reminder.BeforeMinutes,
			StartDate:     &occurrence,
		}
		if spanDays > 1 || state.ReplacementEndDate != nil {
			item.EndDate = &endDate
		}
		items = append(items, item)
	}
	return items
}

func rescheduledTimedTaskPendingReminders(task domain.Task, states map[string]domain.OccurrenceState, dismissals map[string]map[string]time.Time, reminder domain.ReminderRule, offset time.Duration, from, to time.Time) []domain.PendingReminder {
	items := []domain.PendingReminder{}
	for _, state := range states {
		if state.Cancelled || state.ReplacementAt == nil || !taskOccurrenceStateMatchesSchedule(task, state) {
			continue
		}
		occurrence := *state.ReplacementAt
		remindAt := occurrence.Add(-offset)
		if remindAt.Before(from) || !remindAt.Before(to) || reminderDismissed(dismissals, reminder.ID, state.OccurrenceKey) {
			continue
		}
		items = append(items, domain.PendingReminder{
			ID:            reminderOccurrenceID(reminder.ID, state.OccurrenceKey),
			ReminderID:    reminder.ID,
			OwnerKind:     domain.ReminderOwnerKindTask,
			OwnerID:       task.ID,
			CalendarID:    task.CalendarID,
			Title:         task.Title,
			OccurrenceKey: state.OccurrenceKey,
			RemindAt:      remindAt,
			BeforeMinutes: reminder.BeforeMinutes,
			DueAt:         cloneTimePtr(&occurrence),
		})
	}
	return items
}

func rescheduledDateTaskPendingReminders(task domain.Task, states map[string]domain.OccurrenceState, dismissals map[string]map[string]time.Time, reminder domain.ReminderRule, offset time.Duration, from, to time.Time) []domain.PendingReminder {
	items := []domain.PendingReminder{}
	for _, state := range states {
		if state.Cancelled || state.ReplacementDate == nil || !taskOccurrenceStateMatchesSchedule(task, state) {
			continue
		}
		occurrence := *state.ReplacementDate
		remindAt := mustParseDate(occurrence).Add(-offset)
		if remindAt.Before(from) || !remindAt.Before(to) || reminderDismissed(dismissals, reminder.ID, state.OccurrenceKey) {
			continue
		}
		items = append(items, domain.PendingReminder{
			ID:            reminderOccurrenceID(reminder.ID, state.OccurrenceKey),
			ReminderID:    reminder.ID,
			OwnerKind:     domain.ReminderOwnerKindTask,
			OwnerID:       task.ID,
			CalendarID:    task.CalendarID,
			Title:         task.Title,
			OccurrenceKey: state.OccurrenceKey,
			RemindAt:      remindAt,
			BeforeMinutes: reminder.BeforeMinutes,
			DueDate:       &occurrence,
		})
	}
	return items
}

func reminderIDsFor(events []domain.Event, tasks []domain.Task) []string {
	seen := map[string]bool{}
	ids := []string{}
	for _, event := range events {
		for _, reminder := range event.Reminders {
			if !seen[reminder.ID] {
				seen[reminder.ID] = true
				ids = append(ids, reminder.ID)
			}
		}
	}
	for _, task := range tasks {
		for _, reminder := range task.Reminders {
			if !seen[reminder.ID] {
				seen[reminder.ID] = true
				ids = append(ids, reminder.ID)
			}
		}
	}
	return ids
}

func reminderDismissed(dismissals map[string]map[string]time.Time, reminderID string, occurrenceKey string) bool {
	occurrences, ok := dismissals[reminderID]
	if !ok {
		return false
	}
	_, ok = occurrences[occurrenceKey]
	return ok
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

func comparePendingReminders(left, right domain.PendingReminder) int {
	if cmp := left.RemindAt.Compare(right.RemindAt); cmp != 0 {
		return cmp
	}
	if cmp := strings.Compare(string(left.OwnerKind), string(right.OwnerKind)); cmp != 0 {
		return cmp
	}
	if cmp := strings.Compare(left.OwnerID, right.OwnerID); cmp != 0 {
		return cmp
	}
	if cmp := strings.Compare(left.OccurrenceKey, right.OccurrenceKey); cmp != 0 {
		return cmp
	}
	return strings.Compare(left.ReminderID, right.ReminderID)
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

func parseOccurrenceKey(id string, key string) (*time.Time, *string, error) {
	prefix := id + "@"
	if !strings.HasPrefix(key, prefix) {
		return nil, nil, fmt.Errorf("occurrence key owner mismatch")
	}
	value := strings.TrimPrefix(key, prefix)
	if value == "" || value == "single" {
		return nil, nil, fmt.Errorf("occurrence key has no recurring selector")
	}
	if at, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return &at, nil, nil
	}
	if _, err := time.Parse(time.DateOnly, value); err == nil {
		return nil, &value, nil
	}
	return nil, nil, fmt.Errorf("occurrence key selector is invalid")
}

func reminderOccurrenceID(reminderID string, occurrenceKey string) string {
	return encodeCursor(reminderOccurrenceToken{
		ReminderID:    reminderID,
		OccurrenceKey: occurrenceKey,
	})
}

func decodeReminderOccurrenceID(value string) (reminderOccurrenceToken, error) {
	var token reminderOccurrenceToken
	if value == "" {
		return token, invalidCursorError()
	}
	if err := decodeCursor(value, &token); err != nil {
		return reminderOccurrenceToken{}, err
	}
	return token, nil
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

func stringPtrEqual(left *string, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
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

func cloneEventAttendees(values []domain.EventAttendee) []domain.EventAttendee {
	if len(values) == 0 {
		return nil
	}
	out := make([]domain.EventAttendee, 0, len(values))
	for _, value := range values {
		attendee := value
		attendee.DisplayName = cloneStringPtr(value.DisplayName)
		out = append(out, attendee)
	}
	return out
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

func recurringEventIDs(events []domain.Event) []string {
	ids := make([]string, 0, len(events))
	for _, event := range events {
		if event.Recurrence != nil {
			ids = append(ids, event.ID)
		}
	}
	return ids
}

func recurringTaskIDs(tasks []domain.Task) []string {
	ids := make([]string, 0, len(tasks))
	for _, task := range tasks {
		if task.Recurrence != nil {
			ids = append(ids, task.ID)
		}
	}
	return ids
}

func calendarExportName(calendarName string) string {
	if strings.TrimSpace(calendarName) != "" {
		return calendarName
	}
	return "OpenPlanner"
}

func sanitizeICalendarFilename(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	var builder strings.Builder
	lastDash := false
	for _, value := range name {
		switch {
		case value >= 'a' && value <= 'z', value >= '0' && value <= '9':
			builder.WriteRune(value)
			lastDash = false
		case value == '_' || value == '.':
			builder.WriteRune(value)
			lastDash = false
		default:
			if !lastDash {
				builder.WriteByte('-')
				lastDash = true
			}
		}
	}
	base := strings.Trim(builder.String(), "-.")
	if base == "" {
		base = "openplanner"
	}
	return base + ".ics"
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
