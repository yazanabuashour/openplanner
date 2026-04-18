package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/yazanabuashour/openplanner/agentops"
)

const (
	issueID         = "op-agentops"
	harnessID       = "openplanner-agentops-runner"
	defaultParallel = 4
)

type scenario struct {
	ID    string
	Title string
}

type report struct {
	Issue                 string      `json:"issue"`
	Date                  string      `json:"date"`
	Harness               string      `json:"harness"`
	Parallelism           int         `json:"parallelism"`
	HarnessElapsedSeconds float64     `json:"harness_elapsed_seconds"`
	Results               []runResult `json:"results"`
	RawLogsCommitted      bool        `json:"raw_logs_committed"`
	RawLogsNote           string      `json:"raw_logs_note"`
}

type runResult struct {
	Scenario                string             `json:"scenario"`
	ScenarioTitle           string             `json:"scenario_title"`
	Passed                  bool               `json:"passed"`
	ExitCode                int                `json:"exit_code"`
	WallSeconds             float64            `json:"wall_seconds"`
	CommandExecutions       int                `json:"command_executions"`
	Verification            verificationResult `json:"verification"`
	RawLogArtifactReference string             `json:"raw_log_artifact_reference"`
}

type verificationResult struct {
	Passed  bool   `json:"passed"`
	Details string `json:"details"`
}

type job struct {
	Index    int
	Scenario scenario
}

type jobRunner func(context.Context, job) runResult

func main() {
	if len(os.Args) < 2 {
		failf("usage: openplanner-agent-eval <run>")
	}
	switch os.Args[1] {
	case "run":
		runCommand(os.Args[2:])
	default:
		failf("unknown command %q", os.Args[1])
	}
}

func runCommand(args []string) {
	flags := flag.NewFlagSet("run", flag.ExitOnError)
	runRootFlag := flags.String("run-root", "", "directory for raw run artifacts outside the repo")
	dateFlag := flags.String("date", time.Now().Format(time.DateOnly), "report date in YYYY-MM-DD form")
	scenarioFilter := flags.String("scenario", "", "optional comma-separated scenario ids to run")
	parallelFlag := flags.Int("parallel", defaultParallel, "number of independent scenarios to run concurrently")
	if err := flags.Parse(args); err != nil {
		failf("parse flags: %v", err)
	}
	if flags.NArg() != 0 {
		failf("run does not accept positional arguments")
	}
	if *parallelFlag <= 0 {
		failf("--parallel must be greater than 0")
	}

	repoRoot, err := repoRoot()
	if err != nil {
		failf("resolve repo root: %v", err)
	}
	runRoot := *runRootFlag
	if runRoot == "" {
		runRoot, err = os.MkdirTemp("", "openplanner-agent-eval-*")
		if err != nil {
			failf("create run root: %v", err)
		}
	} else if err := os.MkdirAll(runRoot, 0o755); err != nil {
		failf("create run root: %v", err)
	}
	runRoot, err = filepath.Abs(runRoot)
	if err != nil {
		failf("absolute run root: %v", err)
	}
	if isWithin(runRoot, repoRoot) {
		failf("run root must be outside the repository: %s", runRoot)
	}

	selected, err := selectScenarios(*scenarioFilter)
	if err != nil {
		failf("select scenarios: %v", err)
	}
	jobs := make([]job, 0, len(selected))
	for index, selectedScenario := range selected {
		jobs = append(jobs, job{Index: index, Scenario: selectedScenario})
	}

	started := time.Now()
	results := runJobs(context.Background(), jobs, *parallelFlag, func(ctx context.Context, current job) runResult {
		return runOne(ctx, repoRoot, runRoot, current)
	})
	elapsed := time.Since(started).Seconds()

	out := report{
		Issue:                 issueID,
		Date:                  *dateFlag,
		Harness:               harnessID,
		Parallelism:           *parallelFlag,
		HarnessElapsedSeconds: elapsed,
		Results:               results,
		RawLogsCommitted:      false,
		RawLogsNote:           "Raw logs are stored outside the repo and referenced with <run-root> placeholders.",
	}

	outDir := filepath.Join(repoRoot, "docs", "agent-eval-results")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		failf("create report dir: %v", err)
	}
	jsonPath := filepath.Join(outDir, fmt.Sprintf("%s-%s.json", issueID, *dateFlag))
	mdPath := filepath.Join(outDir, fmt.Sprintf("%s-%s.md", issueID, *dateFlag))
	if err := writeJSON(jsonPath, out); err != nil {
		failf("write json report: %v", err)
	}
	if err := writeMarkdown(mdPath, out); err != nil {
		failf("write markdown report: %v", err)
	}
}

