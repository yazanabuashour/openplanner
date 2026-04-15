package openapi_test

import (
	"os"
	"strings"
	"testing"
)

func readSpec(t *testing.T) string {
	t.Helper()

	content, err := os.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}

	return string(content)
}

func getPathBlock(spec, path string) string {
	lines := strings.Split(spec, "\n")
	pathMarker := "  " + path + ":"
	start := -1
	for index, line := range lines {
		if line == pathMarker {
			start = index
			break
		}
	}
	if start == -1 {
		return ""
	}

	end := len(lines)
	for index := start + 1; index < len(lines); index++ {
		if strings.HasPrefix(lines[index], "  /v1/") {
			end = index
			break
		}
	}

	return strings.Join(lines[start:end], "\n")
}

func TestOpenAPITracksRecurringTaskCompletion(t *testing.T) {
	t.Parallel()

	spec := readSpec(t)
	section := getPathBlock(spec, "/v1/tasks/{taskId}/complete")
	if section == "" {
		t.Fatal("task completion path missing")
	}
	if !strings.Contains(section, "CompleteTaskRequest") {
		t.Fatal("task completion request schema missing")
	}
	if !strings.Contains(section, "TaskCompletion") {
		t.Fatal("task completion response schema missing")
	}
}

func TestOpenAPIDocumentsOpaquePagination(t *testing.T) {
	t.Parallel()

	spec := readSpec(t)
	if !strings.Contains(spec, "nextCursor") {
		t.Fatal("nextCursor field missing from spec")
	}
	if !strings.Contains(spec, "Opaque cursor token.") {
		t.Fatal("cursor token description missing")
	}
}

func TestOpenAPIPinsProblemEnvelopeAndULIDs(t *testing.T) {
	t.Parallel()

	spec := readSpec(t)
	if !strings.Contains(spec, "fieldErrors") {
		t.Fatal("problem fieldErrors missing")
	}
	if !strings.Contains(spec, "code:") {
		t.Fatal("problem code field missing")
	}
	if !strings.Contains(spec, "^[0-9A-HJKMNP-TV-Z]{26}$") {
		t.Fatal("ULID resource id pattern missing")
	}
}

func TestOpenAPIPinsRecurrenceRuleShape(t *testing.T) {
	t.Parallel()

	spec := readSpec(t)
	if !strings.Contains(spec, "RecurrenceRule:") {
		t.Fatal("recurrence rule schema missing")
	}
	if !strings.Contains(spec, "enum: [daily, weekly, monthly]") {
		t.Fatal("recurrence frequency enum missing")
	}
	if !strings.Contains(spec, "byWeekday") || !strings.Contains(spec, "byMonthDay") {
		t.Fatal("bounded recurrence selectors missing")
	}
}
