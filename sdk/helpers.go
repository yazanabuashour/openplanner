package sdk

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/yazanabuashour/openplanner/internal/domain"
	internalservice "github.com/yazanabuashour/openplanner/internal/service"
)

func (client *Client) EnsureCalendar(ctx context.Context, input CalendarInput) (CalendarWriteResult, error) {
	if err := checkContext(ctx); err != nil {
		return CalendarWriteResult{}, err
	}
	service, err := client.localService()
	if err != nil {
		return CalendarWriteResult{}, err
	}

	name := strings.TrimSpace(input.Name)
	existing, err := client.findCalendarByName(ctx, name)
	if err != nil {
		return CalendarWriteResult{}, err
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
				return client.EnsureCalendar(ctx, input)
			}
			return CalendarWriteResult{}, err
		}
		return CalendarWriteResult{
			Calendar: fromDomainCalendar(calendar),
			Status:   CalendarWriteStatusCreated,
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
		return CalendarWriteResult{
			Calendar: fromDomainCalendar(*existing),
			Status:   CalendarWriteStatusAlreadyExists,
		}, nil
	}

	updated, err := service.UpdateCalendar(existing.ID, patch)
	if err != nil {
		return CalendarWriteResult{}, err
	}
	return CalendarWriteResult{
		Calendar: fromDomainCalendar(updated),
		Status:   CalendarWriteStatusUpdated,
	}, nil
}

func (client *Client) UpdateCalendar(ctx context.Context, id string, input CalendarPatchInput) (Calendar, error) {
	if err := checkContext(ctx); err != nil {
		return Calendar{}, err
	}
	service, err := client.localService()
	if err != nil {
		return Calendar{}, err
	}
	calendar, err := service.UpdateCalendar(id, domain.CalendarPatch{
		Name:        toDomainStringPatch(input.Name),
		Description: toDomainStringPatch(input.Description),
		Color:       toDomainStringPatch(input.Color),
	})
	if err != nil {
		return Calendar{}, err
	}
	return fromDomainCalendar(calendar), nil
}

func (client *Client) CreateEvent(ctx context.Context, input EventInput) (Event, error) {
	if err := checkContext(ctx); err != nil {
		return Event{}, err
	}
	service, err := client.localService()
	if err != nil {
		return Event{}, err
	}
	event, err := service.CreateEvent(domain.Event{
		CalendarID:  input.CalendarID,
		Title:       input.Title,
		Description: input.Description,
		Location:    input.Location,
		StartAt:     input.StartAt,
		EndAt:       input.EndAt,
		StartDate:   input.StartDate,
		EndDate:     input.EndDate,
		Recurrence:  toDomainRule(input.Recurrence),
	})
	if err != nil {
		return Event{}, err
	}
	return fromDomainEvent(event), nil
}

func (client *Client) UpdateEvent(ctx context.Context, id string, input EventPatchInput) (Event, error) {
	if err := checkContext(ctx); err != nil {
		return Event{}, err
	}
	service, err := client.localService()
	if err != nil {
		return Event{}, err
	}
	event, err := service.UpdateEvent(id, domain.EventPatch{
		Title:       toDomainStringPatch(input.Title),
		Description: toDomainStringPatch(input.Description),
		Location:    toDomainStringPatch(input.Location),
		StartAt:     toDomainTimePatch(input.StartAt),
		EndAt:       toDomainTimePatch(input.EndAt),
		StartDate:   toDomainStringPatch(input.StartDate),
		EndDate:     toDomainStringPatch(input.EndDate),
		Recurrence:  toDomainRulePatch(input.Recurrence),
	})
	if err != nil {
		return Event{}, err
	}
	return fromDomainEvent(event), nil
}