func runJobs(ctx context.Context, jobs []job, parallel int, runner jobRunner) []runResult {
	if parallel <= 0 {
		parallel = 1
	}
	results := make([]runResult, len(jobs))
	jobCh := make(chan job)
	var wg sync.WaitGroup
	for worker := 0; worker < parallel; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for current := range jobCh {
				results[current.Index] = runner(ctx, current)
			}
		}()
	}
	for _, current := range jobs {
		jobCh <- current
	}
	close(jobCh)
	wg.Wait()
	return results
}

func runOne(ctx context.Context, repoRoot string, runRoot string, current job) runResult {
	started := time.Now()
	jobRoot := filepath.Join(runRoot, current.Scenario.ID)
	runRepo := filepath.Join(jobRoot, "repo")
	rawLog := filepath.Join(jobRoot, "raw.log")
	result := runResult{
		Scenario:                current.Scenario.ID,
		ScenarioTitle:           current.Scenario.Title,
		ExitCode:                -1,
		RawLogArtifactReference: filepath.ToSlash(filepath.Join("<run-root>", current.Scenario.ID, "raw.log")),
	}
	if err := os.MkdirAll(jobRoot, 0o755); err != nil {
		result.Verification = verificationResult{Details: fmt.Sprintf("create job root: %v", err)}
		return finishResult(result, started)
	}
	if err := copyRepo(repoRoot, runRepo); err != nil {
		result.Verification = verificationResult{Details: fmt.Sprintf("copy repo: %v", err)}
		return finishResult(result, started)
	}

	dbPath := filepath.Join(jobRoot, "openplanner.db")
	logFile, err := os.Create(rawLog)
	if err != nil {
		result.Verification = verificationResult{Details: fmt.Sprintf("create raw log: %v", err)}
		return finishResult(result, started)
	}
	defer func() {
		_ = logFile.Close()
	}()

	switch current.Scenario.ID {
	case "ensure-calendar":
		result = runEnsureCalendarScenario(ctx, runRepo, dbPath, logFile, result)
	case "create-event":
		result = runCreateEventScenario(ctx, runRepo, dbPath, logFile, result)
	case "create-task":
		result = runCreateTaskScenario(ctx, runRepo, dbPath, logFile, result)
	case "agenda-range":
		result = runAgendaRangeScenario(ctx, runRepo, dbPath, logFile, result)
	case "complete-task":
		result = runCompleteTaskScenario(ctx, runRepo, dbPath, logFile, result)
	case "invalid-date":
		result = runInvalidDateScenario(ctx, runRepo, dbPath, logFile, result)
	default:
		result.Verification = verificationResult{Details: "unknown scenario"}
	}
	return finishResult(result, started)
}

func runEnsureCalendarScenario(ctx context.Context, repo string, dbPath string, log io.Writer, result runResult) runResult {
	first, code, err := runPlanning(ctx, repo, dbPath, log, agentops.PlanningTaskRequest{
		Action:       agentops.PlanningTaskActionEnsureCalendar,
		CalendarName: "Personal",
	})
	result.ExitCode = code
	result.CommandExecutions++
	if err != nil {
		result.Verification = verificationResult{Details: err.Error()}
		return result
	}
	second, code, err := runPlanning(ctx, repo, dbPath, log, agentops.PlanningTaskRequest{
		Action:       agentops.PlanningTaskActionEnsureCalendar,
		CalendarName: "Personal",
	})
	result.ExitCode = code
	result.CommandExecutions++
	if err != nil {
		result.Verification = verificationResult{Details: err.Error()}
		return result
	}
	passed := len(first.Writes) == 1 && first.Writes[0].Status == "created" &&
		len(second.Writes) == 1 && second.Writes[0].Status == "already_exists"
	result.Passed = passed
	result.Verification = verificationResult{Passed: passed, Details: "expected created then already_exists"}
	return result
}

func runCreateEventScenario(ctx context.Context, repo string, dbPath string, log io.Writer, result runResult) runResult {
	output, code, err := runPlanning(ctx, repo, dbPath, log, agentops.PlanningTaskRequest{
		Action:       agentops.PlanningTaskActionCreateEvent,
		CalendarName: "Work",
		Title:        "Standup",
		StartAt:      "2026-04-16T09:00:00Z",
		EndAt:        "2026-04-16T10:00:00Z",
	})
	result.ExitCode = code
	result.CommandExecutions++
	if err != nil {
		result.Verification = verificationResult{Details: err.Error()}
		return result
	}
	passed := !output.Rejected && len(output.Events) == 1 && output.Events[0].Title == "Standup"
	result.Passed = passed
	result.Verification = verificationResult{Passed: passed, Details: "expected one created event"}
	return result
}

