package main

import (
	"context"
	"fmt"
	"time"

	"github.com/yazanabuashour/openplanner/sdk"
	"github.com/yazanabuashour/openplanner/sdk/generated"
)

func main() {
	client, err := sdk.OpenLocal(sdk.Options{DatabasePath: "./openplanner-example.db"})
	if err != nil {
		panic(err)
	}
	defer func() {
		if closeErr := client.Close(); closeErr != nil {
			panic(closeErr)
		}
	}()

	ctx := context.Background()
	calendar, _, err := client.CalendarsAPI.CreateCalendar(ctx).CreateCalendarRequest(generated.CreateCalendarRequest{
		Name: "Personal",
	}).Execute()
	if err != nil {
		panic(err)
	}

	startAt := time.Date(2026, 4, 16, 9, 0, 0, 0, time.UTC)
	endAt := startAt.Add(time.Hour)
	count := int32(2)

	_, _, err = client.EventsAPI.CreateEvent(ctx).CreateEventRequest(generated.CreateEventRequest{
		CalendarId: calendar.Id,
		Title:      "Standup",
		StartAt:    &startAt,
		EndAt:      &endAt,
		Recurrence: &generated.RecurrenceRule{
			Frequency: generated.RECURRENCEFREQUENCY_DAILY,
			Count:     &count,
		},
	}).Execute()
	if err != nil {
		panic(err)
	}

	dueDate := "2026-04-16"
	_, _, err = client.TasksAPI.CreateTask(ctx).CreateTaskRequest(generated.CreateTaskRequest{
		CalendarId: calendar.Id,
		Title:      "Review notes",
		DueDate:    &dueDate,
		Recurrence: &generated.RecurrenceRule{
			Frequency: generated.RECURRENCEFREQUENCY_DAILY,
			Count:     &count,
		},
	}).Execute()
	if err != nil {
		panic(err)
	}

	agenda, _, err := client.AgendaAPI.ListAgenda(ctx).
		From(time.Date(2026, 4, 16, 0, 0, 0, 0, time.UTC)).
		To(time.Date(2026, 4, 18, 0, 0, 0, 0, time.UTC)).
		Execute()
	if err != nil {
		panic(err)
	}

	fmt.Printf("calendar=%s agendaItems=%d\n", calendar.Id, len(agenda.Items))
}
