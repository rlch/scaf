package runner //nolint:testpackage

import (
	"context"
	"errors"
	"testing"
)

var _ Handler = (*mockHandler)(nil)

type mockHandler struct {
	events []Event
	errs   []string
	err    error
}

func (m *mockHandler) Event(_ context.Context, event Event, _ *Result) error {
	m.events = append(m.events, event)

	return m.err
}

func (m *mockHandler) Err(text string) error {
	m.errs = append(m.errs, text)

	return nil
}

func TestMultiHandler_Event(t *testing.T) {
	h1, h2 := &mockHandler{}, &mockHandler{}
	multi := NewMultiHandler(h1, h2)

	event := Event{Action: ActionPass, Path: []string{"Test1"}}

	_ = multi.Event(context.Background(), event, NewResult())

	if len(h1.events) != 1 || len(h2.events) != 1 {
		t.Error("event not dispatched to all handlers")
	}
}

func TestMultiHandler_StopsOnError(t *testing.T) {
	h1 := &mockHandler{err: errTestStop}
	h2 := &mockHandler{}
	multi := NewMultiHandler(h1, h2)

	err := multi.Event(context.Background(), Event{}, NewResult())

	if !errors.Is(err, errTestStop) {
		t.Errorf("got %v, want errTestStop", err)
	}

	if len(h2.events) != 0 {
		t.Error("second handler should not receive event")
	}
}

func TestResultHandler(t *testing.T) {
	h := NewResultHandler()
	result := NewResult()

	_ = h.Event(context.Background(), Event{Action: ActionPass, Path: []string{"Test1"}}, result)

	if result.Total != 1 {
		t.Error("terminal event not added")
	}

	_ = h.Event(context.Background(), Event{Action: ActionOutput, Path: []string{"Test1"}, Output: "log"}, result)

	if len(result.Tests["Test1"].Output) != 1 {
		t.Error("output not added")
	}
}

func TestStopOnFailHandler(t *testing.T) {
	h := NewStopOnFailHandler(2)
	result := NewResult()

	result.Add(Event{Action: ActionFail, Path: []string{"Test1"}})

	err := h.Event(context.Background(), Event{Action: ActionFail}, result)
	if err != nil {
		t.Error("should not stop on first failure")
	}

	result.Add(Event{Action: ActionFail, Path: []string{"Test2"}})

	err = h.Event(context.Background(), Event{Action: ActionFail}, result)

	if !errors.Is(err, ErrMaxFailures) {
		t.Errorf("got %v, want ErrMaxFailures", err)
	}
}

func TestStopOnFailHandler_Disabled(t *testing.T) {
	h := NewStopOnFailHandler(0)
	result := NewResult()

	for range 10 {
		result.Add(Event{Action: ActionFail, Path: []string{"Test"}})

		err := h.Event(context.Background(), Event{Action: ActionFail}, result)
		if err != nil {
			t.Error("should never stop when disabled")
		}
	}
}
