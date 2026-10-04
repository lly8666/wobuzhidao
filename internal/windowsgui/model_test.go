package windowsgui

import "testing"

func TestCloseAndMinimizeKeepProcessAlive(t *testing.T) {
	s := NewWindowState(false)
	if !s.Visible || s.Exiting {
		t.Fatalf("initial state = %+v", s)
	}

	s.Minimize()
	if s.Visible || s.Exiting {
		t.Fatalf("minimize state = %+v", s)
	}

	s.Restore()
	if !s.Visible || s.Exiting {
		t.Fatalf("restore state = %+v", s)
	}

	s.Close()
	if s.Visible || s.Exiting {
		t.Fatalf("close-to-tray state = %+v", s)
	}
}

func TestExplicitExitIsOnlyTerminalAction(t *testing.T) {
	s := NewWindowState(true)
	if s.Visible || s.Exiting {
		t.Fatalf("start-minimized state = %+v", s)
	}

	s.Restore()
	s.Exit()
	if s.Visible || !s.Exiting {
		t.Fatalf("exit state = %+v", s)
	}

	// Terminal state cannot be resurrected by a stale tray/window message.
	s.Restore()
	if s.Visible || !s.Exiting {
		t.Fatalf("restored terminal state = %+v", s)
	}
}

func TestToggleVisibility(t *testing.T) {
	s := NewWindowState(false)
	s.ToggleVisibility()
	if s.Visible {
		t.Fatal("toggle should hide")
	}
	s.ToggleVisibility()
	if !s.Visible {
		t.Fatal("toggle should restore")
	}
}