func (client *Client) ListEvents(ctx context.Context, options ListOptions) (Page[Event], error) {
	if err := checkContext(ctx); err != nil {
		return Page[Event]{}, err
	}
	service, err := client.localService()
	if err != nil {
		return Page[Event]{}, err
	}
	page, err := service.ListEvents(domain.PageParams{
		Cursor:     options.Cursor,
		Limit:      options.Limit,
		CalendarID: options.CalendarID,
	})
	if err != nil {
		return Page[Event]{}, err
	}
	return Page[Event]{
		Items:      fromDomainEvents(page.Items),
		NextCursor: cloneString(page.NextCursor),
	}, nil
}

func (client *Client) ListCalendars(ctx context.Context, options ListOptions) (Page[Calendar], error) {
	if err := checkContext(ctx); err != nil {
		return Page[Calendar]{}, err
	}
	service, err := client.localService()
	if err != nil {
		return Page[Calendar]{}, err
	}
	page, err := service.ListCalendars(domain.PageParams{
		Cursor: options.Cursor,
		Limit:  options.Limit,
	})
	if err != nil {
		return Page[Calendar]{}, err
	}
	return Page[Calendar]{
		Items:      fromDomainCalendars(page.Items),
		NextCursor: cloneString(page.NextCursor),
	}, nil
}

func (client *Client) CreateTask(ctx context.Context, input TaskInput) (Task, error) {
	if err := checkContext(ctx); err != nil {
		return Task{}, err
	}
	service, err := client.localService()
	if err != nil {
		return Task{}, err
	}
	task, err := service.CreateTask(domain.Task{
		CalendarID:  input.CalendarID,
		Title:       input.Title,
		Description: input.Description,
		DueAt:       input.DueAt,
		DueDate:     input.DueDate,
		Recurrence:  toDomainRule(input.Recurrence),
	})
	if err != nil {
		return Task{}, err
	}
	return fromDomainTask(task), nil
}

func (client *Client) UpdateTask(ctx context.Context, id string, input TaskPatchInput) (Task, error) {
	if err := checkContext(ctx); err != nil {
		return Task{}, err
	}
	service, err := client.localService()
	if err != nil {
		return Task{}, err
	}
	task, err := service.UpdateTask(id, domain.TaskPatch{
		Title:       toDomainStringPatch(input.Title),
		Description: toDomainStringPatch(input.Description),
		DueAt:       toDomainTimePatch(input.DueAt),
		DueDate:     toDomainStringPatch(input.DueDate),
		Recurrence:  toDomainRulePatch(input.Recurrence),
	})
	if err != nil {
		return Task{}, err
	}
	return fromDomainTask(task), nil
}

func (client *Client) ListTasks(ctx context.Context, options ListOptions) (Page[Task], error) {
	if err := checkContext(ctx); err != nil {
		return Page[Task]{}, err
	}
	service, err := client.localService()
	if err != nil {
		return Page[Task]{}, err
	}
	page, err := service.ListTasks(domain.PageParams{
		Cursor:     options.Cursor,
		Limit:      options.Limit,
		CalendarID: options.CalendarID,
	})
	if err != nil {
		return Page[Task]{}, err
	}
	return Page[Task]{
		Items:      fromDomainTasks(page.Items),
		NextCursor: cloneString(page.NextCursor),
	}, nil
}

func (client *Client) CompleteTask(ctx context.Context, taskID string, input TaskCompletionInput) (TaskCompletion, error) {
	if err := checkContext(ctx); err != nil {
		return TaskCompletion{}, err
	}
	service, err := client.localService()
	if err != nil {
		return TaskCompletion{}, err
	}
	completion, err := service.CompleteTask(taskID, domain.TaskCompletionRequest{
		OccurrenceAt:   input.OccurrenceAt,
		OccurrenceDate: input.OccurrenceDate,
	})
	if err != nil {
		return TaskCompletion{}, err
	}
	return fromDomainTaskCompletion(completion), nil
}

