package templates_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gszhangwei/open-spdd/internal"
	"github.com/gszhangwei/open-spdd/internal/detector"
	"github.com/gszhangwei/open-spdd/internal/templates"
)

func TestCodexSkillStrategy_GeneratesAllSkillDirectories(t *testing.T) {
	tempDir := t.TempDir()
	mgr := templates.NewEmbeddedTemplateManager()

	results := templates.StrategyFor(detector.Codex, mgr).GenerateAll(tempDir, false)

	if len(results) == 0 {
		t.Fatal("GenerateAll() returned no results")
	}

	core, err := mgr.ListCore()
	if err != nil {
		t.Fatalf("ListCore() error = %v", err)
	}
	if len(core) == 0 {
		t.Fatal("ListCore() returned 0 templates; cannot validate")
	}

	for _, tmpl := range core {
		skillDir := filepath.Join(tempDir, ".agents", "skills", tmpl.ID)
		skillPath := filepath.Join(skillDir, "SKILL.md")
		yamlPath := filepath.Join(skillDir, "agents", "openai.yaml")

		if _, err := os.Stat(skillPath); err != nil {
			t.Errorf("expected SKILL.md at %s: %v", skillPath, err)
		}
		if _, err := os.Stat(yamlPath); err != nil {
			t.Errorf("expected openai.yaml at %s: %v", yamlPath, err)
		}
	}

	for _, r := range results {
		if !r.Success {
			t.Errorf("result for %s failed: %s", r.FilePath, r.Message)
		}
	}
}

func TestCodexSkillStrategy_SkillMdFrontmatter(t *testing.T) {
	tempDir := t.TempDir()
	mgr := templates.NewEmbeddedTemplateManager()
	templates.StrategyFor(detector.Codex, mgr).GenerateAll(tempDir, false)

	core, err := mgr.ListCore()
	if err != nil {
		t.Fatalf("ListCore() error = %v", err)
	}

	for _, tmpl := range core {
		skillPath := filepath.Join(tempDir, ".agents", "skills", tmpl.ID, "SKILL.md")
		raw, err := os.ReadFile(skillPath)
		if err != nil {
			t.Fatalf("read %s: %v", skillPath, err)
		}
		content := string(raw)

		if !strings.HasPrefix(content, "---\n") {
			t.Errorf("%s: SKILL.md does not start with '---\\n'", tmpl.ID)
		}

		parts := strings.SplitN(content, "---", 3)
		if len(parts) < 3 {
			t.Fatalf("%s: SKILL.md missing closing frontmatter delimiter", tmpl.ID)
		}
		frontmatter := parts[1]
		body := parts[2]

		if !strings.Contains(frontmatter, "name:") {
			t.Errorf("%s: SKILL.md frontmatter missing 'name:'", tmpl.ID)
		}
		if !strings.Contains(frontmatter, "description:") {
			t.Errorf("%s: SKILL.md frontmatter missing 'description:'", tmpl.ID)
		}
		if strings.Contains(frontmatter, "id:") {
			t.Errorf("%s: SKILL.md frontmatter should not contain 'id:'", tmpl.ID)
		}
		if strings.Contains(frontmatter, "category:") {
			t.Errorf("%s: SKILL.md frontmatter should not contain 'category:'", tmpl.ID)
		}

		for _, line := range strings.Split(frontmatter, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "name:") {
				value := strings.TrimSpace(strings.TrimPrefix(line, "name:"))
				value = strings.Trim(value, `"`)
				if strings.HasPrefix(value, "/") {
					t.Errorf("%s: SKILL.md name has leading '/': %q", tmpl.ID, value)
				}
			}
		}

		if strings.TrimSpace(body) == "" {
			t.Errorf("%s: SKILL.md body is empty", tmpl.ID)
		}
	}
}

func TestCodexSkillStrategy_OpenAIYamlExplicitOnly(t *testing.T) {
	resetAllowImplicit(t)
	templates.AllowImplicitInvocation = false

	tempDir := t.TempDir()
	mgr := templates.NewEmbeddedTemplateManager()
	templates.StrategyFor(detector.Codex, mgr).GenerateAll(tempDir, false)

	core, err := mgr.ListCore()
	if err != nil {
		t.Fatalf("ListCore() error = %v", err)
	}

	want := "policy:\n  allow_implicit_invocation: false\n"
	for _, tmpl := range core {
		yamlPath := filepath.Join(tempDir, ".agents", "skills", tmpl.ID, "agents", "openai.yaml")
		raw, err := os.ReadFile(yamlPath)
		if err != nil {
			t.Fatalf("read %s: %v", yamlPath, err)
		}
		if string(raw) != want {
			t.Errorf("%s: openai.yaml = %q, want %q", tmpl.ID, string(raw), want)
		}
	}
}

