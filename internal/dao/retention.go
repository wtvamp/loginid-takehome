package dao

import "time"

// RetentionClass matches multi-db-strategy.md §3c exactly.
type RetentionClass string

const (
	RetentionDirect         RetentionClass = "direct"
	RetentionIDPCache       RetentionClass = "idp_cache"
	RetentionIDPCacheOrphan RetentionClass = "idp_cache_orphan"
)

// SweepResult matches multi-db-strategy.md §3c exactly (post-F21-addendum
// batching revision). OldestSurvivingAt is nil only when the class has zero
// rows remaining after this call — never for "nothing was old enough to
// touch" (05's ruling on Oren's F21 review finding #1). It is meaningful
// only when Drained is true: mid-sweep, deletable rows still exist by
// construction, so the oldest survivor reads old on every batch but the
// last — publishing this field from a non-drained result would fire a
// health alert on every sweep, not just a stopped one.
type SweepResult struct {
	RowsExamined      int
	RowsDeleted       int
	OldestSurvivingAt *time.Time
	Drained           bool
}
