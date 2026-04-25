package stt

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"
	"sync"
)

// Mock returns a fixed transcript, useful for bring-up without real STT.
type Mock struct {
	Transcript string
}

func (m Mock) Transcribe(_ context.Context, _ []byte) (string, error) {
	if m.Transcript == "" {
		return "mock transcript", nil
	}
	return m.Transcript, nil
}

// CLI spawns a new process per request (stdin → stdout). Simple but pays
// model-load cost on every call.
type CLI struct {
	Command []string
}

func (c CLI) Transcribe(ctx context.Context, audio []byte) (string, error) {
	if len(c.Command) == 0 {
		return "", fmt.Errorf("missing stt command")
	}

	cmd := exec.CommandContext(ctx, c.Command[0], c.Command[1:]...)
	cmd.Stdin = bytes.NewReader(audio)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("run %q: %w: %s", c.Command[0], err, stderr.String())
	}

	return stdout.String(), nil
}

// Process keeps a single long-running subprocess alive across requests.
// The model is loaded once; each Transcribe call exchanges one message:
//
//	→ [4-byte big-endian uint32: audio length][audio bytes]
//	← [transcript text]\n
type Process struct {
	mu      sync.Mutex
	command []string
	stdin   io.WriteCloser
	stdout  *bufio.Reader
}

func NewProcess(command []string) (*Process, error) {
	if len(command) == 0 {
		return nil, fmt.Errorf("missing stt command")
	}
	p := &Process{command: command}
	if err := p.start(); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *Process) start() error {
	cmd := exec.Command(p.command[0], p.command[1:]...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start: %w", err)
	}

	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			log.Printf("stt: %s", scanner.Text())
		}
	}()

	p.stdin = stdin
	p.stdout = bufio.NewReader(stdoutPipe)
	return nil
}

func (p *Process) Transcribe(ctx context.Context, audio []byte) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.send(audio); err != nil {
		log.Printf("stt: send failed (%v), restarting process", err)
		if err2 := p.start(); err2 != nil {
			return "", fmt.Errorf("send: %w; restart: %v", err, err2)
		}
		if err := p.send(audio); err != nil {
			return "", fmt.Errorf("send after restart: %w", err)
		}
	}

	transcript, err := p.recv(ctx)
	if err == nil {
		log.Printf("stt: %q", transcript)
	}
	return transcript, err
}

func (p *Process) send(audio []byte) error {
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(audio)))
	if _, err := p.stdin.Write(header[:]); err != nil {
		return err
	}
	_, err := p.stdin.Write(audio)
	return err
}

func (p *Process) recv(ctx context.Context) (string, error) {
	type result struct {
		text string
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		line, err := p.stdout.ReadString('\n')
		ch <- result{strings.TrimSpace(line), err}
	}()

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case r := <-ch:
		if r.err != nil {
			return "", fmt.Errorf("read response: %w", r.err)
		}
		return r.text, nil
	}
}
