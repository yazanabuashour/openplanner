package service

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/yazanabuashour/openplanner/internal/domain"
	"github.com/yazanabuashour/openplanner/internal/icalendar"
	"github.com/yazanabuashour/openplanner/internal/store"
)

type ICalendarObjectKind string

const (
	ICalendarObjectKindEvent ICalendarObjectKind = "event"
	ICalendarObjectKindTask  ICalendarObjectKind = "task"
)

type ICalendarObject struct {
	Kind     ICalendarObjectKind
	Calendar domain.Calendar
	Event    *domain.Event
	Task     *domain.Task
}

func (object ICalendarObject) ID() string {
	switch object.Kind {
	case ICalendarObjectKindEvent:
		if object.Event != nil {
			return object.Event.ID
		}
	case ICalendarObjectKindTask:
		if object.Task != nil {
			return object.Task.ID
		}
	}
	return ""
}

func (object ICalendarObject) Title() string {
	switch object.Kind {
	case ICalendarObjectKindEvent:
		if object.Event != nil {
			return object.Event.Title
		}
	case ICalendarObjectKindTask:
		if object.Task != nil {
			return object.Task.Title
		}
	}
	return ""
}

func (object ICalendarObject) ResourceName() string {
	switch object.Kind {
	case ICalendarObjectKindEvent:
		if object.Event != nil && object.Event.ICalendarUID != nil {
			if uid := strings.TrimSpace(*object.Event.ICalendarUID); uid != "" {
				return uidResourceName(uid)
			}
		}
	case ICalendarObjectKindTask:
		if object.Task != nil && object.Task.ICalendarUID != nil {
			if uid := strings.TrimSpace(*object.Task.ICalendarUID); uid != "" {
				return uidResourceName(uid)
			}
		}
	}
	return object.ID() + ".ics"
}

func (service *Service) ListICalendarObjects(calendarID string) ([]ICalendarObject, error) {
	calendar, err := service.GetCalendar(calendarID)
	if err != nil {
		return nil, err
	}

	events, err := service.store.ListEvents(calendarID)
	if err != nil {
		return nil, err
	}
	tasks, err := service.store.ListTasks(domain.TaskListParams{PageParams: domain.PageParams{CalendarID: calendarID}})
	if err != nil {
		return nil, err
	}

	objects := make([]ICalendarObject, 0, len(events)+len(tasks))
	for i := range events {
		event := events[i]
		objects = append(objects, ICalendarObject{
			Kind:     ICalendarObjectKindEvent,
			Calendar: calendar,
			Event:    &event,
		})
	}
	for i := range tasks {
		task := tasks[i]
		objects = append(objects, ICalendarObject{
			Kind:     ICalendarObjectKindTask,
			Calendar: calendar,
			Task:     &task,
		})
	}
	return objects, nil
}

func (service *Service) ResolveICalendarObject(calendarID string, resource string) (ICalendarObject, error) {
	calendar, err := service.GetCalendar(calendarID)
	if err != nil {
		return ICalendarObject{}, err
	}

	token := strings.TrimSpace(strings.TrimSuffix(resource, ".ics"))
	if token == "" {
		return ICalendarObject{}, &ValidationError{
			Message:     "calendar object validation failed",
			FieldErrors: []FieldError{{Field: "resource", Message: "resource is required"}},
		}
	}

	for _, candidate := range localObjectIDCandidates(token) {
		object, found, err := service.resolveICalendarObjectByID(calendar, candidate)
		if err != nil {
			return ICalendarObject{}, err
		}
		if found {
			return object, nil
		}
	}

	for _, candidate := range uidCandidates(token) {
		object, found, err := service.resolveICalendarObjectByUID(calendar, candidate)
		if err != nil {
			return ICalendarObject{}, err
		}
		if found {
			return object, nil
		}
	}

	return ICalendarObject{}, &NotFoundError{
		Resource: "calendar_object",
		ID:       token,
		Message:  "calendar object not found",
	}
}

func (service *Service) ExportICalendarObject(calendarID string, resource string) (domain.ICalendarExport, error) {
	object, err := service.ResolveICalendarObject(calendarID, resource)
	if err != nil {
		return domain.ICalendarExport{}, err
	}
	return service.exportICalendarObjects(object.Calendar, []ICalendarObject{object})
}

func (service *Service) DeleteICalendarObject(calendarID string, resource string) error {
	object, err := service.ResolveICalendarObject(calendarID, resource)
	if err != nil {
		return err
	}

	switch object.Kind {
	case ICalendarObjectKindEvent:
		return service.DeleteEvent(object.ID())
	case ICalendarObjectKindTask:
		return service.DeleteTask(object.ID())
	default:
		return &ValidationError{
			Message:     "calendar object validation failed",
			FieldErrors: []FieldError{{Field: "kind", Message: "calendar object kind is unsupported"}},
		}
	}
}

