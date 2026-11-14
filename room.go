package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	mrand "math/rand"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	maxPlayers = 6
	grace      = 8 * time.Second
)

// Player.Token is private to its owner's browser. Player.ID is the public handle used in pages.
type Player struct {
	Token, ID, Name, Level, Pos string
	Ready, Left                 bool
	Step                        int
	Open                        map[string]bool
}

type LineState struct {
	Status string // pending, ready, failed
	Plan   LinePlan
	Data   *LineData
}

type TurnLog struct {
	Turn, Player           int
	Correct, Auto, Skipped bool
}

type Room struct {
	mu        sync.Mutex
	subs      map[chan struct{}]struct{}
	streams   map[string]int
	prefs     map[string]int
	joinErr   map[string]string
	hideFuri  map[string]bool
	autoT     *time.Timer
	genID     int
	cancelGen context.CancelFunc
	starts    []time.Time

	Solo     bool
	Pass     string
	MaxGames int

	Phase   string // lobby, generating, briefing, playing, finished
	Err     string
	Players []*Player
	Plan    *Plan
	Lines   []*LineState
	Turn    int
	Step    string // scene, reorder, reading, quiz, result
	Shuf    []int
	Placed  []int
	Wrong   map[int]bool
	Pick    int
	Auto    bool
	Log     []TurnLog

	logMu sync.Mutex
	Logs  []string
	logT0 time.Time
}

func NewRoom() *Room {
	return &Room{
		Phase: "lobby", Pick: -1, MaxGames: 10,
		subs: map[chan struct{}]struct{}{}, streams: map[string]int{}, prefs: map[string]int{},
		joinErr: map[string]string{}, hideFuri: map[string]bool{},
	}
}

func randHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (r *Room) addLog(msg string) {
	r.logMu.Lock()
	if r.logT0.IsZero() {
		r.logT0 = time.Now()
	}
	r.Logs = append(r.Logs, fmt.Sprintf("%5.1fs  %s", time.Since(r.logT0).Seconds(), msg))
	if len(r.Logs) > 200 {
		r.Logs = r.Logs[len(r.Logs)-200:]
	}
	r.logMu.Unlock()
	go func() {
		r.mu.Lock()
		r.notify()
		r.mu.Unlock()
	}()
}

func (r *Room) logLines() []string {
	r.logMu.Lock()
	defer r.logMu.Unlock()
	return append([]string(nil), r.Logs...)
}

func (r *Room) resetLog() {
	r.logMu.Lock()
	r.Logs, r.logT0 = nil, time.Now()
	r.logMu.Unlock()
}

func (r *Room) notify() {
	for ch := range r.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (r *Room) on(p *Player) bool { return !p.Left && r.streams[p.Token] > 0 }

func (r *Room) find(token string) (int, *Player) {
	for i, p := range r.Players {
		if p.Token == token && !p.Left {
			return i, p
		}
	}
	return -1, nil
}

// host is the first connected player; they can remove people and skip turns.
func (r *Room) host() *Player {
	for _, p := range r.Players {
		if r.on(p) {
			return p
		}
	}
	return nil
}

func (r *Room) isHost(token string) bool {
	h := r.host()
	return h != nil && h.Token == token
}

func (r *Room) active() *Player {
	if r.Phase != "playing" || r.Turn >= len(r.Lines) {
		return nil
	}
	pi := r.Lines[r.Turn].Plan.Player
	if pi >= len(r.Players) {
		return nil
	}
	return r.Players[pi]
}

func (r *Room) clearReady() {
	for _, p := range r.Players {
		p.Ready = false
	}
}

func (r *Room) prefLv(token string) int {
	if v, ok := r.prefs[token]; ok {
		return v
	}
	return 1
}

func (r *Room) connect(token string, ch chan struct{}) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.subs[ch] = struct{}{}
	r.streams[token]++
	r.notify()
}

