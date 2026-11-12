package show

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gallowaysoftware/fence/internal/llm"
)

// SpeakerCohost marks a co-host's line (Say.Speaker); the narrator's lines have none.
const SpeakerCohost = "cohost"

// Cohost is a second voice in the booth (docs/SHOW.md, "The co-host"): after the narrator speaks
// about one of the player's decisions (the day-0 thesis, a fork, a bet, a change of course, a
// loss), it needles the call in a line of its own, and now and then the narrator answers back. It
// speaks only from the narrator's loop, after the narrator's line, and yields to a waiting moment
// as loud as the decision it answers, so it never takes a big moment's turn; its numbers pass the same fact check, and a line that fails
// twice is not spoken at all.
type Cohost struct {
	// LLM is the co-host's client: the narrator's model, at a lower priority where a gate allows.
	LLM  llm.Client
	Name string
	// Voice is its Kokoro voice; "" contrasts with the narrator's (CohostVoice).
	Voice string
	// Every is how many narrator lines must pass between two exchanges; ReplyEvery says the
	// narrator answers every ReplyEvery-th exchange (1: always, 0: never).
	Every, ReplyEvery int
	// Gap is the pause between the lines of an exchange.
	Gap time.Duration

	lines, exchanges int
	book             *phrasebook
	recent           []string
	started          bool
}

// DefaultCohostName is the co-host's on-air name.
const DefaultCohostName = "Rook"

const cohostStyle = "a dry, sceptical rival pawnbroker riding along in the booth: arms folded, one eyebrow up, quick to doubt a call and slow to be impressed; teasing, never cruel"

// CohostVoice is a Kokoro voice unlike the narrator's: the other sex and the other accent, so a
// viewer hears two people without looking.
func CohostVoice(narrator string) string {
	if strings.HasPrefix(narrator, "af_") || strings.HasPrefix(narrator, "bf_") {
		return "bm_daniel"
	}
	return "bf_alice"
}

func (c *Cohost) defaults() {
	if c.Name == "" {
		c.Name = DefaultCohostName
	}
	if c.Every <= 0 {
		c.Every = 5
	}
	if c.Gap <= 0 {
		c.Gap = time.Second
	}
	if c.book == nil {
		c.book = newPhrasebook(SpeakerCohost)
	}
	if !c.started {
		// The first decision of a run (its thesis) may be needled at once.
		c.lines, c.started = c.Every, true
	}
}

// cohostCue says what the co-host is reacting to; a moment without one is not its business.
func cohostCue(ev Event) string {
	switch ev.Beat {
	case BeatRouteChoice:
		return "the player just chose this run's route and stated its thesis before day one. Doubt it: what they passed over, or what could sink it."
	case BeatFork:
		return "the player just made a big call. Second-guess it: the option they passed over, or the risk in the one they took."
	case BeatBetMade:
		return "the player just staked a bet out loud. Needle it: a sure thing dressed up as a dare, or a reach they will regret."
	case BeatBetWon:
		return "the player's bet came good. Concede it grudgingly, or claim it was never in doubt."
	case BeatBetLost:
		return "the player's bet failed. Rub it in, briefly, without cruelty."
	case BeatBetAbandoned:
		return "the player walked away from a bet they staked. Call it what it is."
	}
	switch {
	case PlanPivot(ev):
		return "the player changed course this morning. Ask why now, or what was wrong with yesterday's plan."
	case ev.Ghost != nil && ev.Ghost.Status == PaceBehind:
		return "the player has fallen behind the book, the lab's test runs of this route. Poke at it."
	case ev.Kind == KindTrade && ev.Trade != nil && ev.Trade.HasProfit && ev.Trade.Profit < 0,
		ev.Kind == KindDay && ev.Summary != nil && ev.Summary.NetWorthDelta < 0:
		return "that one cost the player. Ask what went wrong, or remind them they were warned."
	}
	return ""
}

// noteLine counts a narrator line toward the next exchange.
func (c *Cohost) noteLine() {
	c.defaults()
	c.lines++
}

// due reports whether the co-host reacts to the moment the narrator just told.
func (c *Cohost) due(ev Event) bool {
	c.defaults()
	return c.lines >= c.Every && cohostCue(ev) != ""
}