func (service *Service) exportICalendarObjects(calendar domain.Calendar, objects []ICalendarObject) (domain.ICalendarExport, error) {
	events := []domain.Event{}
	tasks := []domain.Task{}
	for _, object := range objects {
		switch object.Kind {
		case ICalendarObjectKindEvent:
			if object.Event != nil {
				events = append(events, *object.Event)
			}
		case ICalendarObjectKindTask:
			if object.Task != nil {
				tasks = append(tasks, *object.Task)
			}
		default:
			return domain.ICalendarExport{}, fmt.Errorf("unsupported calendar object kind %q", object.Kind)
		}
	}

	eventStates, err := service.store.ListOccurrenceStates(domain.OccurrenceOwnerKindEvent, recurringEventIDs(events))
	if err != nil {
		return domain.ICalendarExport{}, err
	}
	taskStates, err := service.store.ListOccurrenceStates(domain.OccurrenceOwnerKindTask, recurringTaskIDs(tasks))
	if err != nil {
		return domain.ICalendarExport{}, err
	}
	taskCompletions, err := service.store.ListTaskCompletions(recurringTaskIDs(tasks))
	if err != nil {
		return domain.ICalendarExport{}, err
	}

	generatedAt := stableICalendarGeneratedAt(objects, eventStates, taskStates, taskCompletions, service.now())
	result := icalendar.Build(icalendar.Export{
		Calendars:             []domain.Calendar{calendar},
		Events:                events,
		Tasks:                 tasks,
		EventOccurrenceStates: eventStates,
		TaskOccurrenceStates:  taskStates,
		TaskCompletions:       taskCompletions,
		GeneratedAt:           generatedAt,
		Name:                  calendarExportName(calendar.Name),
	})

	filename := sanitizeICalendarFilename(calendar.Name)
	if len(objects) == 1 && objects[0].ID() != "" {
		filename = objects[0].ResourceName()
	}
	return domain.ICalendarExport{
		ContentType:  result.ContentType,
		Filename:     filename,
		CalendarID:   calendar.ID,
		CalendarName: calendar.Name,
		EventCount:   result.EventCount,
		TaskCount:    result.TaskCount,
		Content:      result.Content,
	}, nil
}

func stableICalendarGeneratedAt(objects []ICalendarObject, eventStates map[string]map[string]domain.OccurrenceState, taskStates map[string]map[string]domain.OccurrenceState, taskCompletions map[string]map[string]domain.TaskCompletion, fallback time.Time) time.Time {
	generatedAt := time.Time{}
	for _, object := range objects {
		switch object.Kind {
		case ICalendarObjectKindEvent:
			if object.Event != nil && object.Event.UpdatedAt.After(generatedAt) {
				generatedAt = object.Event.UpdatedAt
			}
			for _, state := range eventStates[object.ID()] {
				if state.UpdatedAt.After(generatedAt) {
					generatedAt = state.UpdatedAt
				}
			}
		case ICalendarObjectKindTask:
			if object.Task != nil && object.Task.UpdatedAt.After(generatedAt) {
				generatedAt = object.Task.UpdatedAt
			}
			for _, state := range taskStates[object.ID()] {
				if state.UpdatedAt.After(generatedAt) {
					generatedAt = state.UpdatedAt
				}
			}
			for _, completion := range taskCompletions[object.ID()] {
				if completion.CompletedAt.After(generatedAt) {
					generatedAt = completion.CompletedAt
				}
			}
		}
	}
	if generatedAt.IsZero() {
		return fallback
	}
	return generatedAt.UTC()
}

func (service *Service) resolveICalendarObjectByID(calendar domain.Calendar, id string) (ICalendarObject, bool, error) {
	if _, err := ulid.ParseStrict(id); err != nil {
		return ICalendarObject{}, false, nil
	}

	event, err := service.GetEvent(id)
	if err == nil {
		if event.CalendarID == calendar.ID {
			return ICalendarObject{Kind: ICalendarObjectKindEvent, Calendar: calendar, Event: &event}, true, nil
		}
		return ICalendarObject{}, false, nil
	}
	var notFound *NotFoundError
	if !errors.As(err, &notFound) {
		return ICalendarObject{}, false, err
	}

	task, err := service.GetTask(id)
	if err == nil {
		if task.CalendarID == calendar.ID {
			return ICalendarObject{Kind: ICalendarObjectKindTask, Calendar: calendar, Task: &task}, true, nil
		}
		return ICalendarObject{}, false, nil
	}
	if !errors.As(err, &notFound) {
		return ICalendarObject{}, false, err
	}
	return ICalendarObject{}, false, nil
}

func (service *Service) resolveICalendarObjectByUID(calendar domain.Calendar, uid string) (ICalendarObject, bool, error) {
	var found []ICalendarObject

	event, err := service.store.GetEventByICalendarUID(calendar.ID, uid)
	if err == nil {
		found = append(found, ICalendarObject{Kind: ICalendarObjectKindEvent, Calendar: calendar, Event: &event})
	} else if !errors.Is(err, store.ErrNotFound) {
		return ICalendarObject{}, false, err
	}

	task, err := service.store.GetTaskByICalendarUID(calendar.ID, uid)
	if err == nil {
		found = append(found, ICalendarObject{Kind: ICalendarObjectKindTask, Calendar: calendar, Task: &task})
	} else if !errors.Is(err, store.ErrNotFound) {
		return ICalendarObject{}, false, err
	}

	switch len(found) {
	case 0:
		return ICalendarObject{}, false, nil
	case 1:
		return found[0], true, nil
	default:
		return ICalendarObject{}, false, &ConflictError{Message: "calendar object UID is ambiguous"}
	}
}

func localObjectIDCandidates(resource string) []string {
	candidates := []string{resource}
	const localUIDSuffix = "@openplanner.local"
	if strings.HasSuffix(resource, localUIDSuffix) {
		candidates = append(candidates, strings.TrimSuffix(resource, localUIDSuffix))
	}
	return candidates
}

func uidResourceName(uid string) string {
	return url.PathEscape(uid) + ".ics"
}

func uidCandidates(resource string) []string {
	candidates := []string{resource}
	if unescaped, err := url.PathUnescape(resource); err == nil && unescaped != resource {
		candidates = append(candidates, unescaped)
	}
	return candidates
}
