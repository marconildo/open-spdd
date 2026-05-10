package templates

import (
	"io/fs"
	"os"
	"path/filepath"

	"github.com/gszhangwei/open-spdd/internal"
	"github.com/gszhangwei/open-spdd/internal/detector"
)

type CopilotInstructionFileStrategy struct {
	manager *EmbeddedTemplateManager
}

func init() {
	RegisterStrategy(detector.GitHubCopilot, func(mgr *EmbeddedTemplateManager) GenerationStrategy {
		return &CopilotInstructionFileStrategy{manager: mgr}
	})
}

func (s *CopilotInstructionFileStrategy) GenerateAll(workingDir string, force bool) []GenerateResult {
	var results []GenerateResult

	githubDir := filepath.Join(workingDir, ".github")
	if err := os.MkdirAll(githubDir, 0755); err != nil {
		return append(results, GenerateResult{
			Success: false,
			Message: "failed to create .github directory: " + err.Error(),
			Error:   err,
		})
	}

	instructionContent, err := fs.ReadFile(embeddedTemplates, "data/tools/copilot/copilot-instructions.md")
	if err != nil {
		return append(results, GenerateResult{
			Success: false,
			Message: "failed to read copilot-instructions template: " + err.Error(),
			Error:   err,
		})
	}

	instructionPath := filepath.Join(githubDir, "copilot-instructions.md")
	markedContent := SPDDMarkerStart + "\n" + string(instructionContent) + "\n" + SPDDMarkerEnd
	results = append(results, s.manager.writeInstructionFile(instructionPath, markedContent, force))

	coreTemplates, err := s.manager.ListCore()
	if err != nil {
		return append(results, GenerateResult{
			Success: false,
			Message: "failed to list core templates: " + err.Error(),
			Error:   err,
		})
	}

	for _, tmpl := range coreTemplates {
		results = append(results, s.GenerateOne(workingDir, tmpl, force)...)
	}

	return results
}

func (s *CopilotInstructionFileStrategy) GenerateOne(workingDir string, tmpl TemplateMeta, force bool) []GenerateResult {
	promptsDir := filepath.Join(workingDir, ".github", "copilot-prompts")
	if err := os.MkdirAll(promptsDir, 0755); err != nil {
		return []GenerateResult{{
			Success: false,
			Message: "failed to create copilot-prompts directory: " + err.Error(),
			Error:   err,
		}}
	}

	targetPath := filepath.Join(promptsDir, tmpl.ID+".md")
	if _, err := os.Stat(targetPath); err == nil && !force {
		return []GenerateResult{{
			Success:  false,
			FilePath: targetPath,
			Message:  "file already exists (use --force to overwrite)",
			Error:    internal.ErrFileExists,
		}}
	}

	if err := os.WriteFile(targetPath, []byte(tmpl.Content), 0644); err != nil {
		return []GenerateResult{{
			Success:  false,
			FilePath: targetPath,
			Message:  "failed to write file: " + err.Error(),
			Error:    err,
		}}
	}

	return []GenerateResult{{
		Success:  true,
		FilePath: targetPath,
		Message:  "template generated successfully",
	}}
}
