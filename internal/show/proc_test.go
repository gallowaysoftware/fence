package show

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGameRunningByImageName(t *testing.T) {
	cases := []struct {
		name, cmdline, comm string
		want                bool
	}{
		{"proton", `Z:\home\u\.local\share\Steam\steamapps\common\Probably Stolen\Probably Stolen.exe` + "\x00", "Probably Stolen", true},
		{"comm only", "", "Probably Stolen", true},
		{"a shell searching for it", "/usr/bin/zsh\x00-c\x00pgrep -f \"Probably Stolen.exe\"\x00", "zsh", false},
		{"a log tail", "tail\x00-f\x00/x/Probably Stolen/GameLog_1.txt\x00", "tail", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "4242")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			os.WriteFile(filepath.Join(dir, "cmdline"), []byte(tc.cmdline), 0o644)
			os.WriteFile(filepath.Join(dir, "comm"), []byte(tc.comm+"\n"), 0o644)
			if got := gameRunningIn(filepath.Dir(dir)); got != tc.want {
				t.Errorf("gameRunningIn = %v, want %v", got, tc.want)
			}
		})
	}
}
