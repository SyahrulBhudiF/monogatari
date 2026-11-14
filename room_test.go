package main

import (
	"slices"
	"strings"
	"testing"
)

// playingRoom builds a room in the playing phase with mock lines and n connected players.
func playingRoom(n int) (*Room, []string) {
	r := NewRoom()
	var toks []string
	var infos []PlayerInfo
	for i := 0; i < n; i++ {
		tok := strings.Repeat(string(rune('a'+i)), 20)
		toks = append(toks, tok)
		r.Join(tok, "P"+string(rune('1'+i)), "")
		r.streams[tok] = 1
		infos = append(infos, PlayerInfo{Index: i, Name: r.Players[i].Name, Level: "N5", Pos: "middle"})
	}
	nl := nLinesFor(n)
	r.Plan = mockPlan(infos, nl)
	for i := 0; i < nl; i++ {
		r.Lines = append(r.Lines, &LineState{Status: "ready", Plan: r.Plan.LinePlan[i], Data: mockLine(i)})
	}
	r.Phase = "playing"
	r.enterTurn(0)
	return r, toks
}

func TestTurnFlowAndRotation(t *testing.T) {
	r, toks := playingRoom(2)
	r.Begin(toks[1]) // not the active player
	if r.Step != "scene" {
		t.Fatal("non-active player started the turn")
	}
	r.Begin(toks[0])
	correct := r.Lines[0].Data.AcceptedOrders[1] // 今日は|たくさん|雨が|... a non-identity accepted order
	for _, c := range correct {
		r.Place(toks[1], c) // ignored
		r.Place(toks[0], c)
	}
	if len(r.Placed) != len(correct) {
		t.Fatalf("placed %v", r.Placed)
	}
	r.Confirm(toks[0])
	if r.Step != "reading" {
		t.Fatalf("an accepted alternative order was rejected: step %s", r.Step)
	}
	r.ReadDone(toks[0])
	r.Answer(toks[0], r.Lines[0].Data.Quiz.Answer)
	r.Next(toks[0])
	if r.Turn != 1 || r.Step != "scene" || r.active().Token != toks[1] {
		t.Fatalf("turn %d step %s", r.Turn, r.Step)
	}
	if ok, n := r.score(0); ok != 1 || n != 1 {
		t.Fatalf("score %d/%d", ok, n)
	}
}

func TestWrongOrderMarksChunks(t *testing.T) {
	r, toks := playingRoom(2)
	r.Begin(toks[0])
	rev := slices.Clone(identity(len(r.Lines[0].Data.Chunks)))
	slices.Reverse(rev)
	for _, c := range rev {
		r.Place(toks[0], c)
	}
	r.Confirm(toks[0])
	if r.Step != "reorder" || len(r.Wrong) == 0 {
		t.Fatalf("step %s wrong %v", r.Step, r.Wrong)
	}
	r.Unplace(toks[0], rev[0])
	if len(r.Wrong) != 0 {
		t.Fatal("marks stay after the player changes the order")
	}
}

func TestAutoPlayEarnsNoStar(t *testing.T) {
	r, toks := playingRoom(2)
	r.Skip(toks[0]) // the host cannot skip their own turn
	if r.Step != "scene" {
		t.Fatal("host skipped own turn")
	}
	r.Skip(toks[1]) // player 2 is not the host
	if r.Step != "scene" {
		t.Fatal("non-host skipped a turn")
	}
	delete(r.streams, toks[0])
	r.Skip(toks[1]) // host is now the first connected player
	if r.Step != "result" || !r.Auto {
		t.Fatalf("step %s auto %v", r.Step, r.Auto)
	}
	if ok, n := r.score(0); ok != 0 || n != 0 {
		t.Fatalf("auto turn scored %d/%d", ok, n)
	}
	if r.autoCount(0) != 1 {
		t.Fatal("auto turn not counted")
	}
}

func TestReadyVoteResets(t *testing.T) {
	r := NewRoom()
	r.Join(strings.Repeat("a", 20), "A", "")
	r.Join(strings.Repeat("b", 20), "B", "")
	r.streams[strings.Repeat("a", 20)] = 1
	r.streams[strings.Repeat("b", 20)] = 1
	r.ToggleReady(strings.Repeat("a", 20))
	if !r.Players[0].Ready {
		t.Fatal("not ready")
	}
	r.SetLevel(strings.Repeat("b", 20), 9)
	if r.Players[0].Ready {
		t.Fatal("level change kept the vote")
	}
	r.ToggleReady(strings.Repeat("a", 20))
	r.Join(strings.Repeat("c", 20), "C", "")
	if r.Players[0].Ready {
		t.Fatal("new player kept the vote")
	}
}

func TestJoinPassphraseAndKick(t *testing.T) {
	r := NewRoom()
	r.Pass = "sesame"
	a, b := strings.Repeat("a", 20), strings.Repeat("b", 20)
	r.Join(a, "A", "wrong")
	if len(r.Players) != 0 || r.joinErr[a] == "" {
		t.Fatal("wrong passphrase accepted")
	}
	r.Join(a, "A", "sesame")
	r.Join(b, "B", "sesame")
	r.streams[a], r.streams[b] = 1, 1
	r.Kick(b, r.Players[0].ID)
	if len(r.Players) != 2 {
		t.Fatal("non-host kicked someone")
	}
	r.Kick(a, r.Players[1].ID)
	if len(r.Players) != 1 {
		t.Fatal("host could not kick")
	}
}

func TestGameStartCap(t *testing.T) {
	r := NewRoom()
	r.MaxGames = 0
	a, b := strings.Repeat("a", 20), strings.Repeat("b", 20)
	r.Join(a, "A", "")
	r.Join(b, "B", "")
	r.streams[a], r.streams[b] = 1, 1
	r.ToggleReady(a)
	r.ToggleReady(b)
	if r.Phase != "lobby" || r.Err == "" {
		t.Fatalf("phase %s err %q", r.Phase, r.Err)
	}
}

func TestPagesNeverContainOtherPlayersTokens(t *testing.T) {
	r, toks := playingRoom(3)
	for _, me := range toks {
		page := r.View(me)
		for _, other := range toks {
			if other != me && strings.Contains(page, other) {
				t.Fatalf("page for %s leaks %s", me, other)
			}
		}
	}
	if !strings.Contains(r.View("zzzzzzzzzzzzzzzzzzzz"), "watching") {
		t.Fatal("a late visitor should see the game as a watcher")
	}
}

func TestFinishedPage(t *testing.T) {
	r, toks := playingRoom(2)
	r.Log = []TurnLog{{Turn: 0, Player: 0, Correct: true}, {Turn: 1, Player: 1, Correct: false}, {Turn: 2, Player: 0, Auto: true}}
	r.Phase = "finished"
	page := r.View(toks[0])
	for _, want := range []string{"Words", "Play again", "+1 auto", `class="missed"`} {
		if !strings.Contains(page, want) {
			t.Errorf("finished page lacks %q", want)
		}
	}
	if strings.Contains(r.View("zzzzzzzzzzzzzzzzzzzz"), "Play again") {
		t.Error("a watcher can restart the game")
	}
}
