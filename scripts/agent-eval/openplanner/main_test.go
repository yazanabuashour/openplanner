package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yazanabuashour/openplanner/internal/runner"
)

func TestParseRunOptionsDefaultsAndValidation(t *testing.T) {
	t.Parallel()

	options, err := parseRunOptions([]string{})
	if err != nil {
		t.Fatalf("parseRunOptions default: %v", err)
	}
	if options.Parallelism != defaultRunParallelism {
		t.Fatalf("parallelism = %d, want %d", options.Parallelism, defaultRunParallelism)
	}
	if options.CacheMode != cacheModeShared {
		t.Fatalf("cache mode = %q, want shared", options.CacheMode)
	}

	options, err = parseRunOptions([]string{"--parallel", "1", "--cache-mode", "isolated", "--scenario", "ensure-calendar"})
	if err != nil {
		t.Fatalf("parseRunOptions explicit: %v", err)
	}
	if options.Parallelism != 1 || options.CacheMode != cacheModeIsolated || options.ScenarioFilter != "ensure-calendar" {
		t.Fatalf("options = %#v", options)
	}

	if _, err := parseRunOptions([]string{"--parallel", "0"}); err == nil || !strings.Contains(err.Error(), "parallel") {
		t.Fatalf("parseRunOptions --parallel 0 error = %v, want validation error", err)
	}
	if _, err := parseRunOptions([]string{"--cache-mode", "bad"}); err == nil || !strings.Contains(err.Error(), "cache-mode") {
		t.Fatalf("parseRunOptions --cache-mode bad error = %v, want validation error", err)
	}
}

func TestParseScaleOptionsDefaultsAndValidation(t *testing.T) {
	t.Parallel()

	options, err := parseScaleOptions([]string{})
	if err != nil {
		t.Fatalf("parseScaleOptions default: %v", err)
	}
	if options.Events != defaultScaleEvents || options.Tasks != defaultScaleTasks || options.Recurring != defaultScaleRecurring {
		t.Fatalf("default scale options = %#v", options)
	}
	if options.Completions != defaultScaleCompletions || options.Limit != defaultScaleLimit {
		t.Fatalf("default scale options = %#v", options)
	}

	options, err = parseScaleOptions([]string{"--run-root", "root", "--date", "2026-04-20", "--events", "10", "--tasks", "11", "--recurring", "3", "--completions", "4", "--limit", "5"})
	if err != nil {
		t.Fatalf("parseScaleOptions explicit: %v", err)
	}
	if options.RunRoot != "root" || options.Date != "2026-04-20" || options.Events != 10 || options.Tasks != 11 || options.Recurring != 3 || options.Completions != 4 || options.Limit != 5 {
		t.Fatalf("explicit scale options = %#v", options)
	}

	for _, args := range [][]string{
		{"--events", "0"},
		{"--tasks", "0"},
		{"--recurring", "0"},
		{"--completions", "-1"},
		{"--limit", "0"},
		{"--limit", "201"},
	} {
		if _, err := parseScaleOptions(args); err == nil {
			t.Fatalf("parseScaleOptions(%v) error = nil, want validation error", args)
		}
	}
}

func TestScaleResultsPassFail(t *testing.T) {
	t.Parallel()

	passed := []scaleResult{{Scenario: "a", Passed: true}, {Scenario: "b", Passed: true}}
	if !scaleResultsPassed(passed) {
		t.Fatalf("scaleResultsPassed returned false for all-pass results")
	}
	failed := []scaleResult{{Scenario: "a", Passed: true}, {Scenario: "b", Passed: false}}
	if scaleResultsPassed(failed) {
		t.Fatalf("scaleResultsPassed returned true for failed results")
	}
	failures := failedScaleResults(failed)
	if len(failures) != 1 || failures[0].Scenario != "b" {
		t.Fatalf("failedScaleResults = %#v", failures)
	}
}

func TestFirstBeadsIDIgnoresWarnings(t *testing.T) {
	t.Parallel()

	output := "op-abc\nWarning: auto-export: git add failed: exit status 1"
	if got := firstBeadsID(output); got != "op-abc" {
		t.Fatalf("firstBeadsID() = %q, want op-abc", got)
	}
}

