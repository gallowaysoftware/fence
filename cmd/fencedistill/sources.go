package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// gameSource says how a game records its reasoning:
//   - "director": the director policy's games (a why.txt per day). Every day is policy
//     output, so routine.
//   - "play-gamenotes": one notes.md for the whole game, with lines such as
//     "- d36-42: ... WHY: ..." or "- day 45: JUDGMENT ...". judged maps each day such a
//     line covers to its reasoning; standing is the WHY for the other days.
//   - "play": a notes.md per day (classify).
func gameSource(dir string, days []playDay) (source string, judged map[int]string, standing string, err error) {
	if slices.ContainsFunc(days, func(d playDay) bool { return d.hasWhy }) {
		return "director", nil, "", nil
	}
	if slices.ContainsFunc(days, func(d playDay) bool { return d.notes != "" }) {
		return "play", nil, "", nil
	}
	b, err := os.ReadFile(filepath.Join(dir, "notes.md"))
	if errors.Is(err, os.ErrNotExist) {
		return "play", nil, "", nil
	}
	if err != nil {
		return "", nil, "", err
	}
	standing = "standing orders unchanged."
	var plan struct {
		Standing map[string]any `json:"standing"`
	}
	if readJSON(filepath.Join(dir, "plan.json"), &plan) == nil && len(plan.Standing) > 0 {
		j, _ := json.Marshal(plan.Standing)
		standing = "standing orders " + string(j)
	}
	return "play-gamenotes", judgedDays(string(b)), standing, nil
}

// dayLine matches a game note's day spec: "- d43:", "- d36-42 (…):", "- day 45-48:".
// Open ranges ("day 85+") are standing orders and do not match.
var dayLine = regexp.MustCompile(`^-\s+(?:d|day )(\d+)(?:-(\d+))?\b([^:+]*):\s*(.*)$`)

// judgedDays reads the days a game note marks as judgment ("WHY:" or "JUDGMENT", not a
// restatement of standing orders) and their reasoning, preferring the WHY clause.
func judgedDays(notes string) map[int]string {
	out := map[int]string{}
	for _, l := range strings.Split(notes, "\n") {
		m := dayLine.FindStringSubmatch(strings.TrimSpace(l))
		if m == nil {
			continue
		}
		text := strings.TrimSpace(m[4])
		if !strings.Contains(text, "WHY:") && !strings.Contains(text, "JUDGMENT") ||
			strings.HasPrefix(strings.ToLower(text), "standing orders") {
			continue
		}
		why := text
		if _, after, ok := strings.Cut(text, "WHY:"); ok {
			why = strings.TrimSpace(after)
		}
		from, _ := strconv.Atoi(m[1])
		to := from
		if m[2] != "" {
			to, _ = strconv.Atoi(m[2])
		}
		for d := from; d <= to && d-from < 60; d++ {
			if _, ok := out[d]; !ok || strings.Contains(text, "WHY:") {
				out[d] = why
			}
		}
	}
	return out
}
