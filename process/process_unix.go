//go:build darwin || linux

package process

import (
	"io"
	"os/exec"
	"sync"
	"syscall"
)

type unixProcess struct {
	command string
	cmd     *exec.Cmd
	stdin   io.WriteCloser
}

func New(command string) Process {
	return &unixProcess{command: command}
}

func (p *unixProcess) Start() (<-chan Event, error) {
	p.cmd = exec.Command("sh", "-lc", p.command)
	p.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdout, err := p.cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	stderr, err := p.cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	p.stdin, err = p.cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	if err := p.cmd.Start(); err != nil {
		return nil, err
	}

	events := make(chan Event, 128)

	var wg sync.WaitGroup
	wg.Add(2)

	go streamPipe(stdout, events, &wg)
	go streamPipe(stderr, events, &wg)

	go func() {
		err := p.cmd.Wait()
		wg.Wait()
		events <- Event{Err: normalizeExitErr(err), Exited: true}
		close(events)
	}()

	return events, nil
}

func (p *unixProcess) Write(value string) error {
	if p.stdin == nil {
		return nil
	}
	_, err := io.WriteString(p.stdin, value)
	return err
}

func (p *unixProcess) Stop() error {
	if p.cmd == nil || p.cmd.Process == nil {
		return nil
	}

	return syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL)
}
