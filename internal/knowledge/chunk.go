package knowledge

import "strings"

// charsPerToken is a rough English average; chunk sizes only need to be in the right range.
const charsPerToken = 4

type Chunk struct {
	Index int
	Text  string
}

// ChunkText splits text into pieces of at most maxTokens, cutting at line boundaries and
// repeating about overlapTokens of trailing lines at the start of the next piece so a
// claim straddling a cut is seen whole at least once. A single line longer than the
// limit is cut at a space.
func ChunkText(text string, maxTokens, overlapTokens int) []Chunk {
	maxChars := maxTokens * charsPerToken
	overlap := overlapTokens * charsPerToken
	if overlap >= maxChars/2 {
		overlap = maxChars / 2
	}
	var lines []string
	for _, l := range strings.Split(text, "\n") {
		for len(l) > maxChars {
			cut := strings.LastIndexByte(l[:maxChars], ' ')
			if cut <= 0 {
				cut = maxChars
			}
			lines = append(lines, l[:cut])
			l = strings.TrimLeft(l[cut:], " ")
		}
		lines = append(lines, l)
	}

	var chunks []Chunk
	var cur []string
	size := 0
	flush := func() {
		body := strings.TrimSpace(strings.Join(cur, "\n"))
		if body != "" {
			chunks = append(chunks, Chunk{Index: len(chunks), Text: body})
		}
	}
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		if size+len(l)+1 > maxChars && size > 0 {
			flush()
			// Carry the tail forward, but never all of cur, or a run of long lines
			// would never advance.
			var tail []string
			t := 0
			for j := len(cur) - 1; j > 0 && t+len(cur[j])+1 <= overlap; j-- {
				tail = append([]string{cur[j]}, tail...)
				t += len(cur[j]) + 1
			}
			cur, size = tail, t
		}
		cur = append(cur, l)
		size += len(l) + 1
	}
	if len(chunks) == 0 || size > 0 && !endsWith(chunks, cur) {
		flush()
	}
	return chunks
}

// endsWith reports whether cur is entirely the overlap already emitted at the end of
// the last chunk, which would make a final flush a pure duplicate.
func endsWith(chunks []Chunk, cur []string) bool {
	last := chunks[len(chunks)-1].Text
	return strings.HasSuffix(last, strings.TrimSpace(strings.Join(cur, "\n")))
}
