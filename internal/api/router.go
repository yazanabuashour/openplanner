package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/yazanabuashour/openplanner/internal/domain"
	"github.com/yazanabuashour/openplanner/internal/service"
	"github.com/yazanabuashour/openplanner/sdk/generated"
)

type Handler struct {
	service *service.Service
}

func NewHandler(svc *service.Service) http.Handler {
	handler := &Handler{service: svc}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /v1/calendars", handler.listCalendars)
	mux.HandleFunc("POST /v1/calendars", handler.createCalendar)
	mux.HandleFunc("GET /v1/calendars/{calendarId}", handler.getCalendar)
	mux.HandleFunc("PATCH /v1/calendars/{calendarId}", handler.updateCalendar)
	mux.HandleFunc("DELETE /v1/calendars/{calendarId}", handler.deleteCalendar)

	mux.HandleFunc("GET /v1/events", handler.listEvents)
	mux.HandleFunc("POST /v1/events", handler.createEvent)
	mux.HandleFunc("GET /v1/events/{eventId}", handler.getEvent)
	mux.HandleFunc("PATCH /v1/events/{eventId}", handler.updateEvent)
	mux.HandleFunc("DELETE /v1/events/{eventId}", handler.deleteEvent)

	mux.HandleFunc("GET /v1/tasks", handler.listTasks)
	mux.HandleFunc("POST /v1/tasks", handler.createTask)
	mux.HandleFunc("GET /v1/tasks/{taskId}", handler.getTask)
	mux.HandleFunc("PATCH /v1/tasks/{taskId}", handler.updateTask)
	mux.HandleFunc("DELETE /v1/tasks/{taskId}", handler.deleteTask)
	mux.HandleFunc("POST /v1/tasks/{taskId}/complete", handler.completeTask)

	mux.HandleFunc("GET /v1/agenda", handler.listAgenda)

	return mux
}

func (handler *Handler) listCalendars(response http.ResponseWriter, request *http.Request) {
	limit, err := parseLimit(request.URL.Query().Get("limit"))
	if err != nil {
		writeProblem(response, request, err)
		return
	}

	page, err := handler.service.ListCalendars(domain.PageParams{
		Cursor: request.URL.Query().Get("cursor"),
		Limit:  limit,
	})
	if err != nil {
		writeProblem(response, request, err)
		return
	}

	payload := generated.CalendarListResponse{
		Items: toGeneratedCalendars(page.Items),
	}
	if page.NextCursor != nil {
		payload.NextCursor = page.NextCursor
	}
	writeJSON(response, http.StatusOK, payload)
}

func (handler *Handler) createCalendar(response http.ResponseWriter, request *http.Request) {
	var body generated.CreateCalendarRequest
	if !decodeJSON(response, request, &body) {
		return
	}

	calendar, err := handler.service.CreateCalendar(domain.Calendar{
		Name:        body.Name,
		Description: body.Description,
		Color:       body.Color,
	})
	if err != nil {
		writeProblem(response, request, err)
		return
	}

	writeJSON(response, http.StatusCreated, toGeneratedCalendar(calendar))
}

func (handler *Handler) getCalendar(response http.ResponseWriter, request *http.Request) {
	calendar, err := handler.service.GetCalendar(request.PathValue("calendarId"))
	if err != nil {
		writeProblem(response, request, err)
		return
	}

	writeJSON(response, http.StatusOK, toGeneratedCalendar(calendar))
}

func (handler *Handler) updateCalendar(response http.ResponseWriter, request *http.Request) {
	var body generated.UpdateCalendarRequest
	if !decodeJSON(response, request, &body) {
		return
	}

	calendar, err := handler.service.UpdateCalendar(request.PathValue("calendarId"), domain.CalendarPatch{
		Name:        body.Name,
		Description: body.Description,
		Color:       body.Color,
	})
	if err != nil {
		writeProblem(response, request, err)
		return
	}

	writeJSON(response, http.StatusOK, toGeneratedCalendar(calendar))
}

