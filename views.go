package main

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"strings"

	"github.com/a-h/templ"
)

// V is what a page render knows about the viewer. Me is nil for someone who has not joined.
type V struct {
	R     *Room
	Me    *Player
	MI    int
	Token string
}

func (v V) mood() string {
	if v.R.Phase == "playing" && v.R.Turn < len(v.R.Lines) {
		return v.R.Lines[v.R.Turn].Plan.Mood
	}
	return "calm"
}

func (v V) isHost() bool { return v.Me != nil && v.R.host() == v.Me }

func (r *Room) View(token string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	mi, me := r.find(token)
	var b bytes.Buffer
	if err := Root(V{R: r, Me: me, MI: mi, Token: token}).Render(context.Background(), &b); err != nil {
		logf("render: %v", err)
	}
	return b.String()
}

func badgeText(p *Player) string { return posLabel(p.Pos) + " " + p.Level }

func initial(name string) string {
	if rs := []rune(name); len(rs) > 0 {
		return string(rs[:1])
	}
	return "?"
}

// Furigana markup is built here with explicit escaping so no stray whitespace lands between ruby elements.
func segsHTML(ss []Seg) string {
	var b strings.Builder
	for _, s := range ss {
		if s.R != "" {
			fmt.Fprintf(&b, "<ruby>%s<rt>%s</rt></ruby>", html.EscapeString(s.T), html.EscapeString(s.R))
		} else {
			b.WriteString(html.EscapeString(s.T))
		}
	}
	return b.String()
}

func Segs(ss []Seg) templ.Component { return templ.Raw(segsHTML(ss)) }

func Notation(s string) templ.Component { return templ.Raw(segsHTML(parseNotation(s))) }

// Sentence wraps each chunk in a no-break span so a long line only breaks between chunks.
func Sentence(d *LineData) templ.Component {
	var b strings.Builder
	for _, c := range d.Chunks {
		b.WriteString(`<span class="ck">` + segsHTML(c) + `</span>`)
	}
	return templ.Raw(b.String())
}

// NotationWrap is Notation with a line-break opportunity after each comma.
func NotationWrap(s string) templ.Component {
	return templ.Raw(strings.ReplaceAll(segsHTML(parseNotation(s)), "、", "、<wbr>"))
}

func (v V) open(k string) bool { return v.Me != nil && v.Me.Open[k] }

func readingLines(s string) []string {
	var out []string
	for _, l := range strings.Split(strings.ReplaceAll(s, "。", "。\n"), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

func widthStyle(pct int) templ.SafeCSS { return templ.SafeCSS(fmt.Sprintf("width:%d%%", pct)) }

var confettiColors = []string{"#58cc02", "#1cb0f6", "#ffc800", "#ff4b4b", "#ce82ff", "#ff9600"}

func confettiStyle(i int) templ.SafeCSS {
	return templ.SafeCSS(fmt.Sprintf("left:%d%%;background:%s;animation-delay:%dms;animation-duration:%dms",
		(i*29)%100, confettiColors[i%len(confettiColors)], (i*97)%1400, 2600+(i*53)%1400))
}

func tileCls(wrong bool) string {
	if wrong {
		return "placed wrong"
	}
	return "placed"
}

func optCls(step string, i, answer, pick int) string {
	switch {
	case step != "result":
		return ""
	case i == answer:
		return "correct"
	case i == pick:
		return "wrong"
	}
	return "dim"
}

func containsInt(s []int, x int) bool {
	for _, v := range s {
		if v == x {
			return true
		}
	}
	return false
}
