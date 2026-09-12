# Decision: Core Claim of `ai-workflow-narrative.md` — Structured Written Debate (S5)

Pattern: structured written debate, amended per `CASTING.md` §5 ⟨02⟩ — the synthesis must state which position was stronger on which point and why; preserving both positions is necessary, not a substitute for a ruling.

**The claim at issue:** does this project's multi-agent, persona-driven, debate-pattern workflow produce *better design* than one strong session working the same problem alone, or is it cost without evidence — since Anthropic's own cited 90.2% figure (research doc §3) is for research-task breadth, not design quality?

## Position A — Felix Adebayo

The narrative undersells its own strongest evidence by citing only a borrowed research-breadth figure with no example of what this project's own structure bought in design quality, when a concrete one now exists. Two instances, produced inside this track:

1. **S3's alternatives seat.** Tomasz's red-seat pass produced five leaves each shaped like "add a control." Felix's seat existed to ask a structurally different question of the same artifact — not "is this control sufficient" but "does the thing it protects need to exist" — and produced two leaves deleted by removing the cache entirely rather than hardened. A single strong session would have to interrupt its own momentum to ask that question of itself, mid-stream, with nobody else in the room to surface it.
2. **Theo's Newcomer cold-read.** Caught two real gaps in `handoff-04-secrets.md` (TOTP feature-status ambiguity, a non-self-contained trust-boundary reference) that survived the document's own author's review, because an author rereading their own hand-off checks "does this say what I meant," not "does this stand alone for a stranger."

Explicitly declined to stretch: two data points from one track is not "multi-agent design beats solo design" as a general law. The defensible, narrower claim is: for *this specific pattern* — a seat whose brief is explicitly to hold a different optimization target than the seat before it — there are now two instances of it catching something a same-frame or same-author pass structurally could not, and zero instances yet of an extra seat producing nothing but turn cost. Flagged: if S8 comes back clean, that's the honest place the claim should soften, and the narrative should say so if it happens.

## Position B — Ingrid Solano (rebuttal)

"Compared to what" is the whole objection. Both instances were produced by seats this project built specifically to catch these failure modes — Felix's alternatives seat exists in order to propose deleting leaves; Theo's Newcomer exists in order to catch what a same-author read misses. That a role staffed to do X sometimes does X is not evidence multi-agent structure produces better design in general; a single strong session working from an explicit checklist ("can this control's leaf be deleted instead," "get one cold read before shipping") could plausibly produce the same two catches without four personas and a debate protocol — until that comparison is run or estimated, the two are indistinguishable from these data points alone.

**Selection-effect point, the sharper version of the same objection:** the narrative cites the hits and doesn't count the misses in the same table. Tomasz's rebuttal conceded two of three points cleanly in S3 — a turn spent producing no new leaf, no design change, nothing citable as a catch. If a clean concession counts as "the pattern working as intended, no false positive," it belongs in the tally as a neutral result, not silently absent while the hits are cited by name. Honest accounting so far: 2 hits, 1 null.

**On citing S8's outcome:** it hasn't run. Citing its hypothetical result in either direction is the same overclaim this track's discipline exists to prevent, aimed at a future data point instead of a past one.

**What would change her mind:** a same-scope comparison (even informal) showing a single strong session with an explicit deletion-prompt and cold-read step failed to catch what these seats caught, or a third instance outside this track from a role with no structural reason to look.

## Ruling (Marcus Ilori), per point

1. **Whether this evidence supports a general law that multi-agent design beats solo design.** Ingrid is stronger. n=2, both instances produced by roles engineered for exactly this outcome, no baseline comparison run or estimated. The narrative must not claim a general empirical law from this.
2. **Whether the narrative should report the null result (Tomasz's clean concession) alongside the hits.** Ingrid is stronger, and this is adopted outright — an honest tally counts 2 hits, 1 null, not 2 hits with the null silently omitted. This is a completeness fix, not a debatable judgment call.
3. **Whether the mechanism-level argument — a role built to hold a different optimization target, or a reader with zero authorial investment, structurally catches what self-review tends not to — is itself defensible.** Felix is stronger here, and this survives Ingrid's critique because it is not the statistical claim Ingrid is rebutting. "A second reviewer with a different brief catches errors the first reviewer's own frame makes invisible to them" is a structural/mechanism argument (adjacent to well-established human-factors reasoning about independent review, not something this project needs n>2 to assert) rather than an empirical generalization requiring a sample size. The two instances are evidence the mechanism operated here, not evidence of a population-level effect size.
4. **Whether to cite S8's unrun outcome.** Ingrid is stronger, outright. Don't.

**What `ai-workflow-narrative.md` (S9) states as a result:** the defensible claim is the mechanism-level one (point 3), evidenced honestly by the S3 and S7 instances and the S3 null result counted alongside them (point 2), explicitly disclaiming any general statistical law about multi-agent versus solo design quality (point 1), and not citing S8 until it has actually run (point 4). This is Ingrid's discipline applied to Felix's real evidence — not a rejection of either seat's contribution.

---
Model: sonnet (Felix Adebayo, Position A) / sonnet (Ingrid Solano, Position B) / sonnet (Marcus Ilori, lead, per-point synthesis). Turns consumed: 2 + lead (Position A, Position B/rebuttal, lead synthesis) — matches the 3-turns-plus-lead budget in `PLAN.md` §3.
