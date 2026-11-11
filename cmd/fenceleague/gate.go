package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// gameImage is the owner's game under Proton. The kernel truncates comm to 15 bytes.
const gameImage = "Probably Stolen.exe"

// GameGate tracks whether the owner is playing. The league never competes with that: a
// yielding endpoint's matches stop and fewer gyms run.
type GameGate struct {
	procDir string

	mu      sync.Mutex
	playing bool
	clear   chan struct{} // closed while not playing
	onSet   []func(playing bool)
}

func NewGameGate(procDir string) *GameGate {
	g := &GameGate{procDir: procDir, clear: make(chan struct{})}
	close(g.clear)
	return g
}

// Watch polls until ctx ends.
func (g *GameGate) Watch(ctx context.Context, every time.Duration) {
	g.set(gameRunning(g.procDir))
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			g.set(gameRunning(g.procDir))
		}
	}
}

// OnChange registers f, called (outside the lock) on every transition.
func (g *GameGate) OnChange(f func(playing bool)) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.onSet = append(g.onSet, f)
}

func (g *GameGate) Playing() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.playing
}

// WaitClear returns once the owner is not playing.
func (g *GameGate) WaitClear(ctx context.Context) error {
	g.mu.Lock()
	c := g.clear
	g.mu.Unlock()
	select {
	case <-c:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (g *GameGate) set(playing bool) {
	g.mu.Lock()
	if playing == g.playing {
		g.mu.Unlock()
		return
	}
	g.playing = playing
	if playing {
		g.clear = make(chan struct{})
	} else {
		close(g.clear)
	}
	fs := append([]func(bool){}, g.onSet...)
	g.mu.Unlock()
	for _, f := range fs {
		f(playing)
	}
}

// gameRunning matches processes by image name: argv[0]'s base name (Windows or Unix
// separators) or comm. Never the whole command line, which a shell searching for the
// game would match too.
func gameRunning(procDir string) bool {
	ents, err := os.ReadDir(procDir)
	if err != nil {
		return false
	}
	self := os.Getpid()
	for _, e := range ents {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid == self {
			continue
		}
		dir := filepath.Join(procDir, e.Name())
		if cmd, err := os.ReadFile(filepath.Join(dir, "cmdline")); err == nil && isGameImage(argv0(cmd)) {
			return true
		}
		if comm, err := os.ReadFile(filepath.Join(dir, "comm")); err == nil && strings.TrimSpace(string(comm)) == gameImage[:15] {
			return true
		}
	}
	return false
}

func argv0(cmdline []byte) string {
	a, _, _ := bytes.Cut(cmdline, []byte{0})
	return string(a)
}

func isGameImage(arg0 string) bool {
	if i := strings.LastIndexAny(arg0, `/\`); i >= 0 {
		arg0 = arg0[i+1:]
	}
	return strings.EqualFold(arg0, gameImage)
}
