# SPDD Analysis: Add OpenCode Support to openspdd CLI

## Original Business Requirement

Support `opencode` as a new AI coding tool target. Generated SPDD command templates must be installed under the project-local `.opencode/commands/` directory.

## Domain Concept Identification

### Existing Concepts (from codebase)

- **`AIToolType` (closed enum)** — defined in `internal/detector/types.go` as a typed string with constants `Cursor`, `ClaudeCode`, `Antigravity`, `GitHubCopilot`, `Unknown`. It is the single point of truth that drives every other tool-aware decision: human-readable name (`String()`), per-tool config directory (`GetConfigDir()`), per-tool signature files (`GetSignatureFiles()`), per-tool instruction-file behavior (`GetInstructionFile()` / `HasInstructionFile()`), and the per-tool embedded-template subdirectory (`GetToolDirName()`). Adding a new tool means adding a new constant and extending every per-method switch.
- **`DefaultDetector.Detect()`** — `internal/detector/detector.go` iterates a hard-coded slice `toolTypes := []AIToolType{Cursor, ClaudeCode, Antigravity, GitHubCopilot}` and stops at the first signature match. Order matters: it determines disambiguation when multiple toolings coexist in the same project. New tools must be registered in this slice.
- **Embedded template store** — `internal/templates/data/` (embedded via `internal/templates/embed.go`) with three categories:
  - `core/` — workflow templates (`spdd-analysis.md`, `spdd-reasons-canvas.md`, `spdd-generate.md`, `spdd-sync.md`, `spdd-prompt-update.md`) that get installed for every supported tool.
  - `tools/<tool-dir-name>/` — tool-specific assets (only `copilot/copilot-instructions.md` is non-empty today; the `cursor/`, `claude-code/`, and `antigravity/` directories exist but are empty, indicating a "no extra assets" branch in the model).
  - `optional/` — opt-in templates (`spdd-story.md`, `spdd-api-test.md`, `spdd-code-review.md`, `spdd-reverse.md`, `aupro-context.md`).
- **`EmbeddedTemplateManager`** — `internal/templates/manager.go` resolves which templates apply for a given tool. Two places enumerate the supported tools explicitly:
  - `ListForTool(tool)` reads `data/tools/<GetToolDirName()>/`.
  - `ListAll()` has a hard-coded `knownTools := []detector.AIToolType{Cursor, ClaudeCode, Antigravity, GitHubCopilot}` slice that must include every tool type or its tool-specific templates will be silently absent from the "list all" output.
  - `GenerateForCopilot(...)` is a Copilot-specific code path that materializes the instruction file with marker-based merge support; it implements the `HasInstructionFile() == true` branch and is invoked only when the active tool is GitHub Copilot.
- **CLI flag parsing (`ParseToolFlag`)** — `cmd/root.go` translates `--tool` flag strings (with aliases like `claude`, `copilot`, `gh-copilot`) into `AIToolType` values. New tools need at least one canonical flag string and may want short aliases.
- **Interactive tool picker (`selectToolInteractively`)** — `cmd/init.go` shows a `huh` selector with one option per tool. The list is hard-coded with four entries. When users have no signature files yet, this is the only way the CLI surfaces tool choice.
- **`init` and `generate` subcommands** — `cmd/init.go` creates the tool's config directory (and a side directory for the Copilot instruction-file branch); `cmd/generate.go` writes templates into `detectedResult.ConfigPath`. Both rely entirely on `AIToolType` methods — no new subcommand wiring is needed for a new tool, only enum extension.
- **Markdown frontmatter** — every embedded template starts with a YAML-style block (`name`, `id`, `category`, `description`) parsed by `ParseFrontmatter` in `internal/templates/types.go`. The parser is forgiving: unknown keys are silently ignored, so templates can carry tool-specific frontmatter keys without breaking other tools.
- **Test layer** — `tests/detector/types_test.go` uses a closed enumeration of cases per `AIToolType` method; missing a new constant in these tables leaves the new tool's behavior untested. `tests/detector/detector_test.go` similarly covers detection paths.
- **Documentation surface** — `README.md` and `README.zh-CN.md` advertise the supported tool list in three places: the bullet under "Cross-platform" (line 86), the `--tool` flag examples (lines 243–245), and the detection/output table (lines 252–255). Each must be updated for parity.

### New Concepts Required

