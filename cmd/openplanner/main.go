package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"

	"github.com/yazanabuashour/openplanner/internal/caldav"
	"github.com/yazanabuashour/openplanner/internal/runner"
)

var serveCalDAV = caldav.ListenAndServe
var version string

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		_ = writeUsage(stderr)
		return 2
	}

	switch args[0] {
	case "help", "-h", "--help":
		if err := writeUsage(stdout); err != nil {
			_, _ = fmt.Fprintf(stderr, "write usage: %v\n", err)
			return 1
		}
		return 0
	case "version", "--version":
		if err := writeVersion(stdout); err != nil {
			_, _ = fmt.Fprintf(stderr, "write version: %v\n", err)
			return 1
		}
		return 0
	case "planning":
		return runPlanning(args[1:], stdin, stdout, stderr)
	case "caldav":
		return runCalDAV(args[1:], stderr)
	default:
		_, _ = fmt.Fprintf(stderr, "unknown subcommand %q\n", args[0])
		return 2
	}
}

func runPlanning(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("planning", flag.ContinueOnError)
	flags.SetOutput(stderr)
	databasePath := flags.String("db", "", "SQLite database path for tests or manual debugging")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		_, _ = fmt.Fprintln(stderr, "planning does not accept positional arguments")
		return 2
	}

	request, err := runner.DecodePlanningTaskRequest(stdin)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "decode planning request: %v\n", err)
		return 1
	}

	resolvedDatabasePath := *databasePath
	if resolvedDatabasePath == "" {
		resolvedDatabasePath = os.Getenv("OPENPLANNER_DATABASE_PATH")
	}

	result, err := runner.RunPlanningTask(context.Background(), runner.Options{DatabasePath: resolvedDatabasePath}, request)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "run planning task: %v\n", err)
		return 1
	}
	if err := json.NewEncoder(stdout).Encode(result); err != nil {
		_, _ = fmt.Fprintf(stderr, "encode planning result: %v\n", err)
		return 1
	}
	return 0
}

func writeUsage(w io.Writer) error {
	_, err := fmt.Fprint(w, `Usage:
  openplanner --version
  openplanner planning [--db path] < request.json
  openplanner caldav [--db path] [--addr host:port]

The agent-facing product surface is openplanner planning. The CalDAV adapter is experimental, local-only compatibility tooling.
`)
	return err
}

func writeVersion(w io.Writer) error {
	info, ok := debug.ReadBuildInfo()
	_, err := fmt.Fprintf(w, "openplanner %s\n", resolvedVersion(version, info, ok))
	return err
}

func resolvedVersion(linkerVersion string, info *debug.BuildInfo, ok bool) string {
	if linkerVersion != "" {
		return linkerVersion
	}
	if ok && info != nil && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}

func runCalDAV(args []string, stderr io.Writer) int {
	flags := flag.NewFlagSet("caldav", flag.ContinueOnError)
	flags.SetOutput(stderr)
	databasePath := flags.String("db", "", "SQLite database path for tests or manual debugging")
	addr := flags.String("addr", "127.0.0.1:8080", "CalDAV bind address")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		_, _ = fmt.Fprintln(stderr, "caldav does not accept positional arguments")
		return 2
	}

	resolvedDatabasePath := *databasePath
	if resolvedDatabasePath == "" {
		resolvedDatabasePath = os.Getenv("OPENPLANNER_DATABASE_PATH")
	}

	if err := caldav.ValidateAddr(*addr); err != nil {
		_, _ = fmt.Fprintf(stderr, "invalid caldav addr: %v\n", err)
		return 2
	}

	_, _ = fmt.Fprintf(stderr, "serving experimental CalDAV adapter on %s\n", *addr)
	if err := serveCalDAV(context.Background(), caldav.Options{Addr: *addr, DatabasePath: resolvedDatabasePath}); err != nil {
		_, _ = fmt.Fprintf(stderr, "serve caldav: %v\n", err)
		return 1
	}
	return 0
}
