package main

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
)

type Character struct {
	Player      int    `json:"player"`
	Name        string `json:"name"`
	Emoji       string `json:"emoji"`
	Personality string `json:"personality"`
	SpeechStyle string `json:"speech_style"`
}

type Word struct {
	JP      string `json:"jp"`
	Reading string `json:"reading"`
	EN      string `json:"en"`
}

type Briefing struct {
	Player    int    `json:"player"`
	Reading   string `json:"reading"`
	ReadingEN string `json:"reading_en"`
	Goal      string `json:"goal"`
	Teach     string `json:"teach"`
	Words     []Word `json:"words"`
}

type LinePlan struct {
	Index   int    `json:"index"`
	Player  int    `json:"player"`
	Beat    string `json:"beat"`
	Grammar string `json:"grammar"`
	Mood    string `json:"mood"`
}

type Plan struct {
	Scenario struct {
		Title   string `json:"title"`
		Setting string `json:"setting"`
		Premise string `json:"premise"`
		Mood    string `json:"mood"`
	} `json:"scenario"`
	Characters []Character `json:"characters"`
	Briefings  []Briefing  `json:"briefings"`
	LinePlan   []LinePlan  `json:"line_plan"`
}

type Seg struct {
	T string `json:"t"`
	R string `json:"r,omitempty"`
}

type Quiz struct {
	Question    string   `json:"question"`
	Options     []string `json:"options"`
	Answer      int      `json:"answer"`
	Explanation string   `json:"explanation"`
}

type LineData struct {
	Chunks         [][]Seg `json:"chunks"`
	AcceptedOrders [][]int `json:"accepted_orders"`
	English        string  `json:"english"`
	Quiz           Quiz    `json:"quiz"`
}

type PlayerInfo struct {
	Index     int
	Name      string
	Level     string
	Pos       string
	LevelDesc string
}

var levels = []string{"N5", "N4", "N3", "N2", "N1"}
var positions = []string{"lower", "middle", "upper"}

func splitLv(lv int) (string, string) {
	lv = max(0, min(14, lv))
	return levels[lv/3], positions[lv%3]
}

func lvIndex(level, pos string) int {
	li, pi := 0, 1
	for i, l := range levels {
		if l == level {
			li = i
		}
	}
	for i, p := range positions {
		if p == pos {
			pi = i
		}
	}
	return li*3 + pi
}

var levelDesc = map[string]string{
	"N5": "beginner: hiragana, katakana, about 100 kanji, basic particles, です/ます",
	"N4": "basic: て-form, たい, から, ている, simple conditionals",
	"N3": "intermediate: ので, たら, passive and causative, common compound verbs",
	"N2": "upper-intermediate: ように, ばかり, ても, formal expressions",
	"N1": "advanced: nuanced, literary and idiomatic expressions",
}

var chunkRange = map[string][2]int{
	"N5": {3, 5}, "N4": {4, 6}, "N3": {5, 8}, "N2": {6, 9}, "N1": {6, 10},
}

func posLabel(p string) string {
	return map[string]string{"lower": "Low", "middle": "Mid", "upper": "High"}[p]
}

func lvLabel(i int) string {
	l, p := splitLv(i)
	return posLabel(p) + " " + l
}

func isHan(r rune) bool { return unicode.Is(unicode.Han, r) || r == '々' }

// parseNotation turns "駅[えき]で" into segments with furigana on the kanji.
func parseNotation(s string) []Seg {
	rs := []rune(s)
	var out []Seg
	var buf []rune
	flush := func() {
		if len(buf) > 0 {
			out = append(out, Seg{T: string(buf)})
			buf = nil
		}
	}
	for i := 0; i < len(rs); {
		if isHan(rs[i]) {
			j := i
			for j < len(rs) && isHan(rs[j]) {
				j++
			}
			if j < len(rs) && (rs[j] == '[' || rs[j] == '［') {
				k := j + 1
				for k < len(rs) && rs[k] != ']' && rs[k] != '］' {
					k++
				}
				if k < len(rs) {
					flush()
					out = append(out, Seg{T: string(rs[i:j]), R: string(rs[j+1 : k])})
					i = k + 1
					continue
				}
			}
			buf = append(buf, rs[i:j]...)
			i = j
			continue
		}
		// A reading after kana (for example a katakana name) is dropped: only kanji get furigana.
		if (rs[i] == '[' || rs[i] == '［') && len(buf) > 0 && !isHan(buf[len(buf)-1]) && isKana(string(buf[len(buf)-1:])) {
			k := i + 1
			for k < len(rs) && rs[k] != ']' && rs[k] != '］' {
				k++
			}
			if k < len(rs) {
				i = k + 1
				continue
			}
		}
		buf = append(buf, rs[i])
		i++
	}
	flush()
	return out
}

func chunkText(c []Seg) string {
	var b strings.Builder
	for _, s := range c {
		b.WriteString(s.T)
	}
	return b.String()
}

