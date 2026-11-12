package show

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// The rehearsal's Shark said "in the till" 17 times in 156 lines: a four-word guard over the
// last 20 lines saw neither three-word phrases nor an image coming back in other words. A
// phrasebook remembers a persona's whole run.

const (
	// gram3Keep and gram4Keep are how many lines back a three- or four-word phrase may not
	// come back: about 10 and 20 minutes at two lines a minute.
	gram3Keep = 20
	gram4Keep = 40
	// motifRest is how many lines an image rests after it is used.
	motifRest = 10
	// openKeep is how many lines back a line may not open with the same two words.
	openKeep = 6
	// avoidMax caps the prompt's list of phrases to rest.
	avoidMax = 12
)

// A motif is an image a persona leans on, in any wording ("in the till", "the till is fat").
type motif struct {
	name string
	re   *regexp.Regexp
}

func motifOf(name, re string) motif { return motif{name, regexp.MustCompile(`\b(?:` + re + `)`)} }

// commonMotifs are the images every narrator voice reached for in the rehearsal and the
// full replay; a persona's bank adds its own.
var commonMotifs = []motif{
	motifOf("the till", `(?:the|my|in|into|to|a) till\b`),
	motifOf("blood in the water", `blood`),
	motifOf("in my pocket", `(?:my|in|into|the) pockets?\b`),
	motifOf("a whisper", `whisper`),
	motifOf("breathe easy", `breath(?:e|ing)? easy`),
	motifOf("a scratch", `scratch`),
	motifOf("a rounding error", `rounding error`),
	motifOf("a joke", `(?:a|just a) joke\b`),
	motifOf("the marks", `marks?\b`),
	motifOf("no sweat", `no sweat`),
	motifOf("the landlord's door", `landlord'?s? (?:door|knock)`),
}

// A phrasebook is what one persona has said this run, for repeats beyond the prompt's few.
type phrasebook struct {
	lines  []string
	motifs []motif
	// uses counts each motif over the whole run.
	uses map[string]int
	// next is the image bank's rotation, by mood group.
	next map[string]int
}

func newPhrasebook(persona string) *phrasebook {
	pb := &phrasebook{uses: map[string]int{}, next: map[string]int{}}
	pb.motifs = append(pb.motifs, commonMotifs...)
	for _, group := range imageBanks[persona] {
		for _, im := range group {
			if key, _, ok := strings.Cut(im, "|"); ok {
				pb.motifs = append(pb.motifs, motifOf(key, regexp.QuoteMeta(key)))
			}
		}
	}
	return pb
}

func (pb *phrasebook) add(line string) {
	pb.lines = append(pb.lines, line)
	if len(pb.lines) > gram4Keep {
		pb.lines = pb.lines[1:]
	}
	for _, m := range pb.motifsIn(line) {
		pb.uses[m]++
	}
}

func (pb *phrasebook) recent(n int) []string { return pb.lines[max(0, len(pb.lines)-n):] }

func (pb *phrasebook) motifsIn(line string) []string {
	l := normLine(line)
	var out []string
	for _, m := range pb.motifs {
		if m.re.MatchString(l) && !slices.Contains(out, m.name) {
			out = append(out, m.name)
		}
	}
	return out
}

// repeats names what line says again: a three-word phrase of the last gram3Keep lines, a
// four-word one of the last gram4Keep, an image of the last motifRest, or the opening two
// words of the last openKeep. news is the moment's own caption and facts: a phrase made of
// its words ("the dumping ground", "Master Appraiser") is news, not a habit.
func (pb *phrasebook) repeats(line, news string) []Fault {
	var out []Fault
	newsWords := setOf(lineWordRe.FindAllString(normLine(news), -1)...)
	for _, k := range []struct{ n, keep int }{{4, gram4Keep}, {3, gram3Keep}} {
		seen := map[string]bool{}
		for _, e := range pb.recent(k.keep) {
			for _, g := range colourGrams(e, k.n, nil) {
				seen[g] = true
			}
		}
		for _, g := range colourGrams(line, k.n, newsWords) {
			if seen[g] {
				out = append(out, Fault(fmt.Sprintf("repeats %q from an earlier line: find a new image", g)))
				return out
			}
		}
	}
	rested := map[string]bool{}
	for _, e := range pb.recent(motifRest) {
		for _, m := range pb.motifsIn(e) {
			rested[m] = true
		}
	}
	inNews := pb.motifsIn(news)
	for _, m := range pb.motifsIn(line) {
		if rested[m] && !slices.Contains(inNews, m) {
			out = append(out, Fault(fmt.Sprintf("leans on %q again, used a few lines ago: pick a different image", m)))
			break
		}
	}
	if o := opening(line); o != "" {
		for _, e := range pb.recent(openKeep) {
			if opening(e) == o {
				out = append(out, Fault(fmt.Sprintf("opens with %q like a recent line: start differently", o)))
				break
			}
		}
	}
	return out
}

