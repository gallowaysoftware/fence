package strategy

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/gallowaysoftware/fence/internal/llm"
	"github.com/gallowaysoftware/fence/internal/protocol"
)

// betLLM writes a fixed plan and answers each delta ask with the next of its answers.
type betLLM struct {
	mu      sync.Mutex
	answers []string
	asks    []string
}

func (f *betLLM) Complete(_ context.Context, req llm.Request) (llm.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	last := req.Messages[len(req.Messages)-1].Content
	if strings.HasPrefix(last, executorRules) {
		f.asks = append(f.asks, last)
		a := "{}"
		if len(f.answers) > 0 {
			a, f.answers = f.answers[0], f.answers[1:]
		}
		return llm.Response{Text: a, Model: "fake"}, nil
	}
	if req.System == reviewSystem {
		return llm.Response{Text: "- MET: x", Model: "fake"}, nil
	}
	return llm.Response{Text: "CALENDAR\n- nothing\nGOALS\n- goal one\n- goal two\nCASH\n- 0\nFACTIONS\n- d\nWINDOW\n- w\nSTOCK\n- e\nKEEP\n- k\nPRODUCTION\n- p\nRISK\n- f\nCUSTOMERS\n- g", Model: "fake"}, nil
}

func TestStrategistBets(t *testing.T) {
	f := &betLLM{answers: []string{
		`{"bet":{"claim":"Cash past 2,000 by day 12","kind":"cash","target":2000,"by":12}}`,
		`{"bet":{"claim":"Cash past 900, easy","kind":"cash","target":900,"by":13}}`,
	}}
	s := New(Options{LLM: f, DeltaDirectives: true, Bets: true})
	defer s.Close()

	s.Event("s1", snapshotEvent("morning", 10, 1000))
	s.wg.Wait()
	if mv := s.CurrentBet("s1"); mv == nil || mv.Stake == nil || mv.Stake.Day != 10 || mv.Stake.Target != 2000 {
		t.Fatalf("bet after day 10: %+v", mv)
	}
	if !strings.Contains(f.asks[0], "You have no open bet.") || !strings.Contains(f.asks[0], `"kind": "<kind>"`) {
		t.Errorf("day 10 ask lacks the bet rules:\n%s", f.asks[0])
	}

	s.Outcome("s1", protocol.Outcome{DecisionID: "d1", GameDay: 10, Result: []byte(`{"kind":"trade","side":"sold","items":[1],"cash_delta":1450}`)})
	s.Event("s1", snapshotEvent("close", 10, 2450))
	s.Event("s1", snapshotEvent("morning", 11, 2450))
	s.wg.Wait()
	ask := f.asks[1]
	if !strings.Contains(ask, `Your bet "Cash past 2,000 by day 12" (staked day 10, due day 12) was WON: cash 2,450 on day 10.`) || !strings.Contains(ask, "You have no open bet.") {
		t.Errorf("day 11 ask does not report the win:\n%s", ask)
	}
	// Day 11's bet is already true: filler, rejected.
	if mv := s.CurrentBet("s1"); mv != nil {
		t.Errorf("a bet already true was staked: %+v", mv)
	}
}
