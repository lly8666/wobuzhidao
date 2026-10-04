package windowsgui

// WindowState models only GUI/tray ownership. VPN runtime state intentionally
// lives outside this package so closing/minimizing the window can never imply
// stopping the tunnel by accident.
type WindowState struct {
	Visible bool
	Exiting bool
}

func NewWindowState(startMinimized bool) WindowState {
	return WindowState{Visible: !startMinimized}
}

// Minimize hides the top-level window while keeping the process and tray icon
// alive.
func (s *WindowState) Minimize() {
	if s.Exiting {
		return
	}
	s.Visible = false
}

// Close is deliberately tray-minimize semantics. The only path that requests
// process termination is Exit.
func (s *WindowState) Close() { s.Minimize() }

// Restore is used by tray activation and the Show menu command.
func (s *WindowState) Restore() {
	if s.Exiting {
		return
	}
	s.Visible = true
}

func (s *WindowState) ToggleVisibility() {
	if s.Visible {
		s.Minimize()
		return
	}
	s.Restore()
}

func (s *WindowState) Exit() {
	s.Exiting = true
	s.Visible = false
}
