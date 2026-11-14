package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"text/template"
	"time"
)

//go:embed prompts/*.txt
var promptFS embed.FS

var (
	promptTpl = template.Must(template.ParseFS(promptFS, "prompts/*.txt"))
	apiKey    = os.Getenv("OPENROUTER_API_KEY")
	models    = splitModels(os.Getenv("MODELS"))
	httpc     = &http.Client{Timeout: 150 * time.Second}
	noFormat  sync.Map
	provider  = envOr("PROVIDER", "stepfun")
	reasoning = envOr("REASONING", "low")
	judgeOn   = os.Getenv("JUDGE") != "off"
)

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func splitModels(s string) []string {
	var out []string
	for _, m := range strings.Split(s, ",") {
		if m = strings.TrimSpace(m); m != "" {
			out = append(out, m)
		}
	}
	if len(out) == 0 {
		out = []string{"deepseek/deepseek-v4.1-flash-1017"}
	}
	return out
}

type apiError struct {
	Status int
	Body   string
}

func (a *apiError) Error() string { return fmt.Sprintf("openrouter %d: %s", a.Status, a.Body) }

func chat(ctx context.Context, model string, msgs []map[string]string, jsonMode bool) (string, error) {
	body := map[string]any{"model": model, "messages": msgs, "temperature": 0.8}
	if provider != "" {
		body["provider"] = map[string]any{"order": []string{provider}, "allow_fallbacks": false}
	}
	if reasoning != "" && reasoning != "off" {
		body["reasoning"] = map[string]string{"effort": reasoning}
	}
	if jsonMode {
		body["response_format"] = map[string]string{"type": "json_object"}
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://openrouter.ai/api/v1/chat/completions", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Title", "Monogatari")
	resp, err := httpc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		s := string(data)
		if len(s) > 300 {
			s = s[:300]
		}
		return "", &apiError{resp.StatusCode, s}
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", err
	}
	if out.Error != nil {
		return "", fmt.Errorf("openrouter: %s", out.Error.Message)
	}
	if len(out.Choices) == 0 || strings.TrimSpace(out.Choices[0].Message.Content) == "" {
		return "", fmt.Errorf("empty response")
	}
	return out.Choices[0].Message.Content, nil
}

func chatAuto(ctx context.Context, model string, msgs []map[string]string) (string, error) {
	_, skip := noFormat.Load(model)
	txt, err := chat(ctx, model, msgs, !skip)
	if ae, ok := err.(*apiError); ok && ae.Status == 400 && !skip {
		noFormat.Store(model, true)
		return chat(ctx, model, msgs, false)
	}
	return txt, err
}

func extractJSON(s string) string {
	i, j := strings.Index(s, "{"), strings.LastIndex(s, "}")
	if i < 0 || j < i {
		return s
	}
	return s[i : j+1]
}

func generate[T any](ctx context.Context, name, prompt string, check func(*T) error) (*T, error) {
	var feedback string
	var last error
	for attempt := 0; attempt < 4; attempt++ {
		model := models[attempt%len(models)]
		msgs := []map[string]string{{"role": "user", "content": prompt}}
		if feedback != "" {
			msgs = append(msgs, map[string]string{"role": "user", "content": "Your previous JSON was rejected: " + feedback + ". Return the corrected JSON only."})
		}
		t0 := time.Now()
		txt, err := chatAuto(ctx, model, msgs)
		if err != nil {
			last, feedback = err, ""
			logf("%s attempt %d (%s): %v", name, attempt+1, model, err)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt+1) * 2 * time.Second):
			}
			continue
		}
		var v T
		if err := json.Unmarshal([]byte(extractJSON(txt)), &v); err != nil {
			last = fmt.Errorf("invalid JSON: %v", err)
			feedback = last.Error()
			logf("%s attempt %d (%s): %v", name, attempt+1, model, last)
			continue
		}
		if err := check(&v); err != nil {
			last, feedback = err, err.Error()
			logf("%s attempt %d (%s): rejected: %v", name, attempt+1, model, err)
			continue
		}
		logf("%s ok via %s in %.1fs (attempt %d)", name, model, time.Since(t0).Seconds(), attempt+1)
		return &v, nil
	}
	return nil, last
}

