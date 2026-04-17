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

var (
	appStyle = lipgloss.NewStyle().Padding(1)
	boxStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	titleBar = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	statusOK = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	statusNG = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
)

func newModel() *model {
	in := textinput.New()
	in.Focus()
	in.Prompt = "> "
	in.Placeholder = "type a command and press Enter"

	out := viewport.New()
	out.SetContent("stdout will appear here")

	return &model{
		input:  in,
		output: out,
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
		outputHeight := max(5, msg.Height-8)

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

	statusStyle := statusOK
	if strings.HasPrefix(m.status, "error") || strings.HasPrefix(m.status, "stop failed") || strings.Contains(m.status, "exited:") {
		statusStyle = statusNG
	}

	body := lipgloss.JoinVertical(
		lipgloss.Left,
		titleBar.Render("goi"),
		boxStyle.Width(max(20, m.width-4)).Render(m.output.View()),
		boxStyle.Width(max(20, m.width-4)).Render(m.input.View()),
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