// avoid is what the prompt asks the narrator to rest: the images and phrases of its recent
// lines, most used first, and the openings it just used.
func (pb *phrasebook) avoid() []string {
	var out []string
	add := func(s string) {
		if s != "" && !slices.Contains(out, s) && len(out) < avoidMax {
			out = append(out, s)
		}
	}
	var ms []string
	for _, e := range pb.recent(motifRest) {
		ms = append(ms, pb.motifsIn(e)...)
	}
	slices.SortStableFunc(ms, func(a, b string) int { return pb.uses[b] - pb.uses[a] })
	for _, m := range ms {
		add(m)
	}
	counts := map[string]int{}
	var grams []string
	for _, e := range pb.recent(gram3Keep) {
		for _, g := range colourGrams(e, 3, nil) {
			if counts[g] == 0 {
				grams = append(grams, g)
			}
			counts[g]++
		}
	}
	slices.SortStableFunc(grams, func(a, b string) int { return counts[b] - counts[a] })
	for _, g := range grams[:min(len(grams), 6)] {
		add(g)
	}
	for _, e := range slices.Backward(pb.recent(3)) {
		if o := opening(e); o != "" {
			add("opening with " + o)
		}
	}
	return out
}

// image is the bank's next image for a mood that the recent lines have not used, "" when
// the persona has no bank.
func (pb *phrasebook) image(persona, mood string) string {
	g := moodGroup(mood)
	bank := imageBanks[persona][g]
	if len(bank) == 0 {
		return ""
	}
	rested := map[string]bool{}
	for _, e := range pb.recent(motifRest) {
		for _, m := range pb.motifsIn(e) {
			rested[m] = true
		}
	}
	for range bank {
		i := pb.next[g] % len(bank)
		pb.next[g]++
		key, image, _ := strings.Cut(bank[i], "|")
		if !rested[key] {
			return image
		}
	}
	return ""
}

func moodGroup(mood string) string {
	switch mood {
	case "flush", "triumphant":
		return "up"
	case "broke", "tight":
		return "down"
	}
	return "even"
}

func normLine(s string) string {
	return strings.ToLower(strings.NewReplacer("’", "'", "‘", "'").Replace(s))
}

// colourGrams are a line's n-word phrases that carry words of colour (two in three, one in
// four), without those whose colour words are all news.
func colourGrams(line string, n int, news map[string]bool) []string {
	var out []string
	// A phrase runs within a clause: "in a row. The board" is two thoughts, not a habit.
	for _, clause := range clauseRe.Split(normLine(line), -1) {
		out = append(out, clauseGrams(lineWordRe.FindAllString(clause, -1), n, news)...)
	}
	return out
}

var clauseRe = regexp.MustCompile(`[^a-z' -]+`)

func clauseGrams(ws []string, n int, news map[string]bool) []string {
	var out []string
	for i := 0; i+n <= len(ws); i++ {
		colour, fresh := 0, false
		for _, w := range ws[i : i+n] {
			if !stopWords[w] && !plainWords[w] {
				colour++
				if !news[w] {
					fresh = true
				}
			}
		}
		// A three-word phrase needs two words of colour: "then hitting the" is grammar, not a
		// habit; a one-word image ("the till") is a motif's job.
		if (colour >= 2 || (colour == 1 && n >= 4)) && fresh {
			out = append(out, strings.Join(ws[i:i+n], " "))
		}
	}
	return out
}

// opening is a line's first two words, "" for a line shorter than that.
func opening(line string) string {
	ws := lineWordRe.FindAllString(normLine(line), 2)
	if len(ws) < 2 {
		return ""
	}
	return ws[0] + " " + ws[1]
}

