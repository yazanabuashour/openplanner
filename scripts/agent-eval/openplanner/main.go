package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/yazanabuashour/openplanner/agentops"
	"github.com/yazanabuashour/openplanner/sdk"
)

const (
	issueID               = "op-agentops"
	modelName             = "gpt-5.4-mini"
	reasoningEffort       = "medium"
	defaultRunParallelism = 4
	cacheModeShared       = "shared"
	cacheModeIsolated     = "isolated"
)

var prewarmCompilePackages = []string{"./cmd/openplanner-agentops", "./agentops"}

type scenario struct {
	ID     string         `json:"id"`
	Title  string         `json:"title"`
	Prompt string         `json:"prompt,omitempty"`
	Turns  []scenarioTurn `json:"turns,omitempty"`
}

type scenarioTurn struct {
	Prompt string `json:"prompt"`
}

type report struct {
	Issue                 string                  `json:"issue"`
	Date                  string                  `json:"date"`
	Model                 string                  `json:"model"`
	ReasoningEffort       string                  `json:"reasoning_effort"`
	Harness               string                  `json:"harness"`
	Parallelism           int                     `json:"parallelism"`
	CacheMode             string                  `json:"cache_mode"`
	CachePrewarmSeconds   float64                 `json:"cache_prewarm_seconds,omitempty"`
	HarnessElapsedSeconds float64                 `json:"harness_elapsed_seconds"`
	PhaseTotals           phaseTimings            `json:"phase_totals"`
	EffectiveSpeedup      float64                 `json:"effective_parallel_speedup,omitempty"`
	ParallelEfficiency    float64                 `json:"parallel_efficiency,omitempty"`
	CodexVersion          string                  `json:"codex_version"`
	HistoryIsolation      historyIsolationSummary `json:"history_isolation"`
	CommandTemplate       []string                `json:"command_template"`
	MetricNotes           []string                `json:"metric_notes,omitempty"`
	ProductionScore       productionScore         `json:"production_score"`
	Results               []runResult             `json:"results"`
	RawLogsCommitted      bool                    `json:"raw_logs_committed"`
	RawLogsNote           string                  `json:"raw_logs_note"`
	TokenUsageCaveat      string                  `json:"token_usage_caveat"`
	ComparisonStatus      string                  `json:"comparison_status"`
}

type historyIsolationSummary struct {
	Status                     string `json:"status"`
	EphemeralFlagRequired      bool   `json:"ephemeral_flag_required"`
	RunDirectoryOutsideRepo    bool   `json:"run_directory_outside_repo"`
	NewSessionFilesAfterRun    int    `json:"new_session_files_after_run"`
	SingleTurnEphemeralRuns    int    `json:"single_turn_ephemeral_runs"`
	MultiTurnPersistedSessions int    `json:"multi_turn_persisted_sessions"`
	MultiTurnPersistedTurns    int    `json:"multi_turn_persisted_turns"`
	OpenPlannerWorkspaceUsed   bool   `json:"openplanner_workspace_used"`
	DesktopAppUsed             bool   `json:"desktop_app_used"`
	VerificationMethod         string `json:"verification_method"`
	VerificationLimitation     string `json:"verification_limitation,omitempty"`
}

type runResult struct {
	Variant                 string             `json:"variant"`
	Scenario                string             `json:"scenario"`
	ScenarioTitle           string             `json:"scenario_title"`
	Passed                  bool               `json:"passed"`
	ExitCode                int                `json:"exit_code"`
	WallSeconds             float64            `json:"wall_seconds"`
	PhaseTimings            phaseTimings       `json:"phase_timings"`
	Metrics                 metrics            `json:"metrics"`
	Verification            verificationResult `json:"verification"`
	Turns                   []turnResult       `json:"turns,omitempty"`
	PromptSummary           string             `json:"prompt_summary"`
	RawLogArtifactReference string             `json:"raw_log_artifact_reference"`
}

type turnResult struct {
	Index                   int                `json:"turn_index"`
	WallSeconds             float64            `json:"wall_seconds"`
	ExitCode                int                `json:"exit_code"`
	Metrics                 metrics            `json:"metrics"`
	Verification            verificationResult `json:"verification"`
	RawLogArtifactReference string             `json:"raw_log_artifact_reference"`
}

type phaseTimings struct {
	PrepareRunDir  float64 `json:"prepare_run_dir_seconds,omitempty"`
	CopyRepo       float64 `json:"copy_repo_seconds,omitempty"`
	InstallVariant float64 `json:"install_variant_seconds,omitempty"`
	WarmCache      float64 `json:"warm_cache_seconds,omitempty"`
	SeedDB         float64 `json:"seed_db_seconds,omitempty"`
	AgentRun       float64 `json:"agent_run_seconds,omitempty"`
	ParseMetrics   float64 `json:"parse_metrics_seconds,omitempty"`
	Verify         float64 `json:"verify_seconds,omitempty"`
	Total          float64 `json:"total_seconds,omitempty"`
}

type metrics struct {
	AssistantCalls                       int            `json:"assistant_calls"`
	ToolCalls                            int            `json:"tool_calls"`
	CommandExecutions                    int            `json:"command_executions"`
	FileInspectionCommands               int            `json:"file_inspection_commands"`
	GeneratedFileInspected               bool           `json:"generated_file_inspected"`
	GeneratedPathFromBroadSearch         bool           `json:"generated_path_from_broad_search"`
	BroadRepoSearch                      bool           `json:"broad_repo_search"`
	ModuleCacheInspected                 bool           `json:"module_cache_inspected"`
	CLIUsed                              bool           `json:"cli_used"`
	DirectSQLiteAccess                   bool           `json:"direct_sqlite_access"`
	GeneratedFileEvidence                []string       `json:"generated_file_evidence,omitempty"`
	GeneratedPathFromBroadSearchEvidence []string       `json:"generated_path_from_broad_search_evidence,omitempty"`
	BroadRepoSearchEvidence              []string       `json:"broad_repo_search_evidence,omitempty"`
	ModuleCacheEvidence                  []string       `json:"module_cache_evidence,omitempty"`
	CLIUsageEvidence                     []string       `json:"cli_usage_evidence,omitempty"`
	DirectSQLiteEvidence                 []string       `json:"direct_sqlite_evidence,omitempty"`
	UsageExposed                         bool           `json:"usage_exposed"`
	InputTokens                          *int           `json:"input_tokens,omitempty"`
	CachedInputTokens                    *int           `json:"cached_input_tokens,omitempty"`
	NonCachedInputTokens                 *int           `json:"non_cached_input_tokens,omitempty"`
	OutputTokens                         *int           `json:"output_tokens,omitempty"`
	EventTypeCounts                      map[string]int `json:"event_type_counts"`
	CommandMetricLimitations             string         `json:"command_metric_limitations"`
}

type verificationResult struct {
	Passed        bool               `json:"passed"`
	DatabasePass  bool               `json:"database_pass"`
	AssistantPass bool               `json:"assistant_pass"`
	Details       string             `json:"details"`
	Calendars     []calendarState    `json:"calendars,omitempty"`
	Events        []eventState       `json:"events,omitempty"`
	Tasks         []taskState        `json:"tasks,omitempty"`
	Agenda        []agendaEntryState `json:"agenda,omitempty"`
}

type calendarState struct {
	Name string `json:"name"`
}

type eventState struct {
	Title      string `json:"title"`
	StartAt    string `json:"start_at,omitempty"`
	StartDate  string `json:"start_date,omitempty"`
	Recurrence string `json:"recurrence,omitempty"`
}

type taskState struct {
	Title       string `json:"title"`
	DueAt       string `json:"due_at,omitempty"`
	DueDate     string `json:"due_date,omitempty"`
	Recurrence  string `json:"recurrence,omitempty"`
	CompletedAt string `json:"completed_at,omitempty"`
}

type agendaEntryState struct {
	Kind        string `json:"kind"`
	Title       string `json:"title"`
	StartAt     string `json:"start_at,omitempty"`
	StartDate   string `json:"start_date,omitempty"`
	DueAt       string `json:"due_at,omitempty"`
	DueDate     string `json:"due_date,omitempty"`
	CompletedAt string `json:"completed_at,omitempty"`
}

type productionScore struct {
	Recommendation string      `json:"recommendation"`
	Passed         bool        `json:"passed"`
	Criteria       []criterion `json:"criteria"`
}

type criterion struct {
	Name    string `json:"name"`
	Passed  bool   `json:"passed"`
	Details string `json:"details"`
}

type codexEvent struct {
	Type     string      `json:"type"`
	ThreadID string      `json:"thread_id"`
	Item     codexItem   `json:"item"`
	Usage    *codexUsage `json:"usage"`
}

type codexItem struct {
	ID               string `json:"id"`
	Type             string `json:"type"`
	Text             string `json:"text"`
	Command          string `json:"command"`
	AggregatedOutput string `json:"aggregated_output"`
}

type codexUsage struct {
	InputTokens       int `json:"input_tokens"`
	CachedInputTokens int `json:"cached_input_tokens"`
	OutputTokens      int `json:"output_tokens"`
}

type runOptions struct {
	RunRoot        string
	Date           string
	ScenarioFilter string
	Parallelism    int
	CacheMode      string
}

type cacheConfig struct {
	Mode    string
	RunRoot string
}

type evalJob struct {
	Index    int
	Scenario scenario
}

type runOneFunc func(repoRoot string, runRoot string, currentScenario scenario, cache cacheConfig) (runResult, error)

type parsedTurn struct {
	metrics      metrics
	finalMessage string
	sessionID    string
	parseError   error
	parseSeconds float64
}

type parsedMetrics struct {
	metrics      metrics
	finalMessage string
	sessionID    string
}

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