func TestSelectScenariosRejectsEmptyFilter(t *testing.T) {
	t.Parallel()

	if _, err := selectScenarios(", ,"); err == nil || !strings.Contains(err.Error(), "scenario filter") {
		t.Fatalf("selectScenarios empty filter error = %v, want validation error", err)
	}
}

func TestRunEvalJobsPreservesResultOrderingAndErrors(t *testing.T) {
	t.Parallel()

	jobs := []evalJob{
		{Index: 0, Scenario: scenario{ID: "slow", Title: "Slow"}},
		{Index: 1, Scenario: scenario{ID: "fast", Title: "Fast"}},
		{Index: 2, Scenario: scenario{ID: "boom", Title: "Boom"}},
	}
	results := runEvalJobs("repo", "run", jobs, 3, cacheConfig{Mode: cacheModeIsolated, RunRoot: "run"}, func(_ string, _ string, sc scenario, _ cacheConfig) (runResult, error) {
		switch sc.ID {
		case "slow":
			time.Sleep(30 * time.Millisecond)
		case "boom":
			return runResult{}, errors.New("boom")
		}
		return runResult{Scenario: sc.ID, ScenarioTitle: sc.Title, Passed: true}, nil
	})

	for index, want := range []string{"slow", "fast", "boom"} {
		if results[index].Scenario != want {
			t.Fatalf("results[%d].Scenario = %q, want %q", index, results[index].Scenario, want)
		}
	}
	if results[2].Passed || !strings.Contains(results[2].Verification.Details, "boom") {
		t.Fatalf("harness error result = %#v", results[2])
	}
}

