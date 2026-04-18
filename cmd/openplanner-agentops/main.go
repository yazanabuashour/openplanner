package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/yazanabuashour/openplanner/agentops"
	"github.com/yazanabuashour/openplanner/sdk"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(stderr, "usage: openplanner-agentops planning [--db path]")
		return 2
	}

	switch args[0] {
	case "planning":
		return runPlanning(args[1:], stdin, stdout, stderr)
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

	var request agentops.PlanningTaskRequest
	decoder := json.NewDecoder(stdin)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		_, _ = fmt.Fprintf(stderr, "decode planning request: %v\n", err)
		return 1
	}

	result, err := agentops.RunPlanningTask(context.Background(), sdk.Options{DatabasePath: *databasePath}, request)
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