// exchange is the co-host's line about the moment the narrator just told (said), and the
// narrator's answer when it is due. playing is how long the narrator's line still plays; it
// returns how long the last line spoken still plays. Each line waits for the one before it to
// finish and stands down if a moment as loud as ev is waiting by then; a quieter one (a counter
// story, the night) waits the few seconds, as it may (Narrator.slack).
func (n *Narrator) exchange(ctx context.Context, ev Event, said Say, playing time.Duration) time.Duration {
	c := n.Cohost
	start := n.now()
	facts := FactsOf(ev)
	n.history(ev, &facts)
	line, sp := n.cohostLine(ctx, ev, facts, said.Line)
	if line == "" {
		return n.left(start, playing)
	}
	c.lines = 0
	c.exchanges++
	sp.Speaker, sp.Name, sp.For = SpeakerCohost, c.Name, ev.Seq
	voice := c.Voice
	if voice == "" {
		voice = CohostVoice(n.voice())
	}
	dur := n.voiceLine(ctx, &sp, voice, fmt.Sprintf("cohost-%06d.wav", ev.Seq))
	if !n.waitTurn(ctx, n.left(start, playing)+c.Gap, Priority(ev)) {
		return 0
	}
	c.book.add(line)
	c.recent = append(c.recent, line)
	if len(c.recent) > 4 {
		c.recent = c.recent[1:]
	}
	pub := n.Publish(Event{Kind: KindSay, Day: ev.Day, Run: ev.Run, Level: LevelNotable, Text: line, Say: &sp})
	n.Log.Info("cohost: said", "kind", ev.Kind, "beat", ev.Beat, "check", sp.Check, "llm_ms", sp.MS, "line", line)
	start, playing = n.now(), dur
	if c.ReplyEvery <= 0 || (c.exchanges-1)%c.ReplyEvery != 0 {
		return playing
	}
	reply, rs := n.replyLine(ctx, ev, facts, said.Line, line)
	if reply == "" {
		return n.left(start, playing)
	}
	rs.Name, rs.For, rs.ReplyTo = n.personaName(), ev.Seq, pub.Seq
	rdur := n.voiceLine(ctx, &rs, n.voice(), fmt.Sprintf("reply-%06d.wav", ev.Seq))
	if !n.waitTurn(ctx, n.left(start, playing)+c.Gap, Priority(ev)) {
		return 0
	}
	n.book().add(reply)
	n.Publish(Event{Kind: KindSay, Day: ev.Day, Run: ev.Run, Level: LevelNotable, Text: reply, Say: &rs})
	n.Log.Info("cohost: answered", "check", rs.Check, "llm_ms", rs.MS, "line", reply)
	return rdur
}

// left is what remains of playing, counted from start.
func (n *Narrator) left(start time.Time, playing time.Duration) time.Duration {
	return max(0, playing-n.now().Sub(start))
}

