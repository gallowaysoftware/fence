package expert

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gallowaysoftware/fence/internal/llm"
)

// GameRunning reports whether the owner's game is running, by process image name. Under
// Proton the image is "Probably Stolen.exe", which the kernel truncates in comm to its
// first 15 bytes.
func GameRunning() (bool, error) {
	procs, err := filepath.Glob("/proc/[0-9]*/comm")
	if err != nil {
		return false, err
	}
	for _, p := range procs {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(string(b)), "Probably Stolen") {
			return true, nil
		}
	}
	return false, nil
}

// FrameRead is what a vision model reads off one gameplay frame. Unreadable fields stay
// empty; the model is told not to guess.
type FrameRead struct {
	Screen     string         `json:"screen"` // counter | night | upgrades | inventory | menu | other
	Day        Num            `json:"day,omitempty"`
	Weekday    string         `json:"weekday,omitempty"`
	Cash       Num            `json:"cash,omitempty"`
	RentInDays Num            `json:"rent_in_days,omitempty"`
	RentDue    Num            `json:"rent_due,omitempty"`
	Reps       map[string]Num `json:"reps,omitempty"`
	Counter    []FrameItem    `json:"counter_items,omitempty"`
	Offer      Num            `json:"offer,omitempty"` // the price on the table at the counter
	Budget     Num            `json:"customer_budget,omitempty"`
	Upgrades   []FrameItem    `json:"upgrades,omitempty"`
	Notes      string         `json:"notes,omitempty"`
}

type FrameItem struct {
	Name     string `json:"name"`
	Price    Num    `json:"price,omitempty"`
	Estimate Num    `json:"estimate,omitempty"`
	Heat     Num    `json:"heat,omitempty"`
}

// Num is a number the model may send as a number, null, or a string with units ("617$",
// "1,200", "6 DAYS"); nil means unread.
type Num struct{ V *int }

func (n *Num) UnmarshalJSON(b []byte) error {
	s := strings.Trim(strings.TrimSpace(string(b)), `"`)
	neg := strings.HasPrefix(s, "-")
	v, seen := 0, false
	for _, r := range s {
		if r >= '0' && r <= '9' {
			v, seen = v*10+int(r-'0'), true
		} else if r == '.' && seen {
			break
		}
	}
	if !seen {
		n.V = nil
		return nil
	}
	if neg {
		v = -v
	}
	n.V = &v
	return nil
}

func (n Num) String() string {
	if n.V == nil {
		return "?"
	}
	return strconv.Itoa(*n.V)
}

func (n Num) MarshalJSON() ([]byte, error) {
	if n.V == nil {
		return []byte("null"), nil
	}
	return []byte(strconv.Itoa(*n.V)), nil
}

const framePrompt = `This is a frame of a let's-play of the shop game "Probably Stolen" (pixel art, space station
shop). Read ONLY what is visibly legible; leave a field out rather than guess. Return one JSON object:
{"screen":"counter|night|upgrades|inventory|menu|other","day":<day number shown>,"weekday":"",
"cash":<player cash on the small wall display left of the day sign; digits only, the trailing currency glyph is not a digit; omit if hidden>,"rent_in_days":<days until rent, from "RENT IN N DAYS">,"rent_due":<rent amount, only if a credit amount is shown>,"reps":{"faction name":<value or bar % if numeric>},
"counter_items":[{"name":"","price":<asking/offered price>,"estimate":<estimated value>,"heat":<heat %>}],
"offer":<total currently on the table>,"customer_budget":<"Customer Budget" shown in a trade panel>,"upgrades":[{"name":"","price":<cost>}],"notes":"one sentence on what is happening"}`

// Vision talks to an OpenAI-compatible endpoint that accepts image_url content parts.
type Vision struct {
	BaseURL, Model string
	HTTP           *http.Client
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// Read asks the model to read one JPEG frame. It returns the raw answer and token usage
// with the parse, so a bad parse is still inspectable.
func (v Vision) Read(ctx context.Context, jpeg []byte) (FrameRead, string, Usage, time.Duration, error) {
	var fr FrameRead
	body := map[string]any{
		"model":                v.Model,
		"max_tokens":           1500,
		"temperature":          0,
		"chat_template_kwargs": map[string]any{"enable_thinking": false},
		"messages": []map[string]any{{
			"role": "user",
			"content": []map[string]any{
				{"type": "text", "text": framePrompt},
				{"type": "image_url", "image_url": map[string]string{"url": "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(jpeg)}},
			},
		}},
	}
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(v.BaseURL, "/")+"/chat/completions", bytes.NewReader(b))
	if err != nil {
		return fr, "", Usage{}, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	t0 := time.Now()
	resp, err := v.HTTP.Do(req)
	if err != nil {
		return fr, "", Usage{}, 0, err
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(resp.Body)
	dt := time.Since(t0)
	if resp.StatusCode != http.StatusOK {
		return fr, "", Usage{}, dt, fmt.Errorf("vision: HTTP %d: %s", resp.StatusCode, truncate(string(rb), 300))
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage Usage `json:"usage"`
	}
	if err := json.Unmarshal(rb, &out); err != nil || len(out.Choices) == 0 {
		return fr, string(rb), Usage{}, dt, fmt.Errorf("vision: bad response")
	}
	text := out.Choices[0].Message.Content
	obj, err := llm.ExtractJSON(text)
	if err != nil {
		return fr, text, out.Usage, dt, err
	}
	err = json.Unmarshal(obj, &fr)
	return fr, text, out.Usage, dt, err
}
