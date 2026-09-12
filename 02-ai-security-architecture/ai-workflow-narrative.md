# AI Tooling & Workflow Narrative

Owner: Marcus Ilori (`02-ai-security-architecture`). This is the full answer to LoginID's explicit ask ("describe any tool, framework, or AI used") — other tracks note their own AI usage briefly and point here for depth, per `../CLAUDE.md`.

This document states what was done, distinguishes what's independently documented by Anthropic from what this project composed on its own, and is deliberately precise about that boundary rather than overclaiming an official pattern that doesn't exist. Full source-by-source detail lives in `research-claude-architecture-best-practices.md`; this document is the applied summary a reviewer can read without the research doc.

## Tool

**Claude Code**, Anthropic's agentic CLI. Chosen because the deliverable itself is an exercise in demonstrating AI-architect judgment — using the tool that is itself the subject of the "how did you use AI" question, rather than a general-purpose chat interface, lets the workflow be inspected as part of the submission.

## Structure: a root brief plus five track-scoped briefs

The repository is organized as one root `CLAUDE.md` (the assignment text, track boundaries, cross-track working principles) plus five track directories, each with its own `CLAUDE.md` scoped to a distinct slice of the problem (product/industry framing, this track's security architecture, engineering delivery, infra/devops, data ops). Each track file is written to be self-contained enough that a fresh agent with no other conversation history can pick it up correctly given only that file plus the root file.

**What this composes, precisely:**

1. **The `CLAUDE.md` mechanism itself** is Anthropic's own, documented at https://code.claude.com/docs/en/memory — auto-loaded at session start, meant to be treated as living documentation, kept short and specific.
2. **The decision to fan that mechanism out per-directory, with a stable root file for cross-cutting state**, is this project's own application of a general principle Anthropic documents separately in "Effective context engineering for AI agents" (https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents): structured notes persisted to files outside the live context window, retrieved just-in-time rather than all loaded upfront, so a track's subagent carries only the ~1-2 files it actually needs rather than the whole project's accumulated context.

I'm stating this distinction explicitly because it matters to how this narrative should be read: Anthropic has not published a named "root CLAUDE.md + per-directory CLAUDE.md" pattern, and claiming they had would be exactly the kind of unsupported assertion this track's whole discipline argues against. What's true and defensible is narrower and still worth the credit — this is a reasonable, motivated composition of two things Anthropic documented separately.

## Orchestration: scoped subagents per track, not one long single-context session

Each track is intended to be worked by an agent (in practice here, a persona-bearing teammate agent per track — see each track's `profiles/` entry) that reads the root file plus its own track file and works within that scope, rather than one agent holding all five tracks' context simultaneously and switching between them. This follows the orchestrator-worker pattern Anthropic describes in "How we built our multi-agent research system" (https://www.anthropic.com/engineering/multi-agent-research-system): a coordinating layer decomposes work into distinct-objective, distinct-tool-scope units; each unit works its own trajectory; only condensed results cross back, not raw exploration. Anthropic's own internal figure for a subagent-to-coordinator handoff is roughly 1,000-2,000 tokens of distilled findings — the discipline this project applies at track boundaries is the same: a track's `CLAUDE.md` plus its finished deliverable files (this document included) are what another track reads, never a transcript of how those conclusions were reached.

Concretely for this track: the threat model, the API auth/authz design, and the connector security design (`threat-model.md`, `api-auth-design.md`, `connector-security.md`) are the condensed artifacts `03-engineering-delivery` needs to implement against. It should read those files, not need this track's reasoning replayed turn-by-turn.

## What was AI-generated vs. human-directed

- **Human-directed**: the assignment itself (LoginID's), the five-track decomposition and ownership boundaries (the root `CLAUDE.md`), and every judgment call in this document and its siblings about *which* security mechanism to choose and why (e.g., OAuth2 client-credentials over static API keys in `api-auth-design.md` — a reasoned choice, not a default).
- **AI-generated, human-directed content**: the research pass that produced `research-claude-architecture-best-practices.md`, and the drafting of this narrative, the threat model, and the design documents themselves, all produced within this track's scope by an agent operating under this track's `CLAUDE.md` and persona brief.
- **Explicitly not claimed**: that any of this constitutes a validated, "certified" methodology. The research doc's §6 covers Anthropic's Claude Certification Program by name and is careful that its existence is not evidence this project's specific techniques are exam-validated — that would be a citation used to imply authority it doesn't carry.

## Why this discipline matters for this specific track

This track's whole mandate is "name assumptions, cite sources, treat security design as a first-class deliverable." Applying the identical standard to the AI-workflow claims — say precisely what's sourced, say precisely what's this project's own extension, don't blur the two — is the same rigor applied to a different subject. A reviewer evaluating "how you use AI tooling" should be able to trust this document's citations exactly as much as they can trust the threat model's.

## Sources

Full list with URLs in `research-claude-architecture-best-practices.md`. Load-bearing ones for this document specifically:

- https://code.claude.com/docs/en/memory
- https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents
- https://www.anthropic.com/engineering/multi-agent-research-system
- https://claude.com/blog/subagents-in-claude-code
