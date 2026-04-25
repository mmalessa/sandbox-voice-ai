package tts

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"os/exec"
)

type Piper struct {
	Command       []string
	SampleRate    int
	Channels      int
	BitsPerSample int
}

func (p Piper) Synthesize(ctx context.Context, text string) ([]byte, error) {
	if len(p.Command) == 0 {
		return nil, fmt.Errorf("missing piper command")
	}

	cmd := exec.CommandContext(ctx, p.Command[0], p.Command[1:]...)
	cmd.Stdin = bytes.NewBufferString(text)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("run %q: %w: %s", p.Command[0], err, stderr.String())
	}

	return wrapPCMAsWAV(stdout.Bytes(), p.SampleRate, p.Channels, p.BitsPerSample)
}

type Mock struct {
	SampleRate    int
	Channels      int
	BitsPerSample int
}

func (m Mock) Synthesize(_ context.Context, text string) ([]byte, error) {
	sampleRate := m.SampleRate
	if sampleRate == 0 {
		sampleRate = 22050
	}

	channels := m.Channels
	if channels == 0 {
		channels = 1
	}

	bits := m.BitsPerSample
	if bits == 0 {
		bits = 16
	}

	pcm := generateTonePCM(sampleRate, 440, timePerCharacter(text), channels)
	return wrapPCMAsWAV(pcm, sampleRate, channels, bits)
}

func timePerCharacter(text string) float64 {
	base := 0.04 * float64(len(text))
	if base < 0.25 {
		return 0.25
	}
	if base > 2.5 {
		return 2.5
	}
	return base
}

func generateTonePCM(sampleRate int, freq float64, seconds float64, channels int) []byte {
	totalSamples := int(float64(sampleRate) * seconds)
	pcm := make([]byte, totalSamples*channels*2)

	for i := 0; i < totalSamples; i++ {
		value := int16(math.Sin(2*math.Pi*freq*float64(i)/float64(sampleRate)) * 0.2 * math.MaxInt16)
		for ch := 0; ch < channels; ch++ {
			offset := (i*channels + ch) * 2
			binary.LittleEndian.PutUint16(pcm[offset:], uint16(value))
		}
	}

	return pcm
}

func wrapPCMAsWAV(pcm []byte, sampleRate, channels, bitsPerSample int) ([]byte, error) {
	if sampleRate <= 0 || channels <= 0 || bitsPerSample <= 0 {
		return nil, fmt.Errorf("invalid wav format")
	}

	var out bytes.Buffer
	byteRate := sampleRate * channels * bitsPerSample / 8
	blockAlign := channels * bitsPerSample / 8
	dataLen := len(pcm)
	riffLen := 36 + dataLen

	if _, err := out.WriteString("RIFF"); err != nil {
		return nil, err
	}
	if err := binary.Write(&out, binary.LittleEndian, uint32(riffLen)); err != nil {
		return nil, err
	}
	if _, err := out.WriteString("WAVE"); err != nil {
		return nil, err
	}
	if _, err := out.WriteString("fmt "); err != nil {
		return nil, err
	}
	if err := binary.Write(&out, binary.LittleEndian, uint32(16)); err != nil {
		return nil, err
	}
	if err := binary.Write(&out, binary.LittleEndian, uint16(1)); err != nil {
		return nil, err
	}
	if err := binary.Write(&out, binary.LittleEndian, uint16(channels)); err != nil {
		return nil, err
	}
	if err := binary.Write(&out, binary.LittleEndian, uint32(sampleRate)); err != nil {
		return nil, err
	}
	if err := binary.Write(&out, binary.LittleEndian, uint32(byteRate)); err != nil {
		return nil, err
	}
	if err := binary.Write(&out, binary.LittleEndian, uint16(blockAlign)); err != nil {
		return nil, err
	}
	if err := binary.Write(&out, binary.LittleEndian, uint16(bitsPerSample)); err != nil {
		return nil, err
	}
	if _, err := out.WriteString("data"); err != nil {
		return nil, err
	}
	if err := binary.Write(&out, binary.LittleEndian, uint32(dataLen)); err != nil {
		return nil, err
	}
	if _, err := out.Write(pcm); err != nil {
		return nil, err
	}

	return out.Bytes(), nil
}