func (handler *Handler) deleteCalendar(response http.ResponseWriter, request *http.Request) {
	if err := handler.service.DeleteCalendar(request.PathValue("calendarId")); err != nil {
		writeProblem(response, request, err)
		return
	}

	response.WriteHeader(http.StatusNoContent)
}

func (handler *Handler) listEvents(response http.ResponseWriter, request *http.Request) {
	limit, err := parseLimit(request.URL.Query().Get("limit"))
	if err != nil {
		writeProblem(response, request, err)
		return
	}

	page, err := handler.service.ListEvents(domain.PageParams{
		Cursor:     request.URL.Query().Get("cursor"),
		Limit:      limit,
		CalendarID: request.URL.Query().Get("calendarId"),
	})
	if err != nil {
		writeProblem(response, request, err)
		return
	}

	payload := generated.EventListResponse{
		Items: toGeneratedEvents(page.Items),
	}
	if page.NextCursor != nil {
		payload.NextCursor = page.NextCursor
	}
	writeJSON(response, http.StatusOK, payload)
}

func (handler *Handler) createEvent(response http.ResponseWriter, request *http.Request) {
	var body generated.CreateEventRequest
	if !decodeJSON(response, request, &body) {
		return
	}

	event, err := handler.service.CreateEvent(domain.Event{
		CalendarID:  body.CalendarId,
		Title:       body.Title,
		Description: body.Description,
		Location:    body.Location,
		StartAt:     body.StartAt,
		EndAt:       body.EndAt,
		StartDate:   body.StartDate,
		EndDate:     body.EndDate,
		Recurrence:  toDomainRule(body.Recurrence),
	})
	if err != nil {
		writeProblem(response, request, err)
		return
	}

	writeJSON(response, http.StatusCreated, toGeneratedEvent(event))
}

func (handler *Handler) getEvent(response http.ResponseWriter, request *http.Request) {
	event, err := handler.service.GetEvent(request.PathValue("eventId"))
	if err != nil {
		writeProblem(response, request, err)
		return
	}

	writeJSON(response, http.StatusOK, toGeneratedEvent(event))
}

func (handler *Handler) updateEvent(response http.ResponseWriter, request *http.Request) {
	var body generated.UpdateEventRequest
	if !decodeJSON(response, request, &body) {
		return
	}

	event, err := handler.service.UpdateEvent(request.PathValue("eventId"), domain.EventPatch{
		Title:       body.Title,
		Description: body.Description,
		Location:    body.Location,
		StartAt:     body.StartAt,
		EndAt:       body.EndAt,
		StartDate:   body.StartDate,
		EndDate:     body.EndDate,
		Recurrence:  toDomainRule(body.Recurrence),
	})
	if err != nil {
		writeProblem(response, request, err)
		return
	}

	writeJSON(response, http.StatusOK, toGeneratedEvent(event))
}

func (handler *Handler) deleteEvent(response http.ResponseWriter, request *http.Request) {
	if err := handler.service.DeleteEvent(request.PathValue("eventId")); err != nil {
		writeProblem(response, request, err)
		return
	}

	response.WriteHeader(http.StatusNoContent)
}

func (handler *Handler) listTasks(response http.ResponseWriter, request *http.Request) {
	limit, err := parseLimit(request.URL.Query().Get("limit"))
	if err != nil {
		writeProblem(response, request, err)
		return
	}

	page, err := handler.service.ListTasks(domain.PageParams{
		Cursor:     request.URL.Query().Get("cursor"),
		Limit:      limit,
		CalendarID: request.URL.Query().Get("calendarId"),
	})
	if err != nil {
		writeProblem(response, request, err)
		return
	}

	payload := generated.TaskListResponse{
		Items: toGeneratedTasks(page.Items),
	}
	if page.NextCursor != nil {
		payload.NextCursor = page.NextCursor
	}
	writeJSON(response, http.StatusOK, payload)
}