func (r *Room) disconnect(token string, ch chan struct{}) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.subs, ch)
	if r.streams[token]--; r.streams[token] <= 0 {
		delete(r.streams, token)
		if _, p := r.find(token); p != nil {
			switch r.Phase {
			case "lobby":
				time.AfterFunc(10*time.Second, func() { r.dropIfGone(token) })
			case "playing":
				r.scheduleAuto(grace)
			}
			r.checkStart()
		}
	}
	r.notify()
}

func (r *Room) dropIfGone(token string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if i, p := r.find(token); p != nil && !r.on(p) && r.Phase == "lobby" {
		r.Players = slices.Delete(r.Players, i, i+1)
		r.checkStart()
		r.notify()
	}
}

func (r *Room) Join(token, name, pass string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Phase != "lobby" {
		return
	}
	name = strings.TrimSpace(name)
	if rs := []rune(name); len(rs) > 16 {
		name = string(rs[:16])
	}
	if name == "" {
		return
	}
	if r.Pass != "" && subtle.ConstantTimeCompare([]byte(pass), []byte(r.Pass)) != 1 {
		r.joinErr[token] = "Wrong passphrase"
		r.notify()
		return
	}
	delete(r.joinErr, token)
	level, pos := splitLv(r.prefLv(token))
	if _, p := r.find(token); p != nil {
		p.Name, p.Level, p.Pos = name, level, pos
	} else {
		if len(r.Players) >= maxPlayers {
			r.joinErr[token] = "The room is full"
			r.notify()
			return
		}
		r.Players = append(r.Players, &Player{Token: token, ID: randHex(6), Name: name, Level: level, Pos: pos, Open: map[string]bool{}})
	}
	r.Solo = len(r.Players) == 1 && r.Solo
	r.clearReady()
	r.notify()
}

// SetSolo lets a lone player in the lobby start a single-player reading game.
func (r *Room) SetSolo(token string, on bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, p := r.find(token); p != nil && r.Phase == "lobby" && len(r.Players) == 1 && r.Solo != on {
		r.Solo = on
		r.clearReady()
		r.notify()
	}
}

func (r *Room) SetLevel(token string, lv int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	lv = max(0, min(14, lv))
	r.prefs[token] = lv
	if _, p := r.find(token); p != nil && r.Phase == "lobby" {
		level, pos := splitLv(lv)
		if p.Level != level || p.Pos != pos {
			p.Level, p.Pos = level, pos
			r.clearReady()
		}
	}
	r.notify()
}

// Leave removes the player in the lobby. During a game the seat stays so lines keep their
// speakers: the player is marked as gone, their turns are played for them, and they become a watcher.
func (r *Room) Leave(token string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	i, p := r.find(token)
	if p == nil {
		return
	}
	switch r.Phase {
	case "lobby":
		r.Players = slices.Delete(r.Players, i, i+1)
		r.Solo = r.Solo && len(r.Players) == 1
		r.checkStart()
	case "finished":
		return
	default:
		p.Left = true
		if r.Phase == "playing" {
			r.scheduleAuto(2 * time.Second)
		}
		r.checkStart()
	}
	r.notify()
}

// toLobby stops any running game and returns everyone still connected to a fresh lobby.
func (r *Room) toLobby(msg string) {
	r.genID++
	if r.cancelGen != nil {
		r.cancelGen()
		r.cancelGen = nil
	}
	if r.autoT != nil {
		r.autoT.Stop()
		r.autoT = nil
	}
	r.Phase, r.Err, r.Plan, r.Lines, r.Log, r.Solo = "lobby", msg, nil, nil, nil, false
	r.Players = slices.DeleteFunc(r.Players, func(p *Player) bool { return !r.on(p) })
	for _, p := range r.Players {
		p.Ready, p.Step = false, 0
	}
}

// Abort lets the host end a running game for everyone.
func (r *Room) Abort(token string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.isHost(token) || r.Phase == "lobby" || r.Phase == "finished" {
		return
	}
	r.toLobby("The host ended the game.")
	r.notify()
}

