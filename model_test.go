package main

import (
	"slices"
	"strings"
	"testing"
)

func line(chunks string, orders ...[]int) *LineData {
	d := &LineData{English: "x", AcceptedOrders: orders, Quiz: Quiz{Question: "q", Options: []string{"a", "b", "c", "d"}, Answer: 1, Explanation: "e"}}
	for _, c := range strings.Split(chunks, "|") {
		d.Chunks = append(d.Chunks, parseNotation(c))
	}
	return d
}

func TestParseNotation(t *testing.T) {
	got := parseNotation("駅[えき]で食[た]べる")
	want := []Seg{{T: "駅", R: "えき"}, {T: "で"}, {T: "食", R: "た"}, {T: "べる"}}
	if !slices.Equal(got, want) {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestCheckLine(t *testing.T) {
	const ok = "私[わたし]は|駅[えき]で|友達[ともだち]に|会[あ]いました"
	cases := []struct {
		name string
		d    *LineData
		err  string
	}{
		{"valid", line(ok), ""},
		{"kanji without furigana", line("私は|駅[えき]で|友達[ともだち]に|会[あ]いました"), "no furigana"},
		{"furigana not kana", line("私[watashi]は|駅[えき]で|友達[ともだち]に|会[あ]いました"), "hiragana or katakana"},
		{"too few chunks", line("私[わたし]は|会[あ]いました"), "3 to 11"},
	}
	for _, c := range cases {
		err := checkLine(c.d)
		if c.err == "" && err != nil || c.err != "" && (err == nil || !strings.Contains(err.Error(), c.err)) {
			t.Errorf("%s: got %v, want %q", c.name, err, c.err)
		}
	}

	bad := line(ok)
	bad.Quiz.Options = []string{"a", "a", "c", "d"}
	if checkLine(bad) == nil {
		t.Error("duplicate options accepted")
	}
	bad = line(ok)
	bad.Quiz.Answer = 4
	if checkLine(bad) == nil {
		t.Error("answer out of range accepted")
	}
}

func TestAcceptedOrdersAreFiltered(t *testing.T) {
	// 0 1 2 3 is implied. {1,0,2,3} is fine, {1,0,3,2} moves the verb, {0,1} is not a permutation, and {1,0,2,3} repeats.
	d := line("私[わたし]は|駅[えき]で|友達[ともだち]に|会[あ]いました", []int{1, 0, 2, 3}, []int{1, 0, 3, 2}, []int{0, 1}, []int{1, 0, 2, 3})
	if err := checkLine(d); err != nil {
		t.Fatal(err)
	}
	want := [][]int{{0, 1, 2, 3}, {1, 0, 2, 3}}
	if !slices.EqualFunc(d.AcceptedOrders, want, slices.Equal[[]int]) {
		t.Fatalf("got %v want %v", d.AcceptedOrders, want)
	}
}

func TestNotationDropsReadingAfterKana(t *testing.T) {
	got := parseNotation("ベン[ベン]さんは猫[ねこ]を")
	want := []Seg{{T: "ベンさんは"}, {T: "猫", R: "ねこ"}, {T: "を"}}
	if !slices.Equal(got, want) {
		t.Fatalf("got %+v want %+v", got, want)
	}
}
