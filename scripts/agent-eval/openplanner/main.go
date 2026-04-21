package main

import (
	"bufio"
	"bytes"
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

	"github.com/yazanabuashour/openplanner/internal/domain"
	"github.com/yazanabuashour/openplanner/internal/runner"
	internalservice "github.com/yazanabuashour/openplanner/internal/service"
	"github.com/yazanabuashour/openplanner/internal/store"
)

const (
	issueID               = "op-runner"
	scaleIssueID          = "op-2vv.3"
	modelName             = "gpt-5.4-mini"
	reasoningEffort       = "medium"
	defaultRunParallelism = 4
	cacheModeShared       = "shared"
	cacheModeIsolated     = "isolated"

	defaultScaleEvents      = 1000
	defaultScaleTasks       = 1000
	defaultScaleRecurring   = 200
	defaultScaleCompletions = 500
	defaultScaleLimit       = 50
	defaultScaleRangeDays   = 30
	maxScaleLimit           = 200

	scaleLargeAgendaThresholdSeconds         = 5.0
	scaleRecurringEventThresholdSeconds      = 3.0
	scaleRecurringCompletionThresholdSeconds = 5.0
	scaleListPaginationThresholdSeconds      = 3.0

	scenarioCategoryRoutine               = "routine"
	scenarioCategoryValidation            = "validation"
	scenarioCategoryUpdate                = "update"
	scenarioCategoryAdvancedRecurrence    = "advanced_recurrence"
	scenarioCategoryMigration             = "migration"
	scenarioCategoryMultiTurn             = "multi_turn_disambiguation"
	scenarioCategoryFutureSurface         = "future_surface"
	scenarioFeatureSupported              = "supported"
	scenarioFeatureUnsupportedUntilLanded = "unsupported_until_landed"
)

var prewarmCompilePackages = []string{"./cmd/openplanner", "./internal/runner"}

var requiredFullSuiteCategories = []string{
	scenarioCategoryRoutine,
	scenarioCategoryUpdate,
	scenarioCategoryAdvancedRecurrence,
	scenarioCategoryMigration,
	scenarioCategoryMultiTurn,
	scenarioCategoryFutureSurface,
}

type scenario struct {
	ID           string         `json:"id"`
	Title        string         `json:"title"`
	Category     string         `json:"category"`
	FeatureState string         `json:"feature_state"`
	Prompt       string         `json:"prompt,omitempty"`
	Turns        []scenarioTurn `json:"turns,omitempty"`
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
	ScenarioCoverage      []scenarioCoverage      `json:"scenario_coverage"`
	ProductionScore       productionScore         `json:"production_score"`
	Results               []runResult             `json:"results"`
	RawLogsCommitted      bool                    `json:"raw_logs_committed"`
	RawLogsNote           string                  `json:"raw_logs_note"`
	TokenUsageCaveat      string                  `json:"token_usage_caveat"`
	ComparisonStatus      string                  `json:"comparison_status"`
}

type scenarioCoverage struct {
	Category     string   `json:"category"`
	FeatureState string   `json:"feature_state"`
	Scenarios    []string `json:"scenarios"`
	Required     bool     `json:"required"`
	Passed       bool     `json:"passed"`
	Details      string   `json:"details"`
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
	ScenarioCategory        string             `json:"scenario_category"`
	FeatureState            string             `json:"feature_state"`
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
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Color       string `json:"color,omitempty"`
}

type eventState struct {
	Title             string   `json:"title"`
	Description       string   `json:"description,omitempty"`
	Location          string   `json:"location,omitempty"`
	LocationCleared   bool     `json:"location_cleared,omitempty"`
	StartAt           string   `json:"start_at,omitempty"`
	StartDate         string   `json:"start_date,omitempty"`
	Recurrence        string   `json:"recurrence,omitempty"`
	RecurrenceCleared bool     `json:"recurrence_cleared,omitempty"`
	Interval          int32    `json:"interval,omitempty"`
	Count             *int32   `json:"count,omitempty"`
	UntilAt           string   `json:"until_at,omitempty"`
	UntilDate         string   `json:"until_date,omitempty"`
	ByWeekday         []string `json:"by_weekday,omitempty"`
	ByMonthDay        []int32  `json:"by_month_day,omitempty"`
}

