# Decision: the `Search()` query shape

Pattern: **adversarial pair** (`../../CASTING.md` §5, named for this decision by name). Decision owner: Priya Nandakumar, 05-data-ops. Story: S2 in `../PLAN.md`. Feeds: S1.

**Question.** The `ProfileQuery` and result shape for `ProfileRepository.Search()` — field set, per-field match semantics, filter combination, pagination, sort, what `total` counts, and how the SQLite parity gap surfaces to a caller.

**Why it was worth three turns.** `Search()` is the flagship query of this contract and the only genuinely open design question in it. Every other DAO method is settled by the assignment text. This one had the most ways to be quietly wrong, and eight of them were found.

## Ruling: adopt the revised proposal

Proposer: Anders Vogel. Skeptic: Yusuf Karadag, eight numbered objections. **All eight accepted and revised; none declined.** That is an unusually clean sweep, and it is a fact about the objections rather than about the proposer — six of the eight identified something the proposal asserted but did not enforce.

```go
type ProfileQuery struct {
    Name    *string // optional: case-folded substring match, caller input escaped
    Phone   *string // optional: exact match on the E.164-normalized value
    Region  *string // optional: case-folded exact match
    Country *string // optional: exact match, ISO 3166-1 alpha-2, upper-cased by the DAO
    Limit   int     // 1..100; 0 means "use default 50", deliberately
    Offset  int     // 0..10_000
}

Search(ctx context.Context, q ProfileQuery) (results []UserProfile, total int, err error)
```

**Settled semantics:**