func (handler *Handler) createTask(response http.ResponseWriter, request *http.Request) {
	var body generated.CreateTaskRequest
	if !decodeJSON(response, request, &body) {
		return
	}

	task, err := handler.service.CreateTask(domain.Task{
		CalendarID:  body.CalendarId,
		Title:       body.Title,
		Description: body.Description,
		DueAt:       body.DueAt,
		DueDate:     body.DueDate,
		Recurrence:  toDomainRule(body.Recurrence),
	})
	if err != nil {
		writeProblem(response, request, err)
		return
	}

	writeJSON(response, http.StatusCreated, toGeneratedTask(task))
}

func (handler *Handler) getTask(response http.ResponseWriter, request *http.Request) {
	task, err := handler.service.GetTask(request.PathValue("taskId"))
	if err != nil {
		writeProblem(response, request, err)
		return
	}

	writeJSON(response, http.StatusOK, toGeneratedTask(task))
}

func (handler *Handler) updateTask(response http.ResponseWriter, request *http.Request) {
	var body generated.UpdateTaskRequest
	if !decodeJSON(response, request, &body) {
		return
	}

	task, err := handler.service.UpdateTask(request.PathValue("taskId"), domain.TaskPatch{
		Title:       body.Title,
		Description: body.Description,
		DueAt:       body.DueAt,
		DueDate:     body.DueDate,
		Recurrence:  toDomainRule(body.Recurrence),
	})
	if err != nil {
		writeProblem(response, request, err)
		return
	}

	writeJSON(response, http.StatusOK, toGeneratedTask(task))
}

func (handler *Handler) deleteTask(response http.ResponseWriter, request *http.Request) {
	if err := handler.service.DeleteTask(request.PathValue("taskId")); err != nil {
		writeProblem(response, request, err)
		return
	}

	response.WriteHeader(http.StatusNoContent)
}

func (handler *Handler) completeTask(response http.ResponseWriter, request *http.Request) {
	var body generated.CompleteTaskRequest
	if !decodeJSON(response, request, &body) {
		return
	}

	completion, err := handler.service.CompleteTask(request.PathValue("taskId"), domain.TaskCompletionRequest{
		OccurrenceAt:   body.OccurrenceAt,
		OccurrenceDate: body.OccurrenceDate,
	})
	if err != nil {
		writeProblem(response, request, err)
		return
	}

	writeJSON(response, http.StatusOK, toGeneratedTaskCompletion(completion))
}

func (handler *Handler) listAgenda(response http.ResponseWriter, request *http.Request) {
	limit, err := parseLimit(request.URL.Query().Get("limit"))
	if err != nil {
		writeProblem(response, request, err)
		return
	}

	from, err := time.Parse(time.RFC3339, request.URL.Query().Get("from"))
	if err != nil {
		writeProblem(response, request, &service.ValidationError{
			Message:     "agenda validation failed",
			FieldErrors: []service.FieldError{{Field: "from", Message: "from must be RFC3339"}},
		})
		return
	}
	to, err := time.Parse(time.RFC3339, request.URL.Query().Get("to"))
	if err != nil {
		writeProblem(response, request, &service.ValidationError{
			Message:     "agenda validation failed",
			FieldErrors: []service.FieldError{{Field: "to", Message: "to must be RFC3339"}},
		})
		return
	}

	page, err := handler.service.ListAgenda(domain.AgendaParams{
		From:   from,
		To:     to,
		Cursor: request.URL.Query().Get("cursor"),
		Limit:  limit,
	})
	if err != nil {
		writeProblem(response, request, err)
		return
	}

	payload := generated.AgendaListResponse{
		Items: toGeneratedAgendaItems(page.Items),
	}
	if page.NextCursor != nil {
		payload.NextCursor = page.NextCursor
	}
	writeJSON(response, http.StatusOK, payload)
}

func writeJSON(response http.ResponseWriter, status int, payload any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(payload)
}

