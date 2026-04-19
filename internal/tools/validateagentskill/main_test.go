package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateSkillDirAcceptsProductionSkill(t *testing.T) {
	if err := validateSkillDir("../../../skills/openplanner"); err != nil {
		t.Fatalf("validate production skill: %v", err)
	}
}

func TestValidateSkillDirRejectsExtraFiles(t *testing.T) {
	skillDir := writeSkill(t, validSkillMarkdown())
	if err := os.Mkdir(filepath.Join(skillDir, "references"), 0o755); err != nil {
		t.Fatalf("mkdir references: %v", err)
	}

	err := validateSkillDir(skillDir)
	if err == nil || !strings.Contains(err.Error(), "must contain only SKILL.md") {
		t.Fatalf("error = %v, want only SKILL.md error", err)
	}
}

func TestValidateSkillDirRejectsStaleGuidance(t *testing.T) {
	cases := []string{
		"go run ./cmd/openplanner planning",
		"openplanner-agentops",
		"AgentOps",
		"sdk/generated",
		"OpenAPI",
		".claude/skills/openplanner",
	}
	for _, bad := range cases {
		t.Run(bad, func(t *testing.T) {
			skillDir := writeSkill(t, validSkillMarkdown()+"\n"+bad+"\n")
			err := validateSkillDir(skillDir)
			if err == nil || !strings.Contains(err.Error(), "stale or agent-specific") {
				t.Fatalf("error = %v, want stale guidance error", err)
			}
		})
	}
}

func writeSkill(t *testing.T, content string) string {
	t.Helper()

	root := t.TempDir()
	skillDir := filepath.Join(root, "openplanner")
	if err := os.Mkdir(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	return skillDir
}

func validSkillMarkdown() string {
	return `---
name: openplanner
description: Manage local calendar and task workflows through OpenPlanner's installed JSON runner.
---

Run openplanner planning.
Use calendar_name.
Reject 2026/04/16.
Agenda results are already chronologically ordered.
`
}
