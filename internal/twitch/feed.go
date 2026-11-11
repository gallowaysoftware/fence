package twitch

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// FeedEvent is the part of a show feed event (docs/SHOW.md, "Feed schema") this package reads.
// It is decoded from the feed's JSON rather than taken from show.Event, so fields another branch
// adds to the feed (the bet) are read as soon as the feed carries them.
type FeedEvent struct {
	Seq        int64       `json:"seq"`
	Run        string      `json:"run"`
	Day        int         `json:"day"`
	Kind       string      `json:"kind"`
	Level      int         `json:"level"`
	Beat       string      `json:"beat"`
	Text       string      `json:"text"`
	Player     *feedPlayer `json:"player"`
	Clock      *feedClock  `json:"clock"`
	Plan       *feedPlan   `json:"plan"`
	Candidates *feedCands  `json:"candidates"`
	Trade      *struct {
		Side string `json:"side"`
	} `json:"trade"`
	End *struct {
		Won    bool   `json:"won"`
		Day    int    `json:"day"`
		Ending string `json:"ending"`
	} `json:"end"`
	Bet *FeedBet `json:"bet"`
}

type feedPlayer struct {
	ID          string `json:"id"`
	PersonaName string `json:"persona_name"`
	Format      *struct {
		Name        string   `json:"name"`
		Title       string   `json:"title"`
		DeadlineDay int      `json:"deadline_day"`
		LabPct      int      `json:"lab_pct"`
		Handicaps   []string `json:"handicaps"`
	} `json:"format"`
}

type feedClock struct {
	Cash        int64 `json:"cash"`
	NetWorth    int64 `json:"net_worth"`
	Owned       bool  `json:"owned"`
	OfferDown   int64 `json:"offer_down"`
	MortgageWk  int64 `json:"mortgage_weekly"`
	BuyoutPrice int64 `json:"buyout_price"`
}

type feedPlan struct {
	Day      int    `json:"day"`
	Source   string `json:"source"`
	Sections []struct {
		Title string   `json:"title"`
		Lines []string `json:"lines"`
	} `json:"sections"`
}

type feedCands struct {
	Day     int    `json:"day"`
	Metric  string `json:"metric"`
	Options []struct {
		Label  string `json:"label"`
		Chosen bool   `json:"chosen"`
	} `json:"options"`
}

// FeedBet is the feed's `bet` payload (the bets-callbacks branch's show.BetState).
type FeedBet struct {
	Claim   string `json:"claim"`
	Kind    string `json:"kind"`
	Target  int64  `json:"target"`
	Item    string `json:"item"`
	By      int    `json:"by"`
	Staked  int    `json:"staked"`
	Outcome string `json:"outcome"`
	Fact    string `json:"fact"`
}

// FeedState is the part of the panel state (GET /state, the SSE `state` message) read on attach.
type FeedState struct {
	Run    string      `json:"run"`
	Day    int         `json:"day"`
	Player *feedPlayer `json:"player"`
	Clock  *feedClock  `json:"clock"`
	End    *FeedEvent  `json:"end"`
}

// Subscribe reads fenceshow's /events stream at base until ctx ends, sending the attach state
// once per connection and then each event; it reconnects 2 s after a drop, reporting it to fail.
func Subscribe(ctx context.Context, base string, state func(FeedState), event func(FeedEvent), fail func(error)) error {
	for {
		if err := subscribeOnce(ctx, base, state, event); err != nil && ctx.Err() == nil && fail != nil {
			fail(err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

func subscribeOnce(ctx context.Context, base string, state func(FeedState), event func(FeedEvent)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+"/events", nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("feed: %s", resp.Status)
	}
	return ReadSSE(resp.Body, state, event)
}

// ReadSSE parses a fenceshow SSE stream: `event: state` once, then unnamed data messages.
func ReadSSE(r io.Reader, state func(FeedState), event func(FeedEvent)) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 8<<20)
	name, data := "", ""
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			if data != "" {
				if name == "state" {
					var s FeedState
					if json.Unmarshal([]byte(data), &s) == nil {
						state(s)
					}
				} else {
					var e FeedEvent
					if json.Unmarshal([]byte(data), &e) == nil {
						event(e)
					}
				}
			}
			name, data = "", ""
		case strings.HasPrefix(line, "event:"):
			name = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			data += strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " ")
		}
	}
	return sc.Err()
}