func (r *Room) Kick(token, id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Phase != "lobby" || !r.isHost(token) {
		return
	}
	for i, p := range r.Players {
		if p.ID == id && p.Token != token {
			r.Players = slices.Delete(r.Players, i, i+1)
			r.checkStart()
			r.notify()
			return
		}
	}
}

func (r *Room) ToggleReady(token string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, p := r.find(token); p != nil && (r.Phase == "lobby" || r.Phase == "briefing") {
		p.Ready = !p.Ready
		r.checkStart()
		r.notify()
	}
}

func (r *Room) SetStep(token string, d int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, p := r.find(token); p != nil && r.Phase == "briefing" {
		p.Step = max(0, min(2, p.Step+d))
		r.notify()
	}
}

func (r *Room) Toggle(token, key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, p := r.find(token); p != nil && (key == "log" || key == "en" || key == "tr") {
		p.Open[key] = !p.Open[key]
		r.notify()
	}
}

func (r *Room) ToggleFuri(token string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hideFuri[token] = !r.hideFuri[token]
	r.notify()
}

func (r *Room) StartNow(token string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Phase == "briefing" && r.isHost(token) {
		r.startPlay()
		r.notify()
	}
}

func (r *Room) readyCount() (int, int) {
	c, rd := 0, 0
	for _, p := range r.Players {
		if r.on(p) {
			c++
			if p.Ready {
				rd++
			}
		}
	}
	return rd, c
}

func (r *Room) checkStart() {
	if r.Phase != "lobby" && r.Phase != "briefing" {
		return
	}
	rd, c := r.readyCount()
	need := 1
	if r.Phase == "lobby" && !r.Solo {
		need = 2
	}
	if c < need || rd < c {
		return
	}
	if r.Phase == "briefing" {
		r.startPlay()
		return
	}
	cut := time.Now().Add(-time.Hour)
	r.starts = slices.DeleteFunc(r.starts, func(t time.Time) bool { return t.Before(cut) })
	if len(r.starts) >= r.MaxGames {
		r.Err = "Too many games were started this hour. Try again later."
		r.clearReady()
		return
	}
	r.starts = append(r.starts, time.Now())
	r.Players = slices.DeleteFunc(r.Players, func(p *Player) bool { return !r.on(p) })
	r.clearReady()
	r.Phase, r.Err = "generating", ""
	r.genID++
	r.resetLog()
	var infos []PlayerInfo
	for i, p := range r.Players {
		p.Step = 0
		infos = append(infos, PlayerInfo{Index: i, Name: p.Name, Level: p.Level, Pos: p.Pos, LevelDesc: levelDesc[p.Level]})
	}
	go r.generate(infos, r.genID)
}

func (r *Room) startPlay() {
	r.Phase = "playing"
	r.Log = nil
	r.enterTurn(0)
}

func (r *Room) generate(ps []PlayerInfo, gen int) {
	n := nLinesFor(len(ps))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	r.mu.Lock()
	if gen == r.genID {
		r.cancelGen = cancel
	}
	r.mu.Unlock()
	logf("generating story: %d players, %d lines", len(ps), n)
	plan, err := genPlan(ctx, ps, n)
	r.mu.Lock()
	defer r.mu.Unlock()
	if gen != r.genID {
		cancel()
		return
	}
	if err != nil {
		logf("plan failed: %v", err)
		cancel()
		r.Phase, r.Err = "lobby", "Story generation failed: "+err.Error()
		r.clearReady()
		r.notify()
		return
	}
	logf("plan ready: %q", plan.Scenario.Title)
	r.Plan = plan
	r.Lines = make([]*LineState, n)
	for i := range r.Lines {
		r.Lines[i] = &LineState{Status: "pending", Plan: plan.LinePlan[i]}
	}
	r.Phase = "briefing"
	r.notify()
	go func() {
		defer cancel()
		t := time.Now()
		lines := genScript(ctx, plan, ps)
		r.mu.Lock()
		defer r.mu.Unlock()
		if gen != r.genID {
			return
		}
		ok := 0
		for i, d := range lines {
			if d == nil {
				r.Lines[i].Status = "failed"
				continue
			}
			ok++
			r.Lines[i].Status, r.Lines[i].Data = "ready", d
			logf("line %d %s: %s", i, plan.Characters[plan.LinePlan[i].Player].Name, sentence(d, identity(len(d.Chunks))))
		}
		logf("script ready: %d/%d lines (%.1fs)", ok, n, time.Since(t).Seconds())
		if r.Phase == "playing" && r.Step == "scene" && r.Lines[r.Turn].Status == "failed" {
			r.enterTurn(r.Turn)
		}
		r.notify()
	}()
}