func runCreateTaskScenario(ctx context.Context, repo string, dbPath string, log io.Writer, result runResult) runResult {
	output, code, err := runPlanning(ctx, repo, dbPath, log, agentops.PlanningTaskRequest{
		Action:       agentops.PlanningTaskActionCreateTask,
		CalendarName: "Personal",
		Title:        "Review notes",
		DueDate:      "2026-04-16",
	})
	result.ExitCode = code
	result.CommandExecutions++
	if err != nil {
		result.Verification = verificationResult{Details: err.Error()}
		return result
	}
	passed := !output.Rejected && len(output.Tasks) == 1 && output.Tasks[0].DueDate == "2026-04-16"
	result.Passed = passed
	result.Verification = verificationResult{Passed: passed, Details: "expected one dated task"}
	return result
}

func runAgendaRangeScenario(ctx context.Context, repo string, dbPath string, log io.Writer, result runResult) runResult {
	_, code, err := runPlanning(ctx, repo, dbPath, log, agentops.PlanningTaskRequest{
		Action:       agentops.PlanningTaskActionCreateEvent,
		CalendarName: "Work",
		Title:        "Standup",
		StartAt:      "2026-04-16T09:00:00Z",
		EndAt:        "2026-04-16T10:00:00Z",
	})
	result.ExitCode = code
	result.CommandExecutions++
	if err != nil {
		result.Verification = verificationResult{Details: err.Error()}
		return result
	}
	_, code, err = runPlanning(ctx, repo, dbPath, log, agentops.PlanningTaskRequest{
		Action:       agentops.PlanningTaskActionCreateTask,
		CalendarName: "Work",
		Title:        "Review notes",
		DueDate:      "2026-04-16",
	})
	result.ExitCode = code
	result.CommandExecutions++
	if err != nil {
		result.Verification = verificationResult{Details: err.Error()}
		return result
	}
	agenda, code, err := runPlanning(ctx, repo, dbPath, log, agentops.PlanningTaskRequest{
		Action: agentops.PlanningTaskActionListAgenda,
		From:   "2026-04-16T00:00:00Z",
		To:     "2026-04-17T00:00:00Z",
	})
	result.ExitCode = code
	result.CommandExecutions++
	if err != nil {
		result.Verification = verificationResult{Details: err.Error()}
		return result
	}
	passed := !agenda.Rejected && len(agenda.Agenda) == 2 && agenda.Agenda[0].Title == "Review notes" && agenda.Agenda[1].Title == "Standup"
	result.Passed = passed
	result.Verification = verificationResult{Passed: passed, Details: "expected date task then timed event"}
	return result
}

func runCompleteTaskScenario(ctx context.Context, repo string, dbPath string, log io.Writer, result runResult) runResult {
	task, code, err := runPlanning(ctx, repo, dbPath, log, agentops.PlanningTaskRequest{
		Action:       agentops.PlanningTaskActionCreateTask,
		CalendarName: "Personal",
		Title:        "Review notes",
		DueDate:      "2026-04-16",
	})
	result.ExitCode = code
	result.CommandExecutions++
	if err != nil {
		result.Verification = verificationResult{Details: err.Error()}
		return result
	}
	if len(task.Tasks) != 1 {
		result.Verification = verificationResult{Details: "task was not created"}
		return result
	}
	completed, code, err := runPlanning(ctx, repo, dbPath, log, agentops.PlanningTaskRequest{
		Action: agentops.PlanningTaskActionCompleteTask,
		TaskID: task.Tasks[0].ID,
	})
	result.ExitCode = code
	result.CommandExecutions++
	if err != nil {
		result.Verification = verificationResult{Details: err.Error()}
		return result
	}
	passed := !completed.Rejected && len(completed.Writes) == 1 && completed.Writes[0].Status == "completed"
	result.Passed = passed
	result.Verification = verificationResult{Passed: passed, Details: "expected completed task write"}
	return result
}

func runInvalidDateScenario(ctx context.Context, repo string, dbPath string, log io.Writer, result runResult) runResult {
	output, code, err := runPlanning(ctx, repo, dbPath, log, agentops.PlanningTaskRequest{
		Action:       agentops.PlanningTaskActionCreateTask,
		CalendarName: "Personal",
		Title:        "Review notes",
		DueDate:      "04/16",
	})
	result.ExitCode = code
	result.CommandExecutions++
	if err != nil {
		result.Verification = verificationResult{Details: err.Error()}
		return result
	}
	passed := output.Rejected && strings.Contains(output.RejectionReason, "YYYY-MM-DD")
	result.Passed = passed
	result.Verification = verificationResult{Passed: passed, Details: "expected date validation rejection"}
	return result
}

