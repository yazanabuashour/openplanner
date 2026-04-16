package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/yazanabuashour/openplanner/sdk"
	"github.com/yazanabuashour/openplanner/sdk/generated"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "openplanner query: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	defaultDatabasePath, err := sdk.DefaultDatabasePath()
	if err != nil {
		return err
	}

	databasePath := flag.String("db", defaultDatabasePath, "SQLite database path")
	fromValue := flag.String("from", "", "inclusive RFC3339 agenda start")
	toValue := flag.String("to", "", "exclusive RFC3339 agenda end")
	limit := flag.Int("limit", 50, "maximum agenda items to return")
	flag.Parse()

	if *fromValue == "" || *toValue == "" {
		return fmt.Errorf("both -from and -to are required")
	}
	if *limit < 1 || *limit > 200 {
		return fmt.Errorf("-limit must be between 1 and 200")
	}

	from, err := time.Parse(time.RFC3339, *fromValue)
	if err != nil {
		return fmt.Errorf("parse -from: %w", err)
	}
	to, err := time.Parse(time.RFC3339, *toValue)
	if err != nil {
		return fmt.Errorf("parse -to: %w", err)
	}
	if !to.After(from) {
		return fmt.Errorf("-to must be after -from")
	}

	client, err := sdk.OpenLocal(sdk.Options{DatabasePath: *databasePath})
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := client.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "openplanner query: close: %v\n", closeErr)
		}
	}()

	ctx := context.Background()
	calendars, _, err := client.CalendarsAPI.ListCalendars(ctx).Limit(200).Execute()
	if err != nil {
		return fmt.Errorf("list calendars: %w", err)
	}
	agenda, _, err := client.AgendaAPI.ListAgenda(ctx).
		From(from).
		To(to).
		Limit(int32(*limit)).
		Execute()
	if err != nil {
		return fmt.Errorf("list agenda: %w", err)
	}

	printCalendars(calendars.Items)
	printAgenda(agenda.Items)
	return nil
}

func printCalendars(calendars []generated.Calendar) {
	fmt.Printf("calendars=%d\n", len(calendars))
	for _, calendar := range calendars {
		fmt.Printf("calendar\t%s\t%s\n", calendar.Id, calendar.Name)
	}
}

func printAgenda(items []generated.AgendaItem) {
	fmt.Printf("agendaItems=%d\n", len(items))
	for _, item := range items {
		fmt.Printf("agenda\t%s\t%s\t%s\t%s\n", item.Kind, occurrenceLabel(item), item.SourceId, item.Title)
	}
}

func occurrenceLabel(item generated.AgendaItem) string {
	switch {
	case item.StartAt != nil:
		return item.StartAt.Format(time.RFC3339)
	case item.StartDate != nil:
		return *item.StartDate
	case item.DueAt != nil:
		return item.DueAt.Format(time.RFC3339)
	case item.DueDate != nil:
		return *item.DueDate
	default:
		return item.OccurrenceKey
	}
}
