package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gallowaysoftware/fence/internal/llm"
)

// askJSON sends one prompt and decodes the first JSON object of the answer into out,
// retrying once with the parse error spelled out. It returns the final raw answer.
func askJSON(ctx context.Context, c llm.Client, system, user string, maxTokens int, out any) (string, error) {
	req := llm.Request{System: system, Messages: []llm.Message{{Role: llm.RoleUser, Content: user}}, MaxTokens: maxTokens}
	var lastErr error
	var text string
	for attempt := 0; attempt < 2; attempt++ {
		resp, err := c.Complete(ctx, req)
		if err != nil {
			lastErr = err
			continue
		}
		text = resp.Text
		obj, err := llm.ExtractJSON(text)
		if err == nil {
			if err = json.Unmarshal(obj, out); err == nil {
				return text, nil
			}
		}
		lastErr = err
		req.Messages = append(req.Messages,
			llm.Message{Role: llm.RoleAssistant, Content: truncate(text, 2000)},
			llm.Message{Role: llm.RoleUser, Content: fmt.Sprintf("That was not valid JSON for the requested shape (%v). Answer again with only the JSON object.", err)})
	}
	return text, lastErr
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func bullet(xs []string) string {
	var b strings.Builder
	for _, x := range xs {
		b.WriteString("- " + x + "\n")
	}
	return b.String()
}