type taskState struct {
	Title             string   `json:"title"`
	Description       string   `json:"description,omitempty"`
	DueAt             string   `json:"due_at,omitempty"`
	DueDate           string   `json:"due_date,omitempty"`
	DueDateCleared    bool     `json:"due_date_cleared,omitempty"`
	Recurrence        string   `json:"recurrence,omitempty"`
	RecurrenceCleared bool     `json:"recurrence_cleared,omitempty"`
	Priority          string   `json:"priority,omitempty"`
	Status            string   `json:"status,omitempty"`
	Tags              []string `json:"tags,omitempty"`
	Interval          int32    `json:"interval,omitempty"`
	Count             *int32   `json:"count,omitempty"`
	UntilAt           string   `json:"until_at,omitempty"`
	UntilDate         string   `json:"until_date,omitempty"`
	ByWeekday         []string `json:"by_weekday,omitempty"`
	ByMonthDay        []int32  `json:"by_month_day,omitempty"`
	CompletedAt       string   `json:"completed_at,omitempty"`
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

type scaleOptions struct {
	RunRoot     string
	Date        string
	Events      int
	Tasks       int
	Recurring   int
	Completions int
	Limit       int
}

type cacheConfig struct {
	Mode    string
	RunRoot string
}

type scaleReport struct {
	Issue              string        `json:"issue"`
	Date               string        `json:"date"`
	Harness            string        `json:"harness"`
	ThresholdPolicy    string        `json:"threshold_policy"`
	RunRoot            string        `json:"run_root"`
	DatabasePath       string        `json:"database_path"`
	Dataset            scaleDataset  `json:"dataset"`
	HarnessWallSeconds float64       `json:"harness_wall_seconds"`
	Passed             bool          `json:"passed"`
	Results            []scaleResult `json:"results"`
	BlockerIssues      []string      `json:"blocker_issues,omitempty"`
	RawArtifactsNote   string        `json:"raw_artifacts_note"`
}

type scaleDataset struct {
	Calendars       int `json:"calendars"`
	Events          int `json:"events"`
	Tasks           int `json:"tasks"`
	RecurringEvents int `json:"recurring_events"`
	RecurringTasks  int `json:"recurring_tasks"`
	RecurrenceRules int `json:"recurrence_rules"`
	CompletionRows  int `json:"completion_rows"`
	AgendaRangeDays int `json:"agenda_range_days"`
	Limit           int `json:"limit"`
}

type scaleResult struct {
	Scenario         string   `json:"scenario"`
	Description      string   `json:"description"`
	Calendars        int      `json:"calendars"`
	Events           int      `json:"events"`
	Tasks            int      `json:"tasks"`
	RecurrenceRules  int      `json:"recurrence_rules"`
	CompletionRows   int      `json:"completion_rows"`
	ItemsReturned    int      `json:"items_returned"`
	PagesTraversed   int      `json:"pages_traversed"`
	WallSeconds      float64  `json:"wall_seconds"`
	ThresholdSeconds float64  `json:"threshold_seconds"`
	Passed           bool     `json:"passed"`
	Notes            []string `json:"notes,omitempty"`
}

type scaleSeed struct {
	WorkCalendarID     string
	PersonalCalendarID string
	RecurringEventIDs  []string
	RecurringTaskIDs   []string
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
		failf("usage: openplanner-agent-eval <run|scale>")
	}
	switch os.Args[1] {
	case "run":
		runCommand(os.Args[2:])
	case "scale":
		scaleCommand(os.Args[2:])
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

func parseScaleOptions(args []string) (scaleOptions, error) {
	options := scaleOptions{
		Date:        time.Now().Format(time.DateOnly),
		Events:      defaultScaleEvents,
		Tasks:       defaultScaleTasks,
		Recurring:   defaultScaleRecurring,
		Completions: defaultScaleCompletions,
		Limit:       defaultScaleLimit,
	}
	fs := flag.NewFlagSet("scale", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&options.RunRoot, "run-root", options.RunRoot, "directory for raw scale artifacts outside the repo")
	fs.StringVar(&options.Date, "date", options.Date, "report date in YYYY-MM-DD form or report suffix")
	fs.IntVar(&options.Events, "events", options.Events, "number of one-off events to seed")
	fs.IntVar(&options.Tasks, "tasks", options.Tasks, "number of one-off tasks to seed")
	fs.IntVar(&options.Recurring, "recurring", options.Recurring, "number of recurring events and recurring tasks to seed")
	fs.IntVar(&options.Completions, "completions", options.Completions, "number of recurring task completion rows to seed")
	fs.IntVar(&options.Limit, "limit", options.Limit, "runner list limit for pagination and agenda probes")
	if err := fs.Parse(args); err != nil {
		return scaleOptions{}, err
	}
	if fs.NArg() != 0 {
		return scaleOptions{}, errors.New("scale does not accept positional arguments")
	}
	if options.Events < 1 {
		return scaleOptions{}, errors.New("events must be greater than or equal to 1")
	}
	if options.Tasks < 1 {
		return scaleOptions{}, errors.New("tasks must be greater than or equal to 1")
	}
	if options.Recurring < 1 {
		return scaleOptions{}, errors.New("recurring must be greater than or equal to 1")
	}
	if options.Completions < 0 {
		return scaleOptions{}, errors.New("completions must be greater than or equal to 0")
	}
	if options.Limit < 1 {
		return scaleOptions{}, errors.New("limit must be greater than or equal to 1")
	}
	if options.Limit > maxScaleLimit {
		return scaleOptions{}, fmt.Errorf("limit must be less than or equal to %d", maxScaleLimit)
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
	filteredRun := strings.TrimSpace(options.ScenarioFilter) != ""
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
		ScenarioCoverage: scenarioCoverageFor(selectedScenarios, filteredRun),
		ProductionScore:  productionScoreFor(results, selectedScenarios, filteredRun),
		Results:          results,
		RawLogsCommitted: false,
		RawLogsNote:      "Raw codex exec event logs and stderr files were retained under <run-root> during execution and intentionally not committed.",
		TokenUsageCaveat: "Token metrics come from codex exec turn.completed usage events when exposed; unavailable usage is recorded as not exposed.",
		ComparisonStatus: "not applicable: OpenPlanner has no human CLI baseline variant; this report scores the production JSON runner surface only",
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

func scaleCommand(args []string) {
	options, err := parseScaleOptions(args)
	if err != nil {
		failf("parse scale flags: %v", err)
	}

	repoRoot, err := repoRoot()
	if err != nil {
		failf("resolve repo root: %v", err)
	}

	runRoot := options.RunRoot
	if runRoot == "" {
		runRoot, err = os.MkdirTemp("", "openplanner-scale-eval-*")
		if err != nil {
			failf("create scale run root: %v", err)
		}
	} else if err := os.MkdirAll(runRoot, 0o755); err != nil {
		failf("create scale run root %s: %v", runRoot, err)
	}
	runRoot, err = filepath.Abs(runRoot)
	if err != nil {
		failf("absolute scale run root: %v", err)
	}
	if isWithin(runRoot, repoRoot) {
		failf("scale run root must be outside the repository: %s", runRoot)
	}

	outReport, err := runScaleEval(runRoot, options)
	if err != nil {
		failf("run scale eval: %v", err)
	}
	if failures := failedScaleResults(outReport.Results); len(failures) > 0 {
		blockers, err := createScaleBlockerIssues(repoRoot, failures)
		if err != nil {
			failf("create scale blocker issues: %v", err)
		}
		outReport.BlockerIssues = blockers
	}

	outDir := filepath.Join(repoRoot, "docs", "agent-eval-results")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		failf("create report dir: %v", err)
	}
	reportBase := fmt.Sprintf("%s-%s-scale", scaleIssueID, options.Date)
	if err := writeJSON(filepath.Join(outDir, reportBase+".json"), outReport); err != nil {
		failf("write scale json report: %v", err)
	}
	if err := writeScaleMarkdown(filepath.Join(outDir, reportBase+".md"), outReport); err != nil {
		failf("write scale markdown report: %v", err)
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
		Variant:          "production",
		Scenario:         sc.ID,
		ScenarioTitle:    sc.Title,
		ScenarioCategory: scenarioCategory(sc),
		FeatureState:     scenarioFeatureState(sc),
		ExitCode:         -1,
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
	if err := timedPhase(&timings.InstallVariant, func() error { return installEvalRunnerAndSkill(runRepo, runDir) }); err != nil {
		return runResult{}, fmt.Errorf("install eval runner and skill: %w", err)
	}
	if err := preflightEvalContext(repoRoot, runRepo, runDir, cache); err != nil {
		return runResult{}, fmt.Errorf("preflight eval context: %w", err)
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
		ScenarioCategory:        scenarioCategory(currentScenario),
		FeatureState:            scenarioFeatureState(currentScenario),
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

func runScaleEval(runRoot string, options scaleOptions) (scaleReport, error) {
	scaleDir := filepath.Join(runRoot, "scale")
	if err := os.RemoveAll(scaleDir); err != nil {
		return scaleReport{}, fmt.Errorf("reset scale dir: %w", err)
	}
	if err := os.MkdirAll(scaleDir, 0o755); err != nil {
		return scaleReport{}, fmt.Errorf("create scale dir: %w", err)
	}
	dbPath := filepath.Join(scaleDir, "openplanner.db")
	dataset := scaleDatasetForOptions(options)

	start := time.Now()
	seed, err := seedScaleDatabase(dbPath, options)
	if err != nil {
		return scaleReport{}, fmt.Errorf("seed scale database: %w", err)
	}

	results := []scaleResult{
		runScaleLargeAgenda(dbPath, dataset, options),
		runScaleRecurringEvents(dbPath, dataset, options, seed),
		runScaleRecurringTaskCompletions(dbPath, dataset, options),
		runScaleListPagination(dbPath, dataset, options),
	}
	return scaleReport{
		Issue:              scaleIssueID,
		Date:               options.Date,
		Harness:            "go runner scale eval using runner.RunPlanningTask against an isolated SQLite database",
		ThresholdPolicy:    "local maintainer thresholds; failures create beads blockers but thresholds are not portable CI guarantees",
		RunRoot:            "<run-root>",
		DatabasePath:       "<run-root>/scale/openplanner.db",
		Dataset:            dataset,
		HarnessWallSeconds: roundSeconds(time.Since(start).Seconds()),
		Passed:             scaleResultsPassed(results),
		Results:            results,
		RawArtifactsNote:   "Raw scale database and transient artifacts were retained under <run-root>/scale during execution and intentionally not committed.",
	}, nil
}

func scaleDatasetForOptions(options scaleOptions) scaleDataset {
	return scaleDataset{
		Calendars:       2,
		Events:          options.Events + options.Recurring,
		Tasks:           options.Tasks + options.Recurring,
		RecurringEvents: options.Recurring,
		RecurringTasks:  options.Recurring,
		RecurrenceRules: options.Recurring * 2,
		CompletionRows:  options.Completions,
		AgendaRangeDays: defaultScaleRangeDays,
		Limit:           options.Limit,
	}
}

func seedScaleDatabase(dbPath string, options scaleOptions) (scaleSeed, error) {
	work, err := runRequiredScaleRequest(dbPath, runner.PlanningTaskRequest{
		Action:       runner.PlanningTaskActionEnsureCalendar,
		CalendarName: "Work",
	})
	if err != nil {
		return scaleSeed{}, err
	}
	personal, err := runRequiredScaleRequest(dbPath, runner.PlanningTaskRequest{
		Action:       runner.PlanningTaskActionEnsureCalendar,
		CalendarName: "Personal",
	})
	if err != nil {
		return scaleSeed{}, err
	}
	if len(work.Calendars) != 1 || len(personal.Calendars) != 1 {
		return scaleSeed{}, errors.New("scale seed calendar result missing calendar details")
	}
	seed := scaleSeed{
		WorkCalendarID:     work.Calendars[0].ID,
		PersonalCalendarID: personal.Calendars[0].ID,
	}

	base := time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC)
	for i := 0; i < options.Events; i++ {
		start := base.AddDate(0, 0, i%60).Add(time.Duration(i%8) * time.Hour)
		calendarID := seed.WorkCalendarID
		if i%2 == 1 {
			calendarID = seed.PersonalCalendarID
		}
		if _, err := runRequiredScaleRequest(dbPath, runner.PlanningTaskRequest{
			Action:     runner.PlanningTaskActionCreateEvent,
			CalendarID: calendarID,
			Title:      fmt.Sprintf("Scale event %04d", i+1),
			StartAt:    start.Format(time.RFC3339),
			EndAt:      start.Add(30 * time.Minute).Format(time.RFC3339),
		}); err != nil {
			return scaleSeed{}, err
		}
	}

	for i := 0; i < options.Tasks; i++ {
		calendarID := seed.PersonalCalendarID
		if i%2 == 1 {
			calendarID = seed.WorkCalendarID
		}
		if _, err := runRequiredScaleRequest(dbPath, runner.PlanningTaskRequest{
			Action:     runner.PlanningTaskActionCreateTask,
			CalendarID: calendarID,
			Title:      fmt.Sprintf("Scale task %04d", i+1),
			DueDate:    base.AddDate(0, 0, i%60).Format(time.DateOnly),
		}); err != nil {
			return scaleSeed{}, err
		}
	}

	for i := 0; i < options.Recurring; i++ {
		eventResult, err := runRequiredScaleRequest(dbPath, scaleRecurringEventRequest(seed, base, i))
		if err != nil {
			return scaleSeed{}, err
		}
		if len(eventResult.Events) != 1 {
			return scaleSeed{}, fmt.Errorf("recurring event seed %d missing event details", i)
		}
		seed.RecurringEventIDs = append(seed.RecurringEventIDs, eventResult.Events[0].ID)
		taskResult, err := runRequiredScaleRequest(dbPath, scaleRecurringTaskRequest(seed, i))
		if err != nil {
			return scaleSeed{}, err
		}
		if len(taskResult.Tasks) != 1 {
			return scaleSeed{}, fmt.Errorf("recurring task seed %d missing task details", i)
		}
		seed.RecurringTaskIDs = append(seed.RecurringTaskIDs, taskResult.Tasks[0].ID)
	}

	for i := 0; i < options.Completions && len(seed.RecurringTaskIDs) > 0; i++ {
		taskID := seed.RecurringTaskIDs[i%len(seed.RecurringTaskIDs)]
		occurrenceOffset := i / len(seed.RecurringTaskIDs)
		if _, err := runRequiredScaleRequest(dbPath, runner.PlanningTaskRequest{
			Action:         runner.PlanningTaskActionCompleteTask,
			TaskID:         taskID,
			OccurrenceDate: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, occurrenceOffset).Format(time.DateOnly),
		}); err != nil {
			return scaleSeed{}, err
		}
	}

	return seed, nil
}

func scaleRecurringEventRequest(seed scaleSeed, base time.Time, index int) runner.PlanningTaskRequest {
	calendarID := seed.WorkCalendarID
	if index%2 == 1 {
		calendarID = seed.PersonalCalendarID
	}
	count := int32(defaultScaleRangeDays + 15)
	switch index % 3 {
	case 0:
		start := base.Add(time.Duration(index%6) * time.Hour)
		return runner.PlanningTaskRequest{
			Action:     runner.PlanningTaskActionCreateEvent,
			CalendarID: calendarID,
			Title:      fmt.Sprintf("Scale recurring daily event %04d", index+1),
			StartAt:    start.Format(time.RFC3339),
			EndAt:      start.Add(30 * time.Minute).Format(time.RFC3339),
			Recurrence: &runner.RecurrenceRuleRequest{Frequency: "daily", Count: &count},
		}
	case 1:
		return runner.PlanningTaskRequest{
			Action:     runner.PlanningTaskActionCreateEvent,
			CalendarID: calendarID,
			Title:      fmt.Sprintf("Scale recurring weekly event %04d", index+1),
			StartDate:  "2026-04-01",
			Recurrence: &runner.RecurrenceRuleRequest{Frequency: "weekly", Count: &count, ByWeekday: []string{"MO", "WE", "FR"}},
		}
	default:
		monthCount := int32(12)
		start := base.Add(time.Duration(index%6) * time.Hour)
		return runner.PlanningTaskRequest{
			Action:     runner.PlanningTaskActionCreateEvent,
			CalendarID: calendarID,
			Title:      fmt.Sprintf("Scale recurring monthly event %04d", index+1),
			StartAt:    start.Format(time.RFC3339),
			EndAt:      start.Add(45 * time.Minute).Format(time.RFC3339),
			Recurrence: &runner.RecurrenceRuleRequest{Frequency: "monthly", Count: &monthCount, ByMonthDay: []int32{1, 15}},
		}
	}
}

func scaleRecurringTaskRequest(seed scaleSeed, index int) runner.PlanningTaskRequest {
	calendarID := seed.PersonalCalendarID
	if index%2 == 1 {
		calendarID = seed.WorkCalendarID
	}
	count := int32(defaultScaleRangeDays + 15)
	return runner.PlanningTaskRequest{
		Action:     runner.PlanningTaskActionCreateTask,
		CalendarID: calendarID,
		Title:      fmt.Sprintf("Scale recurring task %04d", index+1),
		DueDate:    "2026-04-01",
		Recurrence: &runner.RecurrenceRuleRequest{Frequency: "daily", Count: &count},
	}
}

func runRequiredScaleRequest(dbPath string, request runner.PlanningTaskRequest) (runner.PlanningTaskResult, error) {
	result, err := runPlanning(dbPath, request)
	if err != nil {
		return runner.PlanningTaskResult{}, err
	}
	if result.Rejected {
		return runner.PlanningTaskResult{}, errors.New(result.RejectionReason)
	}
	return result, nil
}

func runScaleLargeAgenda(dbPath string, dataset scaleDataset, options scaleOptions) scaleResult {
	result, wallSeconds, err := timedScalePlanning(dbPath, runner.PlanningTaskRequest{
		Action: runner.PlanningTaskActionListAgenda,
		From:   "2026-04-01T00:00:00Z",
		To:     "2026-05-01T00:00:00Z",
		Limit:  &options.Limit,
	})
	out := baseScaleResult("large-agenda-window", "bounded agenda generation over a mixed large local dataset", dataset, scaleLargeAgendaThresholdSeconds, wallSeconds)
	if err != nil {
		out.Notes = append(out.Notes, err.Error())
		return out
	}
	out.ItemsReturned = len(result.Agenda)
	out.PagesTraversed = 1
	out.Passed = !result.Rejected && out.ItemsReturned > 0 && wallSeconds <= out.ThresholdSeconds
	if result.Rejected {
		out.Notes = append(out.Notes, result.RejectionReason)
	}
	return out
}

func runScaleRecurringEvents(dbPath string, dataset scaleDataset, options scaleOptions, seed scaleSeed) scaleResult {
	itemsReturned, pagesTraversed, timedRecurringEventItems, allDayRecurringEventItems, wallSeconds, err := traverseScaleAgendaForRecurringEvents(dbPath, options.Limit, seed.RecurringEventIDs)
	out := baseScaleResult("recurring-event-expansion", "agenda probe that expands recurring timed and all-day events", dataset, scaleRecurringEventThresholdSeconds, wallSeconds)
	if err != nil {
		out.Notes = append(out.Notes, err.Error())
		return out
	}
	out.ItemsReturned = itemsReturned
	out.PagesTraversed = pagesTraversed
	out.Passed = out.ItemsReturned > 0 && timedRecurringEventItems > 0 && allDayRecurringEventItems > 0 && wallSeconds <= out.ThresholdSeconds
	out.Notes = append(out.Notes, fmt.Sprintf("timed recurring event items observed: %d; all-day recurring event items observed: %d", timedRecurringEventItems, allDayRecurringEventItems))
	return out
}

func traverseScaleAgendaForRecurringEvents(dbPath string, limit int, recurringEventIDs []string) (int, int, int, int, float64, error) {
	start := time.Now()
	recurringEvents := map[string]struct{}{}
	for _, id := range recurringEventIDs {
		recurringEvents[id] = struct{}{}
	}
	if len(recurringEvents) == 0 {
		return 0, 0, 0, 0, 0, errors.New("no recurring event ids were seeded")
	}

	cursor := ""
	items := 0
	pages := 0
	timedRecurringEventItems := 0
	allDayRecurringEventItems := 0
	for {
		request := runner.PlanningTaskRequest{
			Action: runner.PlanningTaskActionListAgenda,
			From:   "2026-04-01T00:00:00Z",
			To:     "2026-05-01T00:00:00Z",
			Cursor: cursor,
			Limit:  &limit,
		}
		result, err := runPlanning(dbPath, request)
		if err != nil {
			return items, pages, timedRecurringEventItems, allDayRecurringEventItems, roundSeconds(time.Since(start).Seconds()), err
		}
		if result.Rejected {
			return items, pages, timedRecurringEventItems, allDayRecurringEventItems, roundSeconds(time.Since(start).Seconds()), errors.New(result.RejectionReason)
		}
		pages++
		items += len(result.Agenda)
		for _, item := range result.Agenda {
			if item.Kind != "event" {
				continue
			}
			if _, ok := recurringEvents[item.SourceID]; ok {
				if item.StartAt != "" {
					timedRecurringEventItems++
				}
				if item.StartDate != "" {
					allDayRecurringEventItems++
				}
			}
		}
		if (timedRecurringEventItems > 0 && allDayRecurringEventItems > 0) || result.NextCursor == "" {
			return items, pages, timedRecurringEventItems, allDayRecurringEventItems, roundSeconds(time.Since(start).Seconds()), nil
		}
		cursor = result.NextCursor
		if pages > 100000 {
			return items, pages, timedRecurringEventItems, allDayRecurringEventItems, roundSeconds(time.Since(start).Seconds()), errors.New("agenda pagination did not terminate")
		}
	}
}

func runScaleRecurringTaskCompletions(dbPath string, dataset scaleDataset, options scaleOptions) scaleResult {
	itemsReturned, pagesTraversed, completedItems, wallSeconds, err := traverseScaleAgendaForCompletions(dbPath, 200)
	out := baseScaleResult("recurring-task-completion-lookup", "agenda probe that loads recurring task completion rows", dataset, scaleRecurringCompletionThresholdSeconds, wallSeconds)
	if err != nil {
		out.Notes = append(out.Notes, err.Error())
		return out
	}
	out.ItemsReturned = itemsReturned
	out.PagesTraversed = pagesTraversed
	out.Passed = out.ItemsReturned > 0 && wallSeconds <= out.ThresholdSeconds
	if dataset.CompletionRows > 0 {
		out.Passed = out.Passed && completedItems > 0
	}
	out.Notes = append(out.Notes, fmt.Sprintf("completed agenda items observed: %d", completedItems))
	return out
}

func runScaleListPagination(dbPath string, dataset scaleDataset, options scaleOptions) scaleResult {
	start := time.Now()
	eventItems, eventPages, eventErr := traverseScaleList(dbPath, runner.PlanningTaskActionListEvents, options.Limit)
	taskItems, taskPages, taskErr := traverseScaleList(dbPath, runner.PlanningTaskActionListTasks, options.Limit)
	wallSeconds := roundSeconds(time.Since(start).Seconds())
	out := baseScaleResult("list-pagination", "full cursor traversal for event and task list actions", dataset, scaleListPaginationThresholdSeconds, wallSeconds)
	out.ItemsReturned = eventItems + taskItems
	out.PagesTraversed = eventPages + taskPages
	if eventErr != nil {
		out.Notes = append(out.Notes, "events: "+eventErr.Error())
	}
	if taskErr != nil {
		out.Notes = append(out.Notes, "tasks: "+taskErr.Error())
	}
	wantItems := dataset.Events + dataset.Tasks
	out.Passed = eventErr == nil && taskErr == nil && out.ItemsReturned == wantItems && wallSeconds <= out.ThresholdSeconds
	out.Notes = append(out.Notes, fmt.Sprintf("events=%d/%d tasks=%d/%d", eventItems, dataset.Events, taskItems, dataset.Tasks))
	return out
}

func timedScalePlanning(dbPath string, request runner.PlanningTaskRequest) (runner.PlanningTaskResult, float64, error) {
	start := time.Now()
	result, err := runPlanning(dbPath, request)
	return result, roundSeconds(time.Since(start).Seconds()), err
}

func traverseScaleList(dbPath string, action string, limit int) (int, int, error) {
	cursor := ""
	items := 0
	pages := 0
	for {
		request := runner.PlanningTaskRequest{Action: action, Cursor: cursor, Limit: &limit}
		result, err := runPlanning(dbPath, request)
		if err != nil {
			return items, pages, err
		}
		if result.Rejected {
			return items, pages, errors.New(result.RejectionReason)
		}
		pages++
		switch action {
		case runner.PlanningTaskActionListEvents:
			items += len(result.Events)
		case runner.PlanningTaskActionListTasks:
			items += len(result.Tasks)
		default:
			return items, pages, fmt.Errorf("unsupported scale list action %q", action)
		}
		if result.NextCursor == "" {
			return items, pages, nil
		}
		cursor = result.NextCursor
		if pages > 100000 {
			return items, pages, errors.New("pagination did not terminate")
		}
	}
}

func traverseScaleAgendaForCompletions(dbPath string, limit int) (int, int, int, float64, error) {
	start := time.Now()
	cursor := ""
	items := 0
	pages := 0
	completedItems := 0
	for {
		request := runner.PlanningTaskRequest{
			Action: runner.PlanningTaskActionListAgenda,
			From:   "2026-04-01T00:00:00Z",
			To:     "2026-04-04T00:00:00Z",
			Cursor: cursor,
			Limit:  &limit,
		}
		result, err := runPlanning(dbPath, request)
		if err != nil {
			return items, pages, completedItems, roundSeconds(time.Since(start).Seconds()), err
		}
		if result.Rejected {
			return items, pages, completedItems, roundSeconds(time.Since(start).Seconds()), errors.New(result.RejectionReason)
		}
		pages++
		items += len(result.Agenda)
		for _, item := range result.Agenda {
			if item.CompletedAt != "" {
				completedItems++
			}
		}
		if completedItems > 0 || result.NextCursor == "" {
			return items, pages, completedItems, roundSeconds(time.Since(start).Seconds()), nil
		}
		cursor = result.NextCursor
		if pages > 100000 {
			return items, pages, completedItems, roundSeconds(time.Since(start).Seconds()), errors.New("agenda pagination did not terminate")
		}
	}
}

func baseScaleResult(scenario string, description string, dataset scaleDataset, thresholdSeconds float64, wallSeconds float64) scaleResult {
	return scaleResult{
		Scenario:         scenario,
		Description:      description,
		Calendars:        dataset.Calendars,
		Events:           dataset.Events,
		Tasks:            dataset.Tasks,
		RecurrenceRules:  dataset.RecurrenceRules,
		CompletionRows:   dataset.CompletionRows,
		WallSeconds:      wallSeconds,
		ThresholdSeconds: thresholdSeconds,
	}
}

func scaleResultsPassed(results []scaleResult) bool {
	for _, result := range results {
		if !result.Passed {
			return false
		}
	}
	return true
}

func failedScaleResults(results []scaleResult) []scaleResult {
	failures := []scaleResult{}
	for _, result := range results {
		if !result.Passed {
			failures = append(failures, result)
		}
	}
	return failures
}

func createScaleBlockerIssues(repoRoot string, failures []scaleResult) ([]string, error) {
	ids := []string{}
	for _, failure := range failures {
		title := fmt.Sprintf("Investigate OpenPlanner scale eval failure: %s", failure.Scenario)
		description := fmt.Sprintf(
			"Scale eval `%s` exceeded or failed the local maintainer gate.\n\nDataset: %d events, %d tasks, %d recurrence rules, %d completion rows.\nWall time: %.2fs. Threshold: %.2fs.\nNotes: %s.\n\nCreated from `%s`.",
			failure.Scenario,
			failure.Events,
			failure.Tasks,
			failure.RecurrenceRules,
			failure.CompletionRows,
			failure.WallSeconds,
			failure.ThresholdSeconds,
			strings.Join(failure.Notes, "; "),
			scaleIssueID,
		)
		cmd := exec.Command("bd", "create", "--silent", "--title", title, "--description", description, "--type", "task", "--priority", "2")
		cmd.Dir = repoRoot
		output, err := cmd.CombinedOutput()
		if err != nil {
			return ids, fmt.Errorf("bd create: %w: %s", err, strings.TrimSpace(string(output)))
		}
		id := firstBeadsID(string(output))
		if id == "" {
			return ids, errors.New("bd create returned empty issue id")
		}
		dep := exec.Command("bd", "dep", "add", "op-2vv", id)
		dep.Dir = repoRoot
		if output, err := dep.CombinedOutput(); err != nil {
			return ids, fmt.Errorf("bd dep add op-2vv %s: %w: %s", id, err, strings.TrimSpace(string(output)))
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func firstBeadsID(output string) string {
	for _, field := range strings.Fields(output) {
		if strings.HasPrefix(field, "op-") {
			return strings.Trim(field, ".,:;")
		}
	}
	return ""
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
		"PATH="+filepath.Join(runDir, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"),
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
		{ID: "ensure-calendar", Title: "Ensure a calendar idempotently", Category: scenarioCategoryRoutine, FeatureState: scenarioFeatureSupported, Prompt: "Use the configured local OpenPlanner data path. Ensure a calendar named Personal exists. Then tell me whether the Personal calendar exists."},
		{ID: "create-timed-event", Title: "Create a timed event", Category: scenarioCategoryRoutine, FeatureState: scenarioFeatureSupported, Prompt: "Use the configured local OpenPlanner data path. Add a Work calendar event titled Standup from 2026-04-16T09:00:00Z to 2026-04-16T10:00:00Z. Then tell me what event is stored."},
		{ID: "create-all-day-event", Title: "Create an all-day event", Category: scenarioCategoryRoutine, FeatureState: scenarioFeatureSupported, Prompt: "Use the configured local OpenPlanner data path. Add a Personal all-day event titled Planning day on 2026-04-17. Then tell me what event is stored."},
		{ID: "create-dated-task", Title: "Create a dated task", Category: scenarioCategoryRoutine, FeatureState: scenarioFeatureSupported, Prompt: "Use the configured local OpenPlanner data path. Add a Personal task titled Review notes due on 2026-04-16. Then tell me what task is stored."},
		{ID: "create-timed-task", Title: "Create a timed task", Category: scenarioCategoryRoutine, FeatureState: scenarioFeatureSupported, Prompt: "Use the configured local OpenPlanner data path. Add a Work task titled Send summary due at 2026-04-16T11:00:00Z. Then tell me what task is stored."},
		{ID: "create-recurring-event", Title: "Create a recurring event", Category: scenarioCategoryRoutine, FeatureState: scenarioFeatureSupported, Prompt: "Use the configured local OpenPlanner data path. Add a Work event titled Daily standup from 2026-04-16T09:00:00Z to 2026-04-16T09:30:00Z recurring daily for 3 occurrences. Then tell me the recurrence stored."},
		{ID: "create-recurring-task", Title: "Create a recurring task", Category: scenarioCategoryRoutine, FeatureState: scenarioFeatureSupported, Prompt: "Use the configured local OpenPlanner data path. Add a Personal task titled Daily review due on 2026-04-16 recurring daily for 3 occurrences. Then tell me the recurrence stored."},
		{ID: "agenda-range", Title: "List a bounded agenda range chronologically", Category: scenarioCategoryRoutine, FeatureState: scenarioFeatureSupported, Prompt: "Use the configured local OpenPlanner data path. Show my agenda from 2026-04-16T00:00:00Z to 2026-04-17T00:00:00Z. Mention only items in that range, chronologically."},
		{ID: "list-events-filter-limit", Title: "List events with calendar filter and limit", Category: scenarioCategoryRoutine, FeatureState: scenarioFeatureSupported, Prompt: "Use the configured local OpenPlanner data path. List only the first Work calendar event. Do not mention Personal calendar events."},
		{ID: "list-tasks-filter-limit", Title: "List tasks with calendar filter and limit", Category: scenarioCategoryRoutine, FeatureState: scenarioFeatureSupported, Prompt: "Use the configured local OpenPlanner data path. List only the first Personal calendar task. Do not mention Work calendar tasks."},
		{ID: "list-tasks-metadata-filter", Title: "List tasks with priority, status, and tag filters", Category: scenarioCategoryRoutine, FeatureState: scenarioFeatureSupported, Prompt: "Use the configured local OpenPlanner data path. List Work tasks with high priority, status in_progress, and tags planning and review. Mention only matching tasks."},
		{ID: "complete-task", Title: "Complete a non-recurring task", Category: scenarioCategoryRoutine, FeatureState: scenarioFeatureSupported, Prompt: "Use the configured local OpenPlanner data path. Complete the Personal task titled Review notes due on 2026-04-16. Tell me what was completed."},
		{ID: "complete-recurring-task", Title: "Complete a recurring task occurrence", Category: scenarioCategoryRoutine, FeatureState: scenarioFeatureSupported, Prompt: "Use the configured local OpenPlanner data path. Complete the 2026-04-17 occurrence of the Personal recurring task titled Daily review. Tell me what occurrence was completed."},
		{ID: "delete-task", Title: "Delete a task by listed ID", Category: scenarioCategoryRoutine, FeatureState: scenarioFeatureSupported, Prompt: "Use the configured local OpenPlanner data path. Delete the Personal task titled Old note. Leave the Personal task titled Keep note in place. Tell me what was deleted."},
		{ID: "delete-event", Title: "Delete an event by listed ID", Category: scenarioCategoryRoutine, FeatureState: scenarioFeatureSupported, Prompt: "Use the configured local OpenPlanner data path. Delete the Personal event titled Old appointment. Leave the Personal event titled Keep appointment in place. Tell me what was deleted."},
		{ID: "delete-empty-calendar", Title: "Delete an empty calendar safely", Category: scenarioCategoryRoutine, FeatureState: scenarioFeatureSupported, Prompt: "Use the configured local OpenPlanner data path. Delete the empty Archive calendar. Tell me what calendar was deleted."},
		{ID: "mixed-event-task", Title: "Create an event and a task in one user turn", Category: scenarioCategoryRoutine, FeatureState: scenarioFeatureSupported, Prompt: "Use the configured local OpenPlanner data path. Add a Work event titled Standup from 2026-04-16T09:00:00Z to 2026-04-16T10:00:00Z and a Personal task titled Review notes due on 2026-04-16. Then tell me both stored items."},
		{ID: "ambiguous-short-date", Title: "Clarify an ambiguous short date without writing", Category: scenarioCategoryValidation, FeatureState: scenarioFeatureSupported, Prompt: "Please add a local OpenPlanner task titled Review notes due 04/16. There is no year context in this conversation or my request."},
		{ID: "year-first-slash-date", Title: "Reject a year-first slash date without writing", Category: scenarioCategoryValidation, FeatureState: scenarioFeatureSupported, Prompt: "Please add this local OpenPlanner task exactly as written: Review notes due 2026/04/16. If OpenPlanner requires another date format, reject this request directly without running tools. Do not normalize or rewrite the date."},
		{ID: "invalid-rfc3339", Title: "Reject an invalid RFC3339 time without writing", Category: scenarioCategoryValidation, FeatureState: scenarioFeatureSupported, Prompt: "Please add a local OpenPlanner event titled Standup exactly as written from 2026-04-16 09:00 to 2026-04-16 10:00. If OpenPlanner requires RFC3339 timed fields, reject this request directly without running tools. Do not normalize or rewrite the times."},
		{ID: "missing-title", Title: "Reject a missing required title without writing", Category: scenarioCategoryValidation, FeatureState: scenarioFeatureSupported, Prompt: "Please add a local OpenPlanner task due on 2026-04-16, but I do not have a title for it."},
		{ID: "invalid-range", Title: "Reject an invalid agenda range without writing", Category: scenarioCategoryValidation, FeatureState: scenarioFeatureSupported, Prompt: "Please show my OpenPlanner agenda from 2026-04-18T00:00:00Z to 2026-04-16T00:00:00Z."},
		{ID: "unsupported-recurrence", Title: "Reject unsupported recurrence without writing", Category: scenarioCategoryValidation, FeatureState: scenarioFeatureSupported, Prompt: "Please add a local OpenPlanner task titled Review notes due on 2026-04-16 recurring hourly."},
		{ID: "non-positive-limit", Title: "Reject a non-positive list limit without writing", Category: scenarioCategoryValidation, FeatureState: scenarioFeatureSupported, Prompt: "Please list 0 OpenPlanner tasks."},
		{ID: "invalid-task-priority", Title: "Reject an invalid task priority without writing", Category: scenarioCategoryValidation, FeatureState: scenarioFeatureSupported, Prompt: "Please add a local OpenPlanner task titled Review notes due on 2026-04-16 with urgent priority. If urgent is not a valid priority, reject this request directly without running tools."},
		{ID: "invalid-task-status", Title: "Reject an invalid task status without writing", Category: scenarioCategoryValidation, FeatureState: scenarioFeatureSupported, Prompt: "Please add a local OpenPlanner task titled Review notes due on 2026-04-16 with blocked status. If blocked is not a valid status, reject this request directly without running tools."},
		{ID: "invalid-task-tag", Title: "Reject an invalid task tag without writing", Category: scenarioCategoryValidation, FeatureState: scenarioFeatureSupported, Prompt: "Please add a local OpenPlanner task titled Review notes due on 2026-04-16 with tag \"needs review\". If spaces are not valid inside tags, reject this request directly without running tools."},
		{ID: "update-calendar-metadata", Title: "Update calendar metadata", Category: scenarioCategoryUpdate, FeatureState: scenarioFeatureSupported, Prompt: "Use the configured local OpenPlanner data path. Ensure a Work calendar exists, set its description to Delivery planning and its color to #2563EB, then tell me what calendar metadata is stored."},
		{ID: "update-event-patch-clear", Title: "Clear optional event fields with patch semantics", Category: scenarioCategoryUpdate, FeatureState: scenarioFeatureSupported, Prompt: "Use the configured local OpenPlanner data path. Find the Work event titled Planning sync, clear its location and recurrence, preserve its title and time, then tell me what changed."},
		{ID: "update-task-due-mode", Title: "Convert a dated task to a timed task", Category: scenarioCategoryUpdate, FeatureState: scenarioFeatureSupported, Prompt: "Use the configured local OpenPlanner data path. Find the Personal task titled Review notes and change it from due on 2026-04-16 to due at 2026-04-16T11:00:00Z, clearing the date-only due date. Then tell me what task is stored."},
		{ID: "weekly-recurrence-by-weekday", Title: "Create weekly recurrence by weekday", Category: scenarioCategoryAdvancedRecurrence, FeatureState: scenarioFeatureSupported, Prompt: "Use the configured local OpenPlanner data path. Add a Personal task titled Water plants due on 2026-04-13 recurring weekly on Monday and Wednesday for 4 occurrences. Then tell me the recurrence stored."},
		{ID: "monthly-recurrence-by-month-day", Title: "Create monthly recurrence by month day", Category: scenarioCategoryAdvancedRecurrence, FeatureState: scenarioFeatureSupported, Prompt: "Use the configured local OpenPlanner data path. Add a Personal task titled Pay rent due on 2026-01-31 recurring monthly on the 31st for 3 occurrences. Then tell me the recurrence stored."},
		{ID: "task-metadata-create", Title: "Create task priority, status, and tags", Category: scenarioCategoryUpdate, FeatureState: scenarioFeatureSupported, Prompt: "Use the configured local OpenPlanner data path. Add a Personal OpenPlanner task titled Review notes due on 2026-04-16 with high priority, status in_progress, and tags planning and review. Then tell me the stored priority, status, and tags."},
		{ID: "migration-style-copy", Title: "Copy selected source calendar data into a destination calendar", Category: scenarioCategoryMigration, FeatureState: scenarioFeatureSupported, Prompt: "Use the configured local OpenPlanner data path. Copy the Legacy calendar items titled Team sync and Review notes into the Work calendar, leaving the Legacy items in place. Then tell me what was copied."},
		{ID: "unsupported-import-export", Title: "Reject import/export before runner support lands", Category: scenarioCategoryFutureSurface, FeatureState: scenarioFeatureUnsupportedUntilLanded, Prompt: "Please export my local OpenPlanner calendar to an iCalendar .ics file and import an iCalendar file into OpenPlanner. If the production OpenPlanner skill does not support import or export yet, say that directly without switching interfaces."},
		{ID: "reminder-create-query-dismiss", Title: "Create, query, and dismiss a reminder", Category: scenarioCategoryRoutine, FeatureState: scenarioFeatureSupported, Prompt: "Use the configured local OpenPlanner data path. Add a Personal OpenPlanner task titled Take medicine due at 2026-04-16T10:00:00Z with a reminder one hour before. Then list pending reminders from 2026-04-16T08:00:00Z to 2026-04-16T10:00:00Z, dismiss the reminder you created, and tell me that no pending reminder remains in that range."},
		{ID: "mt-clarify-then-create", Title: "Clarify missing year, then create in a resumed turn", Category: scenarioCategoryMultiTurn, FeatureState: scenarioFeatureSupported, Turns: []scenarioTurn{
			{Prompt: "Please add a local OpenPlanner Personal task titled Review notes due 04/16. There is no year context in this conversation or my request."},
			{Prompt: "Use 2026 as the year for that Personal task."},
		}},
		{ID: "mt-list-then-complete", Title: "List a task, then complete it in a resumed turn", Category: scenarioCategoryMultiTurn, FeatureState: scenarioFeatureSupported, Turns: []scenarioTurn{
			{Prompt: "Use the configured local OpenPlanner data path. What Personal tasks are due on 2026-04-16? Mention only the matching task."},
			{Prompt: "Complete the task you just found and tell me what was completed."},
		}},
		{ID: "mt-disambiguate-calendar", Title: "Clarify destination calendar before copying", Category: scenarioCategoryMultiTurn, FeatureState: scenarioFeatureSupported, Turns: []scenarioTurn{
			{Prompt: "Use the configured local OpenPlanner data path. Copy the task Review notes from the Legacy calendar, but I have not said which destination calendar to use."},
			{Prompt: "Use Work as the destination calendar."},
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

func scenarioCategory(sc scenario) string {
	if strings.TrimSpace(sc.Category) != "" {
		return sc.Category
	}
	if isMultiTurnScenario(sc) {
		return scenarioCategoryMultiTurn
	}
	return scenarioCategoryRoutine
}

func scenarioFeatureState(sc scenario) string {
	if strings.TrimSpace(sc.FeatureState) != "" {
		return sc.FeatureState
	}
	return scenarioFeatureSupported
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
	case "list-tasks-metadata-filter":
		return seedTaskMetadataFilter(dbPath)
	case "complete-recurring-task":
		return seedRecurringTask(dbPath)
	case "delete-task":
		return seedDeleteTaskData(dbPath)
	case "delete-event":
		return seedDeleteEventData(dbPath)
	case "delete-empty-calendar":
		return seedEmptyArchiveCalendar(dbPath)
	case "update-event-patch-clear":
		return seedPatchableEvent(dbPath)
	case "update-task-due-mode":
		return seedDatedReviewTask(dbPath)
	case "migration-style-copy", "mt-disambiguate-calendar":
		return seedLegacyMigrationData(dbPath)
	default:
		return nil
	}
}

func seedAgendaRange(dbPath string) error {
	requests := []runner.PlanningTaskRequest{
		{Action: runner.PlanningTaskActionCreateTask, CalendarName: "Work", Title: "Review notes", DueDate: "2026-04-16"},
		{Action: runner.PlanningTaskActionCreateEvent, CalendarName: "Work", Title: "Standup", StartAt: "2026-04-16T09:00:00Z", EndAt: "2026-04-16T10:00:00Z"},
		{Action: runner.PlanningTaskActionCreateEvent, CalendarName: "Work", Title: "Out of range", StartAt: "2026-04-17T09:00:00Z", EndAt: "2026-04-17T10:00:00Z"},
	}
	return runSeedRequests(dbPath, requests)
}

func seedEventFilter(dbPath string) error {
	requests := []runner.PlanningTaskRequest{
		{Action: runner.PlanningTaskActionCreateEvent, CalendarName: "Work", Title: "Work sync", StartAt: "2026-04-16T09:00:00Z", EndAt: "2026-04-16T09:30:00Z"},
		{Action: runner.PlanningTaskActionCreateEvent, CalendarName: "Personal", Title: "Personal appointment", StartAt: "2026-04-16T10:00:00Z", EndAt: "2026-04-16T10:30:00Z"},
	}
	return runSeedRequests(dbPath, requests)
}

func seedReviewTask(dbPath string) error {
	return runSeedRequests(dbPath, []runner.PlanningTaskRequest{
		{Action: runner.PlanningTaskActionCreateTask, CalendarName: "Personal", Title: "Review notes", DueDate: "2026-04-16"},
		{Action: runner.PlanningTaskActionCreateTask, CalendarName: "Work", Title: "Work backlog", DueDate: "2026-04-16"},
	})
}

func seedTaskMetadataFilter(dbPath string) error {
	return runSeedRequests(dbPath, []runner.PlanningTaskRequest{
		{Action: runner.PlanningTaskActionCreateTask, CalendarName: "Work", Title: "Metadata review", DueDate: "2026-04-16", Priority: "high", Status: "in_progress", Tags: []string{"planning", "review"}},
		{Action: runner.PlanningTaskActionCreateTask, CalendarName: "Work", Title: "Low priority backlog", DueDate: "2026-04-16", Priority: "low", Status: "todo", Tags: []string{"planning"}},
		{Action: runner.PlanningTaskActionCreateTask, CalendarName: "Personal", Title: "Personal review", DueDate: "2026-04-16", Priority: "high", Status: "in_progress", Tags: []string{"planning", "review"}},
	})
}

func seedRecurringTask(dbPath string) error {
	count := int32(3)
	return runSeedRequests(dbPath, []runner.PlanningTaskRequest{
		{Action: runner.PlanningTaskActionCreateTask, CalendarName: "Personal", Title: "Daily review", DueDate: "2026-04-16", Recurrence: &runner.RecurrenceRuleRequest{Frequency: "daily", Count: &count}},
	})
}

func seedDeleteTaskData(dbPath string) error {
	return runSeedRequests(dbPath, []runner.PlanningTaskRequest{
		{Action: runner.PlanningTaskActionCreateTask, CalendarName: "Personal", Title: "Old note", DueDate: "2026-04-16"},
		{Action: runner.PlanningTaskActionCreateTask, CalendarName: "Personal", Title: "Keep note", DueDate: "2026-04-16"},
	})
}

func seedDeleteEventData(dbPath string) error {
	return runSeedRequests(dbPath, []runner.PlanningTaskRequest{
		{Action: runner.PlanningTaskActionCreateEvent, CalendarName: "Personal", Title: "Old appointment", StartAt: "2026-04-16T09:00:00Z", EndAt: "2026-04-16T09:30:00Z"},
		{Action: runner.PlanningTaskActionCreateEvent, CalendarName: "Personal", Title: "Keep appointment", StartAt: "2026-04-16T10:00:00Z", EndAt: "2026-04-16T10:30:00Z"},
	})
}

func seedEmptyArchiveCalendar(dbPath string) error {
	return runSeedRequests(dbPath, []runner.PlanningTaskRequest{
		{Action: runner.PlanningTaskActionEnsureCalendar, CalendarName: "Archive"},
	})
}

func seedPatchableEvent(dbPath string) error {
	count := int32(2)
	location := "Room 2"
	return runSeedRequests(dbPath, []runner.PlanningTaskRequest{
		{
			Action:       runner.PlanningTaskActionCreateEvent,
			CalendarName: "Work",
			Title:        "Planning sync",
			Location:     &location,
			StartAt:      "2026-04-16T15:00:00Z",
			EndAt:        "2026-04-16T16:00:00Z",
			Recurrence:   &runner.RecurrenceRuleRequest{Frequency: "daily", Count: &count},
		},
	})
}

func seedDatedReviewTask(dbPath string) error {
	return runSeedRequests(dbPath, []runner.PlanningTaskRequest{
		{Action: runner.PlanningTaskActionCreateTask, CalendarName: "Personal", Title: "Review notes", DueDate: "2026-04-16"},
	})
}

func seedLegacyMigrationData(dbPath string) error {
	return runSeedRequests(dbPath, []runner.PlanningTaskRequest{
		{Action: runner.PlanningTaskActionCreateEvent, CalendarName: "Legacy", Title: "Team sync", StartAt: "2026-04-16T09:00:00Z", EndAt: "2026-04-16T09:30:00Z"},
		{Action: runner.PlanningTaskActionCreateTask, CalendarName: "Legacy", Title: "Review notes", DueDate: "2026-04-16"},
		{Action: runner.PlanningTaskActionEnsureCalendar, CalendarName: "Work"},
	})
}

func runSeedRequests(dbPath string, requests []runner.PlanningTaskRequest) error {
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
	case "list-tasks-metadata-filter":
		return verifyTasks(dbPath, finalMessage, []taskState{{Title: "Metadata review", Priority: "high", Status: "in_progress", Tags: []string{"planning", "review"}}}, []string{"Low priority backlog", "Personal review"}, false)
	case "complete-task":
		return verifyTasks(dbPath, finalMessage, []taskState{{Title: "Review notes"}}, nil, true)
	case "complete-recurring-task":
		return verifyRecurringTaskCompletion(dbPath, finalMessage)
	case "delete-task":
		return verifyDeletedTask(dbPath, finalMessage)
	case "delete-event":
		return verifyDeletedEvent(dbPath, finalMessage)
	case "delete-empty-calendar":
		return verifyDeletedEmptyCalendar(dbPath, finalMessage)
	case "mixed-event-task":
		eventCheck, err := verifyEvents(dbPath, finalMessage, []eventState{{Title: "Standup", StartAt: "2026-04-16T09:00:00Z"}}, nil)
		if err != nil || !eventCheck.Passed {
			return eventCheck, err
		}
		return verifyTasks(dbPath, finalMessage, []taskState{{Title: "Review notes", DueDate: "2026-04-16"}}, nil, false)
	case "update-calendar-metadata":
		return verifyCalendarDetails(dbPath, finalMessage, calendarState{Name: "Work", Description: "Delivery planning", Color: "#2563EB"})
	case "update-event-patch-clear":
		return verifyEvents(dbPath, finalMessage, []eventState{{Title: "Planning sync", StartAt: "2026-04-16T15:00:00Z", LocationCleared: true, RecurrenceCleared: true}}, nil)
	case "update-task-due-mode":
		return verifyTasks(dbPath, finalMessage, []taskState{{Title: "Review notes", DueAt: "2026-04-16T11:00:00Z", DueDateCleared: true}}, nil, false)
	case "weekly-recurrence-by-weekday":
		taskCheck, err := verifyTasks(dbPath, finalMessage, []taskState{{Title: "Water plants", DueDate: "2026-04-13", Recurrence: "weekly", Count: int32Ptr(4), ByWeekday: []string{"MO", "WE"}}}, nil, false)
		if err != nil || !taskCheck.Passed {
			return taskCheck, err
		}
		return verifyAgendaOccurrences(dbPath, finalMessage, "Water plants", []string{"2026-04-13", "2026-04-15", "2026-04-20", "2026-04-22"}, nil)
	case "monthly-recurrence-by-month-day":
		taskCheck, err := verifyTasks(dbPath, finalMessage, []taskState{{Title: "Pay rent", DueDate: "2026-01-31", Recurrence: "monthly", Count: int32Ptr(3), ByMonthDay: []int32{31}}}, nil, false)
		if err != nil || !taskCheck.Passed {
			return taskCheck, err
		}
		return verifyAgendaOccurrences(dbPath, finalMessage, "Pay rent", []string{"2026-01-31", "2026-03-31"}, []string{"2026-02-28"})
	case "task-metadata-create":
		return verifyTasks(dbPath, finalMessage, []taskState{{Title: "Review notes", DueDate: "2026-04-16", Priority: "high", Status: "in_progress", Tags: []string{"planning", "review"}}}, nil, false)
	case "reminder-create-query-dismiss":
		return verifyReminderCreateQueryDismiss(dbPath, finalMessage)
	case "migration-style-copy":
		return verifyMigrationCopy(dbPath, finalMessage)
	case "unsupported-import-export":
		return verifyUnsupportedWorkflow(dbPath, finalMessage, []string{"unsupported", "not support", "does not support"}, []string{"import", "export", "icalendar", "ics"})
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
	case "invalid-task-priority":
		return verifyFinalAnswerOnlyRejection(dbPath, finalMessage, []string{"priority", "low", "medium", "high"})
	case "invalid-task-status":
		return verifyFinalAnswerOnlyRejection(dbPath, finalMessage, []string{"status", "todo", "in_progress", "done"})
	case "invalid-task-tag":
		return verifyFinalAnswerOnlyRejection(dbPath, finalMessage, []string{"tag", "spaces", "invalid"})
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
	case "mt-disambiguate-calendar":
		if turnIndex == 1 {
			return verifyNoWorkCopyClarification(dbPath, finalMessage)
		}
		return verifyMigrationTaskCopy(dbPath, finalMessage)
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

func verifyCalendarDetails(dbPath string, finalMessage string, expected calendarState) (verificationResult, error) {
	calendars, err := listCalendars(dbPath)
	if err != nil {
		return verificationResult{}, err
	}
	databasePass := false
	for _, calendar := range calendars {
		if calendar.Name != expected.Name {
			continue
		}
		if expected.Description != "" && (calendar.Description == nil || *calendar.Description != expected.Description) {
			continue
		}
		if expected.Color != "" && (calendar.Color == nil || *calendar.Color != expected.Color) {
			continue
		}
		databasePass = true
		break
	}
	assistantPass := mentionsAll(finalMessage, expected.Name)
	if expected.Description != "" {
		assistantPass = assistantPass && mentionsAll(finalMessage, expected.Description)
	}
	if expected.Color != "" {
		assistantPass = assistantPass && mentionsAll(finalMessage, expected.Color)
	}
	return verificationResult{
		Passed:        databasePass && assistantPass,
		DatabasePass:  databasePass,
		AssistantPass: assistantPass,
		Details:       passDetails(databasePass, assistantPass, "expected calendar metadata in DB and final answer"),
		Calendars:     []calendarState{expected},
	}, nil
}

func listCalendars(dbPath string) ([]domain.Calendar, error) {
	if !fileExists(dbPath) {
		return nil, nil
	}
	repository, err := store.Open(dbPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = repository.Close() }()
	service := internalservice.New(repository)
	page, err := service.ListCalendars(domain.PageParams{Limit: 100})
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

func calendarNameExists(calendars []domain.Calendar, name string) bool {
	for _, calendar := range calendars {
		if calendar.Name == name {
			return true
		}
	}
	return false
}

func verifyEvents(dbPath string, finalMessage string, expected []eventState, forbidden []string) (verificationResult, error) {
	result, err := runPlanning(dbPath, runner.PlanningTaskRequest{Action: runner.PlanningTaskActionListEvents, Limit: intPtr(100)})
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

func listEventsForCalendar(dbPath string, calendarName string) ([]runner.EventEntry, error) {
	result, err := runPlanning(dbPath, runner.PlanningTaskRequest{Action: runner.PlanningTaskActionListEvents, CalendarName: calendarName, Limit: intPtr(100)})
	if err != nil {
		return nil, err
	}
	if result.Rejected {
		return nil, errors.New(result.RejectionReason)
	}
	return result.Events, nil
}

func verifyTasks(dbPath string, finalMessage string, expected []taskState, forbidden []string, requireCompleted bool) (verificationResult, error) {
	tasks, err := listTasks(dbPath)
	if err != nil {
		return verificationResult{}, err
	}
	databasePass := true
	for _, want := range expected {
		if !taskExists(tasks, want, requireCompleted) {
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

func listTasks(dbPath string) ([]runner.TaskEntry, error) {
	result, err := runPlanning(dbPath, runner.PlanningTaskRequest{Action: runner.PlanningTaskActionListTasks, Limit: intPtr(100)})
	if err != nil {
		return nil, err
	}
	if result.Rejected {
		return nil, errors.New(result.RejectionReason)
	}
	return result.Tasks, nil
}

func listTasksForCalendar(dbPath string, calendarName string) ([]runner.TaskEntry, error) {
	result, err := runPlanning(dbPath, runner.PlanningTaskRequest{Action: runner.PlanningTaskActionListTasks, CalendarName: calendarName, Limit: intPtr(100)})
	if err != nil {
		return nil, err
	}
	if result.Rejected {
		return nil, errors.New(result.RejectionReason)
	}
	return result.Tasks, nil
}

func verifyReminderCreateQueryDismiss(dbPath string, finalMessage string) (verificationResult, error) {
	tasks, err := listTasksForCalendar(dbPath, "Personal")
	if err != nil {
		return verificationResult{}, err
	}
	taskStored := false
	for _, task := range tasks {
		if task.Title != "Take medicine" || task.DueAt != "2026-04-16T10:00:00Z" {
			continue
		}
		for _, reminder := range task.Reminders {
			if reminder.BeforeMinutes == 60 {
				taskStored = true
				break
			}
		}
	}

	pending, err := runPlanning(dbPath, runner.PlanningTaskRequest{
		Action:       runner.PlanningTaskActionListReminders,
		CalendarName: "Personal",
		From:         "2026-04-16T08:00:00Z",
		To:           "2026-04-16T10:00:00Z",
		Limit:        intPtr(100),
	})
	if err != nil {
		return verificationResult{}, err
	}
	databasePass := taskStored && !pending.Rejected && len(pending.Reminders) == 0
	assistantPass := mentionsAll(finalMessage, "Take medicine", "reminder") && mentionsAny(finalMessage, []string{"dismissed", "no pending", "none pending"})
	return verificationResult{
		Passed:        databasePass && assistantPass,
		DatabasePass:  databasePass,
		AssistantPass: assistantPass,
		Details:       passDetails(databasePass, assistantPass, "expected reminder stored, queried, dismissed, and absent from pending range"),
		Tasks:         []taskState{{Title: "Take medicine", DueAt: "2026-04-16T10:00:00Z"}},
	}, nil
}

func verifyAgendaRange(dbPath string, finalMessage string) (verificationResult, error) {
	result, err := runPlanning(dbPath, runner.PlanningTaskRequest{
		Action: runner.PlanningTaskActionListAgenda,
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
	result, err := runPlanning(dbPath, runner.PlanningTaskRequest{
		Action: runner.PlanningTaskActionListAgenda,
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

func verifyDeletedTask(dbPath string, finalMessage string) (verificationResult, error) {
	tasks, err := listTasksForCalendar(dbPath, "Personal")
	if err != nil {
		return verificationResult{}, err
	}
	databasePass := countMatchingTasks(tasks, taskState{Title: "Old note", DueDate: "2026-04-16"}, false) == 0 &&
		countMatchingTasks(tasks, taskState{Title: "Keep note", DueDate: "2026-04-16"}, false) == 1
	assistantPass := mentionsAll(finalMessage, "Old note") && mentionsAny(finalMessage, []string{"deleted", "removed"})
	return verificationResult{
		Passed:        databasePass && assistantPass,
		DatabasePass:  databasePass,
		AssistantPass: assistantPass,
		Details:       passDetails(databasePass, assistantPass, "expected target task deleted while keep task remains"),
		Tasks:         []taskState{{Title: "Keep note", DueDate: "2026-04-16"}},
	}, nil
}

func verifyDeletedEvent(dbPath string, finalMessage string) (verificationResult, error) {
	events, err := listEventsForCalendar(dbPath, "Personal")
	if err != nil {
		return verificationResult{}, err
	}
	databasePass := countMatchingEvents(events, eventState{Title: "Old appointment", StartAt: "2026-04-16T09:00:00Z"}) == 0 &&
		countMatchingEvents(events, eventState{Title: "Keep appointment", StartAt: "2026-04-16T10:00:00Z"}) == 1
	assistantPass := mentionsAll(finalMessage, "Old appointment") && mentionsAny(finalMessage, []string{"deleted", "removed"})
	return verificationResult{
		Passed:        databasePass && assistantPass,
		DatabasePass:  databasePass,
		AssistantPass: assistantPass,
		Details:       passDetails(databasePass, assistantPass, "expected target event deleted while keep event remains"),
		Events:        []eventState{{Title: "Keep appointment", StartAt: "2026-04-16T10:00:00Z"}},
	}, nil
}

func verifyDeletedEmptyCalendar(dbPath string, finalMessage string) (verificationResult, error) {
	result, err := runPlanning(dbPath, runner.PlanningTaskRequest{
		Action:       runner.PlanningTaskActionEnsureCalendar,
		CalendarName: "Archive",
	})
	if err != nil {
		return verificationResult{}, err
	}
	databasePass := !result.Rejected && len(result.Writes) == 1 && result.Writes[0].Kind == "calendar" && result.Writes[0].Status == "created"
	assistantPass := mentionsAll(finalMessage, "Archive")
	return verificationResult{
		Passed:        databasePass && assistantPass,
		DatabasePass:  databasePass,
		AssistantPass: assistantPass,
		Details:       passDetails(databasePass, assistantPass, "expected empty calendar deleted before verification recreated it"),
		Calendars:     []calendarState{{Name: "Archive"}},
	}, nil
}

func verifyAgendaOccurrences(dbPath string, finalMessage string, title string, expectedDates []string, forbiddenDates []string) (verificationResult, error) {
	result, err := runPlanning(dbPath, runner.PlanningTaskRequest{
		Action: runner.PlanningTaskActionListAgenda,
		From:   "2026-01-01T00:00:00Z",
		To:     "2026-05-01T00:00:00Z",
		Limit:  intPtr(100),
	})
	if err != nil {
		return verificationResult{}, err
	}
	foundDates := map[string]bool{}
	for _, item := range result.Agenda {
		if item.Title == title {
			if item.DueDate != "" {
				foundDates[item.DueDate] = true
			}
			if item.StartDate != "" {
				foundDates[item.StartDate] = true
			}
			if item.DueAt != "" {
				foundDates[item.DueAt[:10]] = true
			}
			if item.StartAt != "" {
				foundDates[item.StartAt[:10]] = true
			}
		}
	}
	databasePass := !result.Rejected
	for _, date := range expectedDates {
		if !foundDates[date] {
			databasePass = false
		}
	}
	for _, date := range forbiddenDates {
		if foundDates[date] {
			databasePass = false
		}
	}
	assistantPass := finalMessage != ""
	return verificationResult{
		Passed:        databasePass && assistantPass,
		DatabasePass:  databasePass,
		AssistantPass: assistantPass,
		Details:       passDetails(databasePass, assistantPass, "expected recurrence occurrences in agenda"),
		Agenda:        agendaStatesForDates(title, expectedDates),
	}, nil
}

func agendaStatesForDates(title string, dates []string) []agendaEntryState {
	out := make([]agendaEntryState, 0, len(dates))
	for _, date := range dates {
		out = append(out, agendaEntryState{Kind: "task", Title: title, DueDate: date})
	}
	return out
}

func verifyUnsupportedWorkflow(dbPath string, finalMessage string, unsupportedKeywords []string, topicKeywords []string) (verificationResult, error) {
	unsupportedKeywords = append([]string{"unsupported", "not supported", "not support", "does not support", "doesn't support"}, unsupportedKeywords...)
	databasePass := !fileExists(dbPath)
	assistantPass := finalMessage != "" && mentionsAny(finalMessage, unsupportedKeywords) && mentionsAll(finalMessage, topicKeywords...)
	return verificationResult{
		Passed:        databasePass && assistantPass,
		DatabasePass:  databasePass,
		AssistantPass: assistantPass,
		Details:       passDetails(databasePass, assistantPass, "expected unsupported-workflow answer without DB writes"),
	}, nil
}

func verifyMigrationCopy(dbPath string, finalMessage string) (verificationResult, error) {
	legacyEvents, err := listEventsForCalendar(dbPath, "Legacy")
	if err != nil {
		return verificationResult{}, err
	}
	workEvents, err := listEventsForCalendar(dbPath, "Work")
	if err != nil {
		return verificationResult{}, err
	}
	legacyTasks, err := listTasksForCalendar(dbPath, "Legacy")
	if err != nil {
		return verificationResult{}, err
	}
	workTasks, err := listTasksForCalendar(dbPath, "Work")
	if err != nil {
		return verificationResult{}, err
	}
	databasePass := eventExists(legacyEvents, eventState{Title: "Team sync", StartAt: "2026-04-16T09:00:00Z"}) &&
		eventExists(workEvents, eventState{Title: "Team sync", StartAt: "2026-04-16T09:00:00Z"}) &&
		taskExists(legacyTasks, taskState{Title: "Review notes", DueDate: "2026-04-16"}, false) &&
		taskExists(workTasks, taskState{Title: "Review notes", DueDate: "2026-04-16"}, false)
	assistantPass := mentionsAll(finalMessage, "Team sync", "Review notes", "Work")
	return verificationResult{
		Passed:        databasePass && assistantPass,
		DatabasePass:  databasePass,
		AssistantPass: assistantPass,
		Details:       passDetails(databasePass, assistantPass, "expected copied Work items while Legacy items remain"),
		Events:        []eventState{{Title: "Team sync", StartAt: "2026-04-16T09:00:00Z"}},
		Tasks:         []taskState{{Title: "Review notes", DueDate: "2026-04-16"}},
	}, nil
}

func verifyNoWorkCopyClarification(dbPath string, finalMessage string) (verificationResult, error) {
	tasks, err := listTasks(dbPath)
	if err != nil {
		return verificationResult{}, err
	}
	databasePass := countMatchingTasks(tasks, taskState{Title: "Review notes", DueDate: "2026-04-16"}, false) == 1
	assistantPass := finalMessage != "" && mentionsAny(finalMessage, []string{"destination", "calendar", "which"})
	return verificationResult{
		Passed:        databasePass && assistantPass,
		DatabasePass:  databasePass,
		AssistantPass: assistantPass,
		Details:       passDetails(databasePass, assistantPass, "expected clarification before creating destination copy"),
	}, nil
}

func countMatchingTasks(tasks []runner.TaskEntry, want taskState, requireCompleted bool) int {
	count := 0
	for _, task := range tasks {
		if taskMatches(task, want, requireCompleted) {
			count++
		}
	}
	return count
}

func countMatchingEvents(events []runner.EventEntry, want eventState) int {
	count := 0
	for _, event := range events {
		if eventExists([]runner.EventEntry{event}, want) {
			count++
		}
	}
	return count
}

func verifyMigrationTaskCopy(dbPath string, finalMessage string) (verificationResult, error) {
	legacyTasks, err := listTasksForCalendar(dbPath, "Legacy")
	if err != nil {
		return verificationResult{}, err
	}
	workTasks, err := listTasksForCalendar(dbPath, "Work")
	if err != nil {
		return verificationResult{}, err
	}
	databasePass := taskExists(legacyTasks, taskState{Title: "Review notes", DueDate: "2026-04-16"}, false) &&
		taskExists(workTasks, taskState{Title: "Review notes", DueDate: "2026-04-16"}, false)
	assistantPass := mentionsAll(finalMessage, "Review notes", "Work")
	return verificationResult{
		Passed:        databasePass && assistantPass,
		DatabasePass:  databasePass,
		AssistantPass: assistantPass,
		Details:       passDetails(databasePass, assistantPass, "expected copied Work task while Legacy task remains"),
		Tasks:         []taskState{{Title: "Review notes", DueDate: "2026-04-16"}},
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

func runPlanning(dbPath string, request runner.PlanningTaskRequest) (runner.PlanningTaskResult, error) {
	return runner.RunPlanningTask(context.Background(), runner.Options{DatabasePath: dbPath}, request)
}

func eventExists(events []runner.EventEntry, want eventState) bool {
	for _, event := range events {
		if event.Title != want.Title {
			continue
		}
		if want.Description != "" && (event.Description == nil || *event.Description != want.Description) {
			continue
		}
		if want.Location != "" && (event.Location == nil || *event.Location != want.Location) {
			continue
		}
		if want.LocationCleared && event.Location != nil {
			continue
		}
		if want.StartAt != "" && event.StartAt != want.StartAt {
			continue
		}
		if want.StartDate != "" && event.StartDate != want.StartDate {
			continue
		}
		if want.RecurrenceCleared && event.Recurrence != nil {
			continue
		}
		if !recurrenceMatches(event.Recurrence, want.Recurrence, want.Interval, want.Count, want.UntilAt, want.UntilDate, want.ByWeekday, want.ByMonthDay) {
			continue
		}
		return true
	}
	return false
}

func taskExists(tasks []runner.TaskEntry, want taskState, requireCompleted bool) bool {
	for _, task := range tasks {
		if taskMatches(task, want, requireCompleted) {
			return true
		}
	}
	return false
}

func taskMatches(task runner.TaskEntry, want taskState, requireCompleted bool) bool {
	if task.Title != want.Title {
		return false
	}
	if want.Description != "" && (task.Description == nil || *task.Description != want.Description) {
		return false
	}
	if want.DueAt != "" && task.DueAt != want.DueAt {
		return false
	}
	if want.DueDate != "" && task.DueDate != want.DueDate {
		return false
	}
	if want.DueDateCleared && task.DueDate != "" {
		return false
	}
	if want.RecurrenceCleared && task.Recurrence != nil {
		return false
	}
	if want.Priority != "" && task.Priority != want.Priority {
		return false
	}
	if want.Status != "" && task.Status != want.Status {
		return false
	}
	if len(want.Tags) > 0 && !sameStringSet(task.Tags, want.Tags) {
		return false
	}
	if !recurrenceMatches(task.Recurrence, want.Recurrence, want.Interval, want.Count, want.UntilAt, want.UntilDate, want.ByWeekday, want.ByMonthDay) {
		return false
	}
	if requireCompleted && task.CompletedAt == "" {
		return false
	}
	return true
}

func recurrenceMatches(actual *runner.RecurrenceRuleResult, frequency string, interval int32, count *int32, untilAt string, untilDate string, weekdays []string, monthDays []int32) bool {
	if frequency == "" && interval == 0 && count == nil && untilAt == "" && untilDate == "" && len(weekdays) == 0 && len(monthDays) == 0 {
		return true
	}
	if actual == nil {
		return false
	}
	if frequency != "" && actual.Frequency != frequency {
		return false
	}
	if interval != 0 && actual.Interval != interval {
		return false
	}
	if count != nil && (actual.Count == nil || *actual.Count != *count) {
		return false
	}
	if untilAt != "" && actual.UntilAt != untilAt {
		return false
	}
	if untilDate != "" && actual.UntilDate != untilDate {
		return false
	}
	if len(weekdays) > 0 && !sameStringSet(actual.ByWeekday, weekdays) {
		return false
	}
	if len(monthDays) > 0 && !sameInt32Set(actual.ByMonthDay, monthDays) {
		return false
	}
	return true
}

func sameStringSet(got []string, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	gotCopy := append([]string(nil), got...)
	wantCopy := append([]string(nil), want...)
	sort.Strings(gotCopy)
	sort.Strings(wantCopy)
	for i := range gotCopy {
		if gotCopy[i] != wantCopy[i] {
			return false
		}
	}
	return true
}

func sameInt32Set(got []int32, want []int32) bool {
	if len(got) != len(want) {
		return false
	}
	gotCopy := append([]int32(nil), got...)
	wantCopy := append([]int32(nil), want...)
	sort.Slice(gotCopy, func(i, j int) bool { return gotCopy[i] < gotCopy[j] })
	sort.Slice(wantCopy, func(i, j int) bool { return wantCopy[i] < wantCopy[j] })
	for i := range gotCopy {
		if gotCopy[i] != wantCopy[i] {
			return false
		}
	}
	return true
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
		if task.Priority != "" {
			values = append(values, task.Priority)
		}
		if task.Status != "" {
			values = append(values, task.Status)
		}
		values = append(values, task.Tags...)
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

func installEvalRunnerAndSkill(runRepo string, runDir string) error {
	if err := buildEvalRunner(runRepo, runDir); err != nil {
		return err
	}
	return installEvalSkill(runRepo)
}

func buildEvalRunner(runRepo string, runDir string) error {
	binDir := filepath.Join(runDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return err
	}
	cmd := exec.Command("go", "build", "-o", filepath.Join(binDir, "openplanner"), "./cmd/openplanner")
	cmd.Dir = runRepo
	cmd.Env = evalEnv(runDir, evalDatabasePath(runRepo), cacheConfig{Mode: cacheModeIsolated})
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("build openplanner runner: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func installEvalSkill(runRepo string) error {
	skillDir := filepath.Join(runRepo, ".agents", "skills", "openplanner")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return err
	}
	if err := copyFile(filepath.Join(runRepo, "skills", "openplanner", "SKILL.md"), filepath.Join(skillDir, "SKILL.md"), 0o644); err != nil {
		return err
	}

	return nil
}

func preflightEvalContext(repoRoot string, runRepo string, runDir string, cache cacheConfig) error {
	sourceSkill := filepath.Join(repoRoot, "skills", "openplanner", "SKILL.md")
	installedSkill := filepath.Join(runRepo, ".agents", "skills", "openplanner", "SKILL.md")
	sourceBytes, err := os.ReadFile(sourceSkill)
	if err != nil {
		return err
	}
	installedBytes, err := os.ReadFile(installedSkill)
	if err != nil {
		return err
	}
	if !bytes.Equal(sourceBytes, installedBytes) {
		return errors.New("installed production skill does not match shipped SKILL.md")
	}
	if _, err := os.Stat(filepath.Join(runRepo, "AGENTS.md")); !os.IsNotExist(err) {
		if err == nil {
			return errors.New("production eval repo must not contain AGENTS.md")
		}
		return err
	}

	cmd := exec.Command("codex", "debug", "prompt-input", "Use OpenPlanner to list today's agenda.")
	cmd.Dir = runRepo
	cmd.Env = evalEnv(runDir, evalDatabasePath(runRepo), cache)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	rendered := string(output)
	if !strings.Contains(rendered, "- openplanner:") {
		return errors.New("rendered prompt is missing openplanner skill discovery")
	}
	if !strings.Contains(rendered, ".agents/skills/openplanner/SKILL.md") {
		return errors.New("rendered prompt does not point openplanner to the installed project skill")
	}
	if containsOpenPlannerAgentsInstructions(rendered) {
		return errors.New("rendered prompt contains OpenPlanner product instructions from AGENTS.md")
	}
	return nil
}

func containsOpenPlannerAgentsInstructions(rendered string) bool {
	const marker = "# AGENTS.md instructions"
	index := strings.Index(rendered, marker)
	if index < 0 {
		return false
	}
	agentsText := rendered[index:]
	for _, forbidden := range []string{
		"openplanner planning",
		`"action"`,
		"calendar_name",
		"YYYY-MM-DD",
		"RFC3339",
		"ambiguous short date",
		"product data agent",
	} {
		if strings.Contains(agentsText, forbidden) {
			return true
		}
	}
	return false
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

func productionScoreFor(results []runResult, selectedScenarios []scenario, filteredRun bool) productionScore {
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

	coverage := scenarioCoverageFor(selectedScenarios, filteredRun)
	coverageFailures := []string{}
	for _, current := range coverage {
		if !current.Passed {
			coverageFailures = append(coverageFailures, current.Category)
		}
	}
	criteria = append(criteria, criterion{
		Name:    "expanded_category_coverage",
		Passed:  len(coverageFailures) == 0,
		Details: categoryCoverageDetails(coverage, filteredRun),
	})

	allPassed := true
	for _, criterion := range criteria {
		if !criterion.Passed {
			allPassed = false
		}
	}
	recommendation := "prefer_runner_for_routine_openplanner_operations"
	if !allPassed {
		recommendation = "review_runner_eval_failures_before_recommending"
	}
	return productionScore{Recommendation: recommendation, Passed: allPassed, Criteria: criteria}
}

func scenarioCoverageFor(selectedScenarios []scenario, filteredRun bool) []scenarioCoverage {
	byCategory := map[string][]scenario{}
	for _, sc := range selectedScenarios {
		category := scenarioCategory(sc)
		byCategory[category] = append(byCategory[category], sc)
	}
	seen := map[string]bool{}
	categories := []string{}
	for _, category := range requiredFullSuiteCategories {
		categories = append(categories, category)
		seen[category] = true
	}
	for category := range byCategory {
		if !seen[category] {
			categories = append(categories, category)
		}
	}
	sort.Strings(categories)

	out := make([]scenarioCoverage, 0, len(categories))
	for _, category := range categories {
		scenarios := byCategory[category]
		ids := make([]string, 0, len(scenarios))
		states := map[string]bool{}
		for _, sc := range scenarios {
			ids = append(ids, sc.ID)
			states[scenarioFeatureState(sc)] = true
		}
		sort.Strings(ids)
		required := isRequiredFullSuiteCategory(category)
		passed := len(ids) > 0 || filteredRun || !required
		details := "category present"
		if len(ids) == 0 {
			details = "category not selected"
		}
		if filteredRun {
			details = "filtered run; full-suite category coverage not enforced"
			passed = true
		}
		out = append(out, scenarioCoverage{
			Category:     category,
			FeatureState: featureStateSummary(states),
			Scenarios:    ids,
			Required:     required,
			Passed:       passed,
			Details:      details,
		})
	}
	return out
}

func isRequiredFullSuiteCategory(category string) bool {
	for _, required := range requiredFullSuiteCategories {
		if category == required {
			return true
		}
	}
	return false
}

func featureStateSummary(states map[string]bool) string {
	if len(states) == 0 {
		return ""
	}
	values := make([]string, 0, len(states))
	for state := range states {
		values = append(values, state)
	}
	sort.Strings(values)
	return strings.Join(values, ",")
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

func categoryCoverageDetails(coverage []scenarioCoverage, filteredRun bool) string {
	if filteredRun {
		return "filtered run; full-suite category coverage not enforced"
	}
	missing := []string{}
	for _, current := range coverage {
		if current.Required && len(current.Scenarios) == 0 {
			missing = append(missing, current.Category)
		}
	}
	if len(missing) == 0 {
		return "expanded production categories covered"
	}
	return fmt.Sprintf("missing expanded production categories: %s", sortedJoin(missing))
}

func forbiddenInspectionDetails(failures []string) string {
	if len(failures) == 0 {
		return "no removed-interface path inspection, module-cache inspection, direct SQLite access, CLI usage, or routine broad repo search detected"
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
	fmt.Fprintf(&b, "# OpenPlanner JSON Runner Eval %s\n\n", value.Date)
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

	b.WriteString("\n## Scenario Coverage\n\n")
	b.WriteString("| Category | Required | Passed | Feature State | Scenarios | Details |\n")
	b.WriteString("| --- | ---: | ---: | --- | --- | --- |\n")
	for _, coverage := range value.ScenarioCoverage {
		fmt.Fprintf(&b, "| `%s` | %t | %t | `%s` | %s | %s |\n",
			coverage.Category,
			coverage.Required,
			coverage.Passed,
			coverage.FeatureState,
			escapeMarkdownTable(strings.Join(coverage.Scenarios, ", ")),
			escapeMarkdownTable(coverage.Details),
		)
	}

	b.WriteString("\n## Results\n\n")
	b.WriteString("| Scenario | Category | Feature State | Passed | Tools | Commands | Assistant Calls | Non-Cached Tokens | Wall Seconds | Details |\n")
	b.WriteString("| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | --- |\n")
	for _, result := range value.Results {
		tokenText := "n/a"
		if tokens, ok := nonCachedInputTokens(result); ok {
			tokenText = fmt.Sprintf("%d", tokens)
		}
		fmt.Fprintf(&b, "| `%s` | `%s` | `%s` | %t | %d | %d | %d | %s | %.2f | %s |\n",
			result.Scenario,
			result.ScenarioCategory,
			result.FeatureState,
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

func writeScaleMarkdown(path string, value scaleReport) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# OpenPlanner Scale Eval %s\n\n", value.Date)
	fmt.Fprintf(&b, "- Issue: `%s`\n", value.Issue)
	fmt.Fprintf(&b, "- Harness: %s\n", value.Harness)
	fmt.Fprintf(&b, "- Threshold policy: %s\n", value.ThresholdPolicy)
	fmt.Fprintf(&b, "- Run root: `%s`\n", value.RunRoot)
	fmt.Fprintf(&b, "- Database path: `%s`\n", value.DatabasePath)
	fmt.Fprintf(&b, "- Harness wall seconds: `%.2f`\n", value.HarnessWallSeconds)
	fmt.Fprintf(&b, "- Scale score: `%s`\n", passFail(value.Passed))
	fmt.Fprintf(&b, "- Raw artifacts: %s\n", value.RawArtifactsNote)
	if len(value.BlockerIssues) > 0 {
		fmt.Fprintf(&b, "- Blocker issues: `%s`\n", strings.Join(value.BlockerIssues, "`, `"))
	}

	b.WriteString("\n## Dataset\n\n")
	b.WriteString("| Calendars | Events | Tasks | Recurring Events | Recurring Tasks | Recurrence Rules | Completion Rows | Agenda Range Days | Limit |\n")
	b.WriteString("| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	fmt.Fprintf(&b, "| %d | %d | %d | %d | %d | %d | %d | %d | %d |\n",
		value.Dataset.Calendars,
		value.Dataset.Events,
		value.Dataset.Tasks,
		value.Dataset.RecurringEvents,
		value.Dataset.RecurringTasks,
		value.Dataset.RecurrenceRules,
		value.Dataset.CompletionRows,
		value.Dataset.AgendaRangeDays,
		value.Dataset.Limit,
	)

	b.WriteString("\n## Results\n\n")
	b.WriteString("| Scenario | Passed | Wall Seconds | Threshold Seconds | Items Returned | Pages | Events | Tasks | Recurrence Rules | Completion Rows | Notes |\n")
	b.WriteString("| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |\n")
	for _, result := range value.Results {
		fmt.Fprintf(&b, "| `%s` | %t | %.2f | %.2f | %d | %d | %d | %d | %d | %d | %s |\n",
			result.Scenario,
			result.Passed,
			result.WallSeconds,
			result.ThresholdSeconds,
			result.ItemsReturned,
			result.PagesTraversed,
			result.Events,
			result.Tasks,
			result.RecurrenceRules,
			result.CompletionRows,
			escapeMarkdownTable(strings.Join(result.Notes, "; ")),
		)
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
	return strings.Contains(lower, "generated/api_") ||
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
		if fields[i] == "go" && i+2 < len(fields) && fields[i+1] == "run" && strings.Contains(fields[i+2], "cmd/openplanner") {
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

func int32Ptr(value int32) *int32 {
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
	case "list-tasks-metadata-filter":
		return "list task metadata filter"
	case "complete-task":
		return "complete seeded non-recurring task"
	case "complete-recurring-task":
		return "complete seeded recurring occurrence"
	case "task-metadata-create":
		return "create task metadata"
	case "reminder-create-query-dismiss":
		return "create query dismiss reminder"
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
