package templates

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/gszhangwei/open-spdd/internal"
	"github.com/gszhangwei/open-spdd/internal/detector"
)

type CodexSkillStrategy struct {
	manager *EmbeddedTemplateManager
}

func init() {
	RegisterStrategy(detector.Codex, func(mgr *EmbeddedTemplateManager) GenerationStrategy {
		return &CodexSkillStrategy{manager: mgr}
	})
}

func (s *CodexSkillStrategy) GenerateAll(workingDir string, force bool) []GenerateResult {
	tmpls, err := s.manager.ListAvailable(detector.Codex)
	if err != nil {
		return []GenerateResult{{
			Success: false,
			Message: "failed to list templates: " + err.Error(),
			Error:   err,
		}}
	}

	results := make([]GenerateResult, 0, len(tmpls)*2)
	for _, tmpl := range tmpls {
		results = append(results, s.GenerateOne(workingDir, tmpl, force)...)
	}
	return results
}

func (s *CodexSkillStrategy) GenerateOne(workingDir string, tmpl TemplateMeta, force bool) []GenerateResult {
	baseDir := filepath.Join(workingDir, ".agents", "skills")
	skillDir := filepath.Join(baseDir, tmpl.ID)
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		return []GenerateResult{{
			Success:  false,
			FilePath: skillDir,
			Message:  "failed to create skill directory: " + err.Error(),
			Error:    err,
		}}
	}

	results := make([]GenerateResult, 0, 2)

	skillPath := filepath.Join(skillDir, "SKILL.md")
	results = append(results, writeSkillFile(skillPath, buildSkillMd(tmpl), force))

	openaiYamlDir := filepath.Join(skillDir, "agents")
	if err := os.MkdirAll(openaiYamlDir, 0755); err != nil {
		return append(results, GenerateResult{
			Success:  false,
			FilePath: openaiYamlDir,
			Message:  "failed to create agents directory: " + err.Error(),
			Error:    err,
		})
	}

	openaiYamlPath := filepath.Join(openaiYamlDir, "openai.yaml")
	return append(results, writeSkillFile(openaiYamlPath, buildOpenAIYaml(AllowImplicitInvocation), force))
}

func writeSkillFile(path string, content []byte, force bool) GenerateResult {
	if _, err := os.Stat(path); err == nil && !force {
		return GenerateResult{
			Success:  false,
			FilePath: path,
			Message:  "file already exists (use --force to overwrite)",
			Error:    internal.ErrFileExists,
		}
	}

	if err := os.WriteFile(path, content, 0644); err != nil {
		return GenerateResult{
			Success:  false,
			FilePath: path,
			Message:  "failed to write file: " + err.Error(),
			Error:    err,
		}
	}

	return GenerateResult{
		Success:  true,
		FilePath: path,
		Message:  "template generated successfully",
	}
}

func buildSkillMd(meta TemplateMeta) []byte {
	skillName := strings.TrimPrefix(meta.Name, "/")
	if skillName == "" {
		skillName = meta.ID
	}

	description := strings.TrimSpace(meta.Description)
	if description == "" {
		description = "SPDD command — see SKILL body for details"
	}

	body := stripFrontmatter(meta.Content)

	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("name: ")
	b.WriteString(escapeYAMLScalar(skillName))
	b.WriteString("\n")
	b.WriteString("description: ")
	b.WriteString(escapeYAMLScalar(description))
	b.WriteString("\n")
	b.WriteString("---\n\n")
	b.WriteString(body)

	return []byte(b.String())
}

func buildOpenAIYaml(allowImplicit bool) []byte {
	value := "false"
	if allowImplicit {
		value = "true"
	}
	return []byte("policy:\n  allow_implicit_invocation: " + value + "\n")
}

func stripFrontmatter(content string) string {
	if !strings.HasPrefix(content, "---") {
		return content
	}
	parts := strings.SplitN(content, "---", 3)
	if len(parts) < 3 {
		return content
	}
	body := parts[2]
	return strings.TrimLeft(body, "\n")
}

func escapeYAMLScalar(s string) string {
	if s == "" {
		return `""`
	}
	if strings.ContainsAny(s, ":#\"") || s != strings.TrimLeft(s, " \t") {
		escaped := strings.ReplaceAll(s, `"`, `\"`)
		return `"` + escaped + `"`
	}
	return s
}
