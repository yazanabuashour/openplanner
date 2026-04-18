package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestDefaultParallelism(t *testing.T) {
	t.Parallel()

	if defaultParallel != 4 {
		t.Fatalf("defaultParallel = %d, want 4", defaultParallel)
	}
}

func TestRunJobsPreservesResultOrdering(t *testing.T) {
	t.Parallel()

	jobs := []job{
		{Index: 0, Scenario: scenario{ID: "slow", Title: "Slow"}},
		{Index: 1, Scenario: scenario{ID: "fast", Title: "Fast"}},
		{Index: 2, Scenario: scenario{ID: "middle", Title: "Middle"}},
	}
	results := runJobs(context.Background(), jobs, 3, func(_ context.Context, current job) runResult {
		switch current.Scenario.ID {
		case "slow":
			time.Sleep(30 * time.Millisecond)
		case "middle":
			time.Sleep(10 * time.Millisecond)
		}
		return runResult{Scenario: current.Scenario.ID, ScenarioTitle: current.Scenario.Title}
	})

	for index, want := range []string{"slow", "fast", "middle"} {
		if results[index].Scenario != want {
			t.Fatalf("results[%d].Scenario = %q, want %q", index, results[index].Scenario, want)
		}
	}
}

func TestRunJobsPreservesErrorIdentity(t *testing.T) {
	t.Parallel()

	jobs := []job{
		{Index: 0, Scenario: scenario{ID: "create-event", Title: "Create event"}},
		{Index: 1, Scenario: scenario{ID: "invalid-date", Title: "Invalid date"}},
	}
	results := runJobs(context.Background(), jobs, 2, func(_ context.Context, current job) runResult {
		return runResult{
			Scenario:      current.Scenario.ID,
			ScenarioTitle: current.Scenario.Title,
			Passed:        current.Scenario.ID != "invalid-date",
			Verification: verificationResult{
				Passed:  current.Scenario.ID != "invalid-date",
				Details: current.Scenario.ID + " details",
			},
		}
	})

	if results[1].Scenario != "invalid-date" || results[1].Passed {
		t.Fatalf("error result identity not preserved: %#v", results[1])
	}
	if !strings.Contains(results[1].Verification.Details, "invalid-date") {
		t.Fatalf("verification details = %q", results[1].Verification.Details)
	}
}

func TestReportMetadataIncludesParallelismAndElapsed(t *testing.T) {
	t.Parallel()

	current := report{
		Issue:                 issueID,
		Date:                  "2026-04-18",
		Harness:               harnessID,
		Parallelism:           4,
		HarnessElapsedSeconds: 1.25,
	}
	if current.Parallelism != 4 {
		t.Fatalf("parallelism = %d, want 4", current.Parallelism)
	}
	if current.HarnessElapsedSeconds <= 0 {
		t.Fatalf("elapsed = %f, want positive", current.HarnessElapsedSeconds)
	}
}
