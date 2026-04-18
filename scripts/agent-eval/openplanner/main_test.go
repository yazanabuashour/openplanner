package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	score := productionScoreFor([]runResult{valid, invalid})
	if !score.Passed {
		t.Fatalf("score should pass: %#v", score)
	}

	invalid.Metrics.CommandExecutions = 1
	score = productionScoreFor([]runResult{valid, invalid})
	if score.Passed {
		t.Fatalf("score should fail when invalid scenario uses commands: %#v", score)
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
		PhaseTotals: phaseTimings{AgentRun: 10, CopyRepo: 2, Total: 15},
		Results: []runResult{{
			Scenario:     "mt-clarify-then-create",
			Passed:       true,
			Metrics:      metrics{UsageExposed: true, NonCachedInputTokens: &tokens},
			Verification: verificationResult{Details: "ok"},
			Turns:        []turnResult{{Index: 1, ExitCode: 0, WallSeconds: 1.2, RawLogArtifactReference: "<run-root>/production/mt-clarify-then-create/turn-1/events.jsonl"}},
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
	for _, want := range []string{"Effective parallel speedup: `2.50x`", "Parallel efficiency: `0.62`", "## Phase Timings", "| agent_run | 10.00 |", "## Turn Details", "turn-1/events.jsonl"} {
		if !strings.Contains(text, want) {
			t.Fatalf("report missing %q:\n%s", want, text)
		}
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