func TestCodexSkillStrategy_OpenAIYamlAllowImplicit(t *testing.T) {
	resetAllowImplicit(t)
	templates.AllowImplicitInvocation = true

	tempDir := t.TempDir()
	mgr := templates.NewEmbeddedTemplateManager()
	templates.StrategyFor(detector.Codex, mgr).GenerateAll(tempDir, false)

	core, err := mgr.ListCore()
	if err != nil {
		t.Fatalf("ListCore() error = %v", err)
	}

	for _, tmpl := range core {
		yamlPath := filepath.Join(tempDir, ".agents", "skills", tmpl.ID, "agents", "openai.yaml")
		raw, err := os.ReadFile(yamlPath)
		if err != nil {
			t.Fatalf("read %s: %v", yamlPath, err)
		}
		if !strings.Contains(string(raw), "allow_implicit_invocation: true") {
			t.Errorf("%s: openai.yaml = %q, want it to contain 'allow_implicit_invocation: true'", tmpl.ID, string(raw))
		}
	}
}

func TestCodexSkillStrategy_SkipExisting(t *testing.T) {
	resetAllowImplicit(t)

	tempDir := t.TempDir()
	mgr := templates.NewEmbeddedTemplateManager()

	core, err := mgr.ListCore()
	if err != nil {
		t.Fatalf("ListCore() error = %v", err)
	}
	if len(core) == 0 {
		t.Fatal("no core templates")
	}

	preID := core[0].ID
	preDir := filepath.Join(tempDir, ".agents", "skills", preID)
	if err := os.MkdirAll(preDir, 0755); err != nil {
		t.Fatal(err)
	}
	prePath := filepath.Join(preDir, "SKILL.md")
	preContent := []byte("# pre-existing\n")
	if err := os.WriteFile(prePath, preContent, 0644); err != nil {
		t.Fatal(err)
	}

	results := templates.StrategyFor(detector.Codex, mgr).GenerateAll(tempDir, false)

	var skipped bool
	for _, r := range results {
		if r.FilePath == prePath {
			if r.Success {
				t.Errorf("expected pre-existing SKILL.md result to be a failure")
			}
			if !errors.Is(r.Error, internal.ErrFileExists) {
				t.Errorf("expected ErrFileExists, got %v", r.Error)
			}
			skipped = true
		}
	}
	if !skipped {
		t.Errorf("did not find a result for pre-existing path %s", prePath)
	}

	got, err := os.ReadFile(prePath)
	if err != nil {
		t.Fatalf("read %s: %v", prePath, err)
	}
	if string(got) != string(preContent) {
		t.Errorf("pre-existing SKILL.md was modified without --force")
	}
}

func TestCodexSkillStrategy_ForceOverwrites(t *testing.T) {
	resetAllowImplicit(t)

	tempDir := t.TempDir()
	mgr := templates.NewEmbeddedTemplateManager()

	core, err := mgr.ListCore()
	if err != nil {
		t.Fatalf("ListCore() error = %v", err)
	}
	if len(core) == 0 {
		t.Fatal("no core templates")
	}

	preID := core[0].ID
	preDir := filepath.Join(tempDir, ".agents", "skills", preID)
	if err := os.MkdirAll(preDir, 0755); err != nil {
		t.Fatal(err)
	}
	prePath := filepath.Join(preDir, "SKILL.md")
	preContent := []byte("# pre-existing\n")
	if err := os.WriteFile(prePath, preContent, 0644); err != nil {
		t.Fatal(err)
	}

	results := templates.StrategyFor(detector.Codex, mgr).GenerateAll(tempDir, true)

	for _, r := range results {
		if !r.Success {
			t.Errorf("result for %s failed under --force: %s", r.FilePath, r.Message)
		}
	}

	got, err := os.ReadFile(prePath)
	if err != nil {
		t.Fatalf("read %s: %v", prePath, err)
	}
	if string(got) == string(preContent) {
		t.Errorf("--force did not overwrite pre-existing SKILL.md")
	}
}

