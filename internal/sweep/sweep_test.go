package sweep

import (
	"context"
	"errors"
	"testing"
	"time"

	"loginid-takehome/internal/dao"
)

// fakeExpirySweeper is a minimal hand-written fake, per
// decisions/test-double-strategy.md — scripted per-class batch
// sequences, so tests can assert exactly how many DeleteExpired calls
// happened and in what order.
type fakeExpirySweeper struct {
	// batches[class] is consumed in order, one entry per call to
	// DeleteExpired for that class. Running out of scripted batches for
	// a class is a test-authoring error (panics), not a valid "keep
	// looping" signal — every test scripts exactly the batches it
	// expects to be consumed.
	batches map[dao.RetentionClass][]batchResult
	calls   map[dao.RetentionClass]int
}

type batchResult struct {
	result dao.SweepResult
	err    error
}

func newFakeExpirySweeper() *fakeExpirySweeper {
	return &fakeExpirySweeper{
		batches: make(map[dao.RetentionClass][]batchResult),
		calls:   make(map[dao.RetentionClass]int),
	}
}

func (f *fakeExpirySweeper) script(class dao.RetentionClass, batches ...batchResult) {
	f.batches[class] = batches
}

func (f *fakeExpirySweeper) DeleteExpired(_ context.Context, class dao.RetentionClass, _ time.Time, _ int) (dao.SweepResult, error) {
	i := f.calls[class]
	f.calls[class] = i + 1
	scripted := f.batches[class]
	if i >= len(scripted) {
		panic("fakeExpirySweeper: ran out of scripted batches for class " + string(class))
	}
	b := scripted[i]
	return b.result, b.err
}

// fakeMetricEmitter records every EmitClassMetrics call, so tests can
// assert exactly which classes got metrics and with what values —
// including, critically, that a non-drained batch never triggers a call
// at all.
type fakeMetricEmitter struct {
	calls []emittedMetric
}

type emittedMetric struct {
	class                        dao.RetentionClass
	rowsExamined, rowsDeleted    int
	oldestSurvivingRowAgeSeconds *float64
}

func (f *fakeMetricEmitter) EmitClassMetrics(class dao.RetentionClass, rowsExamined, rowsDeleted int, oldestSurvivingRowAgeSeconds *float64) {
	f.calls = append(f.calls, emittedMetric{class, rowsExamined, rowsDeleted, oldestSurvivingRowAgeSeconds})
}

// allDrainedFake scripts every class to drain in exactly one batch —
// the common case most tests build on before overriding one class's
// script for their own specific scenario.
func allDrainedFake() *fakeExpirySweeper {
	f := newFakeExpirySweeper()
	for _, c := range allClasses {
		f.script(c, batchResult{result: dao.SweepResult{RowsExamined: 1, RowsDeleted: 1, Drained: true}})
	}
	return f
}

func TestRun_LoopsUntilDrained(t *testing.T) {
	f := allDrainedFake()
	// direct takes 3 batches before draining — RowsExamined/RowsDeleted
	// must sum across all 3, not just report the last batch's numbers.
	f.script(dao.RetentionDirect,
		batchResult{result: dao.SweepResult{RowsExamined: 100, RowsDeleted: 100, Drained: false}},
		batchResult{result: dao.SweepResult{RowsExamined: 100, RowsDeleted: 100, Drained: false}},
		batchResult{result: dao.SweepResult{RowsExamined: 40, RowsDeleted: 40, Drained: true}},
	)
	emit := &fakeMetricEmitter{}

	results := Run(context.Background(), f, time.Now(), emit)

	var direct ClassResult
	for _, r := range results {
		if r.Class == dao.RetentionDirect {
			direct = r
		}
	}
	if !direct.Drained {
		t.Fatalf("direct.Drained = false, want true")
	}
	if direct.RowsExamined != 240 || direct.RowsDeleted != 240 {
		t.Errorf("direct RowsExamined/RowsDeleted = %d/%d, want 240/240 (summed across all 3 batches)", direct.RowsExamined, direct.RowsDeleted)
	}
	if f.calls[dao.RetentionDirect] != 3 {
		t.Errorf("DeleteExpired called %d times for direct, want 3 (one per scripted batch)", f.calls[dao.RetentionDirect])
	}
}

func TestRun_EveryClassCalledOnceEachWithItsOwnPredicate(t *testing.T) {
	f := allDrainedFake()
	Run(context.Background(), f, time.Now(), &fakeMetricEmitter{})

	for _, c := range allClasses {
		if f.calls[c] != 1 {
			t.Errorf("class %q called %d times, want exactly 1 (each class drains in its own scripted single batch)", c, f.calls[c])
		}
	}
	if len(f.calls) != len(allClasses) {
		t.Errorf("DeleteExpired called for %d distinct classes, want exactly %d — never a mixed/shared predicate", len(f.calls), len(allClasses))
	}
}

