package main

import (
	"bufio"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sir-hassan/goi/process"
	"github.com/sir-hassan/goi/ui"
)

type processExitedMsg struct {
	err error
}

type processOutputMsg string

type streamClosedMsg struct{}

type errMsg struct {
	err error
}

type model struct {
	input   textinput.Model
	output  viewport.Model
	table   *ui.Table
	width   int
	height  int
	buffer  strings.Builder
	running bool
	proc    process.Process
	msgs    <-chan tea.Msg
	status  string
}

type processStartedMsg struct {
	proc process.Process
	msgs <-chan tea.Msg
}

const minOutputHeight = 5

var (
	appStyle   = lipgloss.NewStyle().Padding(1)
	boxStyle   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	titleBar   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	statusOK   = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	statusNG   = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	tableData = map[string]string{
		"app":      "goi",
		"runtime":  "bubbletea",
		"usage92":  "%01",
		"usage921":  "%10",
		"usage922":  "%90",
		"usage93":  "%100",
		"usage94":  "%00100",
	}
)

func newModel() *model {
	in := textinput.New()
	in.Focus()
	in.Prompt = "> "
	in.Placeholder = "type a command and press Enter"

	out := viewport.New()
	out.SetContent("stdout will appear here")

	table := ui.NewTable()
	for key, value := range tableData {
		table.SetKeyValue(key, value)
	}

	return &model{
		input:  in,
		output: out,
		table:  table,
		status: "idle",
	}
}

func (m *model) Init() tea.Cmd {
	return textinput.Blink
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		contentWidth := max(20, msg.Width-4)
		tableInnerWidth := contentWidth - boxStyle.GetHorizontalFrameSize()
		m.table.SetWidth(tableInnerWidth)
		outputHeight := max(minOutputHeight, msg.Height-fixedLayoutHeight(m.table))

		m.input.SetWidth(contentWidth - 4)
		m.output.SetWidth(contentWidth - 2)
		m.output.SetHeight(outputHeight)
		m.output.GotoBottom()

	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			m.stopProcess()
			return m, tea.Quit
		case "esc":
			if m.running {
				if err := m.proc.Stop(); err != nil {
					m.status = "stop failed: " + err.Error()
				} else {
					m.status = "stopping..."
				}
				return m, nil
			}
			return m, tea.Quit
		case "enter":
			if m.running {
				line := m.input.Value()
				if err := m.proc.Write(line + "\n"); err != nil {
					m.status = "stdin write failed: " + err.Error()
				}
				m.input.SetValue("")
				return m, nil
			}

			cmdLine := strings.TrimSpace(m.input.Value())
			if cmdLine == "" {
				return m, nil
			}

			m.buffer.Reset()
			m.output.SetContent("")
			m.status = "starting: " + cmdLine
			m.input.SetValue("")

			return m, startProcess(cmdLine)
		}

	case processStartedMsg:
		m.running = true
		m.proc = msg.proc
		m.msgs = msg.msgs
		m.status = "running"
		cmds = append(cmds, waitForStream(msg.msgs))

	case processOutputMsg:
		m.buffer.WriteString(string(msg))
		m.output.SetContent(m.buffer.String())
		m.output.GotoBottom()
		if m.running {
			m.status = "running"
		}
		cmds = append(cmds, waitForStream(m.msgs))

	case processExitedMsg:
		m.running = false
		m.proc = nil
		if msg.err != nil {
			m.status = "process exited: " + msg.err.Error()
		} else {
			m.status = "process exited"
		}
		cmds = append(cmds, waitForStream(m.msgs))

	case streamClosedMsg:
		m.msgs = nil

	case errMsg:
		m.running = false
		m.proc = nil
		m.msgs = nil
		m.status = "error: " + msg.err.Error()
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	cmds = append(cmds, cmd)

	m.output, cmd = m.output.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m *model) View() tea.View {
	commandLabel := "Command"
	if m.running {
		commandLabel = "Stdin"
	}

	contentWidth := max(20, m.width-4)
	m.table.SetWidth(contentWidth - boxStyle.GetHorizontalFrameSize())

	statusStyle := statusOK
	if strings.HasPrefix(m.status, "error") || strings.HasPrefix(m.status, "stop failed") || strings.Contains(m.status, "exited:") {
		statusStyle = statusNG
	}

	body := lipgloss.JoinVertical(
		lipgloss.Left,
		titleBar.Render("goi"),
		boxStyle.Width(contentWidth).Render(m.table.View()),
		boxStyle.Width(contentWidth).Render(m.output.View()),
		boxStyle.Width(contentWidth).Render(m.input.View()),
		fmt.Sprintf("%s  %s", titleBar.Render(commandLabel+":"), statusStyle.Render(m.status)),
		titleBar.Render("Enter: start/send  Esc: stop command or quit  Ctrl+C: quit"),
	)

	view := tea.NewView(appStyle.Render(body))
	view.AltScreen = true
	return view
}

func waitForStream(msgs <-chan tea.Msg) tea.Cmd {
	if msgs == nil {
		return nil
	}

	return func() tea.Msg {
		msg, ok := <-msgs
		if !ok {
			return streamClosedMsg{}
		}
		return msg
	}
}

func (m *model) stopProcess() {
	if m.proc == nil {
		return
	}
	if err := m.proc.Stop(); err != nil {
		m.status = "stop failed: " + err.Error()
	}
}

func fixedLayoutHeight(table *ui.Table) int {
	tableHeight := table.Height() + 2
	inputHeight := 3
	outputBorderHeight := 2
	appPaddingHeight := 2
	titleHeight := 1
	statusHeight := 1
	helpHeight := 1

	return appPaddingHeight + titleHeight + tableHeight + inputHeight + outputBorderHeight + statusHeight + helpHeight
}

func startProcess(command string) tea.Cmd {
	return func() tea.Msg {
		proc := process.New(command)
		events, err := proc.Start()
		if err != nil {
			return errMsg{err: err}
		}

		msgs := make(chan tea.Msg, 128)

		go func() {
			for event := range events {
				if event.Output != "" {
					msgs <- processOutputMsg(event.Output)
				}
				if event.Exited {
					msgs <- processExitedMsg{err: event.Err}
				}
			}
			close(msgs)
		}()

		return processStartedMsg{
			proc: proc,
			msgs: msgs,
		}
	}
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "rand" {
		for {
			fmt.Printf("random: %d\n", rand.Int())
			time.Sleep(300 * time.Millisecond)
		}
	}

	if len(os.Args) > 1 && os.Args[1] == "echo" {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			fmt.Println(scanner.Text())
		}

		if err := scanner.Err(); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
		}
	}

	p := tea.NewProgram(newModel())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "goi failed: %v\n", err)
		os.Exit(1)
	}
}
