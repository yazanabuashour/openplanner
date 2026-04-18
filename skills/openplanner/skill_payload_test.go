package openplannerskill_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestSkillMarkdownLinksReferenceInstalledFiles(t *testing.T) {
	t.Parallel()

	markdownFiles := []string{}
	if err := filepath.WalkDir(".", func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".md" {
			return nil
		}
		markdownFiles = append(markdownFiles, path)
		return nil
	}); err != nil {
		t.Fatalf("walk markdown files: %v", err)
	}

	linkPattern := regexp.MustCompile(`\[[^\]]+\]\(([^)]+)\)`)
	for _, path := range markdownFiles {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, match := range linkPattern.FindAllStringSubmatch(string(content), -1) {
			target := match[1]
			if shouldSkipLinkTarget(target) {
				continue
			}
			targetPath := filepath.Clean(filepath.Join(filepath.Dir(path), target))
			if _, err := os.Stat(targetPath); err != nil {
				t.Fatalf("%s link target %q is not installed with the skill: %v", path, target, err)
			}
		}
	}
}

func TestProductionSkillUsesAgentOpsRunner(t *testing.T) {
	t.Parallel()

	content, err := os.ReadFile("SKILL.md")
	if err != nil {
		t.Fatalf("read skill: %v", err)
	}
	text := string(content)
	for _, want := range []string{
		"go run ./cmd/openplanner-agentops planning",
		"calendar_name",
		"references/planning.md",
		"Agenda results are already",
		"2026/04/16",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("skill missing %q", want)
		}
	}
	for _, forbidden := range []string{
		"Generated Client Fallback",
		"sdk/generated",
		"temporary Go module",
		"sqlite3",
		"SELECT ",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("skill contains forbidden text %q", forbidden)
		}
	}
	if regexp.MustCompile(`go run \./cmd/openplanner(\s|$)`).MatchString(text) {
		t.Fatal("skill contains stale human-facing cmd/openplanner runner guidance")
	}
}

func shouldSkipLinkTarget(target string) bool {
	return target == "" ||
		strings.HasPrefix(target, "#") ||
		strings.Contains(target, "://") ||
		filepath.IsAbs(target)
}