func runPlanning(ctx context.Context, repo string, dbPath string, log io.Writer, request agentops.PlanningTaskRequest) (agentops.PlanningTaskResult, int, error) {
	payload, err := json.Marshal(request)
	if err != nil {
		return agentops.PlanningTaskResult{}, -1, err
	}
	command := exec.CommandContext(ctx, "go", "run", "./cmd/openplanner-agentops", "planning", "--db", dbPath)
	command.Dir = repo
	command.Stdin = bytes.NewReader(payload)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	exitCode := 0
	if err := command.Run(); err != nil {
		exitCode = 1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
		_, _ = fmt.Fprintf(log, "$ go run ./cmd/openplanner-agentops planning --db <db>\n%s\n%s\n", payload, stderr.String())
		return agentops.PlanningTaskResult{}, exitCode, fmt.Errorf("runner failed: %w", err)
	}
	_, _ = fmt.Fprintf(log, "$ go run ./cmd/openplanner-agentops planning --db <db>\n%s\n%s\n", payload, stdout.String())
	var result agentops.PlanningTaskResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return agentops.PlanningTaskResult{}, exitCode, fmt.Errorf("decode runner JSON: %w", err)
	}
	return result, exitCode, nil
}

func scenarios() []scenario {
	return []scenario{
		{ID: "ensure-calendar", Title: "Ensure a calendar twice without duplicates"},
		{ID: "create-event", Title: "Create a timed event with calendar-name ensure"},
		{ID: "create-task", Title: "Create a dated task with calendar-name ensure"},
		{ID: "agenda-range", Title: "List a bounded agenda range chronologically"},
		{ID: "complete-task", Title: "Complete a non-recurring task"},
		{ID: "invalid-date", Title: "Reject an ambiguous short date without writing"},
	}
}

func selectScenarios(filter string) ([]scenario, error) {
	all := scenarios()
	if strings.TrimSpace(filter) == "" {
		return all, nil
	}
	allowed := map[string]scenario{}
	for _, current := range all {
		allowed[current.ID] = current
	}
	selected := []scenario{}
	for _, raw := range strings.Split(filter, ",") {
		id := strings.TrimSpace(raw)
		current, ok := allowed[id]
		if !ok {
			return nil, fmt.Errorf("unknown scenario %q", id)
		}
		selected = append(selected, current)
	}
	return selected, nil
}

func finishResult(result runResult, started time.Time) runResult {
	result.WallSeconds = mathRound(time.Since(started).Seconds())
	result.Verification.Passed = result.Passed
	return result
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func writeMarkdown(path string, report report) error {
	var out strings.Builder
	fmt.Fprintf(&out, "# OpenPlanner AgentOps Eval %s\n\n", report.Date)
	fmt.Fprintf(&out, "- Harness: `%s`\n", report.Harness)
	fmt.Fprintf(&out, "- Parallelism: `%d`\n", report.Parallelism)
	fmt.Fprintf(&out, "- Harness elapsed seconds: `%.2f`\n", report.HarnessElapsedSeconds)
	fmt.Fprintf(&out, "- Raw logs committed: `%t`\n", report.RawLogsCommitted)
	fmt.Fprintf(&out, "- Raw logs note: %s\n\n", report.RawLogsNote)
	out.WriteString("| Scenario | Passed | Commands | Wall Seconds | Details |\n")
	out.WriteString("| --- | ---: | ---: | ---: | --- |\n")
	for _, result := range report.Results {
		fmt.Fprintf(&out, "| `%s` | %t | %d | %.2f | %s |\n",
			result.Scenario,
			result.Passed,
			result.CommandExecutions,
			result.WallSeconds,
			escapeMarkdownTable(result.Verification.Details),
		)
	}
	return os.WriteFile(path, []byte(out.String()), 0o644)
}

func copyRepo(src string, dst string) error {
	return filepath.WalkDir(src, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dst, 0o755)
		}
		if shouldSkipCopy(rel, entry) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(dst, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		return copyFile(path, target, info.Mode().Perm())
	})
}

func shouldSkipCopy(rel string, entry fs.DirEntry) bool {
	clean := filepath.ToSlash(rel)
	switch clean {
	case ".git", ".beads", ".agents", "AGENTS.md", "docs/agent-evals.md", "docs/agent-eval-results", "scripts/agent-eval":
		return true
	}
	return strings.HasPrefix(clean, ".git/") ||
		strings.HasPrefix(clean, ".beads/") ||
		strings.HasPrefix(clean, ".agents/") ||
		strings.HasPrefix(clean, "docs/agent-eval-results/") ||
		strings.HasPrefix(clean, "scripts/agent-eval/")
}

func copyFile(src string, dst string, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	input, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		_ = input.Close()
	}()
	output, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer func() {
		_ = output.Close()
	}()
	_, err = io.Copy(output, input)
	return err
}

func repoRoot() (string, error) {
	output, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func isWithin(path string, parent string) bool {
	rel, err := filepath.Rel(parent, path)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..")
}

func escapeMarkdownTable(value string) string {
	return strings.ReplaceAll(value, "|", "\\|")
}

func mathRound(value float64) float64 {
	return float64(int(value*100+0.5)) / 100
}

func failf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
