package show

// Audience is what the Twitch audience is doing, as the panel draws it (docs/SHOW.md, "Twitch"):
// the prediction in Twitch's one slot and the chat vote, each with its question, its split and its
// result, so a VOD with no chat beside it still reads. internal/twitch publishes it through
// /v1/ingest; the hub keeps the latest.
type Audience struct {
	Prediction *AudiencePrediction `json:"prediction,omitempty"`
	Poll       *AudiencePoll       `json:"poll,omitempty"`
	// LastPoll is the vote before Poll, kept for its result and the strategist's answer.
	LastPoll *AudiencePoll `json:"last_poll,omitempty"`
}

// AudiencePrediction is a channel-points prediction on a bet ("bet") or on the run's format
// verdict ("verdict"). Status: active, locked, resolved, canceled, or failed (Twitch refused it);
// Note says why it was canceled or failed.
type AudiencePrediction struct {
	About    string            `json:"about"`
	Title    string            `json:"title"`
	Claim    string            `json:"claim,omitempty"`
	Day      int               `json:"day,omitempty"`
	By       int               `json:"by,omitempty"`
	Status   string            `json:"status"`
	Outcomes []AudienceOutcome `json:"outcomes"`
	LocksAt  int64             `json:"locks_at,omitempty"`
	Note     string            `json:"note,omitempty"`
	Result   string            `json:"result,omitempty"`
	Skipped  *AudienceSkip     `json:"skipped,omitempty"`
}

// AudienceSkip is a bet that got no prediction because the slot was taken, shown under the
// prediction that holds it.
type AudienceSkip struct {
	Claim string `json:"claim"`
	Why   string `json:"why"`
}

type AudienceOutcome struct {
	Title  string `json:"title"`
	Users  int64  `json:"users"`
	Points int64  `json:"points"`
	Won    bool   `json:"won,omitempty"`
}

// AudiencePoll is a chat vote over a fixed list (`!vote WORD`). For is what it feeds ("day 34's
// plan", "the mortgage fork"); Fact is the one line the strategist was given, Answer the
// strategist's reply to it from its next plan.
type AudiencePoll struct {
	Question string           `json:"question"`
	For      string           `json:"for"`
	Status   string           `json:"status"`
	Options  []AudienceOption `json:"options"`
	Total    int              `json:"total"`
	ClosesAt int64            `json:"closes_at,omitempty"`
	Fact     string           `json:"fact,omitempty"`
	Answer   string           `json:"answer,omitempty"`
	Note     string           `json:"note,omitempty"`
}

type AudienceOption struct {
	Word  string `json:"word"`
	Label string `json:"label"`
	Votes int    `json:"votes"`
}
