# SPDD Analysis: Codex CLI Tool Support for OpenSPDD (Skills Path)

## Original Business Requirement

Support Codex as an AI-assisted coding tool. Give me a reasonable plan. Do NOT use skills as a substitute.

### Requirement Update (after architectural review)

After exploring the two viable paths (Codex custom prompts vs. Codex skills) and weighing how each fits OpenSPDD's existing model, the requirement was revised to **proceed via the Skills path**. Rationale:

- Custom prompts are **user-scope only** (`~/.codex/prompts/`); making OpenSPDD support them requires breaking the project's "never write outside the working directory" trust model with a new install gesture.
- Skills are **project-scope native** (`.agents/skills/`), git-versionable, team-shareable, and aligned with the open agent skills standard ([agentskills.io](https://agentskills.io/)) that the broader agentic-coding industry is converging on.
- Custom prompts are publicly marked deprecated by OpenAI; skills are the long-term investment direction.
- Skill support introduces exactly one new generation archetype (directory-bundle) without disturbing any existing tool's behavior.

This analysis from this point forward targets the Skills path. The original "no skill" preference is preserved verbatim above.

## Domain Concept Identification

### Existing Concepts (from codebase)

- **Closed `AIToolType` taxonomy**: OpenSPDD models tool identity as a closed typed-string enum in `internal/detector/types.go` (`Cursor`, `ClaudeCode`, `Antigravity`, `GitHubCopilot`, `OpenCode`, `Unknown`). Every tool-aware behavior — display name, config directory, signature files, instruction-file branch, embedded-template subdir, and CLI flag string — is anchored in this enum and its per-method `switch` blocks.
- **Project-scoped config-directory model**: Each tool maps to ONE project-local output directory (`.cursor/commands`, `.claude/commands`, `.antigravity/commands`, `.github/copilot-prompts`, `.opencode/commands`). The `init` and `generate` flows write inside this directory; OpenSPDD never writes outside the working directory tree. **Skills fit this model natively** — no exception is required.
- **First-match deterministic detection**: `DefaultDetector.Detect()` iterates a hard-coded `toolTypes` slice and returns the first signature match; order in this slice defines disambiguation in multi-tool repositories. Detection is read-only.
- **Two existing generation archetypes (currently dispatched by a hard-coded `if`)**:
  1. _Flat markdown commands_ (Cursor, Claude Code, Antigravity, OpenCode) — `Generate(req)` writes one file per template into the tool's config directory; filename derives from `tmpl.ID + ".md"`; `HasInstructionFile() == false`. **One template = one .md file = one slash command.**
  2. _Instruction-file with marker-based merge_ (GitHub Copilot only) — `GenerateForCopilot(targetDir, force)` writes a separate `copilot-instructions.md` plus a `.github/copilot-prompts/` directory; uses `<!-- openspdd:start -->` / `<!-- openspdd:end -->` markers for idempotent re-generation; `HasInstructionFile() == true`.

  Today, `cmd/generate.go::generateAllTemplates` dispatches between these two via `if detectedResult.ToolType == detector.GitHubCopilot { ... } else { ... }`. With Codex's directory-bundle becoming the third archetype, this `if`-chain reaches its natural break-point and is the right moment to abstract.
- **Embedded template store with three categories**: `internal/templates/data/{core,optional,tools/<tool>}`. `core/` is shared across all tools (the SPDD command corpus: `spdd-analysis`, `spdd-reasons-canvas`, `spdd-generate`, `spdd-prompt-update`, `spdd-sync`); `tools/<tool>/` is bespoke; `optional/` is opt-in.
- **Frontmatter compatibility model**: All embedded templates carry YAML frontmatter (`name: /spdd-*`, `id`, `category`, `description`); `ParseFrontmatter` ignores unknown keys, so per-tool extra keys are non-breaking. The **leading slash in `name`** (e.g., `name: /spdd-analysis`) is OpenSPDD's display convention, not a format mandated by any downstream tool.
- **Coordinated enumeration sites**: Adding a tool requires synchronized edits to ~9 sites — `String`, `GetConfigDir`, `GetSignatureFiles`, `GetInstructionFile`, `HasInstructionFile`, `GetToolDirName` (`types.go`), `toolTypes` slice (`detector.go`), `knownTools` slice in `ListAll()` (`manager.go`), `ParseToolFlag` (`root.go`), `selectToolInteractively` huh options (`init.go`), plus tests and READMEs.
- **Per-tool generation extension precedent**: GitHub Copilot already established that a tool can opt out of the generic `Generate(req)` flow and use its own `GenerateForCopilot(...)` method invoked from a tool-specific branch in `cmd/generate.go`. This precedent — "one bespoke generation method per archetype" — is the seed of the strategy abstraction this analysis introduces (see _New Concepts Required_); it is being formalized rather than reused as-is.
- **`TemplateManager` interface as a tool-specific leak**: The current `TemplateManager` interface in `internal/templates/manager.go` exposes `GenerateForCopilot(targetDir, force)` as a public method. This is a Copilot-specific concern that has leaked into a tool-agnostic interface, and the leak is the symptom of missing the strategy abstraction.

### New Concepts Required

- **`GenerationStrategy` interface (Strategy pattern at the template-generation layer)**: A new tool-agnostic abstraction that owns "how to generate the full SPDD command set for a given tool." Replaces the current `if ToolType == GitHubCopilot { ... } else { ... }` dispatch in `cmd/generate.go::generateAllTemplates`. Minimal v1 surface: a single method `GenerateAll(workingDir string, force bool) []GenerateResult`. Exactly one strategy implementation per generation archetype:
  - `FlatMarkdownStrategy` — handles Cursor / Claude Code / Antigravity / OpenCode and any future tool that fits the "flat .md files in a config dir" shape; serves as the **default fallback** for any tool without an explicitly registered strategy.
  - `CopilotInstructionFileStrategy` — handles GitHub Copilot; encapsulates the existing `GenerateForCopilot` logic, moved out of `EmbeddedTemplateManager` and into a self-contained strategy file.
  - `CodexSkillStrategy` — handles Codex; new directory-bundle generation (skill directory + SKILL.md + optional `agents/openai.yaml`).
- **Strategy registry (open/closed extension point)**: A package-level `map[detector.AIToolType]strategyFactory` plus a `RegisterStrategy(tool, factory)` function. Each non-default strategy file (`copilot_strategy.go`, `codex_strategy.go`, future `xxx_strategy.go`) calls `RegisterStrategy(...)` from its own `init()` to self-register. Lookup function `StrategyFor(tool, mgr)` consults the registry first and falls back to `FlatMarkdownStrategy` if no entry is found. Adding a new tool with a new archetype later means adding **one new strategy file** (with its own `init()`) — no central switch to update, no factory function to grow.
- **Codex as a first-class AI tool target**: A new `Codex` value in `AIToolType` participating in detection, manual selection, listing, and generation like the existing five tools. With the strategy abstraction in place, "adding Codex" decomposes cleanly into "extend the enum + register one strategy."
- **Skill as a directory-bundle artifact**: Per the [Codex Skills docs](https://developers.openai.com/codex/skills), a skill is a **directory** containing a `SKILL.md` file (with required `name` and `description` frontmatter) plus optional scripts, references, or `agents/openai.yaml` metadata. This breaks OpenSPDD's "1 template = 1 file" implicit assumption — and is exactly the reason `CodexSkillStrategy` cannot reuse the default flat-markdown path.
- **Open agent skills standard (`.agents/skills/`)**: A cross-vendor open standard maintained at [agentskills.io](https://agentskills.io/), not a Codex-proprietary location. Codex scans this directory at multiple levels (`$CWD`, parents up to repo root, `$HOME`, `/etc/codex/skills`, system-bundled). For project-scoped use, `<repo>/.agents/skills/` is the canonical path.
- **Skill invocation namespace**: Skills are invoked explicitly via `$skill-name` mention or the `/skills` menu, **and** can be invoked implicitly when the agent matches the user's task to the skill's `description`. This differs from the slash-command UX (`/spdd-*`) used by all other supported tools.
- **`agents/openai.yaml` Codex-specific metadata extension**: Codex augments the open skill standard with an optional sibling file controlling UI metadata (`display_name`, `icon_small`, `brand_color`), invocation policy (`allow_implicit_invocation`, default `true`), and tool dependencies (MCP server bindings). Whether to ship this file is a deliberate product decision per skill.
- **Implicit invocation policy concern**: Because `allow_implicit_invocation` defaults to `true`, a generated SPDD skill could be auto-triggered by Codex when the user's prompt loosely matches the skill's `description`. SPDD commands are workflow-orchestration prompts that the user is expected to invoke deliberately; **implicit invocation by Codex is undesirable for SPDD's semantics** and must be explicitly disabled.
- **Progressive-disclosure context budget**: Codex pre-loads only `name`, `description`, and file path of every available skill, capped at ~2% of model context window (or 8000 chars when unknown). For very large skill sets, descriptions are shortened or skills are dropped from the initial list. SPDD's command set is small (5 core skills), so this budget is not an immediate constraint, but `description` content should still be concise and front-loaded with trigger words.
- **Project-level Codex marker (`.codex/config.toml`)**: Codex supports a project-level config override at `<repo>/.codex/config.toml` (distinct from `~/.codex/config.toml`). This is the only Codex-specific marker that lives inside the repo and is **not** a cross-vendor convention.
- **Trust-model gating**: Per [openai/codex#9752](https://github.com/openai/codex/issues/9752), Codex ignores skills from untrusted projects; users must mark a project as trusted in their Codex config before generated SPDD skills load. This is a Codex-side concern outside OpenSPDD's responsibility, but is a real onboarding gotcha that must be documented.
- **`SKILL.md` frontmatter shape vs. OpenSPDD frontmatter shape**: Skills require exactly `name` (string identifier without leading `/`) and `description`. OpenSPDD core templates carry `name: /spdd-*` (with leading slash), `id`, `category`, and `description`. The leading slash is invalid as a skill `name` and must be stripped at generation time; `id` and `category` are unknown to Codex and are silently ignored.

### Conceptual Relationships

- `Codex` (new enum value) → consumed by → `Detect()` (registration in `toolTypes`), `init`/`generate` flows (via `ConfigPath`), `EmbeddedTemplateManager.ListAll()` (registration in `knownTools`), `ParseToolFlag` (canonical string → enum), and `selectToolInteractively` (option list entry).
- `Codex` → maps to → `.agents/skills/` (the cross-vendor skill location) for generation. **Important**: this is the same directory that may, in the future, be discovered by other agentic tools also adopting the open skill standard. For v1, OpenSPDD treats this as Codex's output target only; revisit if/when another supported tool adopts skills.
- `Codex` signature surface → checked by → `Detect()`; recommended primary marker is `.codex/config.toml` (project-scoped, Codex-exclusive). **Explicitly NOT** `.agents/skills/` (cross-vendor; would mis-detect any project that adopts skills for any reason) and **NOT** `AGENTS.md` (also cross-vendor, used by Claude Code and Cursor).
- `Codex` requires a **third generation archetype: directory-bundle** with these properties:
  - For each spdd-\* template, generate `<repo>/.agents/skills/<id>/SKILL.md`.
  - `SKILL.md` body = template body verbatim (existing core templates' Markdown is already directly usable as skill instructions).
  - `SKILL.md` frontmatter = `name: <id>` (slash stripped from OpenSPDD's `name: /<id>`), `description: <existing description, optionally trimmed for context budget>`.
  - Optionally generate sibling `agents/openai.yaml` per skill to set `policy.allow_implicit_invocation: false`.
- `Codex` does NOT use `HasInstructionFile() == true` — there is no Codex equivalent of `copilot-instructions.md` in this scope. AGENTS.md exists but is intentionally excluded from v1 (cross-vendor concern).
- The existing core templates (`spdd-*.md`) are **content-compatible** with Codex skills — their Markdown body becomes SKILL.md instructions verbatim. Frontmatter requires a small per-tool transformation (strip `/` from `name`; optionally drop `id`/`category`). This transformation lives inside `CodexSkillStrategy`, keeping `tools/codex/` empty and core templates untouched.
- `GenerationStrategy` registry → consumed by → `cmd/generate.go::generateAllTemplates`, which becomes a 2-line dispatch: `strategy := templates.StrategyFor(detectedResult.ToolType, mgr); results := strategy.GenerateAll(workingDir, forceFlag)`. All tool-specific branches disappear from the cmd layer.
- `CopilotInstructionFileStrategy` → encapsulates → existing `GenerateForCopilot` behavior verbatim (the migration is purely structural; no behavior change). The current public `EmbeddedTemplateManager.GenerateForCopilot(...)` method is removed from the `TemplateManager` interface and either deleted (if no external caller) or kept as a thin shim delegating to the strategy (if backward-compat matters).
- `CodexSkillStrategy` → a new strategy added on top of the new abstraction; **does not exist as a `GenerateForCodex` method on `EmbeddedTemplateManager`**. This is a deliberate departure from the Copilot precedent: rather than perpetuating the leak, Codex is the first tool added through the cleaned-up extension point.

### Key Business Rules

- **Single source of truth for tool support remains `AIToolType`.** Adding Codex must extend every per-tool switch/list site; partial coverage produces silent partial support.
- **Detection is read-only and non-destructive.** Adding Codex must follow this contract — no probing inside `~/`, no probing outside the working directory.
- **Generated artifacts must reach Codex's actual loader to count as "support".** Writing to `<repo>/.agents/skills/<id>/SKILL.md` satisfies this directly because Codex natively scans `<repo>/.agents/skills/` from CWD up to repo root. **No install gesture, no `~/` write, no copy-step required.**
- **Skill names must be valid identifiers** — no slash prefix, no spaces. `SKILL.md` requires `name` and `description`; everything else is optional or vendor-specific.
- **Each SPDD skill must be explicit-only by default.** Auto-invocation by Codex on description-match is incompatible with SPDD's workflow-orchestration semantics. Generated skills MUST set `allow_implicit_invocation: false` (via `agents/openai.yaml`) so users only trigger SPDD via explicit `$spdd-*` mention or the `/skills` menu.
- **Existing tools and outputs MUST remain unchanged.** Codex addition is additive: detection order, existing config paths, existing template lists, and existing CLI flag values must continue to behave identically. Critically, `AGENTS.md` and `.agents/skills/` must NOT be claimed as Codex-exclusive signatures.
- **Slash-command parity is a non-goal for Codex.** Users of Codex must accept `$spdd-*` (or `/skills` selection) invocation; OpenSPDD cannot reshape this without abandoning the skill model.
- **Forward compatibility with cross-vendor skill adoption.** Because `.agents/skills/` is an open standard that other tools may adopt, the design must not encode "Codex owns this directory exclusively" anywhere in the codebase. The mapping `Codex → .agents/skills/` is one-way today (Codex's `GetConfigDir()`) but the directory itself is shared territory.
- **Generation dispatch is centralized in the strategy registry, not in the cmd layer.** Once the strategy abstraction lands, `cmd/generate.go` MUST NOT contain any `if ToolType == X` branches. Every tool-specific generation behavior lives behind `GenerationStrategy.GenerateAll(...)`. This rule is the contract that makes future tool additions a 1-file change.
- **Behavior preservation through the Copilot migration.** Moving `GenerateForCopilot`'s body into `CopilotInstructionFileStrategy` is a pure refactor. All existing Copilot-related tests, file paths, marker conventions (`<!-- openspdd:start -->` / `<!-- openspdd:end -->`), and CLI output messages MUST remain byte-identical from the user's perspective.

## Strategic Approach

### Solution Direction

- **Extend the closed enum, do not refactor it.** Add `Codex AIToolType = "codex"` as a new tool value and extend each `switch t` block in `internal/detector/types.go`. Reuses the proven additive pattern from the OpenCode and Windsurf precedents.
- **Two-step delivery: extract strategy abstraction first, then add Codex on top of it.** This is one logical change delivered as two ordered code drops:
  - **Step 1 (pure refactor, no behavior change):** Extract `GenerationStrategy` interface + registry + `FlatMarkdownStrategy` + `CopilotInstructionFileStrategy` in `internal/templates/`. Move `GenerateForCopilot`'s body into `CopilotInstructionFileStrategy.GenerateAll`. Remove `GenerateForCopilot` from the `TemplateManager` interface (and from `EmbeddedTemplateManager` if no caller remains). Replace the `if ToolType == GitHubCopilot { ... } else { ... }` branch in `cmd/generate.go::generateAllTemplates` with a single dispatch: `templates.StrategyFor(tool, mgr).GenerateAll(workingDir, force)`. All existing Copilot tests pass unchanged — they are the regression contract.
  - **Step 2 (additive Codex):** Implement `CodexSkillStrategy` (directory-bundle generation: skill dir + SKILL.md + optional `agents/openai.yaml`). The strategy registers itself in the registry via `init()`. Extend the `AIToolType` enum + every per-tool switch site + CLI parser + interactive picker + `knownTools` slice + tests + READMEs. **No `cmd/generate.go` change needed in this step** — the dispatch already routes to the right strategy automatically.
  - Doing it in this order means: (a) Step 1 carries the refactor risk in isolation with the existing test suite as the safety net, (b) Step 2 is a near-pure addition, (c) Copilot's hot path is touched exactly once instead of twice, (d) any future tool follows the Step 2 template only.
- **Use registry-based dispatch, not a switch.** `StrategyFor(tool, mgr)` consults a package-level `map[detector.AIToolType]strategyFactory`. Each non-default strategy file (`copilot_strategy.go`, `codex_strategy.go`, future `xxx_strategy.go`) calls `RegisterStrategy(tool, factory)` from its own `init()` to self-register. Lookup falls back to `FlatMarkdownStrategy` for unregistered tools. Adding a future tool with a new archetype = adding one new strategy file with its own `init()`; no central function gets edited.
- **Reuse core templates verbatim; transform frontmatter at generation time.** The Markdown body of existing core templates becomes SKILL.md instructions unchanged. The frontmatter transformation (`name: /spdd-* → name: spdd-*`; drop `id` and `category`; preserve `description`) happens inside `CodexSkillStrategy`. Result: zero edits to `internal/templates/data/core/*.md`; zero new files in `internal/templates/data/tools/codex/`.
- **Default to explicit-only invocation.** Generated skills include a sibling `agents/openai.yaml` with `policy.allow_implicit_invocation: false`. SPDD workflows are deliberate user actions, not opportunistic agent triggers; this is the safer default and aligns with OpenSPDD's "explicit slash-command" UX heritage. Provide a `--allow-implicit` flag on `generate` for users who genuinely want Codex to auto-invoke SPDD skills, but keep it off by default.
- **Use a Codex-specific project marker, avoid cross-vendor signals.** Detect Codex projects via `<repo>/.codex/` (where the project-level Codex config lives) and `<repo>/.codex/config.toml` (the canonical project-level Codex config file). Explicitly DO NOT use `.agents/skills/` (cross-vendor) or `AGENTS.md` (cross-vendor) as primary signatures, because they would mis-classify any project adopting the open agentic conventions.
- **Append Codex at the tail of the detector slice.** Preserves existing detection precedence — no project that detects to Cursor / Claude Code / Antigravity / GitHub Copilot / OpenCode (and Windsurf if landed first) today will silently re-detect to Codex.
- **Treat `.agents/skills/` as Codex's mapped target today, but do not encode "Codex owns it" anywhere else.** Codex's `GetConfigDir()` returns `.agents/skills`. No other code path treats this directory as Codex-exclusive. If a future supported tool adopts skills, OpenSPDD will need to decide between "shared directory, multiple tools generate side-by-side" and "elevate skills to a cross-tool category" — that decision is deferred until it becomes concrete.
- **Cross-layer parity update.** Detector + CLI parser + interactive picker + template-manager `knownTools` + tests + both READMEs + root long-help all updated as one contract set, mirroring the OpenCode and Windsurf playbook.

### Key Design Decisions

- **Dispatch mechanism: registry vs. switch**
  - _Trade-offs_:
    - (A) **Switch in `StrategyFor`** — single function inspecting `tool` and returning the matching strategy; default arm returns `FlatMarkdownStrategy`. Easiest to read at a glance; centralizes the mapping in one place. Adding a tool means editing the central function (small but recurring touch).
    - (B) **Map-based registry with `init()` self-registration** — package-level `map[detector.AIToolType]strategyFactory`; each non-default strategy file calls `RegisterStrategy(...)` from its own `init()`; lookup falls back to `FlatMarkdownStrategy`. Adding a tool means adding one strategy file (zero edits to existing files). True open/closed.
    - (C) **Explicit registration from a single `init` package** — central registration list lives in one place but uses the registry data structure. A hybrid; loses the "one new file is enough" property of (B).
  - _Recommendation_: **Option B (registry)**. The whole point of doing the abstraction now is to make future tool additions a single-file change; a switch would re-introduce the central-edit problem the abstraction was supposed to remove. The known cost of (B) is `init()`-time global state, which is mitigated by keeping registration to one line per file and treating the registry as immutable after package init.

- **Strategy interface scope: `GenerateAll` only vs. also single-template generation**
  - _Trade-offs_:
    - (A) **Only `GenerateAll(workingDir, force)`** — strategy owns batch generation; single-template generation continues through the existing `Generate(req GenerateRequest)` path on `EmbeddedTemplateManager`. Smallest interface; quickest to land.
    - (B) **Add `GenerateOne(name, workingDir, force)`** — strategy owns both batch and single. Conceptually cleaner (every per-tool nuance is encapsulated in the strategy), but requires designing what "single-skill generation" means for Codex (a directory? just SKILL.md?) — extra design surface for marginal v1 value.
  - _Recommendation_: Option A for v1. `GenerateAll` is sufficient to absorb the existing `if`-chain. `Generate(req)` remains the low-level single-file primitive that strategies can call internally if needed. Re-evaluate (B) only if a future tool exposes a meaningful "generate one skill" UX.

- **Factory location: `internal/templates/` vs. `cmd/`**
  - _Trade-offs_:
    - (A) **`internal/templates/`** — registry + strategies live alongside the rest of template generation logic; `cmd/generate.go` only sees the public `StrategyFor(...)` symbol. cmd layer stays thin.
    - (B) **`cmd/`** — keeps strategies near the orchestrator. Couples cmd layer to the dispatch logic; harder to reuse strategies from non-cmd contexts (e.g., future SDK / library use).
  - _Recommendation_: Option A. Matches the existing layering convention where `templates/` owns generation, `cmd/` orchestrates UX.

- **`TemplateManager` interface cleanup: remove `GenerateForCopilot` or keep as shim**
  - _Trade-offs_:
    - (A) **Remove from interface, delete method** — cleanest; no callers remain after the cmd-layer dispatch is rewritten. Risk: any hypothetical external consumer (none known today inside the repo) breaks.
    - (B) **Remove from interface, keep method as deprecated shim that delegates to the strategy** — backward-compat-friendly; adds dead-weight code; signals "we removed this from the contract but did not actually delete it."
    - (C) **Keep on the interface** — defeats half the point of the refactor; tool-specific leakage stays in the public contract.
  - _Recommendation_: Option A. A workspace-wide search confirms `GenerateForCopilot` is called only from `cmd/generate.go::generateAllTemplates`; once that call site is rewritten to use the strategy, the method has zero callers and can be removed outright. Keep the strategy implementation as the new home for the logic.

- **Naming: `GenerationStrategy` vs. `ToolGenerator` vs. `TemplateInstaller`**
  - _Trade-offs_: `GenerationStrategy` makes the pattern intent explicit (Strategy pattern, recognizable to reviewers); `ToolGenerator` is shorter but loses the pattern hint; `TemplateInstaller` overloads the existing `Generate` verb in confusing ways.
  - _Recommendation_: `GenerationStrategy`. The interface, the file `strategy.go`, and the tests `*_strategy_test.go` all telegraph the design intent without comments.

- **Refactor order: extract strategy first vs. add Codex first**
  - _Trade-offs_:
    - (A) **Strategy first, then Codex** — Step 1 is a pure refactor backed by the existing Copilot test suite as the regression contract; Step 2 is a near-pure addition on top of the new abstraction. Copilot's hot path is touched once.
    - (B) **Codex first as `GenerateForCodex` method, then refactor** — adds a third `if` branch first, then refactors three branches into strategies. Copilot's hot path is touched twice. Refactor risk and feature risk are mixed.
  - _Recommendation_: Option A. Reduces blast radius and lets each step ship independently if needed.

- **Codex generation target: `<repo>/.agents/skills/` (recommended) vs. `<repo>/.codex/skills/` vs. somewhere else**
  - _Trade-offs_:
    - (A) `<repo>/.agents/skills/` — the **only** location Codex actually scans for project-scoped skills per official docs. Open cross-vendor standard. Naturally git-versionable.
    - (B) `<repo>/.codex/skills/` — Codex-namespaced; would feel symmetric with other tools' `.tool/` pattern. **But Codex does not scan this path for skills** — generated artifacts here would be invisible to Codex. Non-functional.
    - (C) `<repo>/.codex/prompts/` — would put OpenSPDD back on the deprecated custom-prompts path; explicitly excluded by the requirement update.
  - _Recommendation_: Option A (`<repo>/.agents/skills/`). It is the only target where Codex actually loads skills. The cross-vendor standard nature is a feature (forward compatibility), not a problem.

- **Skill directory naming convention: `<repo>/.agents/skills/<id>/SKILL.md`**
  - _Trade-offs_:
    - (A) Per-skill subdirectory — matches official Codex examples; allows future addition of scripts/references/`agents/openai.yaml` per skill.
    - (B) All skills as siblings (e.g., `<repo>/.agents/skills/spdd.md`) — incompatible with the standard (skills are directories, not files). Non-functional.
    - (C) Group all SPDD skills under a single shared directory (e.g., `<repo>/.agents/skills/spdd/`) — would collapse five skills into one, losing per-skill `description`-based selection. Non-functional.
  - _Recommendation_: Option A. One directory per skill, named after the SPDD command id (e.g., `spdd-analysis/`, `spdd-reasons-canvas/`, ...).

- **Frontmatter transformation strategy (handle the `name: /spdd-*` mismatch)**
  - _Trade-offs_:
    - (A) **Transform at generation time** (inside `CodexSkillStrategy`) — strip leading `/` from `name`, drop `id` and `category`. Core templates remain untouched.
    - (B) **Ship Codex-specific copies under `tools/codex/`** — duplicate every core template with skill-conformant frontmatter. Adds maintenance burden; doubles the source of truth.
    - (C) **Update core templates' `name`** to drop the leading `/` — would change the displayed command name in every other tool's listing. Mild UX impact; cleanest source state.
  - _Recommendation_: Option A. Transformation is mechanical (~10 lines of Go); core templates stay single-source; no other tool's UX changes.

- **Whether to ship `agents/openai.yaml` per skill**
  - _Trade-offs_:
    - (A) **Always ship**, with `policy.allow_implicit_invocation: false` — ensures SPDD skills are explicit-only by default; protects user from surprise auto-invocation; small per-skill file overhead.
    - (B) **Never ship** — relies on Codex defaults; `allow_implicit_invocation` defaults to `true`, meaning Codex MAY auto-invoke an SPDD skill when it thinks the user's prompt matches a `description`. Wrong default for workflow-orchestration prompts.
    - (C) **Single shared `openai.yaml` at the `.agents/skills/` root** — the spec is per-skill (`agents/openai.yaml` lives next to its corresponding `SKILL.md`); a shared root file is non-standard.
  - _Recommendation_: Option A. Default-deny implicit invocation aligns with SPDD's deliberate workflow semantics. Provide a `generate --allow-implicit` flag (off by default) for users who knowingly want auto-invocation.

- **Codex signature files (what counts as "Codex is in use")**
  - _Trade-offs_:
    - (A) Match only `.codex/` directory — simplest.
    - (B) Match `.codex/` OR `.codex/config.toml` — covers the empty-`.codex` case AND the config-only case. Mirrors the dual-signature pattern used for Cursor and Claude Code.
    - (C) Match `.codex/`, `.codex/config.toml`, OR `.agents/skills/` — broadest, but `.agents/skills/` is cross-vendor and would mis-detect any agentic project.
    - (D) Include `AGENTS.md` — also cross-vendor.
  - _Recommendation_: Option B. Signature list `[".codex", ".codex/config.toml"]`. Codex-exclusive markers only.

- **Detection order in `Detect()`**
  - _Trade-offs_: Insert Codex at the head, in the middle, or at the tail. Anywhere except the tail risks changing first-match outcomes for projects that today match an earlier tool AND coincidentally have a `.codex/` directory.
  - _Recommendation_: Append at the tail. Guarantees zero regression for existing detections. Multi-tool projects can still select Codex explicitly via `--tool codex`.

- **Tool flag canonical name and aliases (`ParseToolFlag`)**
  - _Trade-offs_:
    - (A) Only `codex` — minimal, matches the brand.
    - (B) `codex` plus `openai-codex` — disambiguates from any "codex"-named product.
    - (C) `codex` plus `cdx` short alias — convenient but two-letter tokens collide easily.
  - _Recommendation_: Option A (`codex` only) for v1.

- **Tool directory name under `internal/templates/data/tools/`**
  - _Trade-offs_: `codex` (single word, matches brand and `--tool` flag) versus `openai-codex`.
  - _Recommendation_: `codex`.

- **Whether `tools/codex/` ships any bespoke templates in v1**
  - _Trade-offs_:
    - (A) **Empty** — frontmatter transformation handled inside `CodexSkillStrategy`; core templates reused verbatim.
    - (B) **Codex-tailored core template copies** — pre-transform frontmatter and ship as separate files; doubles source of truth.
    - (C) **Codex-only helper skill** (e.g., `spdd-help.md` describing the SPDD workflow inside Codex) — out of scope for v1.
  - _Recommendation_: Option A. Empty tool directory; reuse core templates with on-the-fly frontmatter transformation. Matches the precedent set by Cursor / Claude Code / Antigravity / OpenCode.

- **`HasInstructionFile()` value for Codex**
  - _Trade-offs_: Codex reads `AGENTS.md` for project instructions, but `AGENTS.md` is a cross-vendor convention — generating it would step on Claude Code's territory and on any future tool that adopts it. Setting `HasInstructionFile() == true` would also force a Copilot-style branch that the skill model doesn't need.
  - _Recommendation_: `HasInstructionFile() == false` for Codex. The directory-bundle archetype handles Codex generation without leaning on the instruction-file branch.

- **Whether to surface deprecation/trust gotchas in user-facing output**
  - _Trade-offs_:
    - (A) Silent — minimal noise; users figure it out themselves.
    - (B) Post-`generate` hint for Codex — print a one-line note about (1) explicit invocation via `$spdd-*` or `/skills`, (2) the trust-model gotcha if skills don't appear, (3) the path where files were written.
    - (C) README-only guidance.
  - _Recommendation_: Option B + C. The trust-model issue (openai/codex#9752) is a real onboarding cliff; an in-CLI hint reduces "why isn't it working" friction without being noisy.

### Alternatives Considered

- **Keep the current `if ToolType == X` dispatch in `cmd/generate.go` and add a `GenerateForCodex` method on `EmbeddedTemplateManager`** — would be the third tool-specific method on a tool-agnostic interface and the third arm of an `if`-chain. Rejected: at 2 specials the cost is tolerable, at 3 it crosses the abstraction threshold; deferring the cleanup until a 4th tool comes will only make the eventual refactor larger and riskier.
- **Switch-based `StrategyFor(...)` factory instead of registry** — simpler to read at first glance, but reintroduces the central-edit problem: every new tool with a new archetype edits one shared function. Rejected because the entire reason for doing the abstraction is to make future tool additions a single-file change. Detail in _Key Design Decisions > Dispatch mechanism_.
- **Plugin/registry-based tool model spanning detection + generation + CLI** (full registry refactor across all layers) — rejected on the same grounds the prior analyses (OpenCode, Windsurf) rejected it: large cross-cutting refactor, no compelling business justification today. The strategy abstraction introduced here is **scoped to template generation only**; detection and CLI flag parsing remain enum-switch based until a separate trigger justifies expanding the abstraction.
- **Custom prompts path (`<repo>/.codex/prompts/` + install gesture into `~/.codex/prompts/`)** — was the prior recommendation in this analysis. Rejected after architectural review: requires breaking OpenSPDD's "never write outside the working directory" trust model with an install gesture; targets a feature OpenAI has marked deprecated; provides a worse invocation UX (`/prompts:spdd-*` vs. `$spdd-*` or `/skills`) without any compensating long-term value.
- **Generate `AGENTS.md` as the Codex output (à la Copilot's `copilot-instructions.md`)** — rejected. `AGENTS.md` is a cross-vendor convention, not Codex-exclusive; claiming it as Codex's slot would create silent conflicts with Claude Code and any other agentic tool that adopts it. Also, `AGENTS.md` is one document — it does not provide per-skill description matching or the `/skills` menu UX.
- **Plugin/registry-based tool model** — rejected on the same grounds the prior analyses (OpenCode, Windsurf) rejected it: large refactor, no compelling business justification for one more tool. Defer until at least 3 tools warrant the abstraction.
- **Package SPDD skills as a Codex plugin** (per the official [plugins distribution path](https://developers.openai.com/codex/plugins/build)) — rejected for v1. Plugins are for external distribution beyond a single repo (e.g., `$skill-installer`); OpenSPDD's value is generating the in-repo source of truth on demand, not maintaining a plugin distribution channel. Plugins are a possible future channel for OpenSPDD-as-a-distributed-skill-pack, but that is a separate product question.
- **Use the `~/.agents/skills/` user-scope location instead of `<repo>/.agents/skills/`** — rejected. Project-scope is consistent with OpenSPDD's existing model and gives teams version-controlled, repo-portable SPDD support. User-scope would re-introduce the same "OpenSPDD writes to `~/`" problem we just escaped from the custom-prompts path.
- **Map Codex to a dedicated Codex-namespaced directory like `.codex/skills/` (not the cross-vendor `.agents/skills/`)** — rejected. Codex does not scan `.codex/skills/` for skills; only `.agents/skills/` is loaded. Generating to a path Codex doesn't read defeats the purpose of "support".
- **Wait for the trust-model issue (openai/codex#9752) to fully stabilize before shipping** — rejected. The upstream fix is already in flight per the issue thread; documentation of the trust-mark step is a sufficient mitigation today.

## Risk & Gap Analysis

### Requirement Ambiguities

- **Whether `agents/openai.yaml` should ship by default** — the requirement says "support Codex" but does not specify default invocation policy. Recommendation: ship `agents/openai.yaml` with `allow_implicit_invocation: false` for SPDD skills; this is a strong recommendation rather than a settled answer. The next phase should explicitly confirm.
- **Whether v1 should generate skill scripts or stay instruction-only** — the requirement does not mention scripts. Recommendation: instruction-only (no scripts in `SKILL.md`'s sibling files). Matches Codex docs' default and the existing OpenSPDD model where commands are pure prompts.
- **Project marker scope** — the requirement does not enumerate which files signal a "Codex project". Recommendation `[".codex", ".codex/config.toml"]` is grounded in upstream docs.
- **`--tool codex` flag spelling and aliases** — not specified. Recommendation: `codex` only.
- **Whether generated skills should be added to `.gitignore` or committed** — not specified. Implicit answer from project-scope choice: committed (the whole point of project scope is team sharing). Documentation should make this explicit.
- **Documentation language scope** — both `README.md` and `README.zh-CN.md` exist; assumed both must be updated for parity (project policy implied by their joint existence; same precedent as OpenCode and Windsurf analyses).
- **Behavior when `<repo>/.agents/skills/<id>/SKILL.md` already exists** — the requirement is silent on collision policy. Recommendation: skip-existing by default, `--force` to overwrite, mirroring `Generate(req)`'s existing semantics.
- **Whether to document the trust-mark step in CLI output** — recommended, but the exact wording and threshold ("show always" vs. "show only on first generate") needs the next phase to settle.

### Edge Cases

- **Project with both `.cursor/` and `.codex/` (or any other-tool + `.codex/`).** With Codex appended last in `Detect()`, such a project resolves to the existing tool (no regression). Users wanting Codex must pass `--tool codex` explicitly. Consistent with current multi-tool behavior.
- **Greenfield Codex user with no signature files yet.** Detection returns `Unknown`; user must run `openspdd --tool codex init` or pick Codex from the interactive picker. The picker MUST list Codex for this to work.
- **`<repo>/.agents/` already exists from another tool's adoption of skills.** Generating into `.agents/skills/<spdd-id>/` is namespaced by per-skill subdirectory, so coexistence is safe — OpenSPDD's skills sit alongside any pre-existing skills without name collision (assuming no other tool has shipped a skill literally named `spdd-analysis` etc.). If a user-authored skill with the same name exists, the existing skip-existing/`--force` semantics apply.
- **Pre-existing `<repo>/.agents/skills/<spdd-id>/` from a previous OpenSPDD generate run.** Behavior must be defined: skip / overwrite-with-`--force`. Recommendation: skip-existing by default.
- **Pre-existing user-authored `SKILL.md` inside an OpenSPDD-owned skill directory.** Treated identically to the previous case. The collision policy must protect user data by default.
- **Codex CLI/IDE caching skills.** Codex auto-detects skill changes per the docs, but explicitly mentions "if an update doesn't appear, restart Codex". The post-generate hint should mention restart as a fallback.
- **Trust-model rejection.** Per [openai/codex#9752](https://github.com/openai/codex/issues/9752), Codex ignored skills from untrusted projects after a security change in early 2026. The fix is in flight, but users on older Codex versions may need to mark the project trusted before SPDD skills appear. Cannot be fixed by OpenSPDD; must be documented.
- **Implicit invocation despite `allow_implicit_invocation: false`.** The Codex docs state that with `false`, "Codex won't implicitly invoke the skill based on user prompt; explicit `$skill` invocation still works" — but skills can still appear in the `/skills` menu, which is the desired UX. No edge case here, just a confirmation the chosen default produces the right behavior.
- **Skill directory on case-insensitive filesystems (macOS default, Windows).** Skill names like `spdd-analysis` and `SPDD-Analysis` would collide. Existing OpenSPDD command IDs are all lowercase-with-hyphens; not an immediate risk.
- **Filename → skill-name mapping.** Codex uses the `name` field inside `SKILL.md` as the canonical skill identifier; the directory name is convention. Recommendation: directory name matches the `name` field for clarity (`spdd-analysis/` ↔ `name: spdd-analysis`).
- **Symlinked skill directories.** Codex documents that it follows symlinks in skill scan locations. Not relevant for OpenSPDD's generate path (which always materializes files), but relevant if power users want to symlink generated skills elsewhere.
- **Long-description context-budget pressure.** SPDD command descriptions in current frontmatter are concise (one or two sentences), well under any per-skill budget. Not an immediate constraint, but the next phase should still front-load trigger words in the description for implicit-match accuracy if/when `allow_implicit_invocation: true` is opted-in.
- **`agents/openai.yaml` parsing strictness.** Codex docs do not specify behavior when the YAML is malformed; conservative recommendation is to ship a minimal valid YAML file with only the fields we need (`policy.allow_implicit_invocation: false`), not a maximalist template. Reduces malformed-YAML risk.

### Technical Risks

- **Drift in tool enumerations.** Adding a new `AIToolType` constant requires updating ~9 sites. Missing any one causes silent partial support (e.g., generate works but `list --all` hides it; `--tool codex` parses but `init` picker omits it). _Mitigation_: enumerate them explicitly in the REASONS Canvas Operations sequence; cover with table-driven tests.
- **Test-table omissions.** `tests/detector/types_test.go` has multiple table-driven tests, each with a closed list of cases. Adding Codex means adding rows to all of them. _Mitigation_: enumerate in the next phase.
- **Empty embedded directory and `go:embed`.** `go:embed all:data` is the directive. If `internal/templates/data/tools/codex/` is created empty, it must be tracked by git (e.g., `.gitkeep`) to survive embedding; verify the existing empty `tools/cursor/`, `tools/claude-code/`, `tools/antigravity/` directories' embed behavior. _Mitigation_: confirm in the next phase.
- **Frontmatter transformation correctness.** The transformation (`name: /spdd-* → name: spdd-*`; drop `id`/`category`; preserve `description`; serialize back as valid YAML) must produce SKILL.md that Codex parses cleanly. _Mitigation_: unit-test the transformation against the existing core templates' real frontmatter.
- **Cross-vendor directory collision.** `.agents/skills/` is an open standard. If a future supported OpenSPDD tool (say, Anthropic's Claude Code) also adopts skills and is mapped to the same directory, OpenSPDD will need a generation-coordination decision (do both tools generate the same skills? do they generate different skill sets? does one own the directory?). _Mitigation_: document that `Codex → .agents/skills/` is the v1 mapping with no cross-tool sharing assumption; revisit when a second tool requests skill output.
- **README parity.** Two README files plus the long help text in `cmd/root.go` enumerate supported tools. _Mitigation_: explicit checklist in Operations.
- **Implicit-invocation surprise.** If `agents/openai.yaml` is forgotten or the `--allow-implicit` flag is misused, Codex will auto-invoke SPDD skills on loose description matches, which can disrupt user intent (e.g., user says "analyze this code", Codex implicitly fires `spdd-analysis` workflow). _Mitigation_: ship `allow_implicit_invocation: false` by default; cover with generation tests that assert the YAML file exists with the expected key.
- **Trust-model onboarding cliff.** Codex skills from untrusted projects are silently dropped on some Codex versions. Users see "OpenSPDD said it generated, but Codex shows nothing". _Mitigation_: in-CLI hint after generate; README troubleshooting section.
- **Restart-required edge case.** Codex auto-detects most skill changes but documents that some require restart. _Mitigation_: in-CLI hint mentions restart as fallback.
- **Plugin-future divergence.** OpenAI documents plugins as the long-term distribution unit. If at some point OpenSPDD wants to ship SPDD as a Codex plugin (installable via `$skill-installer`), the project will need plugin packaging / manifest tooling beyond what's in scope here. _Mitigation_: deferred to a future requirement; the directory-bundle archetype is plugin-compatible (a plugin is essentially a packaged collection of skills).
- **Open standard evolution.** [agentskills.io](https://agentskills.io/) may evolve `SKILL.md` schema (e.g., add new required fields). _Mitigation_: SKILL.md generation is centralized in `CodexSkillStrategy`; future schema updates are localized.
- **Copilot regression risk during the Step-1 refactor.** Moving `GenerateForCopilot`'s body into `CopilotInstructionFileStrategy` is structurally simple but touches Copilot's only generation path. Bugs introduced here would silently break GitHub Copilot users. _Mitigation_: existing Copilot tests (`tests/templates/...`, `tests/cmd/...`) are the regression contract — they MUST pass byte-identically before and after Step 1. Step 1 lands as its own commit (or PR) so the Copilot diff is reviewable in isolation, before any Codex code is added.
- **Registry `init()` ordering and global state.** Go runs `init()` functions in deterministic file order within a package, but a registry built from many `init()` calls can be hard to trace ("which file added this entry?"). _Mitigation_: keep one strategy per file; name files `<tool>_strategy.go` so the registry entry is grep-discoverable; add a unit test that asserts the expected set of registered tools after package init (catches accidental missing-import / missing-`init` regressions).
- **`TemplateManager` interface contract change.** Removing `GenerateForCopilot` from the public interface is a breaking change to that interface, even if no in-repo caller exists. _Mitigation_: workspace search confirms zero in-repo callers outside `cmd/generate.go::generateAllTemplates`; the project has no published Go SDK consuming this interface, so the blast radius is the repo itself. Document the removal in the change set's commit message / changelog.
- **Over-engineering perception.** Introducing a Strategy pattern + registry for 3 archetypes can read as overkill in code review. _Mitigation_: keep the v1 interface minimal (`GenerateAll` only); colocate the abstraction with the first beneficiary (Codex addition); explicitly document in code comments and PR description that the abstraction is paying for itself in this same change (Copilot already migrated, Codex slotted in cleanly).
- **Implicit fallback to `FlatMarkdownStrategy` for unregistered tools.** A future engineer adding a new tool that needs a custom archetype could forget to register a strategy and silently get the default flat-markdown behavior, producing wrong output without any error. _Mitigation_: add a debug-level log in `StrategyFor` when falling back to default; in tests, assert that every tool intended to have a custom strategy actually has one registered (table-driven test against `AIToolType` constants).

### Acceptance Criteria Coverage

The original requirement is one sentence and contains no explicit numbered ACs. The implicit ACs derived from the requirement (and the OpenSPDD support model demonstrated by prior tool integrations) are:

| AC# | Description                                                                                                                                      | Addressable? | Gaps/Notes                                                                                                                                                                                                                         |
| --- | ------------------------------------------------------------------------------------------------------------------------------------------------ | ------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1   | `openspdd` recognizes "codex" as a supported AI coding tool target.                                                                              | Yes          | New `Codex` constant in `AIToolType`; extension of every per-tool switch/list. No gap.                                                                                                                                             |
| 2   | `openspdd --tool codex <command>` works without auto-detection.                                                                                  | Yes          | New case in `ParseToolFlag`. No gap.                                                                                                                                                                                               |
| 3   | The interactive tool picker (`init` with no detection) lists Codex as a choice.                                                                  | Yes          | New entry in `selectToolInteractively` `huh` options. No gap.                                                                                                                                                                      |
| 4   | `openspdd` can auto-detect a Codex project from filesystem signatures.                                                                           | Partial      | Recommended signatures `[".codex", ".codex/config.toml"]`; user confirmation needed before lock-in. Definitively excludes `.agents/skills/` (cross-vendor) and `AGENTS.md` (cross-vendor).                                         |
| 5   | Generated SPDD command files install to a Codex-discoverable location.                                                                           | Yes          | Writes to `<repo>/.agents/skills/<id>/SKILL.md`, which Codex natively scans. No install gesture, no `~/` write. Pure project-scope.                                                                                                |
| 6   | Users can invoke SPDD commands inside Codex CLI / Codex IDE extension via skill semantics.                                                       | Yes          | Achievable as `$spdd-analysis` (etc.) or via the `/skills` selection menu. Note: invocation UX differs from `/spdd-*` used in other tools; this is intrinsic to Codex's skill model.                                               |
| 7   | Existing tool support (Cursor, Claude Code, Antigravity, GitHub Copilot, OpenCode[, Windsurf]) is functionally unchanged from the user's perspective. | Yes          | Codex addition is additive at the tool level. The Step-1 strategy refactor changes Copilot's *internal* code path (now routed through `CopilotInstructionFileStrategy`) but its observable behavior — file paths, marker contents, CLI output, idempotency — is preserved byte-identically.                            |
| 8   | Documentation reflects Codex support so users can discover it (English + Chinese).                                                               | Partial      | Needs updates in `README.md`, `README.zh-CN.md`, and the long-help in `cmd/root.go`. Translation parity is the only judgment call.                                                                                                 |
| 9   | Tests cover the new tool's behavior in the existing table-driven style, plus the new generation path.                                            | Yes          | Extend `tests/detector/types_test.go` (table-driven rows); add `ParseToolFlag("codex")` coverage; add `CodexSkillStrategy` tests covering directory creation, SKILL.md frontmatter transformation, and `agents/openai.yaml` content. |
| 10  | Generated skill files load correctly inside Codex CLI / Codex IDE extension and are invokable as `$spdd-*`.                                      | Partial      | Frontmatter transformation produces spec-compliant SKILL.md; real load-test in Codex is verification work for the implementation/QA phase.                                                                                         |
| 11  | OpenSPDD never writes outside the working directory.                                                                                             | Yes          | Skill path inherently respects this — `.agents/skills/` is project-scope. Trust model unchanged from current.                                                                                                                      |
| 12  | Generated skills are explicit-invocation-only by default; auto-invocation by Codex is opt-in.                                                    | Yes          | Sibling `agents/openai.yaml` with `policy.allow_implicit_invocation: false`. `generate --allow-implicit` flag for opt-in. No gap.                                                                                                  |
| 13  | Generation is idempotent and protects user-authored files at the same path.                                                                      | Yes          | Inherits `Generate(req)`'s skip-existing + `--force` semantics. No gap.                                                                                                                                                            |
| 14  | The trust-model gotcha (skills ignored on untrusted projects on some Codex versions) is communicated to users.                                   | Yes          | Single CLI hint after generate + README troubleshooting paragraph. No gap.                                                                                                                                                         |
| 15  | Acceptance includes the cross-vendor nature of `.agents/skills/` being documented so users understand the directory may also serve future tools. | Partial      | Documentation paragraph in README; no code change required. The directory is shared territory by design.                                                                                                                           |
| 16  | A `GenerationStrategy` interface + registry exists in `internal/templates/` and replaces the per-tool `if`-chain in `cmd/generate.go`.            | Yes          | Step 1 of the two-step delivery. `cmd/generate.go::generateAllTemplates` reduces to dispatch + result rendering. New tests assert: (a) `cmd/generate.go` contains zero `ToolType ==` branches in the generate-all path; (b) every registered strategy is reachable via `StrategyFor`. |
| 17  | All existing GitHub Copilot tests pass byte-identically before and after the strategy refactor.                                                  | Yes          | Existing Copilot test suite is the regression contract; if any existing test changes, it indicates a behavior change that must be justified or reverted.                                                                            |
| 18  | `EmbeddedTemplateManager.GenerateForCopilot` and the `GenerateForCopilot` method on the `TemplateManager` interface are removed.                  | Yes          | Workspace-search confirms zero in-repo callers outside the rewritten `generateAllTemplates`. Interface contract change documented in commit / changelog.                                                                            |
| 19  | A future tool with a new generation archetype can be added by adding **one** new `<tool>_strategy.go` file (with its own `init()` registration) plus the standard enum/CLI/test extensions. | Yes          | The acceptance test for this AC is the Codex addition itself: Step 2 lands `codex_strategy.go` with no edits to `strategy.go` or any existing `*_strategy.go` file. Validates the open/closed extension point.                       |
| 20  | Falling back to `FlatMarkdownStrategy` for unregistered tools is observable (logged or asserted in tests), preventing silent default-behavior bugs. | Yes          | Add a debug log in `StrategyFor` on fallback; add a unit test that registers a fake tool type and asserts the fallback path is hit. Catches the "forgot to register a strategy" failure mode.                                       |
