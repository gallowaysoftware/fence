package strategy

import (
	"strings"
	"testing"
)

func TestPlanPromptAudienceGoesLast(t *testing.T) {
	sc := loadScenario(t, "rent-tomorrow-low-cash")
	in := sc.Input()
	in.Audience = "TWITCH CHAT VOTE: zz_chat_tally"
	p, err := BuildPlanPrompt(in)
	if err != nil {
		t.Fatal(err)
	}
	i := strings.Index(p, "zz_chat_tally")
	if i < 0 || strings.Contains(p[i:], "TODAY:") || !strings.Contains(p[i:], "Write the plan for day") {
		t.Fatalf("the audience is not the last thing before the ask:\n%s", p)
	}
}
