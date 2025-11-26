package runner //nolint:testpackage

import (
	"testing"
	"time"
)

func TestResult_Add(t *testing.T) {
	r := NewResult()

	r.Add(Event{Action: ActionRun, Path: []string{"Test1"}})

	if r.Total != 0 {
		t.Error("non-terminal event should not be counted")
	}

	r.Add(Event{Action: ActionPass, Path: []string{"Test1"}})
	r.Add(Event{Action: ActionFail, Path: []string{"Test2"}, Field: "x", Expected: 1, Actual: 2})
	r.Add(Event{Action: ActionSkip, Path: []string{"Test3"}})
	r.Add(Event{Action: ActionError, Path: []string{"Test4"}})

	if r.Total != 4 {
		t.Errorf("Total = %d, want 4", r.Total)
	}

	if r.Passed != 1 || r.Failed != 1 || r.Skipped != 1 || r.Errors != 1 {
		t.Errorf("counts = %d/%d/%d/%d, want 1/1/1/1", r.Passed, r.Failed, r.Skipped, r.Errors)
	}

	tr := r.Tests["Test2"]
	if tr.Field != "x" || tr.Expected != 1 || tr.Actual != 2 {
		t.Error("failure details not stored")
	}
}

func TestResult_Ok(t *testing.T) {
	r := NewResult()

	if !r.Ok() {
		t.Error("empty result should be Ok")
	}

	r.Add(Event{Action: ActionPass, Path: []string{"Test1"}})
	r.Add(Event{Action: ActionSkip, Path: []string{"Test2"}})

	if !r.Ok() {
		t.Error("passed+skipped should be Ok")
	}

	r.Add(Event{Action: ActionFail, Path: []string{"Test3"}})

	if r.Ok() {
		t.Error("failed should not be Ok")
	}
}

func TestResult_FailedTests(t *testing.T) {
	r := NewResult()
	r.Add(Event{Action: ActionPass, Path: []string{"Test1"}})
	r.Add(Event{Action: ActionFail, Path: []string{"Test2"}})
	r.Add(Event{Action: ActionError, Path: []string{"Test3"}})

	failed := r.FailedTests()

	if len(failed) != 2 {
		t.Errorf("len(FailedTests()) = %d, want 2", len(failed))
	}

	if failed[0].PathString() != "Test2" || failed[1].PathString() != "Test3" {
		t.Error("wrong order or paths")
	}
}

func TestResult_Elapsed(t *testing.T) {
	r := NewResult()

	time.Sleep(5 * time.Millisecond)
	r.Finish()

	e1 := r.Elapsed()

	time.Sleep(5 * time.Millisecond)

	e2 := r.Elapsed()

	if e1 != e2 {
		t.Error("elapsed should be fixed after Finish")
	}

	if e1 < 5*time.Millisecond {
		t.Error("elapsed should be at least 5ms")
	}
}
