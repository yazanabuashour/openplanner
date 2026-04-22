package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yazanabuashour/openplanner/internal/domain"
	internalservice "github.com/yazanabuashour/openplanner/internal/service"
	"github.com/yazanabuashour/openplanner/internal/store"
)

const defaultDatabaseName = "openplanner.db"

type Options struct {
	// DatabasePath overrides the default SQLite path.
	// When empty, OpenPlanner stores data under ${XDG_DATA_HOME:-~/.local/share}/openplanner/openplanner.db.
	DatabasePath string
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

type localRuntime struct {
	service *internalservice.Service
	closeFn func() error
}

func openLocal(options Options) (*localRuntime, error) {
	databasePath, err := resolveDatabasePath(options.DatabasePath)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(databasePath), 0o700); err != nil {
		return nil, fmt.Errorf("create database dir: %w", err)
	}

	repository, err := store.Open(databasePath)
	if err != nil {
		return nil, err
	}

	return &localRuntime{
		service: internalservice.New(repository),
		closeFn: repository.Close,
	}, nil
}

func resolveDatabasePath(databasePath string) (string, error) {
	if databasePath != "" {
		return databasePath, nil
	}

	dataDir, err := defaultDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dataDir, defaultDatabaseName), nil
}

func defaultDataDir() (string, error) {
	if dataHome := os.Getenv("XDG_DATA_HOME"); dataHome != "" {
		return filepath.Join(dataHome, "openplanner"), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home: %w", err)
	}
	if home == "" {
		return "", fmt.Errorf("resolve user home: empty value")
	}

	return filepath.Join(home, ".local", "share", "openplanner"), nil
}

func (runtime *localRuntime) Close() error {
	if runtime == nil || runtime.closeFn == nil {
		return nil
	}
	return runtime.closeFn()
}

func (runtime *localRuntime) EnsureCalendar(ctx context.Context, input domain.Calendar) (calendarWriteResult, error) {
	if err := checkContext(ctx); err != nil {
		return calendarWriteResult{}, err
	}
	service, err := runtime.localService()
	if err != nil {
		return calendarWriteResult{}, err
	}

	name := strings.TrimSpace(input.Name)
	existing, err := runtime.findCalendarByName(ctx, name)
	if err != nil {
		return calendarWriteResult{}, err
	}
	if existing == nil {
		calendar, err := service.CreateCalendar(domain.Calendar{
			Name:        input.Name,
			Description: input.Description,
			Color:       input.Color,
		})
		if err != nil {
			var conflictErr *internalservice.ConflictError
			if errors.As(err, &conflictErr) {
				return runtime.EnsureCalendar(ctx, input)
			}
			return calendarWriteResult{}, err
		}
		return calendarWriteResult{
			Calendar: calendar,
			Status:   calendarWriteStatusCreated,
		}, nil
	}

	patch := domain.CalendarPatch{}
	changed := false
	if input.Description != nil && !stringPtrEqual(existing.Description, normalizeOptionalString(input.Description)) {
		patch.Description = domain.SetPatch(*input.Description)
		changed = true
	}
	if input.Color != nil && !stringPtrEqual(existing.Color, normalizeOptionalString(input.Color)) {
		patch.Color = domain.SetPatch(*input.Color)
		changed = true
	}
	if !changed {
		return calendarWriteResult{
			Calendar: *existing,
			Status:   calendarWriteStatusAlreadyExists,
		}, nil
	}

	updated, err := service.UpdateCalendar(existing.ID, patch)
	if err != nil {
		return calendarWriteResult{}, err
	}
	return calendarWriteResult{
		Calendar: updated,
		Status:   calendarWriteStatusUpdated,
	}, nil
}

func (runtime *localRuntime) UpdateCalendar(ctx context.Context, id string, patch domain.CalendarPatch) (domain.Calendar, error) {
	if err := checkContext(ctx); err != nil {
		return domain.Calendar{}, err
	}
	service, err := runtime.localService()
	if err != nil {
		return domain.Calendar{}, err
	}
	return service.UpdateCalendar(id, patch)
}

func (runtime *localRuntime) DeleteCalendar(ctx context.Context, id string) error {
	if err := checkContext(ctx); err != nil {
		return err
	}
	service, err := runtime.localService()
	if err != nil {
		return err
	}
	return service.DeleteCalendar(id)
}

