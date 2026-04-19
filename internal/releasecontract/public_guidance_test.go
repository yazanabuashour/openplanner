package releasecontract_test

import (
	"os"
	"strings"
	"testing"
)

func TestPublicGuidanceUsesCodeFirstRunnerSurface(t *testing.T) {
	t.Parallel()

	files := []string{
		"../../AGENTS.md",
		"../../README.md",
		"../../CONTRIBUTING.md",
		"../../docs/agent-evals.md",
		"../../docs/agent-eval-results/README.md",
		"../../docs/maintainers.md",
		"../../docs/release-verification.md",
		"../../skills/openplanner/SKILL.md",
	}
	forbidden := []string{
		"openplanner-agentops",
		"go run ./cmd/openplanner",
		"openapi/openapi.yaml",
		"OpenAPI contract",
	}

	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		for _, bad := range forbidden {
			if strings.Contains(string(content), bad) {
				t.Fatalf("%s contains stale guidance %q", file, bad)
			}
		}
	}
}
