package main

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	skillNamePattern    = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	markdownLinkPattern = regexp.MustCompile(`\[[^\]]+\]\(([^)]+)\)`)
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	if len(args) != 1 {
		return errors.New("usage: scripts/validate-agent-skill.sh <skill-directory>")
	}
	skillDir := strings.TrimRight(args[0], string(os.PathSeparator))
	if skillDir == "" {
		skillDir = "."
	}
	if err := validateSkillDir(skillDir); err != nil {
		return err
	}
	_, err := fmt.Fprintf(stdout, "validated %s\n", skillDir)
	return err
}

func validateSkillDir(skillDir string) error {
	info, err := os.Stat(skillDir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("skill directory not found: %s", skillDir)
		}
		return fmt.Errorf("stat skill directory %s: %w", skillDir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("skill directory not found: %s", skillDir)
	}

	entries, err := os.ReadDir(skillDir)
	if err != nil {
		return fmt.Errorf("read skill directory %s: %w", skillDir, err)
	}
	if len(entries) != 1 || entries[0].Name() != "SKILL.md" || entries[0].IsDir() {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		sort.Strings(names)
		return fmt.Errorf("%s must contain only SKILL.md; found %s", skillDir, strings.Join(names, ", "))
	}

	skillFile := filepath.Join(skillDir, "SKILL.md")
	content, err := os.ReadFile(skillFile)
	if err != nil {
		return fmt.Errorf("read %s: %w", skillFile, err)
	}
	text := string(content)
	frontmatter, err := extractFrontmatter(skillFile, text)
	if err != nil {
		return err
	}
	metadata, err := parseSimpleFrontmatter(skillFile, frontmatter)
	if err != nil {
		return err
	}
	if err := validateMetadata(skillDir, skillFile, metadata); err != nil {
		return err
	}
	if err := validateMarkdownLinks(skillDir, skillFile, text); err != nil {
		return err
	}
	if err := validateOpenPlannerGuidance(skillFile, text); err != nil {
		return err
	}
	return nil
}

func extractFrontmatter(skillFile string, content string) (string, error) {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSuffix(lines[0], "\r") != "---" {
		return "", fmt.Errorf("%s must start with YAML frontmatter delimited by ---", skillFile)
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSuffix(lines[i], "\r") == "---" {
			if i == 1 {
				return "", fmt.Errorf("%s frontmatter must contain required fields", skillFile)
			}
			return strings.Join(lines[1:i], "\n"), nil
		}
	}
	return "", fmt.Errorf("%s must include a closing --- line for YAML frontmatter", skillFile)
}

func parseSimpleFrontmatter(skillFile string, frontmatter string) (map[string]string, error) {
	out := map[string]string{}
	for _, line := range strings.Split(frontmatter, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return nil, fmt.Errorf("%s frontmatter line %q must be key: value", skillFile, line)
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if key == "" {
			return nil, fmt.Errorf("%s frontmatter contains an empty key", skillFile)
		}
		if _, exists := out[key]; exists {
			return nil, fmt.Errorf("%s frontmatter field %q must not be duplicated", skillFile, key)
		}
		out[key] = value
	}
	return out, nil
}

func validateMetadata(skillDir string, skillFile string, metadata map[string]string) error {
	name := metadata["name"]
	if name == "" {
		return fmt.Errorf("%s frontmatter must define a non-empty name", skillFile)
	}
	parentDir := filepath.Base(skillDir)
	if name != parentDir {
		return fmt.Errorf("%s name must match the parent directory (%q)", skillFile, parentDir)
	}
	if len([]rune(name)) > 64 || !skillNamePattern.MatchString(name) {
		return fmt.Errorf("%s name must use lowercase letters, numbers, and single hyphens only", skillFile)
	}

	description := metadata["description"]
	if description == "" {
		return fmt.Errorf("%s frontmatter must define a non-empty description", skillFile)
	}
	if len([]rune(description)) > 1024 {
		return fmt.Errorf("%s description must be 1024 characters or fewer", skillFile)
	}
	return nil
}

func validateMarkdownLinks(skillDir string, skillFile string, content string) error {
	for _, match := range markdownLinkPattern.FindAllStringSubmatch(content, -1) {
		target := match[1]
		if shouldSkipLinkTarget(target) {
			continue
		}
		targetPath := filepath.Clean(filepath.Join(skillDir, target))
		if _, err := os.Stat(targetPath); err != nil {
			return fmt.Errorf("%s link target %q is not installed with the skill: %w", skillFile, target, err)
		}
	}
	return nil
}

func shouldSkipLinkTarget(target string) bool {
	if target == "" || strings.HasPrefix(target, "#") || filepath.IsAbs(target) {
		return true
	}
	if parsed, err := url.Parse(target); err == nil && parsed.Scheme != "" {
		return true
	}
	return false
}

func validateOpenPlannerGuidance(skillFile string, content string) error {
	required := []string{
		"openplanner planning",
		"calendar_name",
		"2026/04/16",
		"Agenda results are already",
	}
	for _, want := range required {
		if !strings.Contains(content, want) {
			return fmt.Errorf("%s missing required guidance %q", skillFile, want)
		}
	}

	forbidden := []string{
		"go run ./cmd/openplanner",
		"openplanner-agentops",
		"AgentOps",
		"Generated Client Fallback",
		"sdk/generated",
		"openapi",
		"OpenAPI",
		"temporary Go module",
		"sqlite3",
		"SELECT ",
		".agents/skills",
		".claude/skills",
		".openclaw/skills",
		"hermes skills install",
	}
	for _, bad := range forbidden {
		if strings.Contains(content, bad) {
			return fmt.Errorf("%s contains stale or agent-specific guidance %q", skillFile, bad)
		}
	}
	return nil
}