func writeProblem(response http.ResponseWriter, request *http.Request, err error) {
	status := http.StatusInternalServerError
	problem := generated.Problem{
		Type:     "about:blank",
		Title:    "Internal Server Error",
		Status:   int32(http.StatusInternalServerError),
		Detail:   "unexpected server error",
		Code:     "internal_error",
		Instance: generated.PtrString(request.URL.Path),
	}

	var validationErr *service.ValidationError
	var notFoundErr *service.NotFoundError
	var conflictErr *service.ConflictError

	switch {
	case errors.As(err, &validationErr):
		status = http.StatusBadRequest
		problem.Title = "Bad Request"
		problem.Status = int32(status)
		problem.Detail = validationErr.Message
		problem.Code = "validation_error"
		if len(validationErr.FieldErrors) > 0 {
			problem.FieldErrors = make([]generated.ProblemFieldError, 0, len(validationErr.FieldErrors))
			for _, fieldError := range validationErr.FieldErrors {
				problem.FieldErrors = append(problem.FieldErrors, generated.ProblemFieldError{
					Field:   fieldError.Field,
					Message: fieldError.Message,
				})
			}
		}
	case errors.As(err, &notFoundErr):
		status = http.StatusNotFound
		problem.Title = "Not Found"
		problem.Status = int32(status)
		problem.Detail = notFoundErr.Error()
		problem.Code = "not_found"
	case errors.As(err, &conflictErr):
		status = http.StatusConflict
		problem.Title = "Conflict"
		problem.Status = int32(status)
		problem.Detail = conflictErr.Error()
		problem.Code = "conflict"
	default:
		problem.Detail = err.Error()
	}

	response.Header().Set("Content-Type", "application/problem+json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(problem)
}

func decodeJSON(response http.ResponseWriter, request *http.Request, dest any) bool {
	defer func() {
		_ = request.Body.Close()
	}()

	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dest); err != nil {
		writeProblem(response, request, &service.ValidationError{
			Message:     "request body validation failed",
			FieldErrors: []service.FieldError{{Field: "body", Message: err.Error()}},
		})
		return false
	}

	return true
}

func parseLimit(raw string) (int, error) {
	if raw == "" {
		return 0, nil
	}

	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, &service.ValidationError{
			Message:     "query validation failed",
			FieldErrors: []service.FieldError{{Field: "limit", Message: "limit must be an integer between 1 and 200"}},
		}
	}
	if value < 1 || value > 200 {
		return 0, &service.ValidationError{
			Message:     "query validation failed",
			FieldErrors: []service.FieldError{{Field: "limit", Message: "limit must be between 1 and 200"}},
		}
	}

	return value, nil
}

func toGeneratedCalendars(calendars []domain.Calendar) []generated.Calendar {
	result := make([]generated.Calendar, 0, len(calendars))
	for _, calendar := range calendars {
		result = append(result, toGeneratedCalendar(calendar))
	}

	return result
}

func toGeneratedCalendar(calendar domain.Calendar) generated.Calendar {
	return generated.Calendar{
		Id:          calendar.ID,
		Name:        calendar.Name,
		Description: cloneString(calendar.Description),
		Color:       cloneString(calendar.Color),
		CreatedAt:   calendar.CreatedAt,
		UpdatedAt:   calendar.UpdatedAt,
	}
}

func toGeneratedEvents(events []domain.Event) []generated.Event {
	result := make([]generated.Event, 0, len(events))
	for _, event := range events {
		result = append(result, toGeneratedEvent(event))
	}

	return result
}

func toGeneratedEvent(event domain.Event) generated.Event {
	return generated.Event{
		Id:          event.ID,
		CalendarId:  event.CalendarID,
		Title:       event.Title,
		Description: cloneString(event.Description),
		Location:    cloneString(event.Location),
		StartAt:     cloneTime(event.StartAt),
		EndAt:       cloneTime(event.EndAt),
		StartDate:   cloneString(event.StartDate),
		EndDate:     cloneString(event.EndDate),
		Recurrence:  toGeneratedRule(event.Recurrence),
		CreatedAt:   event.CreatedAt,
		UpdatedAt:   event.UpdatedAt,
	}
}