func render(name string, data any) (string, error) {
	var b bytes.Buffer
	err := promptTpl.ExecuteTemplate(&b, name, data)
	return b.String(), err
}

func genPlan(ctx context.Context, ps []PlayerInfo, nLines int) (*Plan, error) {
	if apiKey == "" {
		time.Sleep(1500 * time.Millisecond)
		return mockPlan(ps, nLines), nil
	}
	prompt, err := render("plan.txt", map[string]any{"Players": ps, "NLines": nLines})
	if err != nil {
		return nil, err
	}
	return generate(ctx, "plan", prompt, func(p *Plan) error { return checkPlan(p, len(ps), nLines) })
}

// TLine is a finished line of the story, shown to the model so the next line follows it.
type TLine struct{ Name, JP, EN string }

// lineErr runs the structural checks and requires the chunk count to fit the player's level,
// so one line stays one short sentence.
func lineErr(d *LineData, level string) error {
	if err := checkLine(d); err != nil {
		return err
	}
	rng := chunkRange[level]
	if n := len(d.Chunks); n > rng[1]+1 || n < rng[0]-1 {
		return fmt.Errorf("%d chunks, but a JLPT %s line needs %d to %d. Write one short sentence", n, level, rng[0], rng[1])
	}
	return nil
}

func genLine(ctx context.Context, plan *Plan, ps []PlayerInfo, i int, hist []TLine) (*LineData, error) {
	if apiKey == "" {
		time.Sleep(time.Duration(500+i*400) * time.Millisecond)
		return mockLine(i), nil
	}
	lp := plan.LinePlan[i]
	pl := ps[lp.Player]
	prev, next := "(none, this is the opening)", "(none, this is the ending)"
	if i > 0 {
		prev = plan.LinePlan[i-1].Beat
	}
	if i+1 < len(plan.LinePlan) {
		next = plan.LinePlan[i+1].Beat
	}
	rng := chunkRange[pl.Level]
	prompt, err := render("line.txt", map[string]any{
		"Title": plan.Scenario.Title, "Setting": plan.Scenario.Setting,
		"Characters": plan.Characters, "History": hist, "Prev": prev, "Beat": lp.Beat, "Next": next,
		"Speaker": plan.Characters[lp.Player], "Level": pl.Level, "Pos": pl.Pos, "LevelDesc": pl.LevelDesc,
		"Grammar": lp.Grammar, "MinChunks": rng[0], "MaxChunks": rng[1],
	})
	if err != nil {
		return nil, err
	}
	return generate(ctx, fmt.Sprintf("line %d", i), prompt, func(d *LineData) error {
		if err := lineErr(d, pl.Level); err != nil {
			return err
		}
		return judgeLine(ctx, d, pl.Level)
	})
}

// judgeLine asks a second call to review a line that already passed the structural checks.
// It drops accepted orders the reviewer rejects. If the review itself fails, the line is accepted.
func judgeLine(ctx context.Context, d *LineData, level string) error {
	if !judgeOn || apiKey == "" {
		return nil
	}
	var orders []string
	for _, o := range d.AcceptedOrders {
		var parts []string
		for _, i := range o {
			parts = append(parts, chunkText(d.Chunks[i]))
		}
		orders = append(orders, strings.Join(parts, " | "))
	}
	prompt, err := render("judge.txt", map[string]any{
		"Level": level, "Sentence": sentence(d, identity(len(d.Chunks))), "English": d.English,
		"Orders": orders, "Quiz": d.Quiz,
	})
	if err != nil {
		return nil
	}
	txt, err := chatAuto(ctx, models[0], []map[string]string{{"role": "user", "content": prompt}})
	if err != nil {
		logf("judge skipped: %v", err)
		return nil
	}
	var v struct {
		Natural   bool   `json:"natural"`
		AnswerOK  bool   `json:"answer_ok"`
		BadOrders []int  `json:"bad_orders"`
		Narration bool   `json:"narration"`
		Note      string `json:"note"`
	}
	if json.Unmarshal([]byte(extractJSON(txt)), &v) != nil {
		logf("judge skipped: invalid JSON")
		return nil
	}
	if !v.Natural {
		return fmt.Errorf("a reviewer says the sentence is unnatural or ungrammatical: %s", v.Note)
	}
	if v.Narration {
		return fmt.Errorf("the line is narration. It must be something the character says aloud to the others")
	}
	if !v.AnswerOK {
		return fmt.Errorf("a reviewer says the quiz answer is wrong or ambiguous: %s", v.Note)
	}
	var keep [][]int
	for i, o := range d.AcceptedOrders {
		if i == 0 || !slices.Contains(v.BadOrders, i) {
			keep = append(keep, o)
		}
	}
	if len(keep) < len(d.AcceptedOrders) {
		logf("judge dropped %d accepted order(s)", len(d.AcceptedOrders)-len(keep))
	}
	d.AcceptedOrders = keep
	return nil
}

