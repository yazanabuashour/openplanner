// Package sdk exposes the in-process OpenPlanner runtime for Go callers.
//
// Import github.com/yazanabuashour/openplanner/sdk in application code, then
// call OpenLocal(Options{}) to open a generated API client backed by the local
// in-process transport. OpenPlanner does not start a daemon, bind a localhost
// port, or require a separate server for the default SDK flow.
//
// Use request and response types from
// github.com/yazanabuashour/openplanner/sdk/generated when building API calls.
//
// When Options.DatabasePath is empty, OpenPlanner stores SQLite data at
// ${XDG_DATA_HOME:-~/.local/share}/openplanner/openplanner.db.
package sdk
