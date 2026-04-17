package process

import (
	"errors"
	"io"
	"strings"
	"sync"
)

type Process interface {
	Start() (<-chan Event, error)
	Write(string) error
	Stop() error
}

type Event struct {
	Output string
	Err    error
	Exited bool
}

func streamPipe(r io.Reader, events chan<- Event, wg *sync.WaitGroup) {
	defer wg.Done()

	buf := make([]byte, 1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			events <- Event{Output: string(buf[:n])}
		}
		if err != nil {
			if err != io.EOF {
				events <- Event{Output: "\n[stream error] " + err.Error() + "\n"}
			}
			return
		}
	}
}

func normalizeExitErr(err error) error {
	if err == nil {
		return nil
	}

	var exitErr exitCoder
	if errors.As(err, &exitErr) {
		return nil
	}

	if !strings.Contains(err.Error(), "signal: killed") && !strings.Contains(err.Error(), "killed") && !strings.Contains(err.Error(), "terminated") {
		return err
	}

	return nil
}

type exitCoder interface {
	ExitCode() int
	error
}