func (r *Room) enterTurn(i int) {
	for i < len(r.Lines) && r.Lines[i].Status == "failed" {
		r.Log = append(r.Log, TurnLog{Turn: i, Player: r.Lines[i].Plan.Player, Skipped: true})
		i++
	}
	if i >= len(r.Lines) {
		r.Phase, r.Step = "finished", ""
		return
	}
	r.Turn, r.Step = i, "scene"
	r.Shuf, r.Placed, r.Wrong, r.Pick, r.Auto = nil, nil, nil, -1, false
	r.scheduleAuto(1500 * time.Millisecond)
}

// scheduleAuto fills in the turn for a disconnected active player after d.
func (r *Room) scheduleAuto(d time.Duration) {
	if r.autoT != nil {
		r.autoT.Stop()
		r.autoT = nil
	}
	if r.Phase != "playing" || r.Step == "result" {
		return
	}
	if p := r.active(); p == nil || r.on(p) {
		return
	}
	turn := r.Turn
	r.autoT = time.AfterFunc(d, func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.Phase != "playing" || r.Turn != turn || r.Step == "result" {
			return
		}
		if p := r.active(); p == nil || r.on(p) {
			return
		}
		if r.Lines[turn].Status != "ready" {
			r.scheduleAuto(2 * time.Second)
			return
		}
		r.autoFill(turn)
	})
}

// autoFill plays the turn with the right answers. The turn is logged as auto and earns no star.
func (r *Room) autoFill(turn int) {
	l := r.Lines[turn].Data
	r.Placed = identity(len(l.Chunks))
	r.Pick, r.Auto, r.Step, r.Wrong = l.Quiz.Answer, true, "result", nil
	r.Log = append(r.Log, TurnLog{Turn: turn, Player: r.Lines[turn].Plan.Player, Auto: true})
	r.notify()
	if r.autoT != nil {
		r.autoT.Stop()
	}
	r.autoT = time.AfterFunc(4*time.Second, func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.Phase == "playing" && r.Turn == turn && r.Step == "result" && r.Auto {
			r.enterTurn(turn + 1)
			r.notify()
		}
	})
}

// Skip lets the host move on when the active player is idle.
func (r *Room) Skip(token string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p := r.active()
	if p == nil || p.Token == token || !r.isHost(token) {
		return
	}
	switch {
	case r.Step == "result":
		r.enterTurn(r.Turn + 1)
	case r.Lines[r.Turn].Status == "ready":
		r.autoFill(r.Turn)
	default:
		return
	}
	r.notify()
}

// act runs fn when token is the active player and the turn is in the given step.
func (r *Room) act(token, step string, fn func(l *LineData)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p := r.active()
	if p == nil || p.Token != token || r.Step != step || r.Lines[r.Turn].Status != "ready" {
		return
	}
	fn(r.Lines[r.Turn].Data)
	r.notify()
}