func TestCopyRepoSkipsVariantContaminatingInstructions(t *testing.T) {
	t.Parallel()

	temp := t.TempDir()
	src := filepath.Join(temp, "src")
	dst := filepath.Join(temp, "dst")
	for _, path := range []string{
		filepath.Join(src, ".agents", "skills", "openplanner"),
		filepath.Join(src, "docs", "agent-eval-results"),
	} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for path, content := range map[string]string{
		filepath.Join(src, "AGENTS.md"):                                       "repo agent instructions",
		filepath.Join(src, "README.md"):                                       "kept",
		filepath.Join(src, ".agents", "skills", "openplanner", "SKILL.md"):    "stale skill",
		filepath.Join(src, "docs", "agent-eval-results", "previous.md"):       "previous report",
		filepath.Join(src, "docs", "agent-evals.md"):                          "eval docs",
		filepath.Join(src, "scripts", "agent-eval", "openplanner", "main.go"): "harness",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := copyRepo(src, dst); err != nil {
		t.Fatalf("copyRepo() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "README.md")); err != nil {
		t.Fatalf("kept file stat error = %v", err)
	}
	for _, skipped := range []string{
		"AGENTS.md",
		filepath.Join(".agents", "skills", "openplanner", "SKILL.md"),
		filepath.Join("docs", "agent-eval-results", "previous.md"),
		filepath.Join("docs", "agent-evals.md"),
		filepath.Join("scripts", "agent-eval", "openplanner", "main.go"),
	} {
		if _, err := os.Stat(filepath.Join(dst, skipped)); !os.IsNotExist(err) {
			t.Fatalf("copied skipped path %s: stat error = %v", skipped, err)
		}
	}
}

func TestInstallEvalSkillInstallsExactProductionSkillWithoutAgentsFile(t *testing.T) {
	t.Parallel()

	runRepo := filepath.Join(t.TempDir(), "repo")
	sourceSkillDir := filepath.Join(runRepo, "skills", "openplanner")
	if err := os.MkdirAll(sourceSkillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sourceSkill := []byte("---\nname: openplanner\ndescription: test\n---\n# Skill\n")
	if err := os.WriteFile(filepath.Join(sourceSkillDir, "SKILL.md"), sourceSkill, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := installEvalSkill(runRepo); err != nil {
		t.Fatalf("installEvalSkill: %v", err)
	}
	installed, err := os.ReadFile(filepath.Join(runRepo, ".agents", "skills", "openplanner", "SKILL.md"))
	if err != nil {
		t.Fatalf("read installed skill: %v", err)
	}
	if !bytes.Equal(installed, sourceSkill) {
		t.Fatalf("installed skill = %q, want exact source skill", installed)
	}
	if _, err := os.Stat(filepath.Join(runRepo, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("AGENTS.md stat error = %v, want not exist", err)
	}
}

func TestPromptInputPreflightFlagsOpenPlannerAgentsInstructions(t *testing.T) {
	t.Parallel()

	clean := `{"text":"<skills_instructions>- openplanner: Use this skill. (file: /tmp/run/repo/.agents/skills/openplanner/SKILL.md)</skills_instructions>"}`
	if containsOpenPlannerAgentsInstructions(clean) {
		t.Fatalf("clean rendered prompt flagged as contaminated")
	}
	contaminated := `{"text":"# AGENTS.md instructions for /tmp/run/repo\n\n<INSTRUCTIONS>\nFor valid tasks, pipe JSON to openplanner planning.\n{\"action\":\"list_agenda\"}\n</INSTRUCTIONS>"}`
	if !containsOpenPlannerAgentsInstructions(contaminated) {
		t.Fatalf("contaminated rendered prompt was not flagged")
	}
}

func TestPreflightEvalContextRejectsMismatchedSkillAndAgentsFile(t *testing.T) {
	t.Parallel()

	temp := t.TempDir()
	repoRoot := filepath.Join(temp, "src")
	runRepo := filepath.Join(temp, "run")
	sourceSkillDir := filepath.Join(repoRoot, "skills", "openplanner")
	installedSkillDir := filepath.Join(runRepo, ".agents", "skills", "openplanner")
	for _, dir := range []string{sourceSkillDir, installedSkillDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	sourceSkill := []byte("---\nname: openplanner\ndescription: test\n---\n# Skill\n")
	if err := os.WriteFile(filepath.Join(sourceSkillDir, "SKILL.md"), sourceSkill, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installedSkillDir, "SKILL.md"), []byte("different"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := preflightEvalContext(repoRoot, runRepo, filepath.Join(temp, "run-dir"), cacheConfig{Mode: cacheModeIsolated, RunRoot: temp})
	if err == nil || !strings.Contains(err.Error(), "installed production skill") {
		t.Fatalf("preflight mismatched skill error = %v, want installed skill mismatch", err)
	}

	if err := os.WriteFile(filepath.Join(installedSkillDir, "SKILL.md"), sourceSkill, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runRepo, "AGENTS.md"), []byte("product instructions"), 0o644); err != nil {
		t.Fatal(err)
	}
	err = preflightEvalContext(repoRoot, runRepo, filepath.Join(temp, "run-dir"), cacheConfig{Mode: cacheModeIsolated, RunRoot: temp})
	if err == nil || !strings.Contains(err.Error(), "must not contain AGENTS.md") {
		t.Fatalf("preflight AGENTS.md error = %v, want AGENTS.md rejection", err)
	}
}

func TestCodexArgsForSingleAndMultiTurn(t *testing.T) {
	t.Parallel()

	cache := cacheConfig{Mode: cacheModeShared, RunRoot: "run-root"}
	single := scenario{ID: "single", Prompt: "single prompt"}
	singleArgs := codexArgsForTurn("run-root/production/single/repo", "run-root/production/single", single, scenarioTurn{Prompt: "single prompt"}, 1, "", cache)
	if !containsArg(singleArgs, "--ephemeral") {
		t.Fatalf("single-turn args missing --ephemeral: %v", singleArgs)
	}
	if !containsArgPair(singleArgs, "--add-dir", filepath.Join("run-root", "shared-cache")) {
		t.Fatalf("single-turn args missing shared cache writable root: %v", singleArgs)
	}

	multi := scenario{ID: "multi", Turns: []scenarioTurn{{Prompt: "first"}, {Prompt: "second"}}}
	firstArgs := codexArgsForTurn("run-root/production/multi/repo", "run-root/production/multi", multi, scenarioTurn{Prompt: "first"}, 1, "", cache)
	if containsArg(firstArgs, "--ephemeral") {
		t.Fatalf("first multi-turn args must persist session: %v", firstArgs)
	}
	resumeArgs := codexArgsForTurn("run-root/production/multi/repo", "run-root/production/multi", multi, scenarioTurn{Prompt: "second"}, 2, "session-123", cache)
	if len(resumeArgs) < 5 || resumeArgs[0] != "exec" || resumeArgs[1] != "-C" || resumeArgs[2] != "run-root/production/multi/repo" {
		t.Fatalf("resume args must set workspace before resume: %v", resumeArgs)
	}
	if !containsArgPair(resumeArgs, "--add-dir", "run-root/production/multi") {
		t.Fatalf("resume args missing run dir writable root: %v", resumeArgs)
	}
	if containsArg(resumeArgs, "--ephemeral") {
		t.Fatalf("resume args must not be ephemeral: %v", resumeArgs)
	}
	if resumeArgs[len(resumeArgs)-2] != "session-123" || resumeArgs[len(resumeArgs)-1] != "second" {
		t.Fatalf("resume args must end with session id and prompt: %v", resumeArgs)
	}
}

func TestEvalEnvCacheModesAndPrewarmArgs(t *testing.T) {
	t.Parallel()

	runRoot := "run-root"
	runDir := filepath.Join(runRoot, "production", "scenario")
	dbPath := filepath.Join(runDir, "repo", "openplanner.db")

	sharedEnv := strings.Join(evalEnv(runDir, dbPath, cacheConfig{Mode: cacheModeShared, RunRoot: runRoot}), "\n")
	for _, want := range []string{
		"OPENPLANNER_DATABASE_PATH=" + dbPath,
		"GOCACHE=" + filepath.Join(runRoot, "shared-cache", "gocache"),
		"GOMODCACHE=" + filepath.Join(runRoot, "shared-cache", "gomodcache"),
		"TMPDIR=" + filepath.Join(runDir, "tmp"),
	} {
		if !strings.Contains(sharedEnv, want) {
			t.Fatalf("shared env missing %q in %s", want, sharedEnv)
		}
	}

	isolatedEnv := strings.Join(evalEnv(runDir, dbPath, cacheConfig{Mode: cacheModeIsolated, RunRoot: runRoot}), "\n")
	for _, want := range []string{
		"GOCACHE=" + filepath.Join(runDir, "gocache"),
		"GOMODCACHE=" + filepath.Join(runDir, "gomodcache"),
	} {
		if !strings.Contains(isolatedEnv, want) {
			t.Fatalf("isolated env missing %q in %s", want, isolatedEnv)
		}
	}

	args := strings.Join(prewarmCompileArgs(), " ")
	for _, want := range prewarmCompilePackages {
		if !strings.Contains(args, want) {
			t.Fatalf("prewarm args = %q, want package %q", args, want)
		}
	}
}

func TestAggregateMetricsAndProductionScore(t *testing.T) {
	t.Parallel()

	input := 100
	cached := 40
	nonCached := 60
	output := 20
	valid := runResult{
		Scenario: "create-dated-task",
		Passed:   true,
		Metrics: metrics{
			UsageExposed:         true,
			InputTokens:          &input,
			CachedInputTokens:    &cached,
			NonCachedInputTokens: &nonCached,
			OutputTokens:         &output,
			EventTypeCounts:      map[string]int{},
		},
	}
	invalid := runResult{
		Scenario: "ambiguous-short-date",
		Passed:   true,
		Metrics: metrics{
			AssistantCalls:       1,
			UsageExposed:         true,
			InputTokens:          &input,
			CachedInputTokens:    &cached,
			NonCachedInputTokens: &nonCached,
			OutputTokens:         &output,
			EventTypeCounts:      map[string]int{},
		},
	}
	selected := []scenario{
		{ID: "create-dated-task", Category: scenarioCategoryRoutine, FeatureState: scenarioFeatureSupported},
		{ID: "ambiguous-short-date", Category: scenarioCategoryValidation, FeatureState: scenarioFeatureSupported},
	}
	score := productionScoreFor([]runResult{valid, invalid}, selected, true)
	if !score.Passed {
		t.Fatalf("score should pass: %#v", score)
	}

	invalid.Metrics.CommandExecutions = 1
	score = productionScoreFor([]runResult{valid, invalid}, selected, true)
	if score.Passed {
		t.Fatalf("score should fail when invalid scenario uses commands: %#v", score)
	}
}

func TestScenarioCoverageFullAndFiltered(t *testing.T) {
	t.Parallel()

	full := scenarioCoverageFor([]scenario{
		{ID: "routine", Category: scenarioCategoryRoutine, FeatureState: scenarioFeatureSupported},
		{ID: "update", Category: scenarioCategoryUpdate, FeatureState: scenarioFeatureSupported},
		{ID: "recurrence", Category: scenarioCategoryAdvancedRecurrence, FeatureState: scenarioFeatureSupported},
		{ID: "migration", Category: scenarioCategoryMigration, FeatureState: scenarioFeatureSupported},
		{ID: "multi", Category: scenarioCategoryMultiTurn, FeatureState: scenarioFeatureSupported},
	}, false)
	for _, coverage := range full {
		if coverage.Required && !coverage.Passed {
			t.Fatalf("required full coverage failed: %#v", coverage)
		}
	}

	filtered := scenarioCoverageFor([]scenario{{ID: "update", Category: scenarioCategoryUpdate, FeatureState: scenarioFeatureSupported}}, true)
	for _, coverage := range filtered {
		if !coverage.Passed || !strings.Contains(coverage.Details, "filtered run") {
			t.Fatalf("filtered coverage should pass with filtered details: %#v", coverage)
		}
	}
}

func TestUnsupportedWorkflowScoring(t *testing.T) {
	t.Parallel()

	input := 100
	cached := 40
	nonCached := 60
	output := 20
	result := runResult{
		Scenario: "unsupported-reminder",
		Passed:   true,
		Metrics: metrics{
			AssistantCalls:       1,
			UsageExposed:         true,
			InputTokens:          &input,
			CachedInputTokens:    &cached,
			NonCachedInputTokens: &nonCached,
			OutputTokens:         &output,
			EventTypeCounts:      map[string]int{},
		},
	}
	selected := []scenario{{ID: "unsupported-reminder", Category: scenarioCategoryFutureSurface, FeatureState: scenarioFeatureUnsupportedUntilLanded}}
	score := productionScoreFor([]runResult{result}, selected, true)
	if !score.Passed {
		t.Fatalf("unsupported workflow score should pass: %#v", score)
	}

	result.Metrics.ToolCalls = 1
	score = productionScoreFor([]runResult{result}, selected, true)
	if !score.Passed {
		t.Fatalf("unsupported workflow score should not fail solely because the agent inspected the production skill: %#v", score)
	}
}

func TestVerifyICalendarImportRequiresImportedUID(t *testing.T) {
	t.Parallel()

	finalMessage := "Imported planning review on 2026-04-18."
	manualDBPath := filepath.Join(t.TempDir(), "manual.db")
	manual, err := runPlanning(manualDBPath, runner.PlanningTaskRequest{
		Action:       runner.PlanningTaskActionCreateEvent,
		CalendarName: evalImportICalendarCalendarName,
		Title:        evalImportICalendarEventTitle,
		StartAt:      evalImportICalendarEventStartAt,
		EndAt:        "2026-04-18T15:30:00Z",
	})
	if err != nil || manual.Rejected {
		t.Fatalf("create lookalike event result = %#v, err = %v", manual, err)
	}
	manualCheck, err := verifyICalendarImport(manualDBPath, finalMessage)
	if err != nil {
		t.Fatalf("verify manual lookalike import: %v", err)
	}
	if manualCheck.DatabasePass || manualCheck.Passed {
		t.Fatalf("manual lookalike event should not pass import verification: %#v", manualCheck)
	}

	importDBPath := filepath.Join(t.TempDir(), "import.db")
	imported, err := runPlanning(importDBPath, runner.PlanningTaskRequest{
		Action:  runner.PlanningTaskActionImportICalendar,
		Content: evalImportICalendarContent,
	})
	if err != nil || imported.Rejected {
		t.Fatalf("import result = %#v, err = %v", imported, err)
	}
	importCheck, err := verifyICalendarImport(importDBPath, finalMessage)
	if err != nil {
		t.Fatalf("verify imported event: %v", err)
	}
	if !importCheck.Passed {
		t.Fatalf("imported event verification failed: %#v", importCheck)
	}
}

func TestVerifyDeleteScenarios(t *testing.T) {
	t.Parallel()

	taskDBPath := filepath.Join(t.TempDir(), "tasks.db")
	if err := seedDeleteTaskData(taskDBPath); err != nil {
		t.Fatalf("seedDeleteTaskData: %v", err)
	}
	tasks, err := listTasksForCalendar(taskDBPath, "Personal")
	if err != nil {
		t.Fatalf("list seeded tasks: %v", err)
	}
	var oldTaskID string
	for _, task := range tasks {
		if task.Title == "Old note" {
			oldTaskID = task.ID
		}
	}
	if oldTaskID == "" {
		t.Fatal("seeded Old note task not found")
	}
	if result, err := runPlanning(taskDBPath, runner.PlanningTaskRequest{Action: runner.PlanningTaskActionDeleteTask, TaskID: oldTaskID}); err != nil || result.Rejected {
		t.Fatalf("delete seeded task result = %#v, err = %v", result, err)
	}
	taskCheck, err := verifyDeletedTask(taskDBPath, "Deleted Old note.")
	if err != nil {
		t.Fatalf("verifyDeletedTask: %v", err)
	}
	if !taskCheck.Passed {
		t.Fatalf("deleted task verification failed: %#v", taskCheck)
	}

	eventDBPath := filepath.Join(t.TempDir(), "events.db")
	if err := seedDeleteEventData(eventDBPath); err != nil {
		t.Fatalf("seedDeleteEventData: %v", err)
	}
	events, err := listEventsForCalendar(eventDBPath, "Personal")
	if err != nil {
		t.Fatalf("list seeded events: %v", err)
	}
	var oldEventID string
	for _, event := range events {
		if event.Title == "Old appointment" {
			oldEventID = event.ID
		}
	}
	if oldEventID == "" {
		t.Fatal("seeded Old appointment event not found")
	}
	if result, err := runPlanning(eventDBPath, runner.PlanningTaskRequest{Action: runner.PlanningTaskActionDeleteEvent, EventID: oldEventID}); err != nil || result.Rejected {
		t.Fatalf("delete seeded event result = %#v, err = %v", result, err)
	}
	eventCheck, err := verifyDeletedEvent(eventDBPath, "Deleted Old appointment.")
	if err != nil {
		t.Fatalf("verifyDeletedEvent: %v", err)
	}
	if !eventCheck.Passed {
		t.Fatalf("deleted event verification failed: %#v", eventCheck)
	}

	calendarDBPath := filepath.Join(t.TempDir(), "calendar.db")
	if err := seedEmptyArchiveCalendar(calendarDBPath); err != nil {
		t.Fatalf("seedEmptyArchiveCalendar: %v", err)
	}
	if result, err := runPlanning(calendarDBPath, runner.PlanningTaskRequest{Action: runner.PlanningTaskActionDeleteCalendar, CalendarName: "Archive"}); err != nil || result.Rejected {
		t.Fatalf("delete seeded calendar result = %#v, err = %v", result, err)
	}
	calendarCheck, err := verifyDeletedEmptyCalendar(calendarDBPath, "Deleted Archive.")
	if err != nil {
		t.Fatalf("verifyDeletedEmptyCalendar: %v", err)
	}
	if !calendarCheck.Passed {
		t.Fatalf("deleted calendar verification failed: %#v", calendarCheck)
	}
}

func TestVerifyUnsupportedWorkflowRequiresEveryTopic(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "missing.db")
	partial, err := verifyUnsupportedWorkflow(dbPath, "Import is not supported yet.", nil, []string{"import", "export"})
	if err != nil {
		t.Fatalf("verifyUnsupportedWorkflow partial: %v", err)
	}
	if partial.Passed || partial.AssistantPass {
		t.Fatalf("partial unsupported answer passed: %#v", partial)
	}

	complete, err := verifyUnsupportedWorkflow(dbPath, "Import and export are not supported yet.", nil, []string{"import", "export"})
	if err != nil {
		t.Fatalf("verifyUnsupportedWorkflow complete: %v", err)
	}
	if !complete.Passed {
		t.Fatalf("complete unsupported answer failed: %#v", complete)
	}
}

func TestVerifyNoWorkCopyClarificationRejectsExtraCopy(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "openplanner.db")
	if err := seedLegacyMigrationData(dbPath); err != nil {
		t.Fatalf("seedLegacyMigrationData: %v", err)
	}

	clarification, err := verifyNoWorkCopyClarification(dbPath, "Which destination calendar should I use?")
	if err != nil {
		t.Fatalf("verifyNoWorkCopyClarification clean: %v", err)
	}
	if !clarification.Passed {
		t.Fatalf("clean clarification failed: %#v", clarification)
	}

	if err := runSeedRequests(dbPath, []runner.PlanningTaskRequest{{
		Action:       runner.PlanningTaskActionCreateTask,
		CalendarName: "Personal",
		Title:        "Review notes",
		DueDate:      "2026-04-16",
	}}); err != nil {
		t.Fatalf("seed extra copy: %v", err)
	}

	withExtraCopy, err := verifyNoWorkCopyClarification(dbPath, "Which destination calendar should I use?")
	if err != nil {
		t.Fatalf("verifyNoWorkCopyClarification extra copy: %v", err)
	}
	if withExtraCopy.Passed || withExtraCopy.DatabasePass {
		t.Fatalf("extra pre-clarification copy passed: %#v", withExtraCopy)
	}
}

func TestVerifyCalendarDoesNotCreateMissingCalendar(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "missing.db")
	result, err := verifyCalendar(dbPath, "Personal", "The Personal calendar exists.")
	if err != nil {
		t.Fatalf("verifyCalendar: %v", err)
	}
	if result.Passed || result.DatabasePass {
		t.Fatalf("verifyCalendar result = %#v, want database failure", result)
	}
	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Fatalf("verifyCalendar created database path: %v", err)
	}
}

func TestWriteReportIncludesTimingAndTurnDetails(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "report.md")
	tokens := 42
	value := report{
		Date:                  "report-test",
		Model:                 modelName,
		ReasoningEffort:       reasoningEffort,
		Parallelism:           4,
		CacheMode:             cacheModeShared,
		HarnessElapsedSeconds: 12.34,
		EffectiveSpeedup:      2.5,
		ParallelEfficiency:    0.62,
		ProductionScore: productionScore{
			Passed:   true,
			Criteria: []criterion{{Name: "production_passes_all_scenarios", Passed: true, Details: "ok"}},
		},
		ScenarioCoverage: []scenarioCoverage{{Category: scenarioCategoryMultiTurn, FeatureState: scenarioFeatureSupported, Scenarios: []string{"mt-clarify-then-create"}, Required: true, Passed: true, Details: "category present"}},
		PhaseTotals:      phaseTimings{AgentRun: 10, CopyRepo: 2, Total: 15},
		Results: []runResult{{
			Scenario:         "mt-clarify-then-create",
			ScenarioCategory: scenarioCategoryMultiTurn,
			FeatureState:     scenarioFeatureSupported,
			Passed:           true,
			Metrics:          metrics{UsageExposed: true, NonCachedInputTokens: &tokens},
			Verification:     verificationResult{Details: "ok"},
			Turns:            []turnResult{{Index: 1, ExitCode: 0, WallSeconds: 1.2, RawLogArtifactReference: "<run-root>/production/mt-clarify-then-create/turn-1/events.jsonl"}},
		}},
		ComparisonStatus: "n/a",
	}
	if err := writeMarkdown(path, value); err != nil {
		t.Fatalf("writeMarkdown: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	text := string(data)
	for _, want := range []string{"Effective parallel speedup: `2.50x`", "Parallel efficiency: `0.62`", "## Scenario Coverage", "## Phase Timings", "| agent_run | 10.00 |", "## Turn Details", "turn-1/events.jsonl"} {
		if !strings.Contains(text, want) {
			t.Fatalf("report missing %q:\n%s", want, text)
		}
	}
}

func TestWriteScaleReportIncludesDatasetAndThresholds(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "scale.md")
	value := scaleReport{
		Issue:              scaleIssueID,
		Date:               "2026-04-20",
		Harness:            "scale harness",
		ThresholdPolicy:    "local maintainer thresholds",
		RunRoot:            "<run-root>",
		DatabasePath:       "<run-root>/scale/openplanner.db",
		HarnessWallSeconds: 1.25,
		Passed:             true,
		Dataset: scaleDataset{
			Calendars:       2,
			Events:          12,
			Tasks:           13,
			RecurringEvents: 3,
			RecurringTasks:  3,
			RecurrenceRules: 6,
			CompletionRows:  4,
			AgendaRangeDays: 30,
			Limit:           5,
		},
		Results: []scaleResult{{
			Scenario:         "large-agenda-window",
			Passed:           true,
			WallSeconds:      0.5,
			ThresholdSeconds: 5,
			ItemsReturned:    5,
			PagesTraversed:   1,
			Events:           12,
			Tasks:            13,
			RecurrenceRules:  6,
			CompletionRows:   4,
			Notes:            []string{"ok"},
		}},
		RawArtifactsNote: "Raw scale database stayed under <run-root>/scale.",
	}
	if err := writeScaleMarkdown(path, value); err != nil {
		t.Fatalf("writeScaleMarkdown: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read scale report: %v", err)
	}
	text := string(data)
	for _, want := range []string{"# OpenPlanner Scale Eval 2026-04-20", "## Dataset", "`large-agenda-window`", "Threshold Seconds", "<run-root>/scale/openplanner.db"} {
		if !strings.Contains(text, want) {
			t.Fatalf("scale report missing %q:\n%s", want, text)
		}
	}
	for _, bad := range []string{"/Users/", "/home/", "/tmp/", "/var/folders/"} {
		if strings.Contains(text, bad) {
			t.Fatalf("scale report contains machine-absolute path marker %q:\n%s", bad, text)
		}
	}
}

func TestRunScaleEvalSmallFixture(t *testing.T) {
	t.Parallel()

	report, err := runScaleEval(t.TempDir(), scaleOptions{
		Date:        "test",
		Events:      4,
		Tasks:       4,
		Recurring:   2,
		Completions: 2,
		Limit:       20,
	})
	if err != nil {
		t.Fatalf("runScaleEval: %v", err)
	}
	if len(report.Results) != 4 {
		t.Fatalf("scale results length = %d, want 4", len(report.Results))
	}
	if report.Dataset.Events != 6 || report.Dataset.Tasks != 6 || report.Dataset.RecurrenceRules != 4 || report.Dataset.CompletionRows != 2 {
		t.Fatalf("scale dataset = %#v", report.Dataset)
	}
	for _, result := range report.Results {
		if result.ThresholdSeconds <= 0 {
			t.Fatalf("%s threshold = %.2f, want positive", result.Scenario, result.ThresholdSeconds)
		}
		if result.WallSeconds < 0 {
			t.Fatalf("%s wall = %.2f, want non-negative", result.Scenario, result.WallSeconds)
		}
		if !result.Passed {
			t.Fatalf("%s did not pass small fixture: %#v", result.Scenario, result)
		}
	}
}

func TestCommittedEvalReportsUseNeutralPaths(t *testing.T) {
	t.Parallel()

	resultsDir := filepath.Join("..", "..", "..", "docs", "agent-eval-results")
	forbidden := []string{"/Users/", "/home/", "/tmp/", "/var/folders/"}
	err := filepath.WalkDir(resultsDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || (!strings.HasSuffix(path, ".md") && !strings.HasSuffix(path, ".json")) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(data)
		for _, bad := range forbidden {
			if strings.Contains(text, bad) {
				t.Fatalf("%s contains machine-absolute path marker %q", path, bad)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan eval reports: %v", err)
	}
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func containsArgPair(args []string, key string, value string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == key && args[i+1] == value {
			return true
		}
	}
	return false
}
