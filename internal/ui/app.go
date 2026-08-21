package ui

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rakyll/ate-watch/internal/watcher"
	"github.com/rakyll/ate-watch/pkg/proto/ateapipb"
)

// AppMode represents the active screen in the TUI.
type AppMode int

const (
	ModeTable AppMode = iota
	ModeDescribe
)

type pollMsg struct {
	snap *watcher.Snapshot
	err  error
}

type tickMsg time.Time

// Model is the Bubble Tea TUI model.
type Model struct {
	ctx           context.Context
	watcher       *watcher.Watcher
	opts          RenderOptions
	snapshot      *watcher.Snapshot
	selectedIndex int
	mode          AppMode
	err           error
	quitting      bool
}

// NewModel initializes the Bubble Tea application model.
func NewModel(ctx context.Context, w *watcher.Watcher, opts RenderOptions) Model {
	return Model{
		ctx:           ctx,
		watcher:       w,
		opts:          opts,
		selectedIndex: 0,
		mode:          ModeTable,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.pollCmd(),
	)
}

func (m Model) pollCmd() tea.Cmd {
	return func() tea.Msg {
		snap, err := m.watcher.Poll(m.ctx)
		return pollMsg{snap: snap, err: err}
	}
}

func (m Model) tickCmd() tea.Cmd {
	return tea.Tick(m.opts.Interval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case pollMsg:
		if msg.err != nil && msg.snap == nil {
			m.err = msg.err
		} else if msg.snap != nil {
			m.snapshot = msg.snap
			if len(m.snapshot.Actors) > 0 {
				if m.selectedIndex >= len(m.snapshot.Actors) {
					m.selectedIndex = len(m.snapshot.Actors) - 1
				}
				if m.selectedIndex < 0 {
					m.selectedIndex = 0
				}
			} else {
				m.selectedIndex = 0
				if m.mode == ModeDescribe {
					m.mode = ModeTable
				}
			}
		}
		return m, m.tickCmd()

	case tickMsg:
		return m, m.pollCmd()

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit

		case "esc":
			if m.mode == ModeDescribe {
				m.mode = ModeTable
				return m, nil
			}
			m.quitting = true
			return m, tea.Quit

		case "backspace":
			if m.mode == ModeDescribe {
				m.mode = ModeTable
				return m, nil
			}

		case "up", "k":
			if m.mode == ModeTable && m.selectedIndex > 0 {
				m.selectedIndex--
			}

		case "down", "j":
			if m.mode == ModeTable && m.snapshot != nil && m.selectedIndex < len(m.snapshot.Actors)-1 {
				m.selectedIndex++
			}

		case "d", "enter":
			if m.mode == ModeTable {
				if m.snapshot != nil && len(m.snapshot.Actors) > 0 {
					m.mode = ModeDescribe
				}
			} else {
				m.mode = ModeTable
			}
		}
	}

	return m, nil
}

func (m Model) View() string {
	if m.quitting {
		return ""
	}

	var md string
	if m.mode == ModeDescribe {
		var selectedActor *ateapipb.Actor
		if m.snapshot != nil && len(m.snapshot.Actors) > m.selectedIndex {
			selectedActor = m.snapshot.Actors[m.selectedIndex].Actor
		}
		md = BuildDescribeMarkdown(selectedActor)
	} else {
		opts := m.opts
		opts.SelectedIndex = m.selectedIndex
		if m.snapshot == nil {
			if m.err != nil {
				md = fmt.Sprintf("> **Error:** %v\n\n_Retrying in %s..._\n", m.err, m.opts.Interval)
			} else {
				md = "_Connecting to Substrate control plane..._\n"
			}
		} else {
			md = BuildMarkdown(m.snapshot, opts)
		}
	}

	out, _ := RenderMarkdown(md, m.opts.UseColor)
	return out
}

// RunApp launches the interactive Bubble Tea application.
func RunApp(ctx context.Context, w *watcher.Watcher, opts RenderOptions) error {
	m := NewModel(ctx, w, opts)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
