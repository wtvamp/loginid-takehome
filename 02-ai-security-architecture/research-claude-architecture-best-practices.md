# Research: Anthropic's Own Guidance on Using Claude as an Architecture/Delivery Tool

This document collects Anthropic's official, primary-sourced guidance on agentic coding workflows, context engineering, multi-agent orchestration, prompt caching, and tool-use design — then maps it against how this take-home project (root `CLAUDE.md` + five track `CLAUDE.md` files + scoped subagents) is already structured, and where it should adopt more of Anthropic's own recommended techniques for the work still ahead (the Go DAO, REST API, and IDP connector, plus the other four tracks' deliverables).

Primary sources (Anthropic-published) are called out explicitly as such. Anything third-party is labeled and deprioritized — none of it is treated as authoritative below.

## 1. Claude Code best practices (agentic coding workflows)

**Anthropic official** — "Claude Code: Best Practices for Agentic Coding," https://www.anthropic.com/engineering/claude-code-best-practices (canonical current home: https://code.claude.com/docs/en/best-practices).

Claude Code is deliberately low-level and unopinionated — close to raw model access rather than a fixed workflow. The post's key recommendations:

- `CLAUDE.md` is auto-pulled into context at the start of every session. Use it for things a new contributor would need: bash commands, core files/utilities, code style conventions, testing instructions, repo etiquette. Keep it tight and iterate on it the way you'd iterate on a prompt — it is a living artifact, not a one-time write.
- Favor an explore → plan → code → commit workflow over jumping straight to code, especially for non-trivial problems.
- Use subagents to verify details or investigate side questions without polluting the main agent's context window.
- Course-correct early. Don't let an agent run unsupervised for a long stretch and hope it stays on track — check in and redirect while the cost of doing so is still low.

## 2. Context engineering

**Anthropic official** — "Effective context engineering for AI agents," https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents.

This post frames "context engineering" as the discipline that supersedes prompt engineering for agentic work: the task is no longer just wording one prompt well, but curating the optimal set of tokens available at *each* inference step over a long-running task.

The core technical justification: transformer attention has a finite budget (the mechanism scales roughly quadratically with context length in pairwise token relationships), so as context grows, model attention gets diluted — Anthropic calls this "context rot." This holds regardless of nominal context window size. The governing principle Anthropic states directly: *"find the smallest set of high-signal tokens that maximize the likelihood of your desired outcome."*

Four concrete techniques the post recommends:

1. **Compaction** — as a task approaches the context limit, summarize/distill the conversation history and restart with a condensed version. Preserve architectural decisions and unresolved bugs; discard redundant tool outputs.
2. **Structured note-taking** — persist memory to files or external state outside the active context window (a running progress/notes file) rather than keeping everything live in-context, then re-read it on demand.
3. **Sub-agent architectures** — specialized agents each work in their own clean context window and report back condensed summaries (Anthropic's own multi-agent system, see §3, targets roughly 1,000–2,000 tokens per subagent handoff) to a coordinating agent, rather than the coordinator holding all the raw exploration.
4. **Just-in-time retrieval** — load file paths, queries, and references at runtime instead of pre-loading full documents into context upfront, mirroring how a human keeps lightweight pointers in working memory rather than entire documents.

Anthropic recommends a hybrid: some upfront retrieval for latency/simplicity, plus autonomous just-in-time exploration for anything not needed immediately.

## 3. Multi-agent / subagent orchestration patterns

**Anthropic official** — "How we built our multi-agent research system," https://www.anthropic.com/engineering/multi-agent-research-system.

Describes an orchestrator-worker pattern: a lead agent decomposes a task and spawns parallel subagents, each with a distinct objective, tool scope, and independent trajectory ("separation of concerns... reduces path dependency" — parallel agents don't all inherit the same accumulated context or biases). Subagents report back condensed findings; the lead agent synthesizes and decides whether more work is needed. A separate downstream agent (in their case a "CitationAgent") handles a distinct final pass — staged handoffs using lightweight references rather than raw dumps, specifically to avoid information loss across stages.

Anthropic notes that prompting the orchestrator to delegate *well* — explicit objectives, explicit output format, explicit tool guidance per subagent — mattered as much as the architecture itself. Their internal eval showed a 90.2% improvement over a single-agent Opus baseline using an Opus-lead / Sonnet-subagent configuration, at the cost of materially higher token spend (multi-agent systems use meaningfully more tokens than single-agent chat).

**Anthropic official, product-doc register** — Claude Code subagent mechanism: https://claude.com/blog/subagents-in-claude-code. Project- or user-level `.claude/agents/*.md` files with YAML frontmatter (`name`, `description`, `tools`, `model`) define subagents that each get their own system prompt, their own tool permissions, and their own context window. Intermediate noise (file reads, searches, exploratory tool calls) stays inside the subagent's window; only the final output text returns to the parent's context.

## 4. Prompt caching

**Anthropic official docs** — https://platform.claude.com/docs/en/build-with-claude/prompt-caching.

Up to 4 cache breakpoints per prompt (e.g., after the system prompt, after tool definitions, after a large retrieved document, after few-shot examples), or a single breakpoint at the end of static content with automatic longest-prefix matching applied beneath it. Best practice: order the prompt so stable, reusable content — system instructions, tool definitions, background/reference material — comes first and is cached, with variable, per-turn conversation content appended after the breakpoint. Default cache TTL is 5 minutes, refreshed on every cache hit; a paid 1-hour TTL option exists for longer-lived sessions. Caution: changing `tool_choice`, or adding/removing images anywhere in the prompt, invalidates the cache for that prefix.

## 5. Tool-use design

**Anthropic official** — "Writing effective tools for AI agents," https://www.anthropic.com/engineering/writing-tools-for-agents, plus platform docs at https://platform.claude.com/docs/en/agents-and-tools/tool-use/overview and .../define-tools, and the newer "Advanced tool use" post, https://www.anthropic.com/engineering/advanced-tool-use.

Guidance: tool definitions should be intentional and clearly scoped; avoid many overlapping or near-duplicate tools that force the model to guess which one applies; don't force large schemas or verbose payloads into every call just because they're available; design tools so they compose across workflows rather than being single-use. The "Advanced tool use" post specifically addresses large tool libraries: agents should be able to discover and load tool definitions on demand (deferred/lazy tool schemas) rather than having every possible tool's full schema stuffed into context upfront — directly analogous to this session's own deferred-tool mechanism.

## 6. Claude certification / Anthropic Academy

**Anthropic official, but narrow — do not overclaim.** Anthropic Academy (skilljar-hosted, anthropic.skilljar.com) offers free self-paced courses with completion certificates, including "Building with the Claude API," covering the Messages API, tool use, RAG, and agents. This appears to function as prep material for a separate, proctored "Claude Certification Program" (Associate / Developer / Architect tracks, delivered via Pearson VUE, gated to organizations in the Claude Partner Network — pearsonvue.com/us/en/anthropic.html).

There is no publicly published, granular competency rubric or exam blueprint from Anthropic enumerating exactly what an "Architect"-level credential tests. What's public is course-level topic lists, not a graded competency breakdown. The "Architect" framing (advising on, designing, and building Claude-based solutions) is the closest official language to "using Claude as an architecture tool" — worth citing by name in this track's AI-narrative section — but it should not be characterized as validating specific techniques used in this project, since the exam content itself isn't public.

## 7. Structured memory files (CLAUDE.md) and hand-off documents between sessions

**Anthropic official (Claude Code product docs)** — https://code.claude.com/docs/en/memory.

`CLAUDE.md` is read at the start of every session. Guidance: keep it short and specific, and link out to deeper documentation rather than trying to make the file itself a full manual. It supports project-level, user-level, and local (gitignored) scopes, and is meant to be treated as living documentation that the team edits the way it edits code.

Important distinction to hold onto: **Anthropic has not separately published a named "root CLAUDE.md + per-directory CLAUDE.md fan-out" pattern.** What this project does — a root file plus five track-scoped files, each self-contained enough for a fresh agent to pick up — is a reasonable, well-motivated *composition* of two things Anthropic has documented separately: (a) the single-file CLAUDE.md mechanism itself, and (b) the context-engineering post's general principle of persisting structured memory outside the live context window and retrieving it just-in-time (§2, technique 2 and 4). It should be described in this track's AI narrative as "an application of Anthropic's context-engineering principles to the CLAUDE.md mechanism," not as "Anthropic's documented multi-file pattern" — the latter overclaims a specific endorsement that doesn't exist in the primary sources.

## Applied to this project: concrete recommendations for the work ahead

The remaining work — the Go DAO, the REST API, the IDP connector, plus the other four tracks' deliverables — is exactly the kind of long, multi-phase, multi-context task Anthropic's context-engineering guidance targets. Concrete, actionable steps:

1. **Keep the orchestrator (root-level session) thin; delegate track work to scoped subagents.** This project already does this in spirit via the five-directory structure — formalize it by having each track's real implementation work (writing the DAO, writing the REST handlers, writing the connector) run as a subagent scoped to that track's `CLAUDE.md`, per Anthropic's orchestrator-worker pattern (§3). The root session should hold only cross-track state (the boundaries table, status), never the accumulated exploration/tool noise from any one track.

2. **Use condensed hand-off summaries, not raw transcripts, between tracks.** When `03-engineering-delivery` needs this track's auth/authz design, it should read this track's finished `CLAUDE.md`/deliverable files — not a dump of how this track's subagent arrived at them. Anthropic's own subagents target ~1,000–2,000 tokens of condensed findings passed back to a coordinator (§2, §3); apply the same discipline to inter-track cross-references (the root file's existing rule — "cross-reference other tracks by relative path... rather than restating their content" — is already correctly aligned with this).

3. **Treat each track's `CLAUDE.md` as the "structured note-taking" layer, and keep iterating on it rather than re-deriving context each session.** Per §2, structured notes persisted to files are exactly how Anthropic recommends keeping long tasks context-efficient. Every time a design decision is finalized (e.g., the auth/authz scheme chosen in this track), write it into the track's `CLAUDE.md` or a linked deliverable file immediately, so a future fresh subagent picking up `03-engineering-delivery` doesn't need this session's reasoning replayed — it needs the decision and its rationale, already distilled.

4. **Prefer just-in-time retrieval over pre-loading.** Track subagents should read only the root file plus their own track file at the start (as the root file already instructs), and pull in another track's file only when actually needed for a specific cross-reference — not preemptively load all five tracks "just in case." This matches §2 technique 4 directly.

5. **Exploit prompt caching structurally, not just accidentally.** Since Claude Code sessions re-send the system prompt and tool definitions on every turn, and `CLAUDE.md` content effectively becomes part of that stable prefix, keep `CLAUDE.md` files stable within a working session (avoid mid-session edits to the file currently in context, which would invalidate the cached prefix per §4) — do edits as a deliberate step at the *start* or *end* of a subagent's turn, not interleaved with the rest of its work.

6. **When a track's work legitimately grows long (e.g., iterating on the Go DAO across multiple database backends), compact deliberately rather than letting context degrade.** Per §2 technique 1: before starting the next database backend (SQLite after Postgres, say), have the subagent write a short summary of decisions/interfaces settled so far into a file, and start the next backend's work fresh from that summary rather than carrying the full multi-turn implementation history forward. This is the single most concrete lever available for the DAO/REST/connector work specifically, since it involves several structurally similar but separate implementation passes.

7. **In this track's own AI-narrative deliverable, cite §1–§5 by name and URL, and be precise about §6–§7.** State plainly that the CLAUDE.md-per-track structure is *this project's own extension* of Anthropic's documented context-engineering principles applied to the documented CLAUDE.md mechanism — not a pattern Anthropic separately published or certified. That precision itself is a demonstration of the kind of rigorous "reasoning, not just conclusion" LoginID asked every track to show.

## Source list (primary, Anthropic-official)

- https://www.anthropic.com/engineering/claude-code-best-practices (canonical: https://code.claude.com/docs/en/best-practices)
- https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents
- https://www.anthropic.com/engineering/multi-agent-research-system
- https://claude.com/blog/subagents-in-claude-code
- https://platform.claude.com/docs/en/build-with-claude/prompt-caching
- https://www.anthropic.com/engineering/writing-tools-for-agents
- https://www.anthropic.com/engineering/advanced-tool-use
- https://platform.claude.com/docs/en/agents-and-tools/tool-use/overview
- https://code.claude.com/docs/en/memory
- Anthropic Academy: https://anthropic.skilljar.com (course listings; no public exam competency rubric found)
- Claude Certification Program (Pearson VUE, partner-gated): https://www.pearsonvue.com/us/en/anthropic.html

Third-party explainer posts (Medium/Substack/independent blogs) on subagents, caching internals, and CLAUDE.md conventions were encountered during research and are deliberately excluded from the citations above — they corroborate details but are not treated as authoritative for this track's claims.
