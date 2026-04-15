package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/yazanabuashour/openplanner/internal/api"
	"github.com/yazanabuashour/openplanner/internal/service"
	"github.com/yazanabuashour/openplanner/internal/store"
	"github.com/yazanabuashour/openplanner/sdk/generated"
)

func newTestHandler(t *testing.T) http.Handler {
	t.Helper()

	repository, err := store.Open(filepath.Join(t.TempDir(), "openplanner.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := repository.Close(); closeErr != nil {
			t.Fatalf("close store: %v", closeErr)
		}
	})

	return api.NewHandler(service.New(repository))
}

func TestListTasksRejectsInvalidLimit(t *testing.T) {
	t.Parallel()

	handler := newTestHandler(t)
	tests := []string{"abc", "0", "500"}

	for _, raw := range tests {
		t.Run(raw, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/v1/tasks?limit="+raw, nil)
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
			}

			var problem generated.Problem
			if err := json.NewDecoder(response.Body).Decode(&problem); err != nil {
				t.Fatalf("decode problem: %v", err)
			}
			if problem.Code != "validation_error" {
				t.Fatalf("problem code = %q, want validation_error", problem.Code)
			}
			if len(problem.FieldErrors) == 0 || problem.FieldErrors[0].Field != "limit" {
				t.Fatalf("field errors = %#v, want limit validation", problem.FieldErrors)
			}
		})
	}
}
