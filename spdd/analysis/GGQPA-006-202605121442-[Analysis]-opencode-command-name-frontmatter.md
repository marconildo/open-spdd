# SPDD Analysis: OpenCode Command Naming Without Frontmatter Name Field

## Original Business Requirement
openspdd generates commands for the AI tool `opencode` in the way commands exists in the opnecode UI as //spdd-* but should be just /spdd-*; this happens because in generated files there is a field "name" and it is populated incorrectly (like "name: /spdd-analysis"); the right approach for the `opnecode` will be do not use the "name" field at all -- from the documentation "... The markdown file name becomes the command name. For example, test.md lets you run: /test ..."

## Domain Concept Identification

### Existing Concepts (from codebase)
- `TemplateMeta` + `ParseFrontmatter`: shared template metadata model where `name`, `id`, `category`, and `description` are parsed and retained across tools; this currently preserves slash-style names from core templates.
- `FlatMarkdownStrategy`: default generation strategy used by OpenCode that writes raw template content to `.opencode/commands/<id>.md`; it relies on filename from `id` while still carrying frontmatter content verbatim.
- `EmbeddedTemplateManager` + embedded `data/core/*.md`: central source of SPDD command content, with core templates currently including `name: /spdd-*` frontmatter.
- `AIToolType.OpenCode`: OpenCode is already a first-class tool, mapped to `.opencode/commands` and selected via tool detection/flags.
- Existing OpenCode command artifacts under `.opencode/commands/`: generated files currently include `name: /spdd-*`, confirming parity between embedded core frontmatter and generated OpenCode command files.

### New Concepts Required
- `OpenCode-specific frontmatter compatibility boundary`: a tool-specific policy that treats command identity as filename-only and avoids `name` frontmatter emission for OpenCode command files.
- `Frontmatter normalization mode for OpenCode flat commands`: a generation-time transformation concept that keeps reusable command body content while adapting incompatible metadata fields for OpenCode runtime behavior.

### Key Business Rules
- `Filename is command identity in OpenCode`: command invocation must be derived from markdown filename, not frontmatter `name`.
- `No slash duplication in OpenCode UI`: generated commands must appear as `/spdd-*`, never `//spdd-*`.
- `Tool-specific behavior must not regress other tools`: changes for OpenCode must preserve current behavior for Cursor/Claude/Antigravity/Copilot/Codex outputs.
- `SPDD command corpus remains reusable`: core SPDD command intent/content should stay centrally managed rather than duplicating per-tool command text.

## Strategic Approach

### Solution Direction
- Keep the current architecture (embedded core templates + per-tool generation strategies), and introduce an OpenCode-specific output compatibility rule at generation time so OpenCode receives filename-driven commands without `name` frontmatter while other tools continue using existing template semantics.

### Key Design Decisions
- `Where to adapt`: adapt at generation strategy/output boundary, not by rewriting core template source files globally -> preserves shared template reuse and limits blast radius.
- `Scope of adaptation`: apply only to OpenCode command generation path -> prevents unintended behavior changes for tools that may rely on or tolerate existing frontmatter.
- `Metadata handling`: remove or neutralize `name` for OpenCode artifacts while retaining useful descriptive metadata where compatible -> balances OpenCode command correctness with maintainability.
- `Validation posture`: codify expected OpenCode command shape in tests (no command-identity conflict fields, filename-derived command works) -> prevents future regressions from template/content evolution.

### Alternatives Considered
- `Edit all core templates to remove name permanently`: rejected because it forces cross-tool behavioral change and couples OpenCode-specific runtime rules to shared source content.
- `Keep current files and rely on user workaround`: rejected because it does not satisfy the requirement to produce correct `/spdd-*` commands automatically.
- `Fork a full OpenCode-specific template set`: rejected because it duplicates command content and increases long-term drift/maintenance risk.

## Risk & Gap Analysis

### Requirement Ambiguities
- `Exact failure mechanism in OpenCode`: requirement observes `//spdd-*` behavior but does not define whether OpenCode prepends slash to `name` or displays both filename/name concurrently; this affects strict formatting expectations for generated frontmatter.
- `Allowed frontmatter subset`: requirement explicitly says not to use `name`, but does not confirm whether `id`/`category` are harmless, ignored, or potentially problematic in OpenCode command files.
- `Coverage scope`: unclear whether fix is required only for core SPDD commands or also optional templates generated for OpenCode.

### Edge Cases
- `Existing generated files with stale name fields`: previously generated `.opencode/commands/*.md` may continue producing wrong command labels unless regenerated.
- `Mixed-tool repos`: projects generating for multiple tools need OpenCode-specific adaptation without altering files intended for other assistant ecosystems.
- `Single-template generation vs generate-all`: both command paths must enforce the same OpenCode compatibility behavior.
- `Future template additions`: newly added templates with `name` frontmatter must automatically follow OpenCode compatibility rules.

### Technical Risks
- `Cross-tool regression risk`: changing shared parsing/model behavior (`TemplateMeta.Name`, `GetByName`) can break command selection and tests; mitigation direction is strategy-local transformation rather than global parser changes.
- `Behavior drift between strategies`: OpenCode uses flat generation while Codex uses skill transformation; inconsistent metadata handling across strategies can reintroduce bugs; mitigation direction is explicit per-strategy contract tests.
- `Backward compatibility with list/get semantics`: `GetByName("/spdd-analysis")` is used in tests and UX flows; removing source-level `name` fields globally could impact lookup expectations.

### Acceptance Criteria Coverage
| AC# | Description | Addressable? | Gaps/Notes |
|-----|-------------|--------------|------------|
| 1 | OpenCode generated commands appear as `/spdd-*`, not `//spdd-*` | Yes | Requires OpenCode output to avoid conflicting `name` semantics. |
| 2 | OpenCode command identity is filename-driven per OpenCode docs | Yes | Existing filename generation (`<id>.md`) already aligns. |
| 3 | Generated OpenCode command files do not rely on `name` frontmatter | Yes | Requires OpenCode-specific frontmatter adaptation. |
| 4 | Existing non-OpenCode tool outputs remain unchanged | Partial | Must be protected through scoped strategy changes and regression tests. |
| 5 | Behavior is consistent for both single-template and generate-all flows | Yes | Both flows already converge through strategy abstraction; compatibility rule must apply in both paths. |
| 6 | Optional templates generated for OpenCode follow same command naming rule | Partial | Requirement does not explicitly mention optional templates; recommended to include for consistency. |