func parseRunOptions(args []string) (runOptions, error) {
	options := runOptions{
		Date:        time.Now().Format(time.DateOnly),
		Parallelism: defaultRunParallelism,
		CacheMode:   cacheModeShared,
	}
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&options.RunRoot, "run-root", options.RunRoot, "directory for raw run artifacts outside the repo")
	fs.StringVar(&options.Date, "date", options.Date, "report date in YYYY-MM-DD form or report suffix")
	fs.StringVar(&options.ScenarioFilter, "scenario", options.ScenarioFilter, "optional comma-separated scenario ids to run")
	fs.IntVar(&options.Parallelism, "parallel", options.Parallelism, "number of scenario jobs to run concurrently")
	fs.StringVar(&options.CacheMode, "cache-mode", options.CacheMode, "Go cache mode: shared or isolated")
	if err := fs.Parse(args); err != nil {
		return runOptions{}, err
	}
	if fs.NArg() != 0 {
		return runOptions{}, errors.New("run does not accept positional arguments")
	}
	if options.Parallelism < 1 {
		return runOptions{}, errors.New("parallel must be greater than or equal to 1")
	}
	switch options.CacheMode {
	case cacheModeShared, cacheModeIsolated:
	default:
		return runOptions{}, fmt.Errorf("cache-mode must be %q or %q", cacheModeShared, cacheModeIsolated)
	}
	return options, nil
}

func runCommand(args []string) {
	options, err := parseRunOptions(args)
	if err != nil {
		failf("parse flags: %v", err)
	}

	repoRoot, err := repoRoot()
	if err != nil {
		failf("resolve repo root: %v", err)
	}

	runRoot := options.RunRoot
	if runRoot == "" {
		runRoot, err = os.MkdirTemp("", "openplanner-agent-eval-*")
		if err != nil {
			failf("create run root: %v", err)
		}
	} else if err := os.MkdirAll(runRoot, 0o755); err != nil {
		failf("create run root %s: %v", runRoot, err)
	}
	runRoot, err = filepath.Abs(runRoot)
	if err != nil {
		failf("absolute run root: %v", err)
	}
	if isWithin(runRoot, repoRoot) {
		failf("run root must be outside the repository: %s", runRoot)
	}

	selectedScenarios, err := selectScenarios(options.ScenarioFilter)
	if err != nil {
		failf("select scenarios: %v", err)
	}
	cacheConfig := cacheConfig{Mode: options.CacheMode, RunRoot: runRoot}

	marker := filepath.Join(runRoot, "history-marker")
	if err := os.WriteFile(marker, []byte(time.Now().Format(time.RFC3339Nano)), 0o644); err != nil {
		failf("write history marker: %v", err)
	}
	markerInfo, err := os.Stat(marker)
	if err != nil {
		failf("stat history marker: %v", err)
	}

	codexVersion := commandOutput("codex", "--version")
	cachePrewarmSeconds := 0.0
	if options.CacheMode == cacheModeShared {
		start := time.Now()
		if err := prewarmSharedCache(repoRoot, cacheConfig); err != nil {
			failf("prewarm shared Go cache: %v", err)
		}
		cachePrewarmSeconds = roundSeconds(time.Since(start).Seconds())
	}

	harnessStart := time.Now()
	jobs := evalJobsFor(selectedScenarios)
	results := runEvalJobs(repoRoot, runRoot, jobs, options.Parallelism, cacheConfig, runOne)
	harnessElapsedSeconds := roundSeconds(time.Since(harnessStart).Seconds())
	phaseTotals := aggregatePhaseTimings(results)
	effectiveSpeedup := 0.0
	parallelEfficiency := 0.0
	if harnessElapsedSeconds > 0 {
		effectiveSpeedup = roundSeconds(totalAgentWallSeconds(results) / harnessElapsedSeconds)
		parallelEfficiency = roundSeconds(effectiveSpeedup / float64(options.Parallelism))
	}

	multiTurnJobs := countMultiTurnJobs(jobs)
	newSessionFiles := countNewSessionFiles(markerInfo.ModTime(), runRoot)
	historyStatus := "passed"
	limitation := ""
	if newSessionFiles != 0 && multiTurnJobs == 0 {
		historyStatus = "review"
		limitation = "Session-file count changed even though only single-turn ephemeral scenarios ran; this may be from another Codex process."
	} else if multiTurnJobs > 0 && newSessionFiles < multiTurnJobs {
		historyStatus = "review"
		limitation = "Fewer session files referenced <run-root> than expected for persisted multi-turn eval sessions."
	}

	outReport := report{
		Issue:                 issueID,
		Date:                  options.Date,
		Model:                 modelName,
		ReasoningEffort:       reasoningEffort,
		Harness:               "codex exec --json --full-auto from throwaway run directories; single-turn scenarios use --ephemeral, multi-turn scenarios resume a persisted eval session with explicit writable eval roots",
		Parallelism:           options.Parallelism,
		CacheMode:             options.CacheMode,
		CachePrewarmSeconds:   cachePrewarmSeconds,
		HarnessElapsedSeconds: harnessElapsedSeconds,
		PhaseTotals:           phaseTotals,
		EffectiveSpeedup:      effectiveSpeedup,
		ParallelEfficiency:    parallelEfficiency,
		CodexVersion:          codexVersion,
		HistoryIsolation: historyIsolationSummary{
			Status:                     historyStatus,
			EphemeralFlagRequired:      true,
			RunDirectoryOutsideRepo:    true,
			NewSessionFilesAfterRun:    newSessionFiles,
			SingleTurnEphemeralRuns:    countSingleTurnJobs(jobs),
			MultiTurnPersistedSessions: multiTurnJobs,
			MultiTurnPersistedTurns:    countMultiTurnPersistedTurns(jobs),
			OpenPlannerWorkspaceUsed:   false,
			DesktopAppUsed:             false,
			VerificationMethod:         "Single-turn scenarios use codex exec --ephemeral from <run-root>/production/<scenario>/repo. Multi-turn scenarios create one persisted Codex exec session per scenario and resume it for later turns; all raw logs stay under <run-root>.",
			VerificationLimitation:     limitation,
		},
		CommandTemplate: []string{
			"OPENPLANNER_DATABASE_PATH=<run-root>/production/<scenario>/repo/openplanner.db",
			"GOCACHE=<run-root>/shared-cache/gocache when --cache-mode shared; otherwise <run-root>/production/<scenario>/gocache",
			"GOMODCACHE=<run-root>/shared-cache/gomodcache when --cache-mode shared; otherwise <run-root>/production/<scenario>/gomodcache",
			"single turn: codex exec --json --ephemeral --full-auto --skip-git-repo-check --add-dir <run-root>/production/<scenario> --add-dir <run-root>/shared-cache when --cache-mode shared -C <run-root>/production/<scenario>/repo -m gpt-5.4-mini -c model_reasoning_effort=\"medium\" -c shell_environment_policy.inherit=all <natural user prompt>",
			"multi turn: first turn uses codex exec without --ephemeral; later turns use codex exec -C <run-root>/production/<scenario>/repo --add-dir <writable-eval-roots> resume <thread-id> --json with per-turn logs",
		},
		MetricNotes:      metricNotes(results),
		ProductionScore:  productionScoreFor(results),
		Results:          results,
		RawLogsCommitted: false,
		RawLogsNote:      "Raw codex exec event logs and stderr files were retained under <run-root> during execution and intentionally not committed.",
		TokenUsageCaveat: "Token metrics come from codex exec turn.completed usage events when exposed; unavailable usage is recorded as not exposed.",
		ComparisonStatus: "not applicable: OpenPlanner has no human CLI baseline variant; this report scores the production AgentOps surface only",
	}

	outDir := filepath.Join(repoRoot, "docs", "agent-eval-results")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		failf("create report dir: %v", err)
	}
	jsonPath := filepath.Join(outDir, fmt.Sprintf("%s-%s.json", issueID, options.Date))
	mdPath := filepath.Join(outDir, fmt.Sprintf("%s-%s.md", issueID, options.Date))
	if err := writeJSON(jsonPath, outReport); err != nil {
		failf("write json report: %v", err)
	}
	if err := writeMarkdown(mdPath, outReport); err != nil {
		failf("write markdown report: %v", err)
	}
}

func evalJobsFor(selectedScenarios []scenario) []evalJob {
	jobs := make([]evalJob, 0, len(selectedScenarios))
	for index, sc := range selectedScenarios {
		jobs = append(jobs, evalJob{Index: index, Scenario: sc})
	}
	return jobs
}

