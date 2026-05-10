package templates

import (
	"github.com/gszhangwei/open-spdd/internal/detector"
)

type GenerationStrategy interface {
	GenerateAll(workingDir string, force bool) []GenerateResult
	GenerateOne(workingDir string, tmpl TemplateMeta, force bool) []GenerateResult
}

type strategyFactory func(mgr *EmbeddedTemplateManager) GenerationStrategy

var strategyRegistry = map[detector.AIToolType]strategyFactory{}

func RegisterStrategy(tool detector.AIToolType, factory strategyFactory) {
	strategyRegistry[tool] = factory
}

func StrategyFor(tool detector.AIToolType, mgr *EmbeddedTemplateManager) GenerationStrategy {
	if factory, ok := strategyRegistry[tool]; ok {
		return factory(mgr)
	}
	return &FlatMarkdownStrategy{tool: tool, manager: mgr}
}

var AllowImplicitInvocation bool

func RegisteredToolsForTest() []detector.AIToolType {
	tools := make([]detector.AIToolType, 0, len(strategyRegistry))
	for tool := range strategyRegistry {
		tools = append(tools, tool)
	}
	return tools
}
