package vod

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Write puts a kit's files in dir: title.txt, description.txt (chapters included), chapters.txt,
// tags.txt, highlights.tsv, cut.sh and kit.json.
func Write(dir string, k Kit) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	var ch, hl strings.Builder
	for _, c := range k.Chapters {
		fmt.Fprintf(&ch, "%s %s\n", c.Stamp, c.Title)
	}
	hl.WriteString("start\tend\tlength\tday\tcaption\n")
	for _, c := range k.Clips {
		fmt.Fprintf(&hl, "%s\t%s\t%ds\t%d\t%s\n", Stamp(c.Start), Stamp(c.End), int((c.End-c.Start)/time.Second), c.Day, c.Caption)
	}
	js, err := json.MarshalIndent(k, "", "  ")
	if err != nil {
		return err
	}
	files := []struct {
		name string
		body string
		mode os.FileMode
	}{
		{"title.txt", k.Title + "\n", 0o644},
		{"description.txt", k.Description, 0o644},
		{"chapters.txt", ch.String(), 0o644},
		{"tags.txt", strings.Join(k.Tags, ", ") + "\n", 0o644},
		{"highlights.tsv", hl.String(), 0o644},
		{"cut.sh", CutScript(k), 0o755},
		{"kit.json", string(js) + "\n", 0o644},
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f.name), []byte(f.body), f.mode); err != nil {
			return err
		}
	}
	return nil
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// CutScript is a shell script that cuts the clips out of the recording with ffmpeg into clips/
// beside it. It re-encodes: a stream copy cuts on keyframes, seconds off the window.
func CutScript(k Kit) string {
	var b strings.Builder
	b.WriteString("#!/bin/sh\n# Cuts this VOD's highlight clips from the local recording (written by cmd/fencevod).\n")
	b.WriteString("# Usage: cut.sh [RECORDING]   Clips go to clips/ next to this script.\nset -eu\n")
	fmt.Fprintf(&b, "rec=${1:-%s}\n", shellQuote(k.Recording))
	b.WriteString(`if [ -z "$rec" ] || [ ! -f "$rec" ]; then echo "cut.sh: give the recording's path" >&2; exit 2; fi
command -v ffmpeg >/dev/null 2>&1 || { echo "cut.sh: ffmpeg is not installed" >&2; exit 1; }
out=$(dirname "$0")/clips
mkdir -p "$out"
`)
	for i, c := range k.Clips {
		name := fmt.Sprintf("%02d-day%d-%s", i+1, c.Day, strings.Trim(slugRe.ReplaceAllString(strings.ToLower(c.Kind+"-"+c.Beat), "-"), "-"))
		fmt.Fprintf(&b, "ffmpeg -hide_banner -loglevel error -y -ss %.3f -i \"$rec\" -t %.3f -c:v libx264 -preset veryfast -crf 20 -c:a aac -b:a 160k \"$out/%s.mp4\"\n",
			c.Start.Seconds(), (c.End - c.Start).Seconds(), name)
	}
	fmt.Fprintf(&b, "echo \"%d clips in $out\"\n", len(k.Clips))
	return b.String()
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