// imageBanks are a few images per persona and mood group (up: flush or triumphant; down:
// tight or broke; even: steady), "key|image", offered one at a time in rotation so a voice
// does not settle on one. The key is how a line using it is recognised.
var imageBanks = map[string]map[string][]string{
	"shark": {
		"up":   {"fin|a fin cutting through calm water", "feast|a feeding frenzy after a lean week", "high roller|a high roller with house money", "teeth|a grin with too many teeth"},
		"down": {"tide|the tide going out and leaving you on the rocks", "hungry|an empty belly that sharpens the hunt", "thin ice|a hustler skating on thin ice", "cornered|a cornered animal, at its most dangerous"},
		"even": {"bait|bait on the hook, waiting for a bite", "card|a card sharp counting the deck", "circling|circling slow, patient", "street|the street that taught you to read a face", "angle|every customer an angle to work", "hustle|the long hustle, one mark at a time"},
	},
	"magnate": {
		"up":   {"dividend|a dividend arriving on schedule", "club|a seat at the club by the window", "portfolio|a portfolio quietly compounding"},
		"down": {"weather|weather that a sound house rides out", "belt|a belt tightened a notch, nothing more", "ledger|red ink in a ledger you intend to correct"},
		"even": {"chess|a slow game of chess", "tea|tea served at the proper hour", "line item|a line item, filed and forgotten", "estate|an estate run by the book"},
	},
	"honest-broker": {
		"up":   {"polish|a piece polished until it shines", "reputation|a good name that pays its own way", "hallmark|a hallmark stamped true"},
		"down": {"patience|a jeweller's patience under strain", "loupe|a flaw under the loupe", "scale|a scale that will not lie, even when you wish it would"},
		"even": {"loupe|a jeweller's loupe", "ledger|a clean ledger", "handshake|a handshake that means something", "weigh|weighing every piece twice"},
	},
	"fence": {
		"up":   {"back room|a full back room and a locked door", "quiet money|quiet money that never asks questions", "nod|a nod from the right people"},
		"down": {"cool off|heat on the street and nowhere to cool off", "lie low|lying low until it blows over", "shadow|a shadow that keeps getting shorter"},
		"even": {"wink|a wink and a handshake", "back door|the back door that never squeaks", "no questions|no questions asked", "shady|friends in shady places"},
	},
	"vintner": {
		"up":   {"vintage|a vintage that will be talked about", "toast|a toast to patience", "bouquet|a bouquet opening up"},
		"down": {"frost|a frost on the vines", "sour|a batch turning sour", "drought|a dry season for the grapes"},
		"even": {"vines|vines that cannot be hurried", "barrel|a barrel quietly aging", "harvest|waiting for the harvest", "cork|a cork drawn at the right moment"},
	},
	"cook": {
		"up":   {"crystal|crystal-clear product", "chemistry|chemistry doing exactly what it should", "precision|precision paying off"},
		"down": {"fumes|fumes in a shut room", "ticking|a ticking clock", "pressure cooker|a pressure cooker"},
		"even": {"flame|a flask on a steady flame", "titrat|titrating drop by drop", "to the gram|a recipe followed to the gram", "straight face|a lab coat and a straight face"},
	},
	"smelter": {
		"up":   {"gold bars|ingots stacked like gold bars", "roaring|a forge roaring hot", "vein|a vein of good ore"},
		"down": {"slag|a heap of slag", "gone cold|a furnace gone cold", "rust|rust eating at the edges"},
		"even": {"anvil|hammer on the anvil", "melts down|everything melts down eventually", "faith|a pinch of flux and a little faith", "waiting to be|scrap waiting to be something"},
	},
	"rancher": {
		"up":   {"market day|a fat herd on market day", "full barn|a full barn", "corn|corn as high as the ceiling"},
		"down": {"dry spell|a dry spell on the ranch", "fox|a fox in the henhouse", "lean winter|a lean winter"},
		"even": {"pasture|a quiet pasture", "trough|a full trough", "chores|morning chores", "counting heads|counting heads at dusk"},
	},
	"armourer": {
		"up":   {"armoury|a well-stocked armoury", "in order|paperwork in perfect order", "clean record|a clean record"},
		"down": {"jam|a jammed breech", "audit|an audit looming", "short supply|ammunition in short supply"},
		"even": {"drill|routine drill", "regulation|by the regulation", "safety catch|the safety catch on", "rubber stamp|a stamp on every form"},
	},
	"water-baron": {
		"up":   {"reservoir|a reservoir filled to the brim", "flood|a flood of orders", "faucet|every faucet on the station running to you"},
		"down": {"dry well|a dry well", "leak|a leak in the pipes", "pressure|pressure dropping in the main"},
		"even": {"thirst|a thirsty station", "pipes|water through the right pipes", "meter|a meter quietly ticking", "spring|a steady spring"},
	},
	"fishmonger": {
		"up":   {"catch of the day|the catch of the day on ice", "caviar|caviar on a silver spoon", "full net|a full net hauled in"},
		"down": {"belly up|fish floating belly up", "dark tank|a dark tank and a silent pump", "low tide|low tide and empty nets"},
		"even": {"fillet knife|a fillet knife at dawn", "gills|a fish judged by its gills", "crushed ice|fresh fish on crushed ice", "market bell|the market bell at opening"},
	},
	"pharmacist": {
		"up":   {"lab pure|a lab-pure batch", "to the milligram|a prescription filled to the milligram", "white coat|a spotless white coat"},
		"down": {"recalled|a batch recalled", "side effect|an unwanted side effect", "contaminated|a contaminated flask"},
		"even": {"dosage|the right dosage", "pestle|mortar and pestle", "measure twice|measure twice, press once", "true label|a label that tells the truth"},
	},
	"botanist": {
		"up":   {"full bloom|a bay in full bloom", "took root|cuttings that took root", "canopy|a green canopy overhead"},
		"down": {"wilt|a wilting leaf", "blight|blight in the bays", "root rot|root rot under the surface"},
		"even": {"reeds|reeds doing their quiet work", "seedling|a seedling's patience", "photosynth|light turned into sugar", "clean roots|clean water, clean roots"},
	},
}