func runEvalJobs(repoRoot string, runRoot string, jobs []evalJob, parallelism int, cache cacheConfig, runOne runOneFunc) []runResult {
	if parallelism < 1 {
		parallelism = 1
	}
	workerCount := parallelism
	if len(jobs) < workerCount {
		workerCount = len(jobs)
	}
	results := make([]runResult, len(jobs))
	if workerCount == 0 {
		return results
	}
	jobCh := make(chan evalJob)
	var wg sync.WaitGroup
	for worker := 0; worker < workerCount; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for current := range jobCh {
				result, err := runOne(repoRoot, runRoot, current.Scenario, cache)
				if err != nil {
					result = harnessErrorResult(current.Scenario, err)
				}
				results[current.Index] = result
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

func harnessErrorResult(sc scenario, err error) runResult {
	return runResult{
		Variant:       "production",
		Scenario:      sc.ID,
		ScenarioTitle: sc.Title,
		ExitCode:      -1,
		Verification: verificationResult{
			Passed:        false,
			DatabasePass:  false,
			AssistantPass: false,
			Details:       fmt.Sprintf("harness error: %v", err),
		},
		PromptSummary: promptSummary(sc),
	}
}

func runOne(repoRoot string, runRoot string, currentScenario scenario, cache cacheConfig) (runResult, error) {
	totalStart := time.Now()
	runDir := filepath.Join(runRoot, "production", currentScenario.ID)
	runRepo := filepath.Join(runDir, "repo")
	dbPath := evalDatabasePath(runRepo)
	timings := phaseTimings{}

	if err := timedPhase(&timings.PrepareRunDir, func() error { return prepareRunDir(runDir, cache) }); err != nil {
		return runResult{}, fmt.Errorf("prepare run dir: %w", err)
	}
	if err := timedPhase(&timings.CopyRepo, func() error { return copyRepo(repoRoot, runRepo) }); err != nil {
		return runResult{}, fmt.Errorf("copy repo: %w", err)
	}
	if err := timedPhase(&timings.InstallVariant, func() error { return installEvalAgentsFile(runRepo) }); err != nil {
		return runResult{}, fmt.Errorf("install eval agents file: %w", err)
	}
	if cache.Mode == cacheModeIsolated {
		if err := timedPhase(&timings.WarmCache, func() error { return warmGoModules(runRepo, runDir, dbPath, cache) }); err != nil {
			return runResult{}, fmt.Errorf("warm go modules: %w", err)
		}
	}
	if err := timedPhase(&timings.SeedDB, func() error { return seedScenario(dbPath, currentScenario) }); err != nil {
		return runResult{}, fmt.Errorf("seed scenario: %w", err)
	}

	turns := scenarioTurns(currentScenario)
	turnResults := make([]turnResult, 0, len(turns))
	sessionID := ""
	var agentErr error
	for i, turn := range turns {
		turnIndex := i + 1
		result, parsed, err := runScenarioTurn(runRepo, runDir, dbPath, currentScenario, turn, turnIndex, sessionID, cache)
		timings.AgentRun += result.WallSeconds
		timings.ParseMetrics += parsed.parseSeconds
		if parsed.parseError != nil {
			result.Metrics.CommandMetricLimitations = fmt.Sprintf("failed to parse event log: %v", parsed.parseError)
		}
		verifyStart := time.Now()
		verification, verifyErr := verifyScenarioTurn(dbPath, currentScenario, turnIndex, parsed.finalMessage)
		timings.Verify += roundSeconds(time.Since(verifyStart).Seconds())
		if verifyErr != nil {
			verification = verificationResult{
				Passed:        false,
				DatabasePass:  false,
				AssistantPass: false,
				Details:       fmt.Sprintf("verification error: %v", verifyErr),
			}
		}
		result.Verification = verification
		turnResults = append(turnResults, result)
		if err != nil && agentErr == nil {
			agentErr = err
		}
		if verifyErr != nil && agentErr == nil {
			agentErr = verifyErr
		}
		if i == 0 && len(turns) > 1 {
			sessionID = parsed.sessionID
			if sessionID == "" && agentErr == nil {
				agentErr = errors.New("multi-turn first turn did not expose a thread id")
			}
		}
	}

	metrics := aggregateMetrics(turnResults)
	verification := aggregateVerification(currentScenario, turnResults)
	timings.Total = roundSeconds(time.Since(totalStart).Seconds())
	exitCode := aggregateExitCode(turnResults)
	rawLogRef := ""
	if len(turnResults) > 0 {
		rawLogRef = turnResults[len(turnResults)-1].RawLogArtifactReference
	}
	result := runResult{
		Variant:                 "production",
		Scenario:                currentScenario.ID,
		ScenarioTitle:           currentScenario.Title,
		Passed:                  agentErr == nil && verification.Passed,
		ExitCode:                exitCode,
		WallSeconds:             roundSeconds(sumTurnWallSeconds(turnResults)),
		PhaseTimings:            timings.rounded(),
		Metrics:                 metrics,
		Verification:            verification,
		Turns:                   turnResults,
		PromptSummary:           promptSummary(currentScenario),
		RawLogArtifactReference: rawLogRef,
	}
	_ = writeJSON(filepath.Join(runDir, "run-summary.json"), result)
	return result, nil
}

func runScenarioTurn(runRepo string, runDir string, dbPath string, currentScenario scenario, turn scenarioTurn, turnIndex int, sessionID string, cache cacheConfig) (turnResult, parsedTurn, error) {
	turnDir := filepath.Join(runDir, fmt.Sprintf("turn-%d", turnIndex))
	if err := os.MkdirAll(turnDir, 0o755); err != nil {
		return turnResult{}, parsedTurn{}, err
	}
	eventsPath := filepath.Join(turnDir, "events.jsonl")
	stderrPath := filepath.Join(turnDir, "stderr.log")
	stdoutFile, err := os.Create(eventsPath)
	if err != nil {
		return turnResult{}, parsedTurn{}, err
	}
	defer func() { _ = stdoutFile.Close() }()
	stderrFile, err := os.Create(stderrPath)
	if err != nil {
		return turnResult{}, parsedTurn{}, err
	}
	defer func() { _ = stderrFile.Close() }()

	args := codexArgsForTurn(runRepo, runDir, currentScenario, turn, turnIndex, sessionID, cache)
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "codex", args...)
	cmd.Dir = runRepo
	cmd.Stdout = stdoutFile
	cmd.Stderr = stderrFile
	cmd.Stdin = strings.NewReader("")
	cmd.Env = evalEnv(runDir, dbPath, cache)

	start := time.Now()
	err = cmd.Run()
	wallSeconds := roundSeconds(time.Since(start).Seconds())
	exitCode := commandExitCode(err)
	if ctx.Err() == context.DeadlineExceeded {
		exitCode = -1
	}

	parseStart := time.Now()
	parsedMetrics, parseErr := parseMetrics(eventsPath)
	parsed := parsedTurn{
		metrics:      parsedMetrics.metrics,
		finalMessage: parsedMetrics.finalMessage,
		sessionID:    parsedMetrics.sessionID,
		parseError:   parseErr,
		parseSeconds: roundSeconds(time.Since(parseStart).Seconds()),
	}
	result := turnResult{
		Index:                   turnIndex,
		WallSeconds:             wallSeconds,
		ExitCode:                exitCode,
		Metrics:                 parsedMetrics.metrics,
		RawLogArtifactReference: fmt.Sprintf("<run-root>/production/%s/turn-%d/events.jsonl", currentScenario.ID, turnIndex),
	}
	return result, parsed, err
}

func codexArgsForTurn(runRepo string, runDir string, currentScenario scenario, turn scenarioTurn, turnIndex int, sessionID string, cache cacheConfig) []string {
	baseConfig := []string{
		"-m", modelName,
		"-c", fmt.Sprintf("model_reasoning_effort=%q", reasoningEffort),
		"-c", "shell_environment_policy.inherit=all",
	}
	writableRoots := codexWritableRoots(runDir, cache)
	if len(scenarioTurns(currentScenario)) == 1 {
		args := []string{"exec", "--json", "--ephemeral", "--full-auto", "--skip-git-repo-check", "-C", runRepo}
		args = appendAddDirs(args, writableRoots)
		args = append(args, baseConfig...)
		return append(args, turn.Prompt)
	}
	if turnIndex == 1 {
		args := []string{"exec", "--json", "--full-auto", "--skip-git-repo-check", "-C", runRepo}
		args = appendAddDirs(args, writableRoots)
		args = append(args, baseConfig...)
		return append(args, turn.Prompt)
	}
	args := []string{"exec", "-C", runRepo}
	args = appendAddDirs(args, writableRoots)
	args = append(args, "resume", "--json", "--full-auto", "--skip-git-repo-check")
	args = append(args, baseConfig...)
	args = append(args, sessionID, turn.Prompt)
	return args
}

func codexWritableRoots(runDir string, cache cacheConfig) []string {
	roots := []string{runDir}
	if cache.Mode == cacheModeShared {
		roots = append(roots, filepath.Join(cache.RunRoot, "shared-cache"))
	}
	return roots
}

func appendAddDirs(args []string, roots []string) []string {
	for _, root := range roots {
		args = append(args, "--add-dir", root)
	}
	return args
}

func evalEnv(runDir string, dbPath string, cache cacheConfig) []string {
	env := os.Environ()
	paths := evalPathsFor(runDir, cache)
	env = append(env,
		"OPENPLANNER_DATABASE_PATH="+dbPath,
		"GOCACHE="+paths.GoCache,
		"GOMODCACHE="+paths.GoModCache,
		"TMPDIR="+paths.Temp,
	)
	return env
}

func evalDatabasePath(runRepo string) string {
	return filepath.Join(runRepo, "openplanner.db")
}

func timedPhase(target *float64, fn func() error) error {
	start := time.Now()
	err := fn()
	*target += roundSeconds(time.Since(start).Seconds())
	return err
}

func (p phaseTimings) rounded() phaseTimings {
	return phaseTimings{
		PrepareRunDir:  roundSeconds(p.PrepareRunDir),
		CopyRepo:       roundSeconds(p.CopyRepo),
		InstallVariant: roundSeconds(p.InstallVariant),
		WarmCache:      roundSeconds(p.WarmCache),
		SeedDB:         roundSeconds(p.SeedDB),
		AgentRun:       roundSeconds(p.AgentRun),
		ParseMetrics:   roundSeconds(p.ParseMetrics),
		Verify:         roundSeconds(p.Verify),
		Total:          roundSeconds(p.Total),
	}
}

func aggregatePhaseTimings(results []runResult) phaseTimings {
	total := phaseTimings{}
	for _, result := range results {
		total.PrepareRunDir += result.PhaseTimings.PrepareRunDir
		total.CopyRepo += result.PhaseTimings.CopyRepo
		total.InstallVariant += result.PhaseTimings.InstallVariant
		total.WarmCache += result.PhaseTimings.WarmCache
		total.SeedDB += result.PhaseTimings.SeedDB
		total.AgentRun += result.PhaseTimings.AgentRun
		total.ParseMetrics += result.PhaseTimings.ParseMetrics
		total.Verify += result.PhaseTimings.Verify
		total.Total += result.PhaseTimings.Total
	}
	return total.rounded()
}

func totalAgentWallSeconds(results []runResult) float64 {
	total := 0.0
	for _, result := range results {
		total += result.WallSeconds
	}
	return total
}

func sumTurnWallSeconds(turns []turnResult) float64 {
	total := 0.0
	for _, turn := range turns {
		total += turn.WallSeconds
	}
	return total
}

func aggregateExitCode(turns []turnResult) int {
	for _, turn := range turns {
		if turn.ExitCode != 0 {
			return turn.ExitCode
		}
	}
	return 0
}

func aggregateMetrics(turns []turnResult) metrics {
	out := metrics{
		EventTypeCounts:          map[string]int{},
		CommandMetricLimitations: "Command/file inspection metrics are inferred from codex exec JSON command events, not from OS-level tracing.",
	}
	allUsageExposed := len(turns) > 0
	inputTotal := 0
	cachedTotal := 0
	nonCachedTotal := 0
	outputTotal := 0
	for _, turn := range turns {
		current := turn.Metrics
		out.AssistantCalls += current.AssistantCalls
		out.ToolCalls += current.ToolCalls
		out.CommandExecutions += current.CommandExecutions
		out.FileInspectionCommands += current.FileInspectionCommands
		out.GeneratedFileInspected = out.GeneratedFileInspected || current.GeneratedFileInspected
		out.GeneratedPathFromBroadSearch = out.GeneratedPathFromBroadSearch || current.GeneratedPathFromBroadSearch
		out.BroadRepoSearch = out.BroadRepoSearch || current.BroadRepoSearch
		out.ModuleCacheInspected = out.ModuleCacheInspected || current.ModuleCacheInspected
		out.CLIUsed = out.CLIUsed || current.CLIUsed
		out.DirectSQLiteAccess = out.DirectSQLiteAccess || current.DirectSQLiteAccess
		out.GeneratedFileEvidence = append(out.GeneratedFileEvidence, current.GeneratedFileEvidence...)
		out.GeneratedPathFromBroadSearchEvidence = append(out.GeneratedPathFromBroadSearchEvidence, current.GeneratedPathFromBroadSearchEvidence...)
		out.BroadRepoSearchEvidence = append(out.BroadRepoSearchEvidence, current.BroadRepoSearchEvidence...)
		out.ModuleCacheEvidence = append(out.ModuleCacheEvidence, current.ModuleCacheEvidence...)
		out.CLIUsageEvidence = append(out.CLIUsageEvidence, current.CLIUsageEvidence...)
		out.DirectSQLiteEvidence = append(out.DirectSQLiteEvidence, current.DirectSQLiteEvidence...)
		for eventType, count := range current.EventTypeCounts {
			out.EventTypeCounts[eventType] += count
		}
		if !current.UsageExposed || current.InputTokens == nil || current.CachedInputTokens == nil || current.NonCachedInputTokens == nil || current.OutputTokens == nil {
			allUsageExposed = false
			continue
		}
		inputTotal += *current.InputTokens
		cachedTotal += *current.CachedInputTokens
		nonCachedTotal += *current.NonCachedInputTokens
		outputTotal += *current.OutputTokens
	}
	if allUsageExposed {
		out.UsageExposed = true
		out.InputTokens = &inputTotal
		out.CachedInputTokens = &cachedTotal
		out.NonCachedInputTokens = &nonCachedTotal
		out.OutputTokens = &outputTotal
	}
	return out
}

func aggregateVerification(sc scenario, turns []turnResult) verificationResult {
	out := verificationResult{DatabasePass: true, AssistantPass: true, Passed: true}
	details := []string{}
	for _, turn := range turns {
		verification := turn.Verification
		if !verification.DatabasePass {
			out.DatabasePass = false
		}
		if !verification.AssistantPass {
			out.AssistantPass = false
		}
		if !verification.Passed {
			out.Passed = false
		}
		if verification.Details != "" {
			details = append(details, fmt.Sprintf("turn %d: %s", turn.Index, verification.Details))
		}
		out.Calendars = verification.Calendars
		out.Events = verification.Events
		out.Tasks = verification.Tasks
		out.Agenda = verification.Agenda
	}
	if len(details) > 0 {
		out.Details = strings.Join(details, "; ")
	}
	if len(turns) == 0 {
		out.Passed = false
		out.DatabasePass = false
		out.AssistantPass = false
		out.Details = fmt.Sprintf("scenario %s did not run any turns", sc.ID)
	}
	return out
}

func countSingleTurnJobs(jobs []evalJob) int {
	count := 0
	for _, job := range jobs {
		if !isMultiTurnScenario(job.Scenario) {
			count++
		}
	}
	return count
}

func countMultiTurnJobs(jobs []evalJob) int {
	count := 0
	for _, job := range jobs {
		if isMultiTurnScenario(job.Scenario) {
			count++
		}
	}
	return count
}

func countMultiTurnPersistedTurns(jobs []evalJob) int {
	count := 0
	for _, job := range jobs {
		if isMultiTurnScenario(job.Scenario) {
			count += len(scenarioTurns(job.Scenario))
		}
	}
	return count
}

func scenarios() []scenario {
	return []scenario{
		{ID: "ensure-calendar", Title: "Ensure a calendar idempotently", Prompt: "Use the configured local OpenPlanner data path. Ensure a calendar named Personal exists. Then tell me whether the Personal calendar exists."},
		{ID: "create-timed-event", Title: "Create a timed event", Prompt: "Use the configured local OpenPlanner data path. Add a Work calendar event titled Standup from 2026-04-16T09:00:00Z to 2026-04-16T10:00:00Z. Then tell me what event is stored."},
		{ID: "create-all-day-event", Title: "Create an all-day event", Prompt: "Use the configured local OpenPlanner data path. Add a Personal all-day event titled Planning day on 2026-04-17. Then tell me what event is stored."},
		{ID: "create-dated-task", Title: "Create a dated task", Prompt: "Use the configured local OpenPlanner data path. Add a Personal task titled Review notes due on 2026-04-16. Then tell me what task is stored."},
		{ID: "create-timed-task", Title: "Create a timed task", Prompt: "Use the configured local OpenPlanner data path. Add a Work task titled Send summary due at 2026-04-16T11:00:00Z. Then tell me what task is stored."},
		{ID: "create-recurring-event", Title: "Create a recurring event", Prompt: "Use the configured local OpenPlanner data path. Add a Work event titled Daily standup from 2026-04-16T09:00:00Z to 2026-04-16T09:30:00Z recurring daily for 3 occurrences. Then tell me the recurrence stored."},
		{ID: "create-recurring-task", Title: "Create a recurring task", Prompt: "Use the configured local OpenPlanner data path. Add a Personal task titled Daily review due on 2026-04-16 recurring daily for 3 occurrences. Then tell me the recurrence stored."},
		{ID: "agenda-range", Title: "List a bounded agenda range chronologically", Prompt: "Use the configured local OpenPlanner data path. Show my agenda from 2026-04-16T00:00:00Z to 2026-04-17T00:00:00Z. Mention only items in that range, chronologically."},
		{ID: "list-events-filter-limit", Title: "List events with calendar filter and limit", Prompt: "Use the configured local OpenPlanner data path. List only the first Work calendar event. Do not mention Personal calendar events."},
		{ID: "list-tasks-filter-limit", Title: "List tasks with calendar filter and limit", Prompt: "Use the configured local OpenPlanner data path. List only the first Personal calendar task. Do not mention Work calendar tasks."},
		{ID: "complete-task", Title: "Complete a non-recurring task", Prompt: "Use the configured local OpenPlanner data path. Complete the Personal task titled Review notes due on 2026-04-16. Tell me what was completed."},
		{ID: "complete-recurring-task", Title: "Complete a recurring task occurrence", Prompt: "Use the configured local OpenPlanner data path. Complete the 2026-04-17 occurrence of the Personal recurring task titled Daily review. Tell me what occurrence was completed."},
		{ID: "mixed-event-task", Title: "Create an event and a task in one user turn", Prompt: "Use the configured local OpenPlanner data path. Add a Work event titled Standup from 2026-04-16T09:00:00Z to 2026-04-16T10:00:00Z and a Personal task titled Review notes due on 2026-04-16. Then tell me both stored items."},
		{ID: "ambiguous-short-date", Title: "Clarify an ambiguous short date without writing", Prompt: "Please add a local OpenPlanner task titled Review notes due 04/16. There is no year context in this conversation or my request."},
		{ID: "year-first-slash-date", Title: "Reject a year-first slash date without writing", Prompt: "Please add this local OpenPlanner task exactly as written: Review notes due 2026/04/16. If OpenPlanner requires another date format, reject this request directly without running tools. Do not normalize or rewrite the date."},
		{ID: "invalid-rfc3339", Title: "Reject an invalid RFC3339 time without writing", Prompt: "Please add a local OpenPlanner event titled Standup exactly as written from 2026-04-16 09:00 to 2026-04-16 10:00. If OpenPlanner requires RFC3339 timed fields, reject this request directly without running tools. Do not normalize or rewrite the times."},
		{ID: "missing-title", Title: "Reject a missing required title without writing", Prompt: "Please add a local OpenPlanner task due on 2026-04-16, but I do not have a title for it."},
		{ID: "invalid-range", Title: "Reject an invalid agenda range without writing", Prompt: "Please show my OpenPlanner agenda from 2026-04-18T00:00:00Z to 2026-04-16T00:00:00Z."},
		{ID: "unsupported-recurrence", Title: "Reject unsupported recurrence without writing", Prompt: "Please add a local OpenPlanner task titled Review notes due on 2026-04-16 recurring hourly."},
		{ID: "non-positive-limit", Title: "Reject a non-positive list limit without writing", Prompt: "Please list 0 OpenPlanner tasks."},
		{ID: "mt-clarify-then-create", Title: "Clarify missing year, then create in a resumed turn", Turns: []scenarioTurn{
			{Prompt: "Please add a local OpenPlanner Personal task titled Review notes due 04/16. There is no year context in this conversation or my request."},
			{Prompt: "Use 2026 as the year for that Personal task."},
		}},
		{ID: "mt-list-then-complete", Title: "List a task, then complete it in a resumed turn", Turns: []scenarioTurn{
			{Prompt: "Use the configured local OpenPlanner data path. What Personal tasks are due on 2026-04-16? Mention only the matching task."},
			{Prompt: "Complete the task you just found and tell me what was completed."},
		}},
	}
}

func scenarioTurns(sc scenario) []scenarioTurn {
	if len(sc.Turns) > 0 {
		return sc.Turns
	}
	return []scenarioTurn{{Prompt: sc.Prompt}}
}

func isMultiTurnScenario(sc scenario) bool {
	return len(scenarioTurns(sc)) > 1
}

func selectScenarios(filter string) ([]scenario, error) {
	all := scenarios()
	if strings.TrimSpace(filter) == "" {
		return all, nil
	}
	ids := splitFilterIDs(filter)
	if len(ids) == 0 {
		return nil, errors.New("scenario filter did not include any scenario ids")
	}
	selected := []scenario{}
	for _, id := range ids {
		found := false
		for _, candidate := range all {
			if candidate.ID == id {
				selected = append(selected, candidate)
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("unknown scenario %q", id)
		}
	}
	return selected, nil
}

func splitFilterIDs(filter string) []string {
	ids := []string{}
	for _, raw := range strings.Split(filter, ",") {
		id := strings.TrimSpace(raw)
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func seedScenario(dbPath string, sc scenario) error {
	switch sc.ID {
	case "agenda-range":
		return seedAgendaRange(dbPath)
	case "list-events-filter-limit":
		return seedEventFilter(dbPath)
	case "list-tasks-filter-limit", "complete-task", "mt-list-then-complete":
		return seedReviewTask(dbPath)
	case "complete-recurring-task":
		return seedRecurringTask(dbPath)
	default:
		return nil
	}
}

func seedAgendaRange(dbPath string) error {
	requests := []agentops.PlanningTaskRequest{
		{Action: agentops.PlanningTaskActionCreateTask, CalendarName: "Work", Title: "Review notes", DueDate: "2026-04-16"},
		{Action: agentops.PlanningTaskActionCreateEvent, CalendarName: "Work", Title: "Standup", StartAt: "2026-04-16T09:00:00Z", EndAt: "2026-04-16T10:00:00Z"},
		{Action: agentops.PlanningTaskActionCreateEvent, CalendarName: "Work", Title: "Out of range", StartAt: "2026-04-17T09:00:00Z", EndAt: "2026-04-17T10:00:00Z"},
	}
	return runSeedRequests(dbPath, requests)
}

func seedEventFilter(dbPath string) error {
	requests := []agentops.PlanningTaskRequest{
		{Action: agentops.PlanningTaskActionCreateEvent, CalendarName: "Work", Title: "Work sync", StartAt: "2026-04-16T09:00:00Z", EndAt: "2026-04-16T09:30:00Z"},
		{Action: agentops.PlanningTaskActionCreateEvent, CalendarName: "Personal", Title: "Personal appointment", StartAt: "2026-04-16T10:00:00Z", EndAt: "2026-04-16T10:30:00Z"},
	}
	return runSeedRequests(dbPath, requests)
}

func seedReviewTask(dbPath string) error {
	return runSeedRequests(dbPath, []agentops.PlanningTaskRequest{
		{Action: agentops.PlanningTaskActionCreateTask, CalendarName: "Personal", Title: "Review notes", DueDate: "2026-04-16"},
		{Action: agentops.PlanningTaskActionCreateTask, CalendarName: "Work", Title: "Work backlog", DueDate: "2026-04-16"},
	})
}

func seedRecurringTask(dbPath string) error {
	count := int32(3)
	return runSeedRequests(dbPath, []agentops.PlanningTaskRequest{
		{Action: agentops.PlanningTaskActionCreateTask, CalendarName: "Personal", Title: "Daily review", DueDate: "2026-04-16", Recurrence: &agentops.RecurrenceRuleRequest{Frequency: "daily", Count: &count}},
	})
}

func runSeedRequests(dbPath string, requests []agentops.PlanningTaskRequest) error {
	for _, request := range requests {
		result, err := runPlanning(dbPath, request)
		if err != nil {
			return err
		}
		if result.Rejected {
			return fmt.Errorf("seed request rejected: %s", result.RejectionReason)
		}
	}
	return nil
}

func verifyScenarioTurn(dbPath string, sc scenario, turnIndex int, finalMessage string) (verificationResult, error) {
	switch sc.ID {
	case "ensure-calendar":
		return verifyCalendar(dbPath, "Personal", finalMessage)
	case "create-timed-event":
		return verifyEvents(dbPath, finalMessage, []eventState{{Title: "Standup", StartAt: "2026-04-16T09:00:00Z"}}, nil)
	case "create-all-day-event":
		return verifyEvents(dbPath, finalMessage, []eventState{{Title: "Planning day", StartDate: "2026-04-17"}}, nil)
	case "create-dated-task":
		return verifyTasks(dbPath, finalMessage, []taskState{{Title: "Review notes", DueDate: "2026-04-16"}}, nil, false)
	case "create-timed-task":
		return verifyTasks(dbPath, finalMessage, []taskState{{Title: "Send summary", DueAt: "2026-04-16T11:00:00Z"}}, nil, false)
	case "create-recurring-event":
		return verifyEvents(dbPath, finalMessage, []eventState{{Title: "Daily standup", StartAt: "2026-04-16T09:00:00Z", Recurrence: "daily"}}, nil)
	case "create-recurring-task":
		return verifyTasks(dbPath, finalMessage, []taskState{{Title: "Daily review", DueDate: "2026-04-16", Recurrence: "daily"}}, nil, false)
	case "agenda-range":
		return verifyAgendaRange(dbPath, finalMessage)
	case "list-events-filter-limit":
		return verifyEvents(dbPath, finalMessage, []eventState{{Title: "Work sync"}}, []string{"Personal appointment"})
	case "list-tasks-filter-limit":
		return verifyTasks(dbPath, finalMessage, []taskState{{Title: "Review notes"}}, []string{"Work backlog"}, false)
	case "complete-task":
		return verifyTasks(dbPath, finalMessage, []taskState{{Title: "Review notes"}}, nil, true)
	case "complete-recurring-task":
		return verifyRecurringTaskCompletion(dbPath, finalMessage)
	case "mixed-event-task":
		eventCheck, err := verifyEvents(dbPath, finalMessage, []eventState{{Title: "Standup", StartAt: "2026-04-16T09:00:00Z"}}, nil)
		if err != nil || !eventCheck.Passed {
			return eventCheck, err
		}
		return verifyTasks(dbPath, finalMessage, []taskState{{Title: "Review notes", DueDate: "2026-04-16"}}, nil, false)
	case "ambiguous-short-date":
		return verifyFinalAnswerOnlyRejection(dbPath, finalMessage, []string{"year"})
	case "year-first-slash-date":
		return verifyFinalAnswerOnlyRejection(dbPath, finalMessage, []string{"yyyy-mm-dd", "format", "year-first", "slash", "accept"})
	case "invalid-rfc3339":
		return verifyFinalAnswerOnlyRejection(dbPath, finalMessage, []string{"rfc3339", "format"})
	case "missing-title":
		return verifyFinalAnswerOnlyRejection(dbPath, finalMessage, []string{"title"})
	case "invalid-range":
		return verifyFinalAnswerOnlyRejection(dbPath, finalMessage, []string{"range", "before", "after"})
	case "unsupported-recurrence":
		return verifyFinalAnswerOnlyRejection(dbPath, finalMessage, []string{"recurrence", "unsupported", "daily", "weekly", "monthly"})
	case "non-positive-limit":
		return verifyFinalAnswerOnlyRejection(dbPath, finalMessage, []string{"limit", "positive"})
	case "mt-clarify-then-create":
		if turnIndex == 1 {
			return verifyFinalAnswerOnlyRejection(dbPath, finalMessage, []string{"year"})
		}
		return verifyTasks(dbPath, finalMessage, []taskState{{Title: "Review notes", DueDate: "2026-04-16"}}, nil, false)
	case "mt-list-then-complete":
		if turnIndex == 1 {
			return verifyTasks(dbPath, finalMessage, []taskState{{Title: "Review notes"}}, []string{"Work backlog"}, false)
		}
		return verifyTasks(dbPath, finalMessage, []taskState{{Title: "Review notes"}}, nil, true)
	default:
		return verificationResult{Passed: false, DatabasePass: false, AssistantPass: false, Details: "unknown scenario"}, nil
	}
}

func verifyCalendar(dbPath string, name string, finalMessage string) (verificationResult, error) {
	calendars, err := listCalendars(dbPath)
	if err != nil {
		return verificationResult{}, err
	}
	databasePass := calendarNameExists(calendars, name)
	assistantPass := mentionsAll(finalMessage, name)
	return verificationResult{
		Passed:        databasePass && assistantPass,
		DatabasePass:  databasePass,
		AssistantPass: assistantPass,
		Details:       passDetails(databasePass, assistantPass, "expected calendar in DB and final answer"),
		Calendars:     []calendarState{{Name: name}},
	}, nil
}

func listCalendars(dbPath string) ([]sdk.Calendar, error) {
	if !fileExists(dbPath) {
		return nil, nil
	}
	api, err := sdk.OpenLocal(sdk.Options{DatabasePath: dbPath})
	if err != nil {
		return nil, err
	}
	defer func() { _ = api.Close() }()
	page, err := api.ListCalendars(context.Background(), sdk.ListOptions{Limit: 100})
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

func calendarNameExists(calendars []sdk.Calendar, name string) bool {
	for _, calendar := range calendars {
		if calendar.Name == name {
			return true
		}
	}
	return false
}

func verifyEvents(dbPath string, finalMessage string, expected []eventState, forbidden []string) (verificationResult, error) {
	result, err := runPlanning(dbPath, agentops.PlanningTaskRequest{Action: agentops.PlanningTaskActionListEvents, Limit: intPtr(100)})
	if err != nil {
		return verificationResult{}, err
	}
	databasePass := !result.Rejected
	for _, want := range expected {
		if !eventExists(result.Events, want) {
			databasePass = false
		}
	}
	assistantPass := finalMentionsExpected(finalMessage, eventMentionValues(expected), forbidden)
	return verificationResult{
		Passed:        databasePass && assistantPass,
		DatabasePass:  databasePass,
		AssistantPass: assistantPass,
		Details:       passDetails(databasePass, assistantPass, "expected events in DB and final answer"),
		Events:        expected,
	}, nil
}

func verifyTasks(dbPath string, finalMessage string, expected []taskState, forbidden []string, requireCompleted bool) (verificationResult, error) {
	result, err := runPlanning(dbPath, agentops.PlanningTaskRequest{Action: agentops.PlanningTaskActionListTasks, Limit: intPtr(100)})
	if err != nil {
		return verificationResult{}, err
	}
	databasePass := !result.Rejected
	for _, want := range expected {
		if !taskExists(result.Tasks, want, requireCompleted) {
			databasePass = false
		}
	}
	assistantPass := finalMentionsExpected(finalMessage, taskMentionValues(expected), forbidden)
	return verificationResult{
		Passed:        databasePass && assistantPass,
		DatabasePass:  databasePass,
		AssistantPass: assistantPass,
		Details:       passDetails(databasePass, assistantPass, "expected tasks in DB and final answer"),
		Tasks:         expected,
	}, nil
}

func verifyAgendaRange(dbPath string, finalMessage string) (verificationResult, error) {
	result, err := runPlanning(dbPath, agentops.PlanningTaskRequest{
		Action: agentops.PlanningTaskActionListAgenda,
		From:   "2026-04-16T00:00:00Z",
		To:     "2026-04-17T00:00:00Z",
		Limit:  intPtr(100),
	})
	if err != nil {
		return verificationResult{}, err
	}
	databasePass := !result.Rejected && len(result.Agenda) == 2 && result.Agenda[0].Title == "Review notes" && result.Agenda[1].Title == "Standup"
	assistantPass := mentionsInOrder(finalMessage, "Review notes", "Standup") && !mentionsAny(finalMessage, []string{"Out of range"})
	return verificationResult{
		Passed:        databasePass && assistantPass,
		DatabasePass:  databasePass,
		AssistantPass: assistantPass,
		Details:       passDetails(databasePass, assistantPass, "expected bounded agenda chronologically"),
		Agenda: []agendaEntryState{
			{Kind: "task", Title: "Review notes", DueDate: "2026-04-16"},
			{Kind: "event", Title: "Standup", StartAt: "2026-04-16T09:00:00Z"},
		},
	}, nil
}

func verifyRecurringTaskCompletion(dbPath string, finalMessage string) (verificationResult, error) {
	result, err := runPlanning(dbPath, agentops.PlanningTaskRequest{
		Action: agentops.PlanningTaskActionListAgenda,
		From:   "2026-04-17T00:00:00Z",
		To:     "2026-04-18T00:00:00Z",
		Limit:  intPtr(100),
	})
	if err != nil {
		return verificationResult{}, err
	}
	databasePass := false
	for _, item := range result.Agenda {
		if item.Title == "Daily review" && item.DueDate == "2026-04-17" && item.CompletedAt != "" {
			databasePass = true
		}
	}
	assistantPass := mentionsAll(finalMessage, "Daily review", "2026-04-17")
	return verificationResult{
		Passed:        databasePass && assistantPass,
		DatabasePass:  databasePass,
		AssistantPass: assistantPass,
		Details:       passDetails(databasePass, assistantPass, "expected recurring task occurrence completed"),
		Agenda:        []agendaEntryState{{Kind: "task", Title: "Daily review", DueDate: "2026-04-17", CompletedAt: "set"}},
	}, nil
}

func verifyFinalAnswerOnlyRejection(dbPath string, finalMessage string, anyKeywords []string) (verificationResult, error) {
	databasePass := !fileExists(dbPath)
	assistantPass := finalMessage != "" && mentionsAny(finalMessage, anyKeywords)
	return verificationResult{
		Passed:        databasePass && assistantPass,
		DatabasePass:  databasePass,
		AssistantPass: assistantPass,
		Details:       passDetails(databasePass, assistantPass, "expected direct rejection or clarification without DB writes"),
	}, nil
}

func runPlanning(dbPath string, request agentops.PlanningTaskRequest) (agentops.PlanningTaskResult, error) {
	return agentops.RunPlanningTask(context.Background(), sdk.Options{DatabasePath: dbPath}, request)
}

func eventExists(events []agentops.EventEntry, want eventState) bool {
	for _, event := range events {
		if event.Title != want.Title {
			continue
		}
		if want.StartAt != "" && event.StartAt != want.StartAt {
			continue
		}
		if want.StartDate != "" && event.StartDate != want.StartDate {
			continue
		}
		if want.Recurrence != "" && (event.Recurrence == nil || event.Recurrence.Frequency != want.Recurrence) {
			continue
		}
		return true
	}
	return false
}

func taskExists(tasks []agentops.TaskEntry, want taskState, requireCompleted bool) bool {
	for _, task := range tasks {
		if task.Title != want.Title {
			continue
		}
		if want.DueAt != "" && task.DueAt != want.DueAt {
			continue
		}
		if want.DueDate != "" && task.DueDate != want.DueDate {
			continue
		}
		if want.Recurrence != "" && (task.Recurrence == nil || task.Recurrence.Frequency != want.Recurrence) {
			continue
		}
		if requireCompleted && task.CompletedAt == "" {
			continue
		}
		return true
	}
	return false
}

func eventMentionValues(expected []eventState) []string {
	values := []string{}
	for _, event := range expected {
		if event.Recurrence != "" {
			values = append(values, event.Recurrence)
			continue
		}
		values = append(values, event.Title)
		if event.StartAt != "" {
			values = append(values, event.StartAt[:10])
		}
		if event.StartDate != "" {
			values = append(values, event.StartDate)
		}
	}
	return values
}

func taskMentionValues(expected []taskState) []string {
	values := []string{}
	for _, task := range expected {
		if task.Recurrence != "" {
			values = append(values, task.Recurrence)
			continue
		}
		values = append(values, task.Title)
		if task.DueAt != "" {
			values = append(values, task.DueAt[:10])
		}
		if task.DueDate != "" {
			values = append(values, task.DueDate)
		}
	}
	return values
}

func finalMentionsExpected(message string, expected []string, forbidden []string) bool {
	return mentionsAll(message, expected...) && !mentionsAny(message, forbidden)
}

func mentionsAll(message string, values ...string) bool {
	lower := strings.ToLower(message)
	for _, value := range values {
		if !strings.Contains(lower, strings.ToLower(value)) {
			return false
		}
	}
	return true
}

func mentionsAny(message string, values []string) bool {
	lower := strings.ToLower(message)
	for _, value := range values {
		if strings.Contains(lower, strings.ToLower(value)) {
			return true
		}
	}
	return false
}

func mentionsInOrder(message string, first string, second string) bool {
	lower := strings.ToLower(message)
	firstIndex := strings.Index(lower, strings.ToLower(first))
	secondIndex := strings.Index(lower, strings.ToLower(second))
	return firstIndex >= 0 && secondIndex >= 0 && firstIndex < secondIndex
}

func passDetails(databasePass bool, assistantPass bool, success string) string {
	if databasePass && assistantPass {
		return success
	}
	return fmt.Sprintf("%s; database_pass=%t assistant_pass=%t", success, databasePass, assistantPass)
}

func parseMetrics(path string) (parsedMetrics, error) {
	file, err := os.Open(path)
	if err != nil {
		return parsedMetrics{}, err
	}
	defer func() { _ = file.Close() }()

	out := parsedMetrics{
		metrics: metrics{
			EventTypeCounts:          map[string]int{},
			CommandMetricLimitations: "Command/file inspection metrics are inferred from codex exec JSON command events, not from OS-level tracing.",
		},
	}
	commandIDs := map[string]struct{}{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}
		var event codexEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		out.metrics.EventTypeCounts[event.Type]++
		if event.Type == "thread.started" && event.ThreadID != "" && out.sessionID == "" {
			out.sessionID = event.ThreadID
		}
		switch event.Item.Type {
		case "agent_message":
			if event.Type == "item.completed" {
				out.metrics.AssistantCalls++
				out.finalMessage = event.Item.Text
			}
		case "command_execution":
			if event.Item.ID != "" {
				commandIDs[event.Item.ID] = struct{}{}
			}
			if event.Type == "item.completed" {
				out.metrics.CommandExecutions++
				if isFileInspectionCommand(event.Item.Command) {
					out.metrics.FileInspectionCommands++
				}
				if isBroadRepoSearchCommand(event.Item.Command) {
					out.metrics.BroadRepoSearch = true
					addMetricEvidence(&out.metrics.BroadRepoSearchEvidence, event.Item.Command)
					if mentionsGeneratedPath(event.Item.AggregatedOutput) {
						out.metrics.GeneratedPathFromBroadSearch = true
						addMetricEvidence(&out.metrics.GeneratedPathFromBroadSearchEvidence, event.Item.Command)
					}
				}
				if inspectsGeneratedFileCommand(event.Item.Command, event.Item.AggregatedOutput) {
					out.metrics.GeneratedFileInspected = true
					addMetricEvidence(&out.metrics.GeneratedFileEvidence, event.Item.Command)
				}
				if inspectsModuleCache(event.Item.Command) {
					out.metrics.ModuleCacheInspected = true
					addMetricEvidence(&out.metrics.ModuleCacheEvidence, event.Item.Command)
				}
				if usesOpenPlannerCLI(event.Item.Command) {
					out.metrics.CLIUsed = true
					addMetricEvidence(&out.metrics.CLIUsageEvidence, event.Item.Command)
				}
				if usesDirectSQLite(event.Item.Command) {
					out.metrics.DirectSQLiteAccess = true
					addMetricEvidence(&out.metrics.DirectSQLiteEvidence, event.Item.Command)
				}
			}
		}
		if event.Usage != nil {
			input := event.Usage.InputTokens
			cached := event.Usage.CachedInputTokens
			nonCached := input - cached
			output := event.Usage.OutputTokens
			out.metrics.UsageExposed = true
			out.metrics.InputTokens = &input
			out.metrics.CachedInputTokens = &cached
			out.metrics.NonCachedInputTokens = &nonCached
			out.metrics.OutputTokens = &output
		}
	}
	if err := scanner.Err(); err != nil {
		return out, err
	}
	out.metrics.ToolCalls = len(commandIDs)
	return out, nil
}

func prepareRunDir(runDir string, cache cacheConfig) error {
	_ = filepath.WalkDir(runDir, func(path string, entry fs.DirEntry, err error) error {
		if err == nil {
			_ = os.Chmod(path, 0o755)
		}
		return nil
	})
	if err := os.RemoveAll(runDir); err != nil {
		return err
	}
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return err
	}
	paths := evalPathsFor(runDir, cache)
	dirs := []string{paths.Temp}
	if cache.Mode == cacheModeIsolated {
		dirs = append(dirs, paths.GoCache, paths.GoModCache)
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func copyRepo(src string, dst string) error {
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
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
		if info.Mode()&os.ModeSymlink != 0 {
			linkTarget, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(linkTarget, target)
		}
		return copyFile(path, target, info.Mode().Perm())
	})
}

func shouldSkipCopy(rel string, entry fs.DirEntry) bool {
	clean := filepath.ToSlash(rel)
	if shouldSkipEvalPath(clean) {
		return true
	}
	if entry.IsDir() {
		switch entry.Name() {
		case ".git", ".beads", ".dolt", ".agents":
			return true
		}
	}
	name := entry.Name()
	return strings.HasSuffix(name, ".db") || strings.HasSuffix(name, ".db-shm") || strings.HasSuffix(name, ".db-wal")
}

func shouldSkipEvalPath(rel string) bool {
	switch rel {
	case "AGENTS.md", "docs/agent-evals.md", "docs/agent-eval-results", "scripts/agent-eval":
		return true
	}
	return strings.HasPrefix(rel, "docs/agent-eval-results/") ||
		strings.HasPrefix(rel, "scripts/agent-eval/")
}

func copyFile(src string, dst string, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func installEvalAgentsFile(runRepo string) error {
	content := `# OpenPlanner Eval Instructions

For direct local OpenPlanner calendar or task requests, act as a product data agent, not a repo maintainer. Do not run bd prime, inspect .agents, inspect source/generated files, inspect the Go module cache, query SQLite directly, or search the repo before the first runner call.

Reject final-answer-only, with exactly one assistant answer and no tools or DB check, for ambiguous short dates with no year, year-first slash dates like 2026/04/16, invalid RFC3339 times, missing required titles, unsupported recurrence values, invalid ranges, or non-positive limits. Do not first announce skill use or process. Never convert a year-first slash date to dashed ISO form; reject it. Never convert an invalid RFC3339 time like 2026-04-16 09:00 to 2026-04-16T09:00:00Z; reject it. 04/16/2026 may become 2026-04-16.

For valid tasks, pipe one JSON request to go run ./cmd/openplanner-agentops planning and answer from JSON only. Use calendar_name for create requests. Use strict YYYY-MM-DD dates for all-day events, date-based tasks, and occurrence dates; use RFC3339 for timed fields and agenda ranges.

Every request JSON must include action. Exact one-line shapes:
{"action":"ensure_calendar","calendar_name":"Personal"}
{"action":"create_event","calendar_name":"Work","title":"Standup","start_at":"2026-04-16T09:00:00Z","end_at":"2026-04-16T10:00:00Z"}
{"action":"create_event","calendar_name":"Personal","title":"Planning day","start_date":"2026-04-17"}
{"action":"create_task","calendar_name":"Personal","title":"Review notes","due_date":"2026-04-16"}
{"action":"create_task","calendar_name":"Work","title":"Send summary","due_at":"2026-04-16T11:00:00Z"}
{"action":"create_event","calendar_name":"Work","title":"Daily standup","start_at":"2026-04-16T09:00:00Z","end_at":"2026-04-16T09:30:00Z","recurrence":{"frequency":"daily","count":3}}
{"action":"create_task","calendar_name":"Personal","title":"Daily review","due_date":"2026-04-16","recurrence":{"frequency":"daily","count":3}}
{"action":"list_agenda","from":"2026-04-16T00:00:00Z","to":"2026-04-17T00:00:00Z","limit":100}
{"action":"list_events","calendar_name":"Work","limit":1}
{"action":"list_tasks","calendar_name":"Personal","limit":1}
{"action":"complete_task","task_id":"<id-from-prior-runner-result>"}
{"action":"complete_task","task_id":"<id-from-prior-runner-result>","occurrence_date":"2026-04-17"}
`
	return os.WriteFile(filepath.Join(runRepo, "AGENTS.md"), []byte(content), 0o644)
}

func warmGoModules(runRepo string, runDir string, dbPath string, cache cacheConfig) error {
	cmd := exec.Command("go", "mod", "download")
	cmd.Dir = runRepo
	cmd.Env = evalEnv(runDir, dbPath, cache)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func prewarmSharedCache(repoRoot string, cache cacheConfig) error {
	paths := sharedEvalPaths(cache)
	for _, dir := range []string{paths.GoCache, paths.GoModCache, paths.Temp} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	dbPath := filepath.Join(filepath.Dir(paths.Temp), "prewarm.db")
	if err := warmGoModules(repoRoot, filepath.Dir(paths.Temp), dbPath, cache); err != nil {
		return err
	}
	cmd := exec.Command("go", prewarmCompileArgs()...)
	cmd.Dir = repoRoot
	cmd.Env = evalEnv(filepath.Dir(paths.Temp), dbPath, cache)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func prewarmCompileArgs() []string {
	args := []string{"test", "-run", "^$"}
	return append(args, prewarmCompilePackages...)
}

type evalPaths struct {
	GoCache    string
	GoModCache string
	Temp       string
}

func evalPathsFor(runDir string, cache cacheConfig) evalPaths {
	if cache.Mode == cacheModeShared {
		paths := sharedEvalPaths(cache)
		paths.Temp = filepath.Join(runDir, "tmp")
		return paths
	}
	return evalPaths{
		GoCache:    filepath.Join(runDir, "gocache"),
		GoModCache: filepath.Join(runDir, "gomodcache"),
		Temp:       filepath.Join(runDir, "tmp"),
	}
}

func sharedEvalPaths(cache cacheConfig) evalPaths {
	root := filepath.Join(cache.RunRoot, "shared-cache")
	return evalPaths{
		GoCache:    filepath.Join(root, "gocache"),
		GoModCache: filepath.Join(root, "gomodcache"),
		Temp:       filepath.Join(root, "tmp"),
	}
}

func productionScoreFor(results []runResult) productionScore {
	criteria := []criterion{}
	total := len(results)
	passed := countPassed(results)
	criteria = append(criteria, criterion{
		Name:    "production_passes_all_scenarios",
		Passed:  passed == total,
		Details: fmt.Sprintf("%d/%d scenarios passed", passed, total),
	})

	invalidFailures := []string{}
	for _, result := range results {
		if isFinalAnswerOnlyValidationScenario(result.Scenario) && (result.Metrics.ToolCalls != 0 || result.Metrics.CommandExecutions != 0 || result.Metrics.AssistantCalls > 1) {
			invalidFailures = append(invalidFailures, result.Scenario)
		}
	}
	criteria = append(criteria, criterion{
		Name:    "invalid_inputs_are_final_answer_only",
		Passed:  len(invalidFailures) == 0,
		Details: finalAnswerOnlyDetails(invalidFailures),
	})

	inspectionFailures := []string{}
	for _, result := range results {
		if result.Metrics.GeneratedFileInspected || result.Metrics.GeneratedPathFromBroadSearch || result.Metrics.ModuleCacheInspected || result.Metrics.DirectSQLiteAccess || result.Metrics.CLIUsed || (isRoutineScenario(result.Scenario) && result.Metrics.BroadRepoSearch) {
			inspectionFailures = append(inspectionFailures, result.Scenario)
		}
	}
	criteria = append(criteria, criterion{
		Name:    "no_forbidden_inspection_or_cli_usage",
		Passed:  len(inspectionFailures) == 0,
		Details: forbiddenInspectionDetails(inspectionFailures),
	})

	tokenScenarios := 0
	totalNonCached := 0
	for _, result := range results {
		if value, ok := nonCachedInputTokens(result); ok {
			tokenScenarios++
			totalNonCached += value
		}
	}
	criteria = append(criteria, criterion{
		Name:    "aggregate_non_cached_tokens_reported",
		Passed:  tokenScenarios == total,
		Details: fmt.Sprintf("%d/%d scenarios exposed usage; aggregate non-cached input tokens: %d", tokenScenarios, total, totalNonCached),
	})

	allPassed := true
	for _, criterion := range criteria {
		if !criterion.Passed {
			allPassed = false
		}
	}
	recommendation := "prefer_agentops_for_routine_openplanner_operations"
	if !allPassed {
		recommendation = "review_agentops_eval_failures_before_recommending"
	}
	return productionScore{Recommendation: recommendation, Passed: allPassed, Criteria: criteria}
}

func metricNotes(results []runResult) []string {
	totalTools := 0
	totalCommands := 0
	totalNonCached := 0
	exposed := 0
	for _, result := range results {
		totalTools += result.Metrics.ToolCalls
		totalCommands += result.Metrics.CommandExecutions
		if tokens, ok := nonCachedInputTokens(result); ok {
			exposed++
			totalNonCached += tokens
		}
	}
	return []string{
		fmt.Sprintf("production total tools: %d", totalTools),
		fmt.Sprintf("production total command executions: %d", totalCommands),
		fmt.Sprintf("non-cached input tokens exposed for %d/%d scenarios; aggregate=%d", exposed, len(results), totalNonCached),
		"CLI comparison gates are intentionally n/a because OpenPlanner has no human CLI baseline variant.",
	}
}

func countPassed(results []runResult) int {
	count := 0
	for _, result := range results {
		if result.Passed {
			count++
		}
	}
	return count
}

func nonCachedInputTokens(result runResult) (int, bool) {
	if !result.Metrics.UsageExposed || result.Metrics.NonCachedInputTokens == nil {
		return 0, false
	}
	return *result.Metrics.NonCachedInputTokens, true
}

func isFinalAnswerOnlyValidationScenario(id string) bool {
	switch id {
	case "ambiguous-short-date", "year-first-slash-date", "invalid-rfc3339", "missing-title", "invalid-range", "unsupported-recurrence", "non-positive-limit":
		return true
	default:
		return false
	}
}

func isRoutineScenario(id string) bool {
	return !isFinalAnswerOnlyValidationScenario(id)
}

func finalAnswerOnlyDetails(failures []string) string {
	if len(failures) == 0 {
		return "invalid-input scenarios used no tools, no command executions, and at most one assistant answer"
	}
	return fmt.Sprintf("invalid-input scenarios were not final-answer-only: %s", sortedJoin(failures))
}

func forbiddenInspectionDetails(failures []string) string {
	if len(failures) == 0 {
		return "no generated-file inspection, generated-path broad search, module-cache inspection, direct SQLite access, CLI usage, or routine broad repo search detected"
	}
	return fmt.Sprintf("forbidden inspection or CLI usage detected in: %s", sortedJoin(failures))
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func writeMarkdown(path string, value report) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# OpenPlanner AgentOps Eval %s\n\n", value.Date)
	fmt.Fprintf(&b, "- Model: `%s`\n", value.Model)
	fmt.Fprintf(&b, "- Reasoning effort: `%s`\n", value.ReasoningEffort)
	fmt.Fprintf(&b, "- Parallelism: `%d`\n", value.Parallelism)
	fmt.Fprintf(&b, "- Cache mode: `%s`\n", value.CacheMode)
	if value.CachePrewarmSeconds > 0 {
		fmt.Fprintf(&b, "- Cache prewarm seconds: `%.2f`\n", value.CachePrewarmSeconds)
	}
	fmt.Fprintf(&b, "- Harness elapsed seconds: `%.2f`\n", value.HarnessElapsedSeconds)
	if value.EffectiveSpeedup > 0 {
		fmt.Fprintf(&b, "- Effective parallel speedup: `%.2fx`\n", value.EffectiveSpeedup)
	}
	if value.ParallelEfficiency > 0 {
		fmt.Fprintf(&b, "- Parallel efficiency: `%.2f`\n", value.ParallelEfficiency)
	}
	fmt.Fprintf(&b, "- Production score: `%s`\n", passFail(value.ProductionScore.Passed))
	fmt.Fprintf(&b, "- Comparison status: %s\n", value.ComparisonStatus)
	fmt.Fprintf(&b, "- Raw logs committed: `%t`\n", value.RawLogsCommitted)
	fmt.Fprintf(&b, "- Raw logs note: %s\n\n", value.RawLogsNote)

	b.WriteString("## Production Gates\n\n")
	b.WriteString("| Criterion | Passed | Details |\n")
	b.WriteString("| --- | ---: | --- |\n")
	for _, criterion := range value.ProductionScore.Criteria {
		fmt.Fprintf(&b, "| `%s` | %t | %s |\n", criterion.Name, criterion.Passed, escapeMarkdownTable(criterion.Details))
	}

	b.WriteString("\n## Results\n\n")
	b.WriteString("| Scenario | Passed | Tools | Commands | Assistant Calls | Non-Cached Tokens | Wall Seconds | Details |\n")
	b.WriteString("| --- | ---: | ---: | ---: | ---: | ---: | ---: | --- |\n")
	for _, result := range value.Results {
		tokenText := "n/a"
		if tokens, ok := nonCachedInputTokens(result); ok {
			tokenText = fmt.Sprintf("%d", tokens)
		}
		fmt.Fprintf(&b, "| `%s` | %t | %d | %d | %d | %s | %.2f | %s |\n",
			result.Scenario,
			result.Passed,
			result.Metrics.ToolCalls,
			result.Metrics.CommandExecutions,
			result.Metrics.AssistantCalls,
			tokenText,
			result.WallSeconds,
			escapeMarkdownTable(result.Verification.Details),
		)
	}

	b.WriteString("\n## Phase Timings\n\n")
	b.WriteString("| Phase | Seconds |\n")
	b.WriteString("| --- | ---: |\n")
	for _, row := range []struct {
		name  string
		value float64
	}{
		{"prepare_run_dir", value.PhaseTotals.PrepareRunDir},
		{"copy_repo", value.PhaseTotals.CopyRepo},
		{"install_variant", value.PhaseTotals.InstallVariant},
		{"warm_cache", value.PhaseTotals.WarmCache},
		{"seed_db", value.PhaseTotals.SeedDB},
		{"agent_run", value.PhaseTotals.AgentRun},
		{"parse_metrics", value.PhaseTotals.ParseMetrics},
		{"verify", value.PhaseTotals.Verify},
		{"total", value.PhaseTotals.Total},
	} {
		fmt.Fprintf(&b, "| %s | %.2f |\n", row.name, row.value)
	}

	b.WriteString("\n## Turn Details\n\n")
	for _, result := range value.Results {
		for _, turn := range result.Turns {
			fmt.Fprintf(&b, "- `production/%s` turn %d: exit `%d`, tools `%d`, assistant calls `%d`, wall `%.2f`, raw `%s`.\n",
				result.Scenario, turn.Index, turn.ExitCode, turn.Metrics.ToolCalls, turn.Metrics.AssistantCalls, turn.WallSeconds, turn.RawLogArtifactReference)
		}
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func isFileInspectionCommand(command string) bool {
	return commandHasExecutable(command, "rg", "grep", "sed", "cat", "find", "ls", "awk", "head", "tail", "nl")
}

func isBroadRepoSearchCommand(command string) bool {
	fields := commandFields(stripEnvAssignments(command))
	if len(fields) == 0 {
		return false
	}
	switch fields[0] {
	case "rg", "grep":
		return len(nonFlagArgs(fields[1:])) <= 1 || hasRepoRootTarget(nonFlagArgs(fields[1:]))
	case "find":
		return len(fields) < 2 || fields[1] == "." || fields[1] == "./"
	default:
		return false
	}
}

func inspectsGeneratedFileCommand(command string, output string) bool {
	lower := strings.ToLower(command + "\n" + output)
	return strings.Contains(lower, "sdk/generated/") ||
		strings.Contains(lower, "generated/api_") ||
		strings.Contains(lower, "generated/model_") ||
		strings.Contains(lower, "openapi/openapi.yaml")
}

func mentionsGeneratedPath(output string) bool {
	return inspectsGeneratedFileCommand("", output)
}

func inspectsModuleCache(command string) bool {
	lower := strings.ToLower(command)
	return strings.Contains(lower, "gomodcache") ||
		strings.Contains(lower, "$(go env gomodcache)") ||
		strings.Contains(lower, "`go env gomodcache`") ||
		strings.Contains(lower, "go env gomodcache")
}

func usesOpenPlannerCLI(command string) bool {
	fields := commandFields(command)
	for i := 0; i < len(fields); i++ {
		if fields[i] == "openplanner" || strings.HasSuffix(fields[i], "/openplanner") {
			return true
		}
		if fields[i] == "./cmd/openplanner" || strings.HasSuffix(fields[i], "/cmd/openplanner") {
			return true
		}
		if fields[i] == "go" && i+2 < len(fields) && fields[i+1] == "run" && strings.Contains(fields[i+2], "cmd/openplanner") && !strings.Contains(fields[i+2], "openplanner-agentops") {
			return true
		}
	}
	return false
}

func usesDirectSQLite(command string) bool {
	return commandHasExecutable(command, "sqlite3") || strings.Contains(strings.ToLower(command), "select ")
}

func commandHasExecutable(command string, executables ...string) bool {
	fields := commandFields(stripEnvAssignments(command))
	if len(fields) == 0 {
		return false
	}
	executable := filepath.Base(fields[0])
	for _, candidate := range executables {
		if executable == candidate {
			return true
		}
	}
	return false
}

func stripEnvAssignments(command string) string {
	fields := commandFields(command)
	for len(fields) > 0 && strings.Contains(fields[0], "=") && !strings.HasPrefix(fields[0], "-") {
		fields = fields[1:]
	}
	return strings.Join(fields, " ")
}

func commandFields(command string) []string {
	return strings.FieldsFunc(command, func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune("'\"`;&|()", r)
	})
}

func nonFlagArgs(fields []string) []string {
	args := []string{}
	for i := 0; i < len(fields); i++ {
		field := fields[i]
		if strings.HasPrefix(field, "-") {
			if field == "-e" || field == "--regexp" || field == "-g" || field == "--glob" {
				i++
			}
			continue
		}
		args = append(args, field)
	}
	return args
}

func hasRepoRootTarget(targets []string) bool {
	for _, target := range targets {
		if target == "." || target == "./" {
			return true
		}
	}
	return len(targets) == 0
}

func addMetricEvidence(target *[]string, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	for _, existing := range *target {
		if existing == value {
			return
		}
	}
	if len(*target) < 5 {
		*target = append(*target, value)
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func commandOutput(name string, args ...string) string {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return "unavailable"
	}
	return strings.TrimSpace(string(out))
}

func countNewSessionFiles(since time.Time, runRoot string) int {
	home, err := os.UserHomeDir()
	if err != nil {
		return -1
	}
	sessionsRoot := filepath.Join(home, ".codex", "sessions")
	count := 0
	_ = filepath.WalkDir(sessionsRoot, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			return nil
		}
		info, err := entry.Info()
		if err != nil || !info.ModTime().After(since) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err == nil && strings.Contains(string(data), runRoot) {
			count++
		}
		return nil
	})
	return count
}

func repoRoot() (string, error) {
	output, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	return filepath.Abs(strings.TrimSpace(string(output)))
}

func isWithin(path string, parent string) bool {
	rel, err := filepath.Rel(parent, path)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel))
}

func commandExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func intPtr(value int) *int {
	return &value
}

func roundSeconds(value float64) float64 {
	return math.Round(value*100) / 100
}

func escapeMarkdownTable(value string) string {
	return strings.ReplaceAll(value, "|", "\\|")
}

func passFail(value bool) string {
	if value {
		return "pass"
	}
	return "fail"
}

func sortedJoin(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	sorted := append([]string(nil), values...)
	sort.Strings(sorted)
	return strings.Join(sorted, ", ")
}

func promptSummary(sc scenario) string {
	switch sc.ID {
	case "ensure-calendar":
		return "ensure Personal calendar"
	case "create-timed-event":
		return "create Work Standup timed event"
	case "create-all-day-event":
		return "create Personal all-day event"
	case "create-dated-task":
		return "create Personal dated task"
	case "create-timed-task":
		return "create Work timed task"
	case "create-recurring-event":
		return "create daily recurring event"
	case "create-recurring-task":
		return "create daily recurring task"
	case "agenda-range":
		return "list bounded agenda range"
	case "list-events-filter-limit":
		return "list one Work event"
	case "list-tasks-filter-limit":
		return "list one Personal task"
	case "complete-task":
		return "complete seeded non-recurring task"
	case "complete-recurring-task":
		return "complete seeded recurring occurrence"
	case "mixed-event-task":
		return "create an event and task"
	case "mt-clarify-then-create":
		return "ask for missing year, then create task"
	case "mt-list-then-complete":
		return "list task, then complete it"
	default:
		return sc.Title
	}
}

func failf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
