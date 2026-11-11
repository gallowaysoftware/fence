package knowledge

import (
	"encoding/xml"
	"html"
	"regexp"
	"strings"
)

var (
	reDropBlock = regexp.MustCompile(`(?is)<(script|style|noscript|svg|template|iframe|head|nav|footer|aside|form|button)\b[^>]*>.*?</(?:script|style|noscript|svg|template|iframe|head|nav|footer|aside|form|button)\s*>`)
	reComment   = regexp.MustCompile(`(?s)<!--.*?-->`)
	reHeading   = regexp.MustCompile(`(?is)<h([1-6])\b[^>]*>`)
	reListItem  = regexp.MustCompile(`(?i)<li\b[^>]*>`)
	reCell      = regexp.MustCompile(`(?i)</t[dh]\s*>`)
	reBlock     = regexp.MustCompile(`(?i)</?(p|div|br|tr|table|ul|ol|li|section|article|main|h[1-6]|dl|dt|dd|blockquote|pre|header)\b[^>]*>`)
	reTag       = regexp.MustCompile(`(?s)<[^>]*>`)
	reBlankRuns = regexp.MustCompile(`\n{3,}`)
)

// HTMLToText reduces a page to readable lines: headings become "## …" lines, list items
// "- …", tables " | "-separated rows. It prefers <article>, then <main>, then the
// Steam guide body, then the whole document.
func HTMLToText(page string) string {
	page = reComment.ReplaceAllString(page, "")
	body := page
	for _, pick := range []func(string) string{
		func(s string) string {
			return innerOf(s, `(?is)<div[^>]+class="[^"]*\bguide subSections\b[^"]*"[^>]*>`, "")
		},
		func(s string) string { return innerOf(s, `(?is)<article\b[^>]*>`, "</article>") },
		func(s string) string { return innerOf(s, `(?is)<main\b[^>]*>`, "</main>") },
	} {
		if inner := pick(page); len(inner) > 500 {
			body = inner
			break
		}
	}
	body = reDropBlock.ReplaceAllString(body, "\n")
	body = reHeading.ReplaceAllStringFunc(body, func(h string) string {
		return "\n" + strings.Repeat("#", int(reHeading.FindStringSubmatch(h)[1][0]-'0')) + " "
	})
	body = reListItem.ReplaceAllString(body, "\n- ")
	body = reCell.ReplaceAllString(body, " | ")
	body = reBlock.ReplaceAllString(body, "\n")
	body = reTag.ReplaceAllString(body, " ")
	body = html.UnescapeString(body)

	var out []string
	for _, l := range strings.Split(body, "\n") {
		l = collapseSpace(strings.ReplaceAll(l, " ", " "))
		l = strings.TrimSuffix(l, " |")
		if l == "" || l == "-" || l == "|" {
			if len(out) > 0 && out[len(out)-1] != "" {
				out = append(out, "")
			}
			continue
		}
		out = append(out, l)
	}
	return strings.TrimSpace(reBlankRuns.ReplaceAllString(strings.Join(out, "\n"), "\n\n"))
}

// innerOf returns what follows the first match of open, up to the last occurrence of
// closeTag (or to the end when closeTag is empty).
func innerOf(s, open, closeTag string) string {
	loc := regexp.MustCompile(open).FindStringIndex(s)
	if loc == nil {
		return ""
	}
	rest := s[loc[1]:]
	if closeTag == "" {
		return rest
	}
	if j := strings.LastIndex(strings.ToLower(rest), closeTag); j >= 0 {
		return rest[:j]
	}
	return rest
}

// RedditAtomToText renders a thread's .rss (Atom) feed: the post, then each comment,
// with its author as the locator.
func RedditAtomToText(feed []byte) (string, error) {
	var f struct {
		Title   string `xml:"title"`
		Entries []struct {
			Title   string `xml:"title"`
			Author  string `xml:"author>name"`
			Updated string `xml:"updated"`
			Content string `xml:"content"`
		} `xml:"entry"`
	}
	if err := xml.Unmarshal(feed, &f); err != nil {
		return "", err
	}
	var b strings.Builder
	for i, e := range f.Entries {
		if i == 0 {
			b.WriteString("# " + e.Title + "\n")
		}
		b.WriteString("\n[" + strings.TrimPrefix(e.Author, "/u/") + "]\n")
		b.WriteString(HTMLToText(e.Content) + "\n")
	}
	return b.String(), nil
}