- **Absent vs. empty.** `nil` = absent (don't filter). Non-nil but empty-after-trim is a caller bug → `ErrInvalidQuery`. Coercing `""` to "match everything" is how a full-table scan reaches production.
- **Combination.** AND across present filters. No OR, no negation.
- **Empty query.** Legal, returns everything paginated. Whether a caller *may* do that is authorization policy owned by `../../02-ai-security-architecture/`; the DAO is the wrong layer to invent one, and the pagination ceiling is protection, not a policy veto.
- **Case folding.** `LOWER(column) LIKE LOWER(pattern)`, applied explicitly in both implementations. Residual accepted gap: PostgreSQL's `lower()` is locale-aware, SQLite's is ASCII-only, so non-ASCII folding still diverges. Narrow and named, unlike the ASCII divergence it replaces.
- **Escaping.** The DAO escapes `%`, `_` and `\` in the caller's term and emits an explicit `ESCAPE '\'` on **both** backends, rather than relying on PostgreSQL's default and SQLite's absence of one.
- **Collation and sort.** `ORDER BY LOWER(name) COLLATE "C" ASC, id ASC`, fixed, not caller-configurable, with collation named in DDL as well — `COLLATE "C"` on PostgreSQL, `BINARY` on SQLite. Stated cost, not hidden: `C` is byte order, so non-English names sort in an order a human would call wrong. **Identical-across-backends is chosen over locale-correct**, because a POC that returns different page-1 rows per backend is broken in a way that a POC sorting diacritics oddly is not.
- **Trigram index is an accelerator, never a ranker.** It accelerates the same `LIKE` predicate; no `similarity()` scoring, no rank ordering. This is what keeps the backends' row sets and order identical for ASCII input and confines the parity gap to speed. Adding ranking re-opens this decision — it does not get added quietly inside the `postgres` package.
- **Index.** Expression index `GIN (LOWER(name) gin_trgm_ops)`, not the bare column. **The predicate expression must match the index expression textually**, so `LOWER(name) LIKE ?` is the only permitted form; a future `name ILIKE ?` silently loses the index.
- **Pagination.** Limit/offset. Max page 100, default 50, offset ceiling 10 000. `Limit < 0` or `> 100`, and `Offset < 0` or `> 10_000`, return `ErrInvalidQuery`. Keyset rejected as designing for an uncommitted dataset size. Stated cost: offset pages are unstable under concurrent inserts — a row can shift or repeat across pages.
- **`total`.** Exact count of all matching rows, ignoring limit/offset, via a second `COUNT(*)` in no wrapping transaction, so it may disagree with the page by a row under concurrent writes. **`total` is a magnitude, not a promise of reachability** — it can exceed what the offset ceiling allows a caller to page to.
- **`Country`.** `CHECK (country = upper(country) AND length(country) = 2)` plus write-side normalization in both backends. SQLite's `upper()` is ASCII-only — sufficient for A–Z alpha-2 codes, written down so nobody assumes it generalizes.
- **Out of scope, stated rather than left silent:** searching for *absent* address values. `nil` means don't filter and `""` is an error, so no legal query reaches rows with a NULL `region`. Callers needing that list and filter client-side. This matters more than it sounds — partial addresses from `/identity` are the normal case for `idp_cache` rows, not an edge case.
- **Transactions.** `Search()` is read-only, two independent statements, no explicit transaction, so it never encounters a client-retryable `40001`. The CockroachDB retry seam (`../research-cockroachdb-postgres-semantics.md`) is scoped to `Update()`, which is S1's problem.
- **Sentinels.** `ErrInvalidQuery` joins the S1 set, triggers enumerated: empty-after-trim filter, `Limit` out of range, `Offset` negative or above ceiling, `Country` not two characters.

## The general principle — the most valuable output of this run

Objections 1, 2, 3 and 8 are one failure repeated: **a guarantee asserted at the interface with no enforcement site in the schema.** Interface prose enforces nothing.

**Adopted into S1 as a requirement.** Every caller-visible guarantee in the contract carries a named enforcement site — a DDL constraint, an index definition, a normalization step, or a conformance test. *A guarantee with no site listed is not a guarantee, it is a hope.* S1 will carry this as a table, so the ninth instance is caught by the table rather than by a reviewer.

This generalizes the `LIKE` finding from S3 and both of this run's worst defects, and it is worth more to 03 than the eight individual patches are.

## Lead's addition at synthesis

One point neither seat raised, added on my own authority rather than routed back for another turn: **no index supports the ordering.** A GIN trigram index cannot serve `ORDER BY`, so every `Search()` sorts its filtered set. Acceptable at this scale and for this POC, and explicitly *not* worth adding a second B-tree on `LOWER(name)` to fix — but it is a caller-visible performance characteristic and therefore, by the principle just adopted, needs its enforcement-site row to read "none — accepted." A guarantee we are choosing not to make still gets written down.

## Objections and their disposition

All eight objections, each closing with an explicit "what would change my mind" as `../../CASTING.md` §5 ⟨02⟩ requires, and all eight answered in writing. Full text in both hires' returned results. Disposition:

| # | Objection | Disposition |
|---|---|---|
| 1 | `ORDER BY name` differs per backend (PG collation vs SQLite `BINARY` byte order); `id` tiebreak makes order total, not identical | **Accepted** — collation named in DDL and `ORDER BY` |
| 2 | `LOWER(name) LIKE` cannot use a GIN index on the bare column; accelerator accelerates nothing | **Accepted** — expression index; predicate must match textually |
| 3 | Caller input unescaped in `LIKE`; escape character differs across engines | **Accepted** — DAO escapes, explicit `ESCAPE '\'` both backends |
| 4 | No legal query can reach rows with a missing `region` | **Accepted as framed** — the objection was to the silence, not the choice; one line added |
| 5 | `total` can report a count the offset ceiling forbids reaching | **Accepted** — `total` restated as a magnitude |
| 6 | `Limit int` collapses "unspecified" and "zero"; negatives unaddressed | **Accepted** — validation rule stated; `int` retained over `*int` because `0` is not a meaningful page size and is therefore free as a sentinel, whereas `""` is a meaningful string, which is why filters need pointers |
| 7 | `ErrInvalidQuery` not in S1's sentinel set | **Accepted** — added with triggers |
| 8 | `Country` upper-cased into the query but not into the table | **Accepted** — `CHECK` constraint plus write-side normalization |

Objection 2 is the one I was independently watching for and deliberately did not feed to the Skeptic seat, to keep the pattern's integrity. He reached it unaided, which is evidence the seat is doing real work rather than confirming the lead's prior.

---

*Pattern footer (`../../CASTING.md` §5 ⟨02⟩). Seats: Proposer Anders Vogel — sonnet, 2 turns (proposal, answers); Skeptic Yusuf Karadag — sonnet, 1 turn (eight numbered objections); ruling Priya Nandakumar — Claude Opus 5. Hire turns consumed: 3. Objections raised: 8. Accepted: 8. Declined: 0.*
