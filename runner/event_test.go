//nolint:testpackage // Tests need access to internal types
package runner

import "testing"

func TestAction_IsTerminal(t *testing.T) {
	terminal := map[Action]bool{
		ActionRun:    false,
		ActionPass:   true,
		ActionFail:   true,
		ActionSkip:   true,
		ActionError:  true,
		ActionOutput: false,
		ActionSetup:  false,
	}

	for action, want := range terminal {
		if got := action.IsTerminal(); got != want {
			t.Errorf("%q.IsTerminal() = %v, want %v", action, got, want)
		}
	}
}

func TestEvent_PathString(t *testing.T) {
	tests := []struct {
		path []string
		want string
	}{
		{nil, ""},
		{[]string{"GetUser"}, "GetUser"},
		{[]string{"GetUser", "group", "test"}, "GetUser/group/test"},
	}

	for _, tt := range tests {
		if got := (Event{Path: tt.path}).PathString(); got != tt.want {
			t.Errorf("PathString(%v) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestEvent_TestName(t *testing.T) {
	tests := []struct {
		path []string
		want string
	}{
		{nil, ""},
		{[]string{"GetUser"}, "GetUser"},
		{[]string{"GetUser", "group", "test"}, "test"},
	}

	for _, tt := range tests {
		if got := (Event{Path: tt.path}).TestName(); got != tt.want {
			t.Errorf("TestName(%v) = %q, want %q", tt.path, got, tt.want)
		}
	}
}
