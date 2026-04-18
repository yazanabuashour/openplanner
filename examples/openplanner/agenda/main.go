package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/yazanabuashour/openplanner/sdk"
)

func main() {
	// Use a throwaway database so the example stays rerunnable.
	tempDir, err := os.MkdirTemp("", "openplanner-agenda-example-*")
	if err != nil {
		panic(err)
	}
	defer func() {
		if removeErr := os.RemoveAll(tempDir); removeErr != nil {
			panic(removeErr)
		}
	}()

	client, err := sdk.OpenLocal(sdk.Options{DatabasePath: filepath.Join(tempDir, "openplanner-example.db")})
	if err != nil {
		panic(err)
	}
	defer func() {
		if closeErr := client.Close(); closeErr != nil {
			panic(closeErr)
		}
	}()

	ctx := context.Background()
	calendar, err := client.EnsureCalendar(ctx, sdk.CalendarInput{Name: "Personal"})
	if err != nil {
		panic(err)
	}

	startAt := time.Date(2026, 4, 16, 9, 0, 0, 0, time.UTC)
	endAt := startAt.Add(time.Hour)
	count := int32(2)

	_, err = client.CreateEvent(ctx, sdk.EventInput{
		CalendarID: calendar.Calendar.ID,
		Title:      "Standup",
		StartAt:    &startAt,
		EndAt:      &endAt,
		Recurrence: &sdk.RecurrenceRule{
			Frequency: sdk.RecurrenceFrequencyDaily,
			Count:     &count,
		},
	})
	if err != nil {
		panic(err)
	}

	dueDate := "2026-04-16"
	_, err = client.CreateTask(ctx, sdk.TaskInput{
		CalendarID: calendar.Calendar.ID,
		Title:      "Review notes",
		DueDate:    &dueDate,
		Recurrence: &sdk.RecurrenceRule{
			Frequency: sdk.RecurrenceFrequencyDaily,
			Count:     &count,
		},
	})
	if err != nil {
		panic(err)
	}

	agenda, err := client.ListAgenda(ctx, sdk.AgendaOptions{
		From: time.Date(2026, 4, 16, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 4, 18, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		panic(err)
	}

	fmt.Printf("calendar=%s agendaItems=%d\n", calendar.Calendar.ID, len(agenda.Items))
}
