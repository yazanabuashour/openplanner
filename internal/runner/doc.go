// Package runner exposes JSON-friendly task facades for production agent
// workflows.
//
// Agents should call these facades through the installed openplanner runner so
// each routine task is one structured JSON command and response. Go
// applications should continue to import github.com/yazanabuashour/openplanner/sdk
// directly.
package runner