- **`OpenCode` AI tool type** — a new value in the `AIToolType` enum representing the `opencode` CLI tool from `opencode.ai`. Treats per-project commands as markdown files under `.opencode/commands/<command-name>.md`, where the filename (sans `.md`) is the slash-command name visible in OpenCode's TUI.
- **OpenCode signature surface** — the artifacts whose presence indicates an OpenCode-managed project. Per OpenCode's docs, the canonical project markers are the `.opencode/` directory (per-project commands and state) and an `opencode.json` config file at the project root. Either one signals an OpenCode environment.
- **OpenCode tool dir name** — the lowercase identifier used inside `internal/templates/data/tools/<name>/`; selected as `opencode` for parity with `cursor`, `claude-code`, `antigravity`, `copilot`.
- **OpenCode tool flag identifier** — the canonical `--tool opencode` value (and a possible `oc` short alias) that maps through `ParseToolFlag` into the new enum constant.

### Conceptual Relationships

- `OpenCode` (new enum value) → consumed by → `Detect()` (registration in `toolTypes`), `init`/`generate` flows (via `ConfigPath`), `EmbeddedTemplateManager.ListAll()` (registration in `knownTools`), `ParseToolFlag` (canonical string → enum), and `selectToolInteractively` (option list entry).
- `OpenCode` → maps to → `.opencode/commands` (config directory) and `opencode` (tool dir name).
- `OpenCode` signature surface → checked by → `Detect()`; first match wins, so signature ordering with respect to other tools matters.
- OpenCode does **not** introduce a new `HasInstructionFile() == true` branch: like Cursor, Claude Code, and Antigravity, it stores commands as a flat directory of markdown files; no separate "instruction file with markers" concept is needed (in contrast to GitHub Copilot's `copilot-instructions.md`).
- The existing core template content (frontmatter `name`, `id`, `category`, `description` plus body) is the same artifact OpenCode loads as a slash command — the filename becomes the command name and OpenCode reads its own subset of the frontmatter (e.g., `description`). No content rewrite is required; the existing core templates are reusable as-is.

### Key Business Rules

- **Single source of truth for tool support is `AIToolType`.** Any code path that branches on tool identity must do so through `AIToolType` methods or explicit constant comparisons. Adding OpenCode means extending every existing switch/list that enumerates supported tools — leaving a switch unupdated produces silent partial support (e.g., generate works but `list --all` hides the new templates).
- **Detection is read-only and non-destructive.** `Detect()` only inspects the working directory for signature files; OpenCode detection MUST follow this contract — no writes, no probing of files outside the working dir.
- **Generated commands MUST land under `.opencode/commands/`.** This is the explicit user requirement and matches the OpenCode upstream contract (per-project commands location). Any divergence (e.g., installing under `.opencode/` directly or under `.opencode/prompts/`) breaks OpenCode's own loader.
- **Existing tools and outputs MUST remain unchanged.** Adding OpenCode is additive: detection order, existing config paths, existing template lists for other tools, and existing CLI flag values must continue to behave identically.
- **CLI surface MUST be discoverable.** The new tool appears in `--tool` accepted values, in the interactive `selectToolInteractively` picker, in the README's supported-tools table, and in the supported `--tool` examples. Otherwise users cannot select OpenCode when no signature is yet present (greenfield case).

## Strategic Approach

### Solution Direction

- **Extend the closed enum, do not refactor it.** Adding OpenCode as a sixth value (`OpenCode AIToolType = "opencode"`) and extending each `switch t` block in `internal/detector/types.go` is the smallest, lowest-risk change that follows the established pattern. A "tool registry" abstraction (map-based dispatch, plugin lookup) would be cleaner long-term but is over-engineering for one new tool and would touch every tool's behavior, inflating risk surface and review burden.
- **Reuse the "no extra assets" tool branch.** OpenCode's command model — flat directory of markdown files where the filename becomes the slash-command name — exactly matches Cursor's, Claude Code's, and Antigravity's model. The existing empty `internal/templates/data/tools/<tool>/` convention applies: create an empty `internal/templates/data/tools/opencode/` directory (kept tracked via a placeholder if needed for `go:embed`), and the existing `ListForTool` machinery transparently returns zero tool-specific templates. Core templates (`spdd-*`) are the deliverable; they install verbatim.
- **Treat OpenCode like Cursor/Antigravity at the detector level.** Match on the project-local marker (`.opencode/` directory) and on `opencode.json` (the documented config file). Append OpenCode to the `toolTypes` slice in `Detect()` after the existing four; this preserves disambiguation behavior for all currently-detected projects (existing detections cannot regress because OpenCode is checked last).
- **Mirror the documentation in three places to keep the user-facing story consistent.** Update both READMEs' bullet, table, and `--tool` example blocks; update CLI long-help strings in `cmd/root.go` if they enumerate tools (today: line 28 of `cmd/root.go`).
- **Cover the new enum value with unit tests in the existing table-driven style.** `tests/detector/types_test.go` already drives every method off a struct slice; adding one row per test for OpenCode locks the contract in.

### Key Design Decisions

- **OpenCode signature files (what counts as "OpenCode is in use")**
  - _Trade-offs_: (A) Match only `.opencode/` — the most direct project marker, mirrors how `Antigravity` is detected. Risk: a brand-new OpenCode project that has only `opencode.json` (config defined but no commands yet) won't be detected. (B) Match `.opencode/` _or_ `opencode.json` — covers both the "has commands" and the "config-only" cases. Risk: marginally more code; trivial extra fragility if upstream renames the config file. (C) Match a longer list including `AGENTS.md` — too aggressive; `AGENTS.md` is a generic convention that may be present for other agentic tools.
  - _Recommendation_: Option B. Signature list `[".opencode", "opencode.json"]`. Aligns with the upstream docs (which document `opencode.json` as the canonical config) and matches the dual-signature pattern already used for Cursor (`.cursor`, `.cursorrules`) and Claude Code (`.claude`, `CLAUDE.md`).
- **Detection order in `Detect()`**
  - _Trade-offs_: Insert OpenCode at the head, in the middle, or at the tail. Anywhere except the tail risks changing first-match outcomes for projects that today match a tool earlier in the list and ALSO have an OpenCode marker (theoretically possible, e.g., a Cursor project that adopts OpenCode as a secondary tooling).
  - _Recommendation_: Append at the tail (`Cursor, ClaudeCode, Antigravity, GitHubCopilot, OpenCode`). Guarantees zero regression for existing detections. Users who deliberately want OpenCode in a multi-tool project can still select it explicitly via `--tool opencode`.
- **Tool flag canonical name and aliases (`ParseToolFlag`)**
  - _Trade-offs_: (A) Only `opencode` — minimal, unambiguous. (B) `opencode` plus `oc` short alias — convenient but the two-letter token risks colliding with future tools (e.g., a hypothetical "OpenContext"). (C) `opencode` plus `open-code` — covers a plausible misspelling but adds maintenance.
  - _Recommendation_: Start with Option A (`opencode` only). If users request a shortcut later, adding aliases is a one-line, non-breaking change. Avoids premature alias-namespace consumption.
- **Tool directory name under `internal/templates/data/tools/`**
  - _Trade-offs_: `opencode` versus `open-code`. The other tool dirs use lowercase, hyphen-separated names that match a human-readable convention (`claude-code`). OpenCode is conventionally written as a single word in its own docs.
  - _Recommendation_: `opencode` (single word). Matches the canonical brand spelling and the `--tool` flag value. Avoids artificial divergence.
- **Whether to keep the `internal/templates/data/tools/opencode/` directory empty or seed it with an OpenCode-specific helper**
  - _Trade-offs_: Empty directory matches the precedent for tools without bespoke assets (Cursor, Claude Code, Antigravity). Seeding (e.g., an `AGENTS.md` template) would over-shoot the requirement and create extra surface to maintain. The existing core templates already render correctly under `.opencode/commands/` because OpenCode reads markdown files with descriptive frontmatter — exactly the format the core templates already use.
  - _Recommendation_: Empty (no bespoke assets). The requirement scope is "install commands under `.opencode/commands`", which is satisfied entirely by reusing core templates.
- **`HasInstructionFile()` value for OpenCode**
  - _Trade-offs_: OpenCode supports a project-level `AGENTS.md` for persistent agent instructions (similar to Claude Code's `CLAUDE.md`). However, the requirement is specifically about commands under `.opencode/commands`, not about an instructions file. Adding `HasInstructionFile() == true` would force a Copilot-style code path (`GenerateForOpenCode`) that the requirement does not ask for and that the existing `EmbeddedTemplateManager` is not currently designed to abstract.
  - _Recommendation_: `HasInstructionFile() == false` for OpenCode (same as Cursor, Claude Code, Antigravity). If `AGENTS.md` support is later requested, generalize the Copilot-style instruction-file path into a tool-method abstraction at that time — not as part of this requirement.

### Alternatives Considered

- **Plugin/registry-based tool model** — replace the closed `AIToolType` enum and per-method switches with a `ToolDescriptor` struct registered into a map. Rejected: large refactor, touches every existing tool, blocks the simple additive change, and the requirement does not justify it.
- **Single "generic markdown commands" tool** — collapse all "flat directory of markdown" tools (Cursor, Claude Code, Antigravity, OpenCode) into one type with a configurable directory. Rejected: would change the meaning of `AIToolType` from "tool identity" to "tool capability shape", breaking the detection model and external tooling that depends on the enum string values (e.g., `--tool claude-code`).
- **Opt-in OpenCode template under `optional/`** — instead of registering OpenCode at the detector/CLI level, ship an `optional/opencode-readme.md` template only. Rejected: does not satisfy the requirement (`init` and `generate --tool opencode` must work, signature detection must work, and the install path must be `.opencode/commands/`).

## Risk & Gap Analysis

### Requirement Ambiguities

- **Instruction file (`AGENTS.md`) support is not specified.** The requirement covers "commands installed under `.opencode/commands`" but is silent on whether OpenCode's `AGENTS.md` (project-level agent instructions) should also be generated. Decision in this analysis: out of scope; revisit if explicitly requested.
- **Signature surface beyond `.opencode/` is not specified.** The user did not define what counts as "OpenCode is in use". The recommendation (`[".opencode", "opencode.json"]`) is grounded in OpenCode's upstream docs; if the user disagrees, the signature list is a one-line change.
- **`--tool` flag string and aliases are not specified.** Recommendation (`opencode` canonical, no aliases) follows the established pattern; user may want `oc` or `open-code` aliases.
- **Tool ordering in interactive picker is not specified.** Recommendation: append at the end of the existing four-option list to preserve muscle memory for current users.
- **Documentation language scope is not specified.** Both `README.md` and `README.zh-CN.md` exist; analysis assumes both must be updated for parity (project policy implied by their joint existence).

### Edge Cases

- **Project with both `.cursor/` and `.opencode/`.** Today the multi-tool case is handled by first-match in `Detect()`. With OpenCode appended last, a Cursor+OpenCode project resolves to Cursor (no regression). Users wanting OpenCode in such a project must pass `--tool opencode` explicitly. This is consistent with the current behavior for any multi-tool project.
- **Greenfield OpenCode user with no signature files yet.** Detection returns `Unknown`; the user must run `openspdd --tool opencode init` or pick OpenCode from the interactive picker. The picker MUST list OpenCode for this case to work.
- **Pre-existing `.opencode/commands/` with user-authored commands.** `generate` already protects against overwrites unless `--force` is passed. No additional guard is required, but the behavior should be verified for OpenCode just as it is for other tools.
- **Frontmatter compatibility.** Core templates' frontmatter declares `name: /spdd-analysis`, `id: spdd-analysis`, `category: Development`, `description: ...`. OpenCode parses `description`, `agent`, `model`, `subtask` and ignores unknown keys (per upstream docs); the existing frontmatter is therefore safe. The slash-prefixed `name` value is a documentation aid for openspdd's own listing — OpenCode derives the slash command from the filename, not from `name`. No content edits are required, but the next phase should explicitly verify by loading a generated file in OpenCode.
- **Filename → command-name mapping.** OpenCode uses the `.md` filename as the command name. The existing `tmpl.ID + ".md"` scheme in `cmd/generate.go` produces filenames like `spdd-analysis.md` → `/spdd-analysis`, which is exactly the desired CLI command name. No renaming needed.
- **`opencode.json`-only project.** A project that has `opencode.json` but no `.opencode/` directory will be detected; `init` then creates `.opencode/commands/` (via `os.MkdirAll`). This is the correct behavior, but the test plan should cover it explicitly.

### Technical Risks

- **Drift in tool enumerations.** Adding a new `AIToolType` constant requires updating at least eight switch/list sites: `String`, `GetConfigDir`, `GetSignatureFiles`, `GetInstructionFile`, `HasInstructionFile`, `GetToolDirName` (all in `types.go`), the `toolTypes` slice in `Detect()` (`detector.go`), the `knownTools` slice in `ListAll()` (`manager.go`), `ParseToolFlag` (`root.go`), and the `selectToolInteractively` huh options (`init.go`). Missing any one causes silent partial support. _Mitigation_: enumerate them explicitly in the REASONS Canvas Operations sequence; cover with table-driven tests that include the new constant.
- **Test-table omissions.** `tests/detector/types_test.go` has six table-driven tests, each with a closed list of cases. Adding OpenCode means adding rows to all six. Forgetting a row leaves the new method behavior untested. _Mitigation_: enumerate in the next phase.
- **Empty embedded directory and `go:embed`.** `go:embed` ignores empty directories on some Go versions/setups. The existing `cursor/`, `claude-code/`, and `antigravity/` directories appear to ship empty in the repo; verify by inspecting `internal/templates/embed.go`'s embed directive and whether a tracked placeholder file (e.g., `.gitkeep`) is needed for the new `opencode/` directory. _Mitigation_: confirm the embed directive's pattern in the next phase; add a placeholder if needed for git tracking, knowing `loadTemplatesFromDir` already filters non-`.md` entries.
- **README parity.** Two README files plus the long help text in `cmd/root.go` enumerate supported tools. Forgetting one leaves users confused. _Mitigation_: explicit checklist in Operations.
- **Future `AGENTS.md` request.** If OpenCode `AGENTS.md` instruction-file support is later requested, the current Copilot-specific code path (`GenerateForCopilot`, the `HasInstructionFile()` boolean) does not generalize cleanly — it would either need to be duplicated as `GenerateForOpenCode` or refactored into a method-on-tool. _Mitigation_: out of scope now; flag for design attention if/when raised.
- **`.opencode/` directory naming collisions.** No known collision with other tool directories. The flag string `opencode` does not collide with existing flag strings (`cursor`, `claude-code`, `claude`, `antigravity`, `github-copilot`, `copilot`, `gh-copilot`).

### Acceptance Criteria Coverage

The original requirement is one sentence and contains no explicit numbered ACs. The implicit ACs derived from the requirement are:

| AC# | Description                                                                                         | Addressable? | Gaps/Notes                                                                                                                                                                                    |
| --- | --------------------------------------------------------------------------------------------------- | ------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1   | `openspdd` recognizes "opencode" as a supported AI tool target.                                     | Yes          | New `OpenCode` constant in `AIToolType` plus extension of every per-tool switch/list. No gap.                                                                                                 |
| 2   | Generated SPDD command files install under `.opencode/commands/` relative to the working directory. | Yes          | `GetConfigDir()` for `OpenCode` returns `.opencode/commands`; `cmd/generate.go` already uses `detectedResult.ConfigPath`. No gap.                                                             |
| 3   | `openspdd` can auto-detect an OpenCode project from filesystem signatures.                          | Yes          | Signature list `[".opencode", "opencode.json"]` registered in `Detect()`. Possible ambiguity on full signature list (see "Requirement Ambiguities").                                          |
| 4   | `openspdd --tool opencode <command>` works without auto-detection.                                  | Yes          | New case in `ParseToolFlag`. No gap.                                                                                                                                                          |
| 5   | The interactive tool picker (`init` with no detection) lists OpenCode as a choice.                  | Yes          | New entry in `selectToolInteractively` huh options. No gap.                                                                                                                                   |
| 6   | Existing tool support (Cursor, Claude Code, Antigravity, GitHub Copilot) is unchanged.              | Yes          | Change is purely additive: appended detection slot, new switch arms, new picker option. No existing behavior is touched.                                                                      |
| 7   | Documentation reflects OpenCode support so users can discover it.                                   | Partial      | Needs updates in `README.md`, `README.zh-CN.md`, and the long-help in `cmd/root.go`. Translation parity is the only judgment call.                                                            |
| 8   | Tests cover the new tool's behavior in the existing table-driven style.                             | Yes          | Extend `tests/detector/types_test.go` (six test tables) and detection-test fixtures. No gap.                                                                                                  |
| 9   | Generated command files load correctly inside OpenCode's TUI.                                       | Partial      | Existing core-template frontmatter is non-conflicting per OpenCode docs, but a real load-test inside OpenCode is verification work that belongs to the implementation/QA phase, not analysis. |
