package stt

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

type Mock struct {
	Transcript string
}

func (m Mock) Transcribe(_ context.Context, _ []byte) (string, error) {
	if m.Transcript == "" {
		return "mock transcript", nil
	}
	return m.Transcript, nil
}

type CLI struct {
	Command []string
}

func (c CLI) Transcribe(ctx context.Context, audio []byte) (string, error) {
	if len(c.Command) == 0 {
		return "", fmt.Errorf("missing stt command")
	}

	cmd := exec.CommandContext(ctx, c.Command[0], c.Command[1:]...)
	cmd.Stdin = bytes.NewReader(audio)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("run %q: %w: %s", c.Command[0], err, stderr.String())
	}

	return stdout.String(), nil
}
