// Package sdk exposes the in-process OpenPlanner runtime for Go callers.
//
// Import github.com/yazanabuashour/openplanner/sdk in application code, then
// call OpenLocal(Options{}) to open a local client backed directly by the local
// planning service. OpenPlanner does not start a daemon, bind a localhost port,
// or require a separate server for the default SDK flow.
//
// For common local planning tasks, prefer the Client helper methods
// EnsureCalendar, CreateEvent, CreateTask, ListAgenda, ListCalendars,
// ListEvents, ListTasks, and CompleteTask.
//
// Agent-driven workflows should use the installed openplanner JSON runner. The
// SDK remains the Go developer surface.
//
// When Options.DatabasePath is empty, OpenPlanner stores SQLite data at
// ${XDG_DATA_HOME:-~/.local/share}/openplanner/openplanner.db.
package sdk