func toGeneratedTasks(tasks []domain.Task) []generated.Task {
	result := make([]generated.Task, 0, len(tasks))
	for _, task := range tasks {
		result = append(result, toGeneratedTask(task))
	}

	return result
}

func toGeneratedTask(task domain.Task) generated.Task {
	return generated.Task{
		Id:          task.ID,
		CalendarId:  task.CalendarID,
		Title:       task.Title,
		Description: cloneString(task.Description),
		DueAt:       cloneTime(task.DueAt),
		DueDate:     cloneString(task.DueDate),
		Recurrence:  toGeneratedRule(task.Recurrence),
		CompletedAt: cloneTime(task.CompletedAt),
		CreatedAt:   task.CreatedAt,
		UpdatedAt:   task.UpdatedAt,
	}
}

func toGeneratedTaskCompletion(completion domain.TaskCompletion) generated.TaskCompletion {
	return generated.TaskCompletion{
		TaskId:         completion.TaskID,
		OccurrenceAt:   cloneTime(completion.OccurrenceAt),
		OccurrenceDate: cloneString(completion.OccurrenceDate),
		CompletedAt:    completion.CompletedAt,
	}
}

func toGeneratedAgendaItems(items []domain.AgendaItem) []generated.AgendaItem {
	result := make([]generated.AgendaItem, 0, len(items))
	for _, item := range items {
		result = append(result, generated.AgendaItem{
			Kind:          generated.AgendaItemKind(item.Kind),
			OccurrenceKey: item.OccurrenceKey,
			CalendarId:    item.CalendarID,
			SourceId:      item.SourceID,
			Title:         item.Title,
			Description:   cloneString(item.Description),
			StartAt:       cloneTime(item.StartAt),
			EndAt:         cloneTime(item.EndAt),
			StartDate:     cloneString(item.StartDate),
			EndDate:       cloneString(item.EndDate),
			DueAt:         cloneTime(item.DueAt),
			DueDate:       cloneString(item.DueDate),
			CompletedAt:   cloneTime(item.CompletedAt),
		})
	}

	return result
}

func toDomainRule(rule *generated.RecurrenceRule) *domain.RecurrenceRule {
	if rule == nil {
		return nil
	}

	result := &domain.RecurrenceRule{
		Frequency:  domain.RecurrenceFrequency(rule.Frequency),
		Interval:   1,
		Count:      rule.Count,
		UntilAt:    cloneTime(rule.UntilAt),
		UntilDate:  cloneString(rule.UntilDate),
		ByMonthDay: append([]int32(nil), rule.ByMonthDay...),
	}
	if rule.Interval != nil {
		result.Interval = *rule.Interval
	}
	if len(rule.ByWeekday) > 0 {
		result.ByWeekday = make([]domain.Weekday, 0, len(rule.ByWeekday))
		for _, weekday := range rule.ByWeekday {
			result.ByWeekday = append(result.ByWeekday, domain.Weekday(weekday))
		}
	}

	return result
}

func toGeneratedRule(rule *domain.RecurrenceRule) *generated.RecurrenceRule {
	if rule == nil {
		return nil
	}

	result := &generated.RecurrenceRule{
		Frequency:  generated.RecurrenceFrequency(rule.Frequency),
		Interval:   generated.PtrInt32(rule.Interval),
		Count:      rule.Count,
		UntilAt:    cloneTime(rule.UntilAt),
		UntilDate:  cloneString(rule.UntilDate),
		ByMonthDay: append([]int32(nil), rule.ByMonthDay...),
	}
	if len(rule.ByWeekday) > 0 {
		result.ByWeekday = make([]generated.Weekday, 0, len(rule.ByWeekday))
		for _, weekday := range rule.ByWeekday {
			result.ByWeekday = append(result.ByWeekday, generated.Weekday(weekday))
		}
	}

	return result
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}

	result := *value
	return &result
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}

	result := *value
	return &result
}
