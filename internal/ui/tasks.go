package ui

import (
	"os"
	"strings"
	"sync"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type taskState int

const (
	taskPending taskState = iota
	taskRunning
	taskDone
	taskFailed
)

type task struct {
	label  string
	state  taskState
	detail string
}

type taskMsg struct {
	idx    int
	state  taskState
	detail string
}

// Tasks renders a list of steps with a live spinner next to the ones in
// flight. Without a terminal it prints one line per finished step instead.
// All methods are safe to call from multiple goroutines.
type Tasks struct {
	labels  []string
	width   int
	program *tea.Program
	exited  chan struct{}
	mu      sync.Mutex
}

// StartTasks begins rendering the given steps as pending.
func StartTasks(labels ...string) *Tasks {
	width := 0
	for _, l := range labels {
		width = max(width, lipgloss.Width(l))
	}
	t := &Tasks{labels: labels, width: width, exited: make(chan struct{})}
	if !Animated() {
		close(t.exited)
		return t
	}

	m := tasksModel{
		spinner: spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(Accent)),
		width:   width,
	}
	for _, l := range labels {
		m.tasks = append(m.tasks, task{label: l})
	}
	t.program = tea.NewProgram(m, tea.WithOutput(os.Stderr), tea.WithInput(nil))
	go func() {
		_, _ = t.program.Run()
		close(t.exited)
	}()
	return t
}

// Running marks a step as in progress.
func (t *Tasks) Running(i int, detail string) {
	if t.program != nil {
		t.program.Send(taskMsg{idx: i, state: taskRunning, detail: detail})
	}
}

// Done marks a step as finished.
func (t *Tasks) Done(i int, detail string) {
	t.finish(i, taskDone, detail)
}

// Fail marks a step as failed.
func (t *Tasks) Fail(i int, detail string) {
	t.finish(i, taskFailed, detail)
}

func (t *Tasks) finish(i int, state taskState, detail string) {
	if t.program != nil {
		t.program.Send(taskMsg{idx: i, state: state, detail: detail})
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	eprintln(renderTask(task{label: t.labels[i], state: state, detail: detail}, t.width, ""))
}

// Wait blocks until every step has finished or the user pressed Ctrl+C.
func (t *Tasks) Wait() {
	<-t.exited
}

// Step runs fn behind a spinner and leaves a ✓ or ✗ line behind.
// fn returns a short detail shown next to the label on success.
func Step(label string, fn func() (string, error)) error {
	t := StartTasks(label)
	t.Running(0, "")
	detail, err := fn()
	if err != nil {
		t.Fail(0, "failed")
	} else {
		t.Done(0, detail)
	}
	t.Wait()
	return err
}

type tasksModel struct {
	tasks   []task
	spinner spinner.Model
	width   int
}

func (m tasksModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m tasksModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case taskMsg:
		m.tasks[msg.idx].state = msg.state
		m.tasks[msg.idx].detail = msg.detail
		for _, t := range m.tasks {
			if t.state != taskDone && t.state != taskFailed {
				return m, nil
			}
		}
		return m, tea.Quit
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m tasksModel) View() tea.View {
	var b strings.Builder
	frame := m.spinner.View()
	for _, t := range m.tasks {
		b.WriteString(renderTask(t, m.width, frame))
		b.WriteString("\n")
	}
	return tea.NewView(b.String())
}

func renderTask(t task, width int, frame string) string {
	var mark string
	label := t.label + strings.Repeat(" ", max(0, width-lipgloss.Width(t.label)))
	detail := Muted.Render(t.detail)
	switch t.state {
	case taskPending:
		mark = Muted.Render("·")
		label = Muted.Render(label)
	case taskRunning:
		mark = frame
	case taskDone:
		mark = okMark
	case taskFailed:
		mark = failMark
		detail = lipgloss.NewStyle().Foreground(red).Render(t.detail)
	}
	line := "  " + mark + " " + label
	if t.detail != "" {
		line += "  " + detail
	}
	return line
}
