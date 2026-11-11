package show

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
)

// gameImage is the game under Proton; the kernel truncates comm to its first 15 bytes.
const gameImage = "Probably Stolen.exe"

// GameRunning reports whether Probably Stolen.exe is running (under Proton), when the
// narrator must leave the GPU and the shared LLM to the game.
func GameRunning() bool { return gameRunningIn("/proc") }

// gameRunningIn matches by image name: argv[0]'s base name or comm. Never the whole command
// line, which a shell or grep that merely mentions the game would match too.
func gameRunningIn(procDir string) bool {
	dirs, _ := filepath.Glob(filepath.Join(procDir, "[0-9]*"))
	for _, dir := range dirs {
		if b, err := os.ReadFile(filepath.Join(dir, "cmdline")); err == nil {
			arg0, _, _ := bytes.Cut(b, []byte{0})
			name := string(arg0)
			if i := strings.LastIndexAny(name, `/\`); i >= 0 {
				name = name[i+1:]
			}
			if strings.EqualFold(name, gameImage) {
				return true
			}
		}
		if b, err := os.ReadFile(filepath.Join(dir, "comm")); err == nil && strings.TrimSpace(string(b)) == gameImage[:15] {
			return true
		}
	}
	return false
}
