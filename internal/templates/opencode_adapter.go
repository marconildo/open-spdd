package templates

import (
	"strings"

	"github.com/gszhangwei/open-spdd/internal/detector"
)

type OpenCodeTemplateAdapter struct{}

func (a OpenCodeTemplateAdapter) IsApplicable(tool detector.AIToolType) bool {
	return tool == detector.OpenCode
}

func (a OpenCodeTemplateAdapter) NormalizeForOpenCode(tmpl TemplateMeta) string {
	return a.StripFrontmatterName(tmpl.Content)
}

func (a OpenCodeTemplateAdapter) StripFrontmatterName(content string) string {
	if !strings.HasPrefix(content, "---") {
		return content
	}

	parts := strings.SplitN(content, "---", 3)
	if len(parts) < 3 {
		return content
	}

	lines := strings.Split(parts[1], "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "name:") {
			continue
		}
		filtered = append(filtered, line)
	}

	return "---" + strings.Join(filtered, "\n") + "---" + parts[2]
}
