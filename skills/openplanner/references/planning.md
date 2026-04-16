# Planning Task Recipes

Use these snippets after opening the local runtime:

```go
api, err := sdk.OpenLocal(sdk.Options{})
if err != nil {
	log.Fatal(err)
}
defer api.Close()
ctx := context.Background()
```

## Ensure A Calendar

```go
calendar, err := api.EnsureCalendar(ctx, sdk.CalendarInput{
	Name: "Personal",
})
if err != nil {
	log.Fatal(err)
}
log.Printf("%s %s", calendar.Calendar.ID, calendar.Status)
```

## Add Timed Or All-Day Events

```go
startAt := time.Date(2026, 4, 16, 9, 0, 0, 0, time.Local)
endAt := startAt.Add(time.Hour)
event, err := api.CreateEvent(ctx, sdk.EventInput{
	CalendarID: calendar.Calendar.ID,
	Title:      "Standup",
	StartAt:    &startAt,
	EndAt:      &endAt,
})
if err != nil {
	log.Fatal(err)
}
log.Printf("created event %s", event.ID)
```

```go
startDate := "2026-04-17"
event, err := api.CreateEvent(ctx, sdk.EventInput{
	CalendarID: calendar.Calendar.ID,
	Title:      "Planning day",
	StartDate:  &startDate,
})
if err != nil {
	log.Fatal(err)
}
log.Printf("created all-day event %s", event.ID)
```

## Add Tasks

```go
dueDate := "2026-04-16"
count := int32(5)
task, err := api.CreateTask(ctx, sdk.TaskInput{
	CalendarID: calendar.Calendar.ID,
	Title:      "Review notes",
	DueDate:    &dueDate,
	Recurrence: &sdk.RecurrenceRule{
		Frequency: sdk.RecurrenceFrequencyDaily,
		Count:     &count,
	},
})
if err != nil {
	log.Fatal(err)
}
log.Printf("created task %s", task.ID)
```

## Query Agenda

```go
from := time.Date(2026, 4, 16, 0, 0, 0, 0, time.Local)
to := from.AddDate(0, 0, 7)
agenda, err := api.ListAgenda(ctx, sdk.AgendaOptions{
	From:  from,
	To:    to,
	Limit: 200,
})
if err != nil {
	log.Fatal(err)
}
for _, item := range agenda.Items {
	log.Printf("%s %s %s", item.Kind, item.OccurrenceKey, item.Title)
}
```

## Complete A Task Occurrence

Use an empty completion input for a non-recurring task:

```go
_, err := api.CompleteTask(ctx, task.ID, sdk.TaskCompletionInput{})
if err != nil {
	log.Fatal(err)
}
```

Use the exact occurrence date for a recurring date-based task:

```go
occurrenceDate := "2026-04-16"
_, err := api.CompleteTask(ctx, task.ID, sdk.TaskCompletionInput{
	OccurrenceDate: &occurrenceDate,
})
if err != nil {
	log.Fatal(err)
}
```

## Date Handling

If a user gives a short date like `04/16`, resolve the year from the
conversation or ask for it when the context is ambiguous. Pass OpenPlanner an
explicit `YYYY-MM-DD` date for all-day events and date-based tasks.
