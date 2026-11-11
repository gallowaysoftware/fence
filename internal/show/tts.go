package show

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

// TTS turns a line into WAV audio.
type TTS interface {
	Name() string
	Speak(ctx context.Context, text, voice string) ([]byte, error)
}

// Kokoro is a Kokoro-FastAPI server (OpenAI-compatible /v1/audio/speech), the same
// service llm-economic-domination's video pipeline uses.
type Kokoro struct {
	URL   string
	Speed float64
	HTTP  *http.Client
}

func (k *Kokoro) Name() string { return "kokoro" }

// minWAV is the smallest answer that can be speech; Kokoro sometimes returns a bare header.
const minWAV = 800

func (k *Kokoro) Speak(ctx context.Context, text, voice string) ([]byte, error) {
	body := map[string]any{"model": "kokoro", "input": text, "voice": voice, "response_format": "wav"}
	if k.Speed > 0 {
		body["speed"] = k.Speed
	}
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(k.URL, "/")+"/v1/audio/speech", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	hc := k.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("kokoro: HTTP %d: %s", resp.StatusCode, clip(string(out), 200))
	}
	if len(out) < minWAV {
		return nil, errors.New("kokoro: empty audio")
	}
	return out, nil
}

// Piper is the piper CLI with one local voice model: lower quality than Kokoro, but a
// fraction of the CPU.
type Piper struct {
	Bin   string
	Model string
}

func (p *Piper) Name() string { return "piper" }

func (p *Piper) Speak(ctx context.Context, text, _ string) ([]byte, error) {
	f, err := os.CreateTemp("", "fenceshow-*.wav")
	if err != nil {
		return nil, err
	}
	name := f.Name()
	f.Close()
	defer os.Remove(name)
	cmd := exec.CommandContext(ctx, p.Bin, "--model", p.Model, "--output_file", name)
	cmd.Stdin = strings.NewReader(text)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("piper: %w: %s", err, clip(string(out), 200))
	}
	b, err := os.ReadFile(name)
	if err != nil {
		return nil, err
	}
	if len(b) < minWAV {
		return nil, errors.New("piper: empty audio")
	}
	return b, nil
}

// Chain tries each TTS in turn.
type Chain []TTS

func (c Chain) Name() string {
	ns := make([]string, len(c))
	for i, t := range c {
		ns[i] = t.Name()
	}
	return strings.Join(ns, ">")
}

func (c Chain) Speak(ctx context.Context, text, voice string) ([]byte, error) {
	var errs []error
	for _, t := range c {
		b, err := t.Speak(ctx, text, voice)
		if err == nil {
			return b, nil
		}
		errs = append(errs, err)
		if ctx.Err() != nil {
			break
		}
	}
	return nil, errors.Join(errs...)
}
