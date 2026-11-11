package twitch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"
)

// DryRun is Predictions without Twitch: it keeps the predictions in memory under Helix's rules
// (one at a time, the lengths, the window, the status changes) and logs each call it stands in
// for as a JSON line, the request Helix would have been sent and what it would have answered.
// Viewers' predictions come from Back (the chat script's simulated viewers).
type DryRun struct {
	mu   sync.Mutex
	log  io.Writer
	now  func() time.Time
	seq  int
	all  map[string]*Prediction
	cur  string
	seen map[string]map[string]bool
}

// NewDryRun logs to w (nil: nowhere); now nil is time.Now.
func NewDryRun(w io.Writer, now func() time.Time) *DryRun {
	if now == nil {
		now = time.Now
	}
	return &DryRun{log: w, now: now, all: map[string]*Prediction{}, seen: map[string]map[string]bool{}}
}

// LogEntry is one line of the dry-run log.
type LogEntry struct {
	T      string `json:"t"`
	Call   string `json:"call"`
	Body   any    `json:"body,omitempty"`
	Status string `json:"status,omitempty"`
	Err    string `json:"err,omitempty"`
}

// Note logs something the show did that is not an API call (a vote counted, a tally sent).
func (d *DryRun) Note(call string, body any) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.write(LogEntry{Call: call, Body: body})
}

func (d *DryRun) write(e LogEntry) {
	if d.log == nil {
		return
	}
	e.T = d.now().UTC().Format(time.RFC3339)
	if b, err := json.Marshal(e); err == nil {
		_, _ = d.log.Write(append(b, '\n'))
	}
}

func (d *DryRun) Create(_ context.Context, title string, outcomes []string, window time.Duration) (Prediction, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	body := map[string]any{"broadcaster_id": "$TWITCH_BROADCASTER_ID", "title": title, "outcomes": titles(outcomes), "prediction_window": int(window / time.Second)}
	fail := func(err error) (Prediction, error) {
		d.write(LogEntry{Call: "POST /helix/predictions", Body: body, Status: "400", Err: err.Error()})
		return Prediction{}, err
	}
	if err := CheckCreate(title, outcomes, window); err != nil {
		return fail(err)
	}
	if c := d.all[d.cur]; c != nil && (c.Status == StatusActive || c.Status == StatusLocked) {
		return fail(ErrBusy)
	}
	d.seq++
	p := &Prediction{ID: fmt.Sprintf("dry-%d", d.seq), Title: title, Status: StatusActive, Window: window, Created: d.now()}
	for i, o := range outcomes {
		p.Outcomes = append(p.Outcomes, Outcome{ID: fmt.Sprintf("%s-o%d", p.ID, i+1), Title: o})
	}
	d.all[p.ID], d.cur, d.seen[p.ID] = p, p.ID, map[string]bool{}
	d.write(LogEntry{Call: "POST /helix/predictions", Body: body, Status: "200 " + p.ID})
	return *p, nil
}

func titles(outcomes []string) []map[string]string {
	out := make([]map[string]string, len(outcomes))
	for i, o := range outcomes {
		out[i] = map[string]string{"title": o}
	}
	return out
}

func (d *DryRun) End(_ context.Context, id, status, winner string) (Prediction, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	body := map[string]any{"broadcaster_id": "$TWITCH_BROADCASTER_ID", "id": id, "status": status}
	if winner != "" {
		body["winning_outcome_id"] = winner
	}
	p := d.all[id]
	if p == nil {
		d.write(LogEntry{Call: "PATCH /helix/predictions", Body: body, Status: "404"})
		return Prediction{}, fmt.Errorf("twitch: prediction %q not found", id)
	}
	d.expire(p)
	if err := CheckEnd(*p, status, winner); err != nil {
		d.write(LogEntry{Call: "PATCH /helix/predictions", Body: body, Status: "400", Err: err.Error()})
		return *p, err
	}
	p.Status = status
	d.write(LogEntry{Call: "PATCH /helix/predictions", Body: body, Status: "200"})
	return *p, nil
}

// expire locks p once its window has run out, as Twitch does.
func (d *DryRun) expire(p *Prediction) {
	if p.Status == StatusActive && !d.now().Before(p.Created.Add(p.Window)) {
		p.Status = StatusLocked
	}
}

func (d *DryRun) Get(_ context.Context, id string) (Prediction, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	p := d.all[id]
	if p == nil {
		return Prediction{}, fmt.Errorf("twitch: prediction %q not found", id)
	}
	d.expire(p)
	return *p, nil
}

func (d *DryRun) Active(context.Context) (*Prediction, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	p := d.all[d.cur]
	if p == nil {
		return nil, nil
	}
	d.expire(p)
	if p.Status != StatusActive && p.Status != StatusLocked {
		return nil, nil
	}
	c := *p
	return &c, nil
}

// Back is a simulated viewer putting points on outcome i (0-based) of the active prediction, as
// the Twitch UI would; a viewer predicts once. It reports whether it counted.
func (d *DryRun) Back(user string, i int, points int64) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	p := d.all[d.cur]
	if p == nil {
		return false
	}
	d.expire(p)
	if p.Status != StatusActive || i < 0 || i >= len(p.Outcomes) || points <= 0 || d.seen[p.ID][user] {
		return false
	}
	d.seen[p.ID][user] = true
	p.Outcomes[i].Users++
	p.Outcomes[i].Points += points
	return true
}
