package knowledge

import (
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
)

var (
	vttTag    = regexp.MustCompile(`<[^>]*>`)
	vttTiming = regexp.MustCompile(`^(\d+:)?(\d{1,2}):(\d{2})[.,](\d{3})\s+-->`)
	vttNoise  = regexp.MustCompile(`^\[(music|applause|laughter|__)\]$`)
)

// CleanVTT turns a WebVTT transcript into plain text grouped into paragraphs of about
// paraSeconds, each prefixed with its start time as [m:ss] or [h:mm:ss]. YouTube's
// automatic captions repeat every line in the following cue and extend lines word by
// word; both repetitions are removed.
func CleanVTT(vtt string, paraSeconds int) string {
	type cue struct {
		at    int
		lines []string
	}
	var cues []cue
	var cur *cue
	for _, raw := range strings.Split(strings.ReplaceAll(vtt, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		// YouTube pads cue payloads with whitespace-only lines; only a truly empty line
		// ends a cue.
		if line == "" && strings.TrimRight(raw, "\r") != "" {
			continue
		}
		if m := vttTiming.FindStringSubmatch(line); m != nil {
			cues = append(cues, cue{at: vttSeconds(m)})
			cur = &cues[len(cues)-1]
			continue
		}
		if line == "" {
			cur = nil
			continue
		}
		if cur == nil {
			continue // header, NOTE, STYLE, cue identifiers
		}
		text := collapseSpace(html.UnescapeString(vttTag.ReplaceAllString(line, "")))
		if text == "" || vttNoise.MatchString(strings.ToLower(text)) {
			continue
		}
		cur.lines = append(cur.lines, text)
	}

	var b strings.Builder
	var para []string
	paraStart := -1
	last := ""
	emit := func() {
		if len(para) > 0 {
			fmt.Fprintf(&b, "[%s] %s\n", fmtStamp(paraStart), strings.Join(para, " "))
		}
		para, paraStart = nil, -1
	}
	for _, c := range cues {
		for _, l := range c.lines {
			var add string
			switch {
			case l == last || strings.HasSuffix(last, l):
				continue
			case last != "" && strings.HasPrefix(l, last):
				add = strings.TrimSpace(l[len(last):])
			default:
				add = l
			}
			last = l
			if add == "" {
				continue
			}
			if paraStart >= 0 && c.at-paraStart >= paraSeconds {
				emit()
			}
			if paraStart < 0 {
				paraStart = c.at
			}
			para = append(para, add)
		}
	}
	emit()
	return b.String()
}

func vttSeconds(m []string) int {
	h := 0
	if m[1] != "" {
		h, _ = strconv.Atoi(strings.TrimSuffix(m[1], ":"))
	}
	mi, _ := strconv.Atoi(m[2])
	s, _ := strconv.Atoi(m[3])
	return h*3600 + mi*60 + s
}

func fmtStamp(sec int) string {
	if sec >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", sec/3600, sec%3600/60, sec%60)
	}
	return fmt.Sprintf("%d:%02d", sec/60, sec%60)
}