func (runtime *localRuntime) CreateEvent(ctx context.Context, input domain.Event) (domain.Event, error) {
	if err := checkContext(ctx); err != nil {
		return domain.Event{}, err
	}
	service, err := runtime.localService()
	if err != nil {
		return domain.Event{}, err
	}
	return service.CreateEvent(input)
}

func (runtime *localRuntime) UpdateEvent(ctx context.Context, id string, patch domain.EventPatch) (domain.Event, error) {
	if err := checkContext(ctx); err != nil {
		return domain.Event{}, err
	}
	service, err := runtime.localService()
	if err != nil {
		return domain.Event{}, err
	}
	return service.UpdateEvent(id, patch)
}

func (runtime *localRuntime) DeleteEvent(ctx context.Context, id string) error {
	if err := checkContext(ctx); err != nil {
		return err
	}
	service, err := runtime.localService()
	if err != nil {
		return err
	}
	return service.DeleteEvent(id)
}

func (runtime *localRuntime) ListEvents(ctx context.Context, params domain.PageParams) (domain.Page[domain.Event], error) {
	if err := checkContext(ctx); err != nil {
		return domain.Page[domain.Event]{}, err
	}
	service, err := runtime.localService()
	if err != nil {
		return domain.Page[domain.Event]{}, err
	}
	return service.ListEvents(params)
}

func (runtime *localRuntime) ListCalendars(ctx context.Context, params domain.PageParams) (domain.Page[domain.Calendar], error) {
	if err := checkContext(ctx); err != nil {
		return domain.Page[domain.Calendar]{}, err
	}
	service, err := runtime.localService()
	if err != nil {
		return domain.Page[domain.Calendar]{}, err
	}
	return service.ListCalendars(params)
}

func (runtime *localRuntime) CreateTask(ctx context.Context, input domain.Task) (domain.Task, error) {
	if err := checkContext(ctx); err != nil {
		return domain.Task{}, err
	}
	service, err := runtime.localService()
	if err != nil {
		return domain.Task{}, err
	}
	return service.CreateTask(input)
}

func (runtime *localRuntime) UpdateTask(ctx context.Context, id string, patch domain.TaskPatch) (domain.Task, error) {
	if err := checkContext(ctx); err != nil {
		return domain.Task{}, err
	}
	service, err := runtime.localService()
	if err != nil {
		return domain.Task{}, err
	}
	return service.UpdateTask(id, patch)
}

func (runtime *localRuntime) DeleteTask(ctx context.Context, id string) error {
	if err := checkContext(ctx); err != nil {
		return err
	}
	service, err := runtime.localService()
	if err != nil {
		return err
	}
	return service.DeleteTask(id)
}

func (runtime *localRuntime) ListTasks(ctx context.Context, params domain.TaskListParams) (domain.Page[domain.Task], error) {
	if err := checkContext(ctx); err != nil {
		return domain.Page[domain.Task]{}, err
	}
	service, err := runtime.localService()
	if err != nil {
		return domain.Page[domain.Task]{}, err
	}
	return service.ListTasks(params)
}

func (runtime *localRuntime) ExportICalendar(ctx context.Context, calendarID string) (domain.ICalendarExport, error) {
	if err := checkContext(ctx); err != nil {
		return domain.ICalendarExport{}, err
	}
	service, err := runtime.localService()
	if err != nil {
		return domain.ICalendarExport{}, err
	}
	return service.ExportICalendar(calendarID)
}

func (runtime *localRuntime) CreateEventTaskLink(ctx context.Context, eventID string, taskID string) (domain.EventTaskLink, error) {
	if err := checkContext(ctx); err != nil {
		return domain.EventTaskLink{}, err
	}
	service, err := runtime.localService()
	if err != nil {
		return domain.EventTaskLink{}, err
	}
	return service.CreateEventTaskLink(eventID, taskID)
}

func (runtime *localRuntime) DeleteEventTaskLink(ctx context.Context, eventID string, taskID string) error {
	if err := checkContext(ctx); err != nil {
		return err
	}
	service, err := runtime.localService()
	if err != nil {
		return err
	}
	return service.DeleteEventTaskLink(eventID, taskID)
}

func (runtime *localRuntime) ListEventTaskLinks(ctx context.Context, filter domain.EventTaskLinkFilter) ([]domain.EventTaskLink, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	service, err := runtime.localService()
	if err != nil {
		return nil, err
	}
	return service.ListEventTaskLinks(filter)
}