type scriptLine struct {
	Index, MinChunks, MaxChunks             int
	Name, Level, Pos, LevelDesc, Beat, Gram string
}

// genScript writes the whole conversation in one call so it reads as one scene. Lines that fail the
// checks or the reviewer are then rewritten one at a time, with the lines before them as context.
// A nil entry means the line could not be written.
func genScript(ctx context.Context, plan *Plan, ps []PlayerInfo) []*LineData {
	n := len(plan.LinePlan)
	if apiKey == "" {
		time.Sleep(2 * time.Second)
		out := make([]*LineData, n)
		for i := range out {
			out[i] = mockLine(i)
		}
		return out
	}
	var sl []scriptLine
	for i, lp := range plan.LinePlan {
		pl := ps[lp.Player]
		rng := chunkRange[pl.Level]
		sl = append(sl, scriptLine{Index: i, MinChunks: rng[0], MaxChunks: rng[1], Name: plan.Characters[lp.Player].Name,
			Level: pl.Level, Pos: pl.Pos, LevelDesc: pl.LevelDesc, Beat: lp.Beat, Gram: lp.Grammar})
	}
	out := make([]*LineData, n)
	var bad []int
	prompt, err := render("script.txt", map[string]any{
		"Title": plan.Scenario.Title, "Setting": plan.Scenario.Setting, "Premise": plan.Scenario.Premise,
		"Characters": plan.Characters, "Lines": sl, "N": n,
	})
	var sc *struct {
		Lines []LineData `json:"lines"`
	}
	if err == nil {
		sc, err = generate(ctx, "script", prompt, func(s *struct {
			Lines []LineData `json:"lines"`
		}) error {
			if len(s.Lines) < n {
				return fmt.Errorf("need exactly %d lines, got %d", n, len(s.Lines))
			}
			s.Lines = s.Lines[:n]
			good, first := 0, error(nil)
			for i := range s.Lines {
				if e := lineErr(&s.Lines[i], ps[plan.LinePlan[i].Player].Level); e == nil {
					good++
				} else if first == nil {
					first = fmt.Errorf("line %d: %v", i, e)
				}
			}
			if good < (n+1)/2 {
				return fmt.Errorf("too many invalid lines, for example %v", first)
			}
			return nil
		})
	}
	if err != nil {
		logf("script failed, writing lines one by one: %v", err)
		for i := range out {
			bad = append(bad, i)
		}
	} else {
		for i := range sc.Lines {
			if e := lineErr(&sc.Lines[i], ps[plan.LinePlan[i].Player].Level); e != nil {
				logf("line %d rejected: %v", i, e)
				bad = append(bad, i)
				continue
			}
			out[i] = &sc.Lines[i]
		}
		var mu sync.Mutex
		var wg sync.WaitGroup
		for i := range out {
			if out[i] == nil {
				continue
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				if e := judgeLine(ctx, out[i], ps[plan.LinePlan[i].Player].Level); e != nil {
					logf("line %d rejected: %v", i, e)
					mu.Lock()
					bad = append(bad, i)
					out[i] = nil
					mu.Unlock()
				}
			}()
		}
		wg.Wait()
	}
	slices.Sort(bad)
	for _, i := range bad {
		var hist []TLine
		for j := 0; j < i; j++ {
			if out[j] != nil {
				hist = append(hist, TLine{Name: plan.Characters[plan.LinePlan[j].Player].Name, JP: sentence(out[j], identity(len(out[j].Chunks))), EN: out[j].English})
			}
		}
		d, e := genLine(ctx, plan, ps, i, hist)
		if e != nil {
			logf("line %d failed: %v", i, e)
			continue
		}
		out[i] = d
	}
	return out
}