func (r *Room) Begin(token string) {
	r.act(token, "scene", func(l *LineData) {
		n := len(l.Chunks)
		for try := 0; try < 30; try++ {
			r.Shuf = mrand.Perm(n)
			if !acceptedOrder(l, r.Shuf) {
				break
			}
		}
		r.Placed, r.Wrong, r.Step = []int{}, nil, "reorder"
	})
}

func acceptedOrder(l *LineData, o []int) bool {
	return slices.ContainsFunc(l.AcceptedOrders, func(a []int) bool { return slices.Equal(a, o) })
}

func (r *Room) Place(token string, c int) {
	r.act(token, "reorder", func(l *LineData) {
		if c >= 0 && c < len(l.Chunks) && !slices.Contains(r.Placed, c) {
			r.Placed = append(r.Placed, c)
			r.Wrong = nil
		}
	})
}

func (r *Room) Unplace(token string, c int) {
	r.act(token, "reorder", func(l *LineData) {
		if i := slices.Index(r.Placed, c); i >= 0 {
			r.Placed = slices.Delete(r.Placed, i, i+1)
			r.Wrong = nil
		}
	})
}

func (r *Room) Confirm(token string) {
	r.act(token, "reorder", func(l *LineData) {
		if len(r.Placed) != len(l.Chunks) {
			return
		}
		if acceptedOrder(l, r.Placed) {
			r.Step, r.Wrong = "reading", nil
			return
		}
		best, bestMiss := l.AcceptedOrders[0], 1<<30
		for _, a := range l.AcceptedOrders {
			miss := 0
			for k := range a {
				if a[k] != r.Placed[k] {
					miss++
				}
			}
			if miss < bestMiss {
				best, bestMiss = a, miss
			}
		}
		r.Wrong = map[int]bool{}
		for k := range best {
			if best[k] != r.Placed[k] {
				r.Wrong[r.Placed[k]] = true
			}
		}
	})
}

func (r *Room) ReadDone(token string) {
	r.act(token, "reading", func(*LineData) { r.Step = "quiz" })
}

func (r *Room) Answer(token string, o int) {
	r.act(token, "quiz", func(l *LineData) {
		if o < 0 || o >= len(l.Quiz.Options) {
			return
		}
		r.Pick, r.Step = o, "result"
		r.Log = append(r.Log, TurnLog{Turn: r.Turn, Player: r.Lines[r.Turn].Plan.Player, Correct: o == l.Quiz.Answer})
	})
}

func (r *Room) Next(token string) {
	r.act(token, "result", func(*LineData) { r.enterTurn(r.Turn + 1) })
}

// Reset starts a new lobby after a finished game. Anyone can do it, including people who left.
func (r *Room) Reset(token string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Phase != "finished" {
		return
	}
	r.toLobby("")
	r.notify()
}

// score counts stars: correct quiz answers on turns the player played themselves.
func (r *Room) score(i int) (ok, n int) {
	for _, l := range r.Log {
		if l.Player == i && !l.Skipped && !l.Auto {
			n++
			if l.Correct {
				ok++
			}
		}
	}
	return
}

func (r *Room) autoCount(i int) (n int) {
	for _, l := range r.Log {
		if l.Player == i && l.Auto {
			n++
		}
	}
	return
}

// turnMissed reports whether the turn was played and its quiz answered wrong.
func (r *Room) turnMissed(turn int) bool {
	for _, l := range r.Log {
		if l.Turn == turn && !l.Auto && !l.Skipped && !l.Correct {
			return true
		}
	}
	return false
}

func (r *Room) linesReady() int {
	n := 0
	for _, l := range r.Lines {
		if l.Status != "pending" {
			n++
		}
	}
	return n
}

func (r *Room) progressPct() int {
	frac := map[string]int{"scene": 0, "reorder": 20, "reading": 55, "quiz": 70, "result": 95}[r.Step]
	return (r.Turn*100 + frac) / len(r.Lines)
}

func (r *Room) String() string { return fmt.Sprintf("room(%s, %d players)", r.Phase, len(r.Players)) }
