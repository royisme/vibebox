package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"vibebox/internal/image"
)

type imageItem struct {
	desc image.Descriptor
}

func (i imageItem) FilterValue() string { return i.desc.ID }
func (i imageItem) Title() string       { return i.desc.DisplayName }
func (i imageItem) Description() string {
	sizeMB := float64(i.desc.SizeBytes) / 1024.0 / 1024.0
	return fmt.Sprintf("%s | version=%s | %.1f MB", i.desc.ID, i.desc.Version, sizeMB)
}

type selectModel struct {
	list   list.Model
	chosen *image.Descriptor
	err    error
}

func newSelectModel(images []image.Descriptor) selectModel {
	items := make([]list.Item, 0, len(images))
	for _, img := range images {
		items = append(items, imageItem{desc: img})
	}

	l := list.New(items, list.NewDefaultDelegate(), 80, 20)
	l.Title = "Select VM base image"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)

	return selectModel{list: l}
}

func (m selectModel) Init() tea.Cmd { return nil }

func (m selectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			m.err = tea.ErrInterrupted
			return m, tea.Quit
		case "enter":
			item, ok := m.list.SelectedItem().(imageItem)
			if !ok {
				m.err = fmt.Errorf("no image selected")
				return m, tea.Quit
			}
			picked := item.desc
			m.chosen = &picked
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m selectModel) View() string {
	if m.err != nil && m.err != tea.ErrInterrupted {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(m.err.Error())
	}
	return "\n" + m.list.View()
}

// SelectImage runs a bubbletea selector for image choice.
func SelectImage(images []image.Descriptor) (image.Descriptor, error) {
	if len(images) == 0 {
		return image.Descriptor{}, fmt.Errorf("no images available")
	}

	m := newSelectModel(images)
	prog := tea.NewProgram(m)
	model, err := prog.Run()
	if err != nil {
		return image.Descriptor{}, err
	}
	out := model.(selectModel)
	if out.err != nil {
		return image.Descriptor{}, out.err
	}
	if out.chosen == nil {
		return image.Descriptor{}, fmt.Errorf("selection canceled")
	}
	return *out.chosen, nil
}
