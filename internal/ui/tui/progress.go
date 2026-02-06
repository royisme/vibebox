package tui

import (
	"fmt"
	"sync"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	p "vibebox/internal/progress"
)

type progressMsg struct {
	event p.Event
}

type progressModel struct {
	bar      progress.Model
	mu       *sync.Mutex
	ch       <-chan p.Event
	last     p.Event
	lastText string
	done     bool
	err      error
}

func newProgressModel(ch <-chan p.Event) progressModel {
	bar := progress.New(progress.WithDefaultGradient())
	bar.Width = 50
	return progressModel{bar: bar, ch: ch, mu: &sync.Mutex{}}
}

func waitEvent(ch <-chan p.Event) tea.Cmd {
	return func() tea.Msg {
		e, ok := <-ch
		if !ok {
			return progressMsg{event: p.Event{Done: true, Phase: p.PhaseCompleted, Message: "done"}}
		}
		return progressMsg{event: e}
	}
}

func (m progressModel) Init() tea.Cmd {
	return waitEvent(m.ch)
}

func (m progressModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case progressMsg:
		m.last = msg.event
		if msg.event.Message != "" {
			m.lastText = msg.event.Message
		}
		if msg.event.Err != nil {
			m.err = msg.event.Err
			m.done = true
			return m, tea.Quit
		}
		if msg.event.Done {
			m.done = true
			return m, tea.Quit
		}
		return m, waitEvent(m.ch)
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.err = tea.ErrInterrupted
			m.done = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m progressModel) View() string {
	if m.err != nil {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(m.err.Error())
	}
	percent := m.last.Percent / 100.0
	if percent < 0 {
		percent = 0
	}
	if percent > 1 {
		percent = 1
	}

	line := fmt.Sprintf("[%s] %s", m.last.Phase, m.lastText)
	status := ""
	if m.last.BytesTotal > 0 {
		status = fmt.Sprintf("%0.1f%%  %s/%s", m.last.Percent, humanBytes(m.last.BytesDone), humanBytes(m.last.BytesTotal))
		if m.last.SpeedBps > 0 {
			status += fmt.Sprintf("  %s/s", humanBytes(int64(m.last.SpeedBps)))
		}
		if m.last.ETA > 0 {
			status += fmt.Sprintf("  ETA %s", m.last.ETA.String())
		}
	} else if m.last.Percent > 0 {
		status = fmt.Sprintf("%0.1f%%", m.last.Percent)
	}

	return "\n" + line + "\n\n" + m.bar.ViewAs(percent) + "\n" + status + "\n"
}

// RunProgress renders progress events in a TUI until completion or failure.
func RunProgress(ch <-chan p.Event) error {
	m := newProgressModel(ch)
	prog := tea.NewProgram(m)
	model, err := prog.Run()
	if err != nil {
		return err
	}
	out := model.(progressModel)
	if out.err != nil {
		return out.err
	}
	return nil
}

func humanBytes(v int64) string {
	const unit = 1024
	if v < unit {
		return fmt.Sprintf("%dB", v)
	}
	div, exp := int64(unit), 0
	for n := v / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%ciB", float64(v)/float64(div), "KMGTPE"[exp])
}