func TestRun_MetricsOnlyEmittedOnDrained(t *testing.T) {
	f := newFakeExpirySweeper()
	f.script(dao.RetentionDirect,
		batchResult{result: dao.SweepResult{RowsExamined: 5, RowsDeleted: 5, Drained: false}},
		batchResult{result: dao.SweepResult{RowsExamined: 5, RowsDeleted: 5, Drained: true}},
	)
	f.script(dao.RetentionIDPCache, batchResult{result: dao.SweepResult{RowsExamined: 1, RowsDeleted: 1, Drained: true}})
	f.script(dao.RetentionIDPCacheOrphan, batchResult{result: dao.SweepResult{RowsExamined: 1, RowsDeleted: 1, Drained: true}})
	emit := &fakeMetricEmitter{}

	Run(context.Background(), f, time.Now(), emit)

	if len(emit.calls) != len(allClasses) {
		t.Fatalf("EmitClassMetrics called %d times, want %d (exactly once per class, only on the drained batch — the direct class's first, non-drained batch must not have triggered a call)", len(emit.calls), len(allClasses))
	}
	for _, c := range emit.calls {
		if c.class == dao.RetentionDirect && (c.rowsExamined != 10 || c.rowsDeleted != 10) {
			t.Errorf("direct metrics = examined=%d deleted=%d, want 10/10 (summed across both batches, emitted once at Drained)", c.rowsExamined, c.rowsDeleted)
		}
	}
}

func TestRun_OldestSurvivingAtNilWhenNoRowsRemain(t *testing.T) {
	f := allDrainedFake() // default: OldestSurvivingAt nil on every class
	emit := &fakeMetricEmitter{}

	Run(context.Background(), f, time.Now(), emit)

	for _, c := range emit.calls {
		if c.oldestSurvivingRowAgeSeconds != nil {
			t.Errorf("class %q: oldestSurvivingRowAgeSeconds = %v, want nil (SweepResult.OldestSurvivingAt was nil)", c.class, *c.oldestSurvivingRowAgeSeconds)
		}
	}
}

func TestRun_OldestSurvivingAtConvertedToAgeSeconds(t *testing.T) {
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	oldest := now.Add(-2 * time.Hour)
	f := newFakeExpirySweeper()
	f.script(dao.RetentionDirect, batchResult{result: dao.SweepResult{RowsExamined: 1, RowsDeleted: 0, Drained: true, OldestSurvivingAt: &oldest}})
	f.script(dao.RetentionIDPCache, batchResult{result: dao.SweepResult{Drained: true}})
	f.script(dao.RetentionIDPCacheOrphan, batchResult{result: dao.SweepResult{Drained: true}})
	emit := &fakeMetricEmitter{}

	Run(context.Background(), f, now, emit)

	var got *float64
	for _, c := range emit.calls {
		if c.class == dao.RetentionDirect {
			got = c.oldestSurvivingRowAgeSeconds
		}
	}
	if got == nil {
		t.Fatalf("direct: oldestSurvivingRowAgeSeconds = nil, want ~7200 (2 hours)")
	}
	if *got != 2*60*60 {
		t.Errorf("direct: oldestSurvivingRowAgeSeconds = %v, want 7200", *got)
	}
}

func TestRun_AlreadyCancelledContextDeletesNothing(t *testing.T) {
	f := allDrainedFake() // would panic if DeleteExpired is ever actually called, past the first cancellation check
	emit := &fakeMetricEmitter{}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	results := Run(ctx, f, time.Now(), emit)

	for _, r := range results {
		if r.Err == nil {
			t.Errorf("class %q: Err = nil, want context.Canceled — an already-cancelled context must stop before deleting anything", r.Class)
		}
		if !errors.Is(r.Err, context.Canceled) {
			t.Errorf("class %q: Err = %v, want context.Canceled", r.Class, r.Err)
		}
	}
	for c, n := range f.calls {
		if n != 0 {
			t.Errorf("class %q: DeleteExpired called %d times against an already-cancelled context, want 0", c, n)
		}
	}
	if len(emit.calls) != 0 {
		t.Errorf("EmitClassMetrics called %d times, want 0 — nothing drained, nothing to report", len(emit.calls))
	}
}

func TestRun_OneClassErrorDoesNotBlockTheOthers(t *testing.T) {
	f := allDrainedFake()
	f.script(dao.RetentionIDPCache, batchResult{err: errors.New("connection reset by peer")})
	emit := &fakeMetricEmitter{}

	results := Run(context.Background(), f, time.Now(), emit)

	byClass := make(map[dao.RetentionClass]ClassResult)
	for _, r := range results {
		byClass[r.Class] = r
	}
	if byClass[dao.RetentionIDPCache].Err == nil {
		t.Errorf("idp_cache: Err = nil, want the scripted error")
	}
	if !byClass[dao.RetentionDirect].Drained || !byClass[dao.RetentionIDPCacheOrphan].Drained {
		t.Errorf("direct/idp_cache_orphan must still complete even though idp_cache failed: %+v / %+v", byClass[dao.RetentionDirect], byClass[dao.RetentionIDPCacheOrphan])
	}

	for _, c := range emit.calls {
		if c.class == dao.RetentionIDPCache {
			t.Errorf("idp_cache metrics were emitted despite the class ending in error — a failed class must never report a (misleadingly partial) metric")
		}
	}
}

func TestStdoutMetricEmitter_DoesNotPanic(t *testing.T) {
	age := 3600.0
	StdoutMetricEmitter{}.EmitClassMetrics(dao.RetentionDirect, 10, 5, &age)
	StdoutMetricEmitter{}.EmitClassMetrics(dao.RetentionIDPCacheOrphan, 0, 0, nil)
}