func (client *Client) ListAgenda(ctx context.Context, options AgendaOptions) (Page[AgendaItem], error) {
	if err := checkContext(ctx); err != nil {
		return Page[AgendaItem]{}, err
	}
	service, err := client.localService()
	if err != nil {
		return Page[AgendaItem]{}, err
	}
	page, err := service.ListAgenda(domain.AgendaParams{
		From:   options.From,
		To:     options.To,
		Cursor: options.Cursor,
		Limit:  options.Limit,
	})
	if err != nil {
		return Page[AgendaItem]{}, err
	}
	return Page[AgendaItem]{
		Items:      fromDomainAgendaItems(page.Items),
		NextCursor: cloneString(page.NextCursor),
	}, nil
}

func (client *Client) localService() (*internalservice.Service, error) {
	if client == nil || client.service == nil {
		return nil, fmt.Errorf("local OpenPlanner client is required")
	}
	return client.service, nil
}

func (client *Client) findCalendarByName(ctx context.Context, name string) (*domain.Calendar, error) {
	service, err := client.localService()
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

func toDomainRule(rule *RecurrenceRule) *domain.RecurrenceRule {
	if rule == nil {
		return nil
	}
	return &domain.RecurrenceRule{
		Frequency:  domain.RecurrenceFrequency(rule.Frequency),
		Interval:   rule.Interval,
		Count:      cloneInt32(rule.Count),
		UntilAt:    cloneTime(rule.UntilAt),
		UntilDate:  cloneString(rule.UntilDate),
		ByWeekday:  toDomainWeekdays(rule.ByWeekday),
		ByMonthDay: append([]int32(nil), rule.ByMonthDay...),
	}
}

func toDomainStringPatch(field PatchField[string]) domain.PatchField[string] {
	if !field.IsSet() {
		return domain.PatchField[string]{}
	}
	if field.IsClear() {
		return domain.ClearPatch[string]()
	}
	return domain.SetPatch(field.Value())
}

func toDomainTimePatch(field PatchField[time.Time]) domain.PatchField[time.Time] {
	if !field.IsSet() {
		return domain.PatchField[time.Time]{}
	}
	if field.IsClear() {
		return domain.ClearPatch[time.Time]()
	}
	return domain.SetPatch(field.Value())
}

func toDomainRulePatch(field PatchField[RecurrenceRule]) domain.PatchField[domain.RecurrenceRule] {
	if !field.IsSet() {
		return domain.PatchField[domain.RecurrenceRule]{}
	}
	if field.IsClear() {
		return domain.ClearPatch[domain.RecurrenceRule]()
	}
	rule := toDomainRuleValue(field.Value())
	return domain.SetPatch(rule)
}

func toDomainRuleValue(rule RecurrenceRule) domain.RecurrenceRule {
	return domain.RecurrenceRule{
		Frequency:  domain.RecurrenceFrequency(rule.Frequency),
		Interval:   rule.Interval,
		Count:      cloneInt32(rule.Count),
		UntilAt:    cloneTime(rule.UntilAt),
		UntilDate:  cloneString(rule.UntilDate),
		ByWeekday:  toDomainWeekdays(rule.ByWeekday),
		ByMonthDay: append([]int32(nil), rule.ByMonthDay...),
	}
}

func fromDomainRule(rule *domain.RecurrenceRule) *RecurrenceRule {
	if rule == nil {
		return nil
	}
	return &RecurrenceRule{
		Frequency:  RecurrenceFrequency(rule.Frequency),
		Interval:   rule.Interval,
		Count:      cloneInt32(rule.Count),
		UntilAt:    cloneTime(rule.UntilAt),
		UntilDate:  cloneString(rule.UntilDate),
		ByWeekday:  fromDomainWeekdays(rule.ByWeekday),
		ByMonthDay: append([]int32(nil), rule.ByMonthDay...),
	}
}

func toDomainWeekdays(weekdays []Weekday) []domain.Weekday {
	if len(weekdays) == 0 {
		return nil
	}
	out := make([]domain.Weekday, 0, len(weekdays))
	for _, weekday := range weekdays {
		out = append(out, domain.Weekday(weekday))
	}
	return out
}

func fromDomainWeekdays(weekdays []domain.Weekday) []Weekday {
	if len(weekdays) == 0 {
		return nil
	}
	out := make([]Weekday, 0, len(weekdays))
	for _, weekday := range weekdays {
		out = append(out, Weekday(weekday))
	}
	return out
}

func fromDomainCalendar(calendar domain.Calendar) Calendar {
	return Calendar{
		ID:          calendar.ID,
		Name:        calendar.Name,
		Description: cloneString(calendar.Description),
		Color:       cloneString(calendar.Color),
		CreatedAt:   calendar.CreatedAt,
		UpdatedAt:   calendar.UpdatedAt,
	}
}

func fromDomainCalendars(calendars []domain.Calendar) []Calendar {
	out := make([]Calendar, 0, len(calendars))
	for _, calendar := range calendars {
		out = append(out, fromDomainCalendar(calendar))
	}
	return out
}

func fromDomainEvent(event domain.Event) Event {
	return Event{
		ID:          event.ID,
		CalendarID:  event.CalendarID,
		Title:       event.Title,
		Description: cloneString(event.Description),
		Location:    cloneString(event.Location),
		StartAt:     cloneTime(event.StartAt),
		EndAt:       cloneTime(event.EndAt),
		StartDate:   cloneString(event.StartDate),
		EndDate:     cloneString(event.EndDate),
		Recurrence:  fromDomainRule(event.Recurrence),
		CreatedAt:   event.CreatedAt,
		UpdatedAt:   event.UpdatedAt,
	}
}

func fromDomainEvents(events []domain.Event) []Event {
	out := make([]Event, 0, len(events))
	for _, event := range events {
		out = append(out, fromDomainEvent(event))
	}
	return out
}

func fromDomainTask(task domain.Task) Task {
	return Task{
		ID:          task.ID,
		CalendarID:  task.CalendarID,
		Title:       task.Title,
		Description: cloneString(task.Description),
		DueAt:       cloneTime(task.DueAt),
		DueDate:     cloneString(task.DueDate),
		Recurrence:  fromDomainRule(task.Recurrence),
		CompletedAt: cloneTime(task.CompletedAt),
		CreatedAt:   task.CreatedAt,
		UpdatedAt:   task.UpdatedAt,
	}
}

func fromDomainTasks(tasks []domain.Task) []Task {
	out := make([]Task, 0, len(tasks))
	for _, task := range tasks {
		out = append(out, fromDomainTask(task))
	}
	return out
}

func fromDomainTaskCompletion(completion domain.TaskCompletion) TaskCompletion {
	return TaskCompletion{
		TaskID:         completion.TaskID,
		OccurrenceKey:  completion.OccurrenceKey,
		OccurrenceAt:   cloneTime(completion.OccurrenceAt),
		OccurrenceDate: cloneString(completion.OccurrenceDate),
		CompletedAt:    completion.CompletedAt,
	}
}

func fromDomainAgendaItem(item domain.AgendaItem) AgendaItem {
	return AgendaItem{
		Kind:          AgendaItemKind(item.Kind),
		OccurrenceKey: item.OccurrenceKey,
		CalendarID:    item.CalendarID,
		SourceID:      item.SourceID,
		Title:         item.Title,
		Description:   cloneString(item.Description),
		StartAt:       cloneTime(item.StartAt),
		EndAt:         cloneTime(item.EndAt),
		StartDate:     cloneString(item.StartDate),
		EndDate:       cloneString(item.EndDate),
		DueAt:         cloneTime(item.DueAt),
		DueDate:       cloneString(item.DueDate),
		CompletedAt:   cloneTime(item.CompletedAt),
	}
}

func fromDomainAgendaItems(items []domain.AgendaItem) []AgendaItem {
	out := make([]AgendaItem, 0, len(items))
	for _, item := range items {
		out = append(out, fromDomainAgendaItem(item))
	}
	return out
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func cloneInt32(value *int32) *int32 {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}