// waitTurn sleeps d and reports whether the floor is still free: no moment of priority prio or
// more waits for the voice.
func (n *Narrator) waitTurn(ctx context.Context, d time.Duration, prio int) bool {
	if Sleep(ctx, d) != nil {
		return false
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.pending != nil && Priority(*n.pending) >= prio {
		n.Log.Info("cohost: a moment is waiting; stood down")
		return false
	}
	return true
}

// voiceLine writes a line's audio under name and returns how long it plays.
func (n *Narrator) voiceLine(ctx context.Context, s *Say, voice, name string) time.Duration {
	if n.TTS == nil || n.AudioDir == "" {
		return 0
	}
	tctx, cancel := context.WithTimeout(ctx, n.MaxLag)
	wav, err := n.TTS.Speak(tctx, speakable(s.Line), voice)
	cancel()
	if err != nil {
		n.Log.Warn("cohost: tts failed; caption only", "err", err)
		return 0
	}
	if os.WriteFile(filepath.Join(n.AudioDir, name), wav, 0o644) != nil {
		return 0
	}
	s.Audio, s.Voice = n.AudioURL+name, voice
	return wavDuration(wav)
}

func (n *Narrator) personaName() string {
	id := n.personaID()
	if id == "" {
		return ""
	}
	return LoadPersona(n.PersonaD, id).Name
}

// cohostLine asks for the co-host's line, checked like the narrator's; "" when none passes.
func (n *Narrator) cohostLine(ctx context.Context, ev Event, f Facts, told string) (string, Say) {
	c := n.Cohost
	p := LoadPersona(n.PersonaD, n.personaID())
	var sys strings.Builder
	fmt.Fprintf(&sys, "You are %s, the co-host in the booth of a live stream where an AI pawnbroker, the %s, plays the game Probably Stolen and narrates its own play. The player's voice: %s.\n", c.Name, p.Name, p.Style)
	fmt.Fprintf(&sys, "Your voice: %s. You are not the player: you react to the player's decisions, speaking to them as you, or aside to the viewers.\n", cohostStyle)
	sys.WriteString("Speak ONE short sentence, two at most, at most 22 words. It is read aloud by text-to-speech: plain words only, no emojis, no stage directions, no quotation marks, no names of real people. " +
		"Invent attitude freely, but the numbers are sacred: use at most one number, only one from the facts you are given, written in digits, and never claim anything about money, rent or trades that the facts do not say. " +
		"Never name a weekday. Money is credits. Do not repeat your previous lines or their images.")
	var b strings.Builder
	fmt.Fprintf(&b, "Day %d. Moment: %s\nThis moment: %s\n", ev.Day, ev.Text, cohostCue(ev))
	if pl := ev.Plan; pl != nil {
		if len(pl.Headline) > 0 {
			fmt.Fprintf(&b, "Their plan: %s\n", strings.Join(pl.Headline, "; "))
		}
		if len(pl.Why) > 0 {
			fmt.Fprintf(&b, "Their reasons: %s\n", strings.Join(pl.Why[:min(3, len(pl.Why))], "; "))
		}
	}
	if facts := f.Prompt(); facts != "" {
		b.WriteString("Facts:\n" + facts)
	}
	b.WriteString(f.history)
	fmt.Fprintf(&b, "The player just said: %s\n", told)
	if len(c.recent) > 0 {
		fmt.Fprintf(&b, "Your previous lines: %s\n", strings.Join(c.recent, " / "))
	}
	if avoid := c.book.avoid(); len(avoid) > 0 {
		fmt.Fprintf(&b, "Words to rest (do not use them): %s\n", strings.Join(avoid, "; "))
	}
	b.WriteString("Your line:")
	return n.checked(ctx, c.LLM, sys.String(), b.String(), f, c.book, ev.Text+"\n"+told)
}

// replyLine is the narrator answering the co-host, in the narrator's own voice and phrasebook.
func (n *Narrator) replyLine(ctx context.Context, ev Event, f Facts, told, jab string) (string, Say) {
	var b strings.Builder
	fmt.Fprintf(&b, "Day %d. Moment: %s\nYou said: %s\nYour co-host %s, a sceptical rival in the booth, came back at you: %s\n", ev.Day, ev.Text, told, n.Cohost.Name, jab)
	if facts := f.Prompt(); facts != "" {
		b.WriteString("Facts:\n" + facts)
	}
	if avoid := n.book().avoid(); len(avoid) > 0 {
		fmt.Fprintf(&b, "Words to rest (do not use them): %s\n", strings.Join(avoid, "; "))
	}
	b.WriteString("Answer back in ONE short sentence, at most 18 words, in your character: brush it off, defend the call, or concede with a wink. Do not just repeat your line. Your line:")
	return n.checked(ctx, n.Cohost.LLM, n.system(n.mood(ev)), b.String(), f, n.book(), ev.Text+"\n"+told+"\n"+jab)
}

// checked asks for a line and holds it to the facts and the phrasebook, once more with the faults
// named; "" when neither try passes. An exchange has no caption to fall back on: a co-host
// reading the caption would be the narrator's job done twice.
func (n *Narrator) checked(ctx context.Context, client llm.Client, system, prompt string, f Facts, pb *phrasebook, news string) (string, Say) {
	start := n.now()
	msgs := []llm.Message{{Role: llm.RoleUser, Content: prompt}}
	var first []string
	for try := 0; try < 2; try++ {
		cctx, cancel := context.WithTimeout(ctx, n.Timeout)
		resp, err := client.Complete(cctx, llm.Request{System: system, Messages: msgs, MaxTokens: 100})
		cancel()
		if err != nil {
			n.Log.Info("cohost: llm failed; no exchange", "err", err)
			return "", Say{}
		}
		l := cleanLine(resp.Text)
		if l == "" {
			return "", Say{}
		}
		faults := CheckLine(l, f)
		if len(faults) == 0 {
			faults = pb.repeats(l, news)
		}
		if len(faults) == 0 {
			check := "pass"
			if try > 0 {
				check = "retry"
			}
			return l, Say{Line: l, Source: "llm", Check: check, Faults: first, MS: n.now().Sub(start).Milliseconds()}
		}
		fs := make([]string, len(faults))
		for i, ft := range faults {
			fs[i] = string(ft)
		}
		if try == 0 {
			first = fs
		}
		n.Log.Info("cohost: line rejected", "try", try+1, "line", l, "faults", fs)
		msgs = append(msgs, llm.Message{Role: llm.RoleAssistant, Content: l},
			llm.Message{Role: llm.RoleUser, Content: "That line will not do: " + strings.Join(fs, "; ") + ". Say it again, using only the facts listed. Your line:"})
	}
	return "", Say{}
}