func (runtime *localRuntime) CancelEventOccurrence(ctx context.Context, eventID string, input domain.OccurrenceMutationRequest) (domain.OccurrenceState, error) {
	if err := checkContext(ctx); err != nil {
		return domain.OccurrenceState{}, err
	}
	service, err := runtime.localService()
	if err != nil {
		return domain.OccurrenceState{}, err
	}
	return service.CancelEventOccurrence(eventID, input)
}

func (runtime *localRuntime) RescheduleEventOccurrence(ctx context.Context, eventID string, input domain.OccurrenceMutationRequest) (domain.OccurrenceState, error) {
	if err := checkContext(ctx); err != nil {
		return domain.OccurrenceState{}, err
	}
	service, err := runtime.localService()
	if err != nil {
		return domain.OccurrenceState{}, err
	}
	return service.RescheduleEventOccurrence(eventID, input)
}

func (runtime *localRuntime) CancelTaskOccurrence(ctx context.Context, taskID string, input domain.OccurrenceMutationRequest) (domain.OccurrenceState, error) {
	if err := checkContext(ctx); err != nil {
		return domain.OccurrenceState{}, err
	}
	service, err := runtime.localService()
	if err != nil {
		return domain.OccurrenceState{}, err
	}
	return service.CancelTaskOccurrence(taskID, input)
}

func (runtime *localRuntime) RescheduleTaskOccurrence(ctx context.Context, taskID string, input domain.OccurrenceMutationRequest) (domain.OccurrenceState, error) {
	if err := checkContext(ctx); err != nil {
		return domain.OccurrenceState{}, err
	}
	service, err := runtime.localService()
	if err != nil {
		return domain.OccurrenceState{}, err
	}
	return service.RescheduleTaskOccurrence(taskID, input)
}

func (runtime *localRuntime) CompleteTask(ctx context.Context, taskID string, input domain.TaskCompletionRequest) (domain.TaskCompletion, error) {
	if err := checkContext(ctx); err != nil {
		return domain.TaskCompletion{}, err
	}
	service, err := runtime.localService()
	if err != nil {
		return domain.TaskCompletion{}, err
	}
	return service.CompleteTask(taskID, input)
}

func (runtime *localRuntime) ListAgenda(ctx context.Context, params domain.AgendaParams) (domain.Page[domain.AgendaItem], error) {
	if err := checkContext(ctx); err != nil {
		return domain.Page[domain.AgendaItem]{}, err
	}
	service, err := runtime.localService()
	if err != nil {
		return domain.Page[domain.AgendaItem]{}, err
	}
	return service.ListAgenda(params)
}

func (runtime *localRuntime) ListPendingReminders(ctx context.Context, params domain.ReminderQueryParams) (domain.Page[domain.PendingReminder], error) {
	if err := checkContext(ctx); err != nil {
		return domain.Page[domain.PendingReminder]{}, err
	}
	service, err := runtime.localService()
	if err != nil {
		return domain.Page[domain.PendingReminder]{}, err
	}
	return service.ListPendingReminders(params)
}

func (runtime *localRuntime) DismissReminderOccurrence(ctx context.Context, reminderOccurrenceID string) (domain.ReminderDismissal, error) {
	if err := checkContext(ctx); err != nil {
		return domain.ReminderDismissal{}, err
	}
	service, err := runtime.localService()
	if err != nil {
		return domain.ReminderDismissal{}, err
	}
	return service.DismissReminderOccurrence(reminderOccurrenceID)
}

func (runtime *localRuntime) localService() (*internalservice.Service, error) {
	if runtime == nil || runtime.service == nil {
		return nil, fmt.Errorf("local OpenPlanner runtime is required")
	}
	return runtime.service, nil
}

func (runtime *localRuntime) findCalendarByName(ctx context.Context, name string) (*domain.Calendar, error) {
	service, err := runtime.localService()
	if err != nil {
		return nil, err
	}

	cursor := ""
	for {
		if err := checkContext(ctx); err != nil {
			return nil, err
		}
		page, err := service.ListCalendars(domain.PageParams{
			Cursor: cursor,
			Limit:  200,
		})
		if err != nil {
			return nil, err
		}
		for _, calendar := range page.Items {
			if calendar.Name == name {
				return &calendar, nil
			}
		}
		if page.NextCursor == nil {
			return nil, nil
		}
		cursor = *page.NextCursor
	}
}

func checkContext(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

func normalizeOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	normalized := strings.TrimSpace(*value)
	if normalized == "" {
		return nil
	}
	return &normalized
}

func stringPtrEqual(left *string, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