func TestCodexSkillStrategy_FrontmatterStrippedFromBody(t *testing.T) {
	resetAllowImplicit(t)

	tempDir := t.TempDir()
	mgr := templates.NewEmbeddedTemplateManager()
	templates.StrategyFor(detector.Codex, mgr).GenerateAll(tempDir, false)

	core, err := mgr.ListCore()
	if err != nil {
		t.Fatalf("ListCore() error = %v", err)
	}

	for _, tmpl := range core {
		skillPath := filepath.Join(tempDir, ".agents", "skills", tmpl.ID, "SKILL.md")
		raw, err := os.ReadFile(skillPath)
		if err != nil {
			t.Fatalf("read %s: %v", skillPath, err)
		}
		content := string(raw)

		parts := strings.SplitN(content, "---", 3)
		if len(parts) < 3 {
			t.Fatalf("%s: SKILL.md missing closing frontmatter delimiter", tmpl.ID)
		}
		body := strings.TrimLeft(parts[2], "\n")

		if strings.HasPrefix(body, "---") {
			t.Errorf("%s: body unexpectedly starts with '---' (original frontmatter not stripped)", tmpl.ID)
		}
		if strings.HasPrefix(body, "name: /") {
			t.Errorf("%s: body unexpectedly starts with 'name: /' (original frontmatter leaked)", tmpl.ID)
		}
	}
}

func TestCodexSkillStrategy_GenerateOne_ProducesSkillBundle(t *testing.T) {
	resetAllowImplicit(t)

	tempDir := t.TempDir()
	mgr := templates.NewEmbeddedTemplateManager()

	tmpl, err := mgr.GetByName("/spdd-analysis")
	if err != nil {
		t.Fatalf("GetByName(/spdd-analysis) error = %v", err)
	}

	results := templates.StrategyFor(detector.Codex, mgr).GenerateOne(tempDir, tmpl, false)

	if len(results) != 2 {
		t.Fatalf("GenerateOne() returned %d results, want 2", len(results))
	}
	for _, r := range results {
		if !r.Success {
			t.Errorf("result for %s failed: %s", r.FilePath, r.Message)
		}
	}

	skillPath := filepath.Join(tempDir, ".agents", "skills", tmpl.ID, "SKILL.md")
	yamlPath := filepath.Join(tempDir, ".agents", "skills", tmpl.ID, "agents", "openai.yaml")
	if _, err := os.Stat(skillPath); err != nil {
		t.Errorf("expected SKILL.md at %s: %v", skillPath, err)
	}
	if _, err := os.Stat(yamlPath); err != nil {
		t.Errorf("expected openai.yaml at %s: %v", yamlPath, err)
	}

	flatPath := filepath.Join(tempDir, ".agents", "skills", tmpl.ID+".md")
	if _, err := os.Stat(flatPath); err == nil {
		t.Errorf("unexpected flat .md file at %s — should be a skill bundle, not a flat command", flatPath)
	}
}

func TestCodexSkillStrategy_GenerateOne_OptionalTemplateIsSkill(t *testing.T) {
	resetAllowImplicit(t)

	tempDir := t.TempDir()
	mgr := templates.NewEmbeddedTemplateManager()

	tmpl, err := mgr.GetByName("/spdd-reverse")
	if err != nil {
		t.Fatalf("GetByName(/spdd-reverse) error = %v", err)
	}

	results := templates.StrategyFor(detector.Codex, mgr).GenerateOne(tempDir, tmpl, false)

	if len(results) != 2 {
		t.Fatalf("GenerateOne() returned %d results, want 2", len(results))
	}
	for _, r := range results {
		if !r.Success {
			t.Errorf("result for %s failed: %s", r.FilePath, r.Message)
		}
	}

	skillPath := filepath.Join(tempDir, ".agents", "skills", "spdd-reverse", "SKILL.md")
	yamlPath := filepath.Join(tempDir, ".agents", "skills", "spdd-reverse", "agents", "openai.yaml")
	if _, err := os.Stat(skillPath); err != nil {
		t.Errorf("expected SKILL.md at %s: %v", skillPath, err)
	}
	if _, err := os.Stat(yamlPath); err != nil {
		t.Errorf("expected openai.yaml at %s: %v", yamlPath, err)
	}

	flatPath := filepath.Join(tempDir, ".agents", "skills", "spdd-reverse.md")
	if _, err := os.Stat(flatPath); err == nil {
		t.Errorf("unexpected flat .md file at %s — optional template should also be a skill bundle", flatPath)
	}
}

func resetAllowImplicit(t *testing.T) {
	t.Helper()
	prev := templates.AllowImplicitInvocation
	t.Cleanup(func() {
		templates.AllowImplicitInvocation = prev
	})
}