func nLinesFor(players int) int {
	rounds := (8 + players - 1) / players
	return players * rounds
}

func checkPlan(p *Plan, n, nLines int) error {
	if strings.TrimSpace(p.Scenario.Title) == "" {
		return fmt.Errorf("scenario.title is empty")
	}
	if len(p.Characters) < n {
		return fmt.Errorf("need %d characters, got %d", n, len(p.Characters))
	}
	if len(p.Briefings) < n {
		return fmt.Errorf("need %d briefings, got %d", n, len(p.Briefings))
	}
	if len(p.LinePlan) < nLines {
		return fmt.Errorf("need %d line_plan entries, got %d", nLines, len(p.LinePlan))
	}
	p.Characters = p.Characters[:n]
	p.Briefings = p.Briefings[:n]
	p.LinePlan = p.LinePlan[:nLines]
	for i := range p.Characters {
		p.Characters[i].Player = i
		if strings.TrimSpace(p.Characters[i].Name) == "" {
			return fmt.Errorf("character %d has no name", i)
		}
		if p.Characters[i].Emoji == "" {
			p.Characters[i].Emoji = "🙂"
		}
	}
	for i := range p.Briefings {
		p.Briefings[i].Player = i
		if strings.TrimSpace(p.Briefings[i].Reading) == "" {
			return fmt.Errorf("briefing %d has no reading", i)
		}
	}
	moods := map[string]bool{"calm": true, "happy": true, "excited": true, "sad": true, "tense": true, "mysterious": true}
	for i := range p.LinePlan {
		lp := &p.LinePlan[i]
		lp.Index, lp.Player = i, i%n
		if strings.TrimSpace(lp.Beat) == "" {
			return fmt.Errorf("line_plan %d has no beat", i)
		}
		if !moods[lp.Mood] {
			lp.Mood = "calm"
		}
	}
	return nil
}

func isPerm(o []int, n int) bool {
	if len(o) != n {
		return false
	}
	seen := make([]bool, n)
	for _, x := range o {
		if x < 0 || x >= n || seen[x] {
			return false
		}
		seen[x] = true
	}
	return true
}

func isKana(s string) bool {
	for _, r := range s {
		if !unicode.Is(unicode.Hiragana, r) && !unicode.Is(unicode.Katakana, r) && r != 'ー' && r != '・' {
			return false
		}
	}
	return s != ""
}

func checkLine(d *LineData) error {
	n := len(d.Chunks)
	if n < 3 || n > 11 {
		return fmt.Errorf("chunks must have 3 to 11 entries, got %d", n)
	}
	for i, c := range d.Chunks {
		if strings.TrimSpace(chunkText(c)) == "" {
			return fmt.Errorf("chunk %d is empty", i)
		}
		for j, sg := range c {
			han := strings.ContainsFunc(sg.T, isHan)
			switch {
			case han && sg.R == "":
				return fmt.Errorf("chunk %d: %q has kanji but no furigana (r)", i, sg.T)
			case sg.R != "" && !isKana(sg.R):
				return fmt.Errorf("chunk %d: furigana %q must be hiragana or katakana only", i, sg.R)
			case sg.R != "" && !han:
				d.Chunks[i][j].R = ""
			}
		}
	}
	id := identity(n)
	orders := [][]int{id}
	for _, o := range d.AcceptedOrders {
		// Japanese is predicate-final: an order that moves the last chunk is dropped.
		if !isPerm(o, n) || o[n-1] != n-1 || len(orders) >= 8 {
			continue
		}
		if !slices.ContainsFunc(orders, func(x []int) bool { return slices.Equal(x, o) }) {
			orders = append(orders, o)
		}
	}
	d.AcceptedOrders = orders
	if strings.TrimSpace(d.English) == "" {
		return fmt.Errorf("english is empty")
	}
	q := &d.Quiz
	if strings.TrimSpace(q.Question) == "" || len(q.Options) != 4 {
		return fmt.Errorf("quiz needs a question and exactly 4 options")
	}
	if strings.TrimSpace(q.Explanation) == "" {
		return fmt.Errorf("quiz explanation is empty")
	}
	seen := map[string]bool{}
	for _, o := range q.Options {
		o = strings.TrimSpace(o)
		if o == "" || seen[o] {
			return fmt.Errorf("quiz options must be non-empty and distinct")
		}
		seen[o] = true
	}
	if q.Answer < 0 || q.Answer > 3 {
		return fmt.Errorf("quiz answer must be 0 to 3")
	}
	return nil
}

func identity(n int) []int {
	o := make([]int, n)
	for i := range o {
		o[i] = i
	}
	return o
}

// sentence joins a line's chunks in the given order.
func sentence(d *LineData, order []int) string {
	var b strings.Builder
	for _, i := range order {
		b.WriteString(chunkText(d.Chunks[i]))
	}
	return b.String()
}
