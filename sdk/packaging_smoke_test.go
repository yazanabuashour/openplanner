package sdk_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSDKPackageCanBeConsumedFromTempModule(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	repoRoot := filepath.Dir(mustGetwd(t))

	if err := os.WriteFile(filepath.Join(workDir, "main.go"), []byte(`package main

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/yazanabuashour/openplanner/sdk"
)

func main() {
	client, err := sdk.OpenLocal(sdk.Options{DatabasePath: filepath.Join(".", "smoke.db")})
	if err != nil {
		panic(err)
	}
	defer client.Close()

	ctx := context.Background()
	calendar, err := client.EnsureCalendar(ctx, sdk.CalendarInput{Name: "Smoke"})
	if err != nil {
		panic(err)
	}

	startAt := time.Date(2026, 4, 16, 9, 0, 0, 0, time.UTC)
	endAt := startAt.Add(time.Hour)
	count := int32(1)
	if _, err := client.CreateEvent(ctx, sdk.EventInput{
		CalendarID: calendar.Calendar.ID,
		Title:      "Checkin",
		StartAt:    &startAt,
		EndAt:      &endAt,
		Recurrence: &sdk.RecurrenceRule{Frequency: sdk.RecurrenceFrequencyDaily, Count: &count},
	}); err != nil {
		panic(err)
	}

	agenda, err := client.ListAgenda(ctx, sdk.AgendaOptions{
		From: time.Date(2026, 4, 16, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 4, 17, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		panic(err)
	}

	fmt.Printf("agenda=%d\n", len(agenda.Items))
}
`), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}

	runCommand(t, workDir, "go", "mod", "init", "smoke")
	runCommand(t, workDir, "go", "mod", "edit", "-replace=github.com/yazanabuashour/openplanner="+repoRoot)
	runCommand(t, workDir, "go", "get", "github.com/yazanabuashour/openplanner/sdk@v0.0.0")
	output := runCommand(t, workDir, "go", "run", "-mod=mod", ".")

	if !strings.Contains(string(output), "agenda=1") {
		t.Fatalf("unexpected output: %s", output)
	}
	if _, err := os.Stat(filepath.Join(workDir, "smoke.db")); err != nil {
		t.Fatalf("smoke.db missing: %v", err)
	}
}

func mustGetwd(t *testing.T) string {
	t.Helper()

	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	return workingDirectory
}

func runCommand(t *testing.T, dir string, name string, args ...string) []byte {
	t.Helper()

	command := exec.Command(name, args...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, output)
	}

	return output
}
