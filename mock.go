package main

import "strings"

// Offline story used when OPENROUTER_API_KEY is not set, so the app can be tried without a key.

type mockSrc struct {
	chunks  string
	orders  [][]int
	en      string
	q       string
	opts    []string
	ans     int
	why     string
	beat    string
	grammar string
	mood    string
}

var mockBank = []mockSrc{
	{"今日[きょう]は|雨[あめ]が|たくさん|降[ふ]っています", [][]int{{0, 2, 1, 3}}, "It is raining a lot today.",
		"「雨が降っています」の「が」はなに？ (What does が mark?)", []string{"The subject", "The place", "The time", "The tool"}, 0, "が marks the subject that does the action: rain.", "A rainy day begins.", "Subject particle が", "calm"},
	{"わたしは|友達[ともだち]と|カフェで|待[ま]っています", [][]int{{0, 2, 1, 3}}, "I am waiting with a friend at a café.",
		"「友達と」の「と」は？ (What does と mean here?)", []string{"from", "with", "to (a place)", "only"}, 1, "と means 'together with' a person.", "Two friends wait in a café.", "Particle と (with)", "happy"},
	{"田中[たなか]さんが|遅[おく]れて|来[き]ました", nil, "Mr. Tanaka came late.",
		"「遅れて」の意味は？ (What does 遅れて mean?)", []string{"being early", "being late", "being angry", "being quiet"}, 1, "遅れる means to be late.", "A third friend arrives late.", "て-form linking verbs", "tense"},
	{"彼[かれ]は|傘[かさ]を|忘[わす]れたので|濡[ぬ]れました", nil, "He forgot his umbrella, so he got wet.",
		"「ので」の意味は？ (What does ので express?)", []string{"a reason", "a wish", "a contrast", "a question"}, 0, "ので gives the reason for what follows.", "He is soaked.", "Reason ので", "sad"},
	{"わたしは|タオルを|貸[か]して|あげました", [][]int{{0, 1, 2, 3}}, "I lent him a towel.",
		"「貸してあげました」の「あげる」は？ (What does あげる add?)", []string{"Doing a favor for someone", "A past dream", "A strong order", "A guess"}, 0, "〜てあげる means doing something for someone.", "A small kindness.", "〜てあげる", "happy"},
	{"三人[さんにん]で|温[あたた]かい|コーヒーを|飲[の]みました", [][]int{{0, 2, 1, 3}}, "The three of us drank hot coffee.",
		"「三人で」の「で」は？ (What does で mean here?)", []string{"Place of birth", "As a group of", "Direction", "Cause of sadness"}, 1, "で after a number of people means 'as a group of'.", "They warm up together.", "Particle で (by/as a group)", "calm"},
	{"雨[あめ]が|止[や]んだら|公園[こうえん]へ|行[い]きましょう", [][]int{{2, 0, 1, 3}}, "Once the rain stops, let's go to the park.",
		"「止んだら」の「たら」は？ (What does たら express?)", []string{"If / once", "Because", "Although", "Never"}, 0, "〜たら means 'if' or 'once' something happens.", "The sky clears.", "Conditional たら", "excited"},
	{"外[そと]に|出[で]ると|空[そら]に|虹[にじ]が|ありました", [][]int{{2, 0, 1, 3, 4}}, "When we went outside, there was a rainbow in the sky.",
		"「虹」はなに？ (What is 虹?)", []string{"rainbow", "cloud", "star", "wind"}, 0, "虹 means rainbow.", "A rainbow appears.", "Discovery with と", "mysterious"},
	{"みんなで|写真[しゃしん]を|撮[と]りました", [][]int{{1, 0, 2}}, "Everyone took a photo together.",
		"「撮りました」の意味は？ (What does 撮りました mean?)", []string{"took (a photo)", "wrote", "bought", "lost"}, 0, "撮る is the verb for taking photos.", "A photo to remember.", "Verb 撮る", "happy"},
	{"今日[きょう]は|楽[たの]しい|一日[いちにち]でした", nil, "Today was a fun day.",
		"「楽しい」の反対は？ (What is the opposite of 楽しい?)", []string{"つまらない", "おいしい", "あたたかい", "ちかい"}, 0, "つまらない means boring, the opposite of fun.", "A warm ending.", "い-adjective + でした", "calm"},
}

func mockPlan(ps []PlayerInfo, nLines int) *Plan {
	p := &Plan{}
	p.Scenario.Title = "雨の日のカフェ"
	p.Scenario.Setting = "A small café on a rainy afternoon in Kyoto."
	p.Scenario.Premise = "Friends meet at a café while it pours outside. A late arrival and a forgotten umbrella lead to a small adventure."
	p.Scenario.Mood = "calm"
	emojis := []string{"🦊", "🐼", "🐰", "🐱", "🐸", "🐻"}
	names := []string{"ハル", "ミオ", "ソラ", "リン", "ユキ", "アオ"}
	reads := map[string]string{
		"N5": "今日[きょう]は雨[あめ]です。\nわたしは友達[ともだち]とカフェにいます。\nコーヒーを飲[の]みます。",
		"N4": "今日[きょう]は雨[あめ]がたくさん降[ふ]っています。\n友達[ともだち]と会[あ]うので、カフェで待[ま]っています。",
		"N3": "雨[あめ]が降[ふ]っているので、カフェで友達[ともだち]を待[ま]っていた。\nそこへ、傘[かさ]を忘[わす]れた友達[ともだち]が来[き]た。",
		"N2": "突然[とつぜん]の雨[あめ]に降[ふ]られ、一同[いちどう]はカフェに駆[か]け込[こ]んだ。",
		"N1": "にわか雨[あめ]に足止[あしど]めを食[く]らった面々[めんめん]は、カフェで雨宿[あまやど]りをすることにした。",
	}
	goals := map[string]string{
		"N5": "Practise は, が, と and basic ます-form verbs.",
		"N4": "Practise ので (reason) and て-form linking.",
		"N3": "Practise たら and ので in natural sentences.",
		"N2": "Notice formal phrasing and passive forms.",
		"N1": "Notice nuance and literary vocabulary.",
	}
	for _, pl := range ps {
		p.Characters = append(p.Characters, Character{Player: pl.Index, Name: names[pl.Index%6], Emoji: emojis[pl.Index%6], Personality: "friendly", SpeechStyle: "casual polite"})
		p.Briefings = append(p.Briefings, Briefing{Player: pl.Index, Reading: reads[pl.Level], ReadingEN: "It is raining. Friends wait for each other in a café and share coffee.",
			Goal: goals[pl.Level], Teach: "Explain one particle from your lines to the group.",
			Words: []Word{{"雨", "あめ", "rain"}, {"友達", "ともだち", "friend"}, {"傘", "かさ", "umbrella"}}})
	}
	for i := 0; i < nLines; i++ {
		m := mockBank[i%len(mockBank)]
		p.LinePlan = append(p.LinePlan, LinePlan{Index: i, Player: i % len(ps), Beat: m.beat, Grammar: m.grammar, Mood: m.mood})
	}
	return p
}

func mockLine(i int) *LineData {
	m := mockBank[i%len(mockBank)]
	parts := strings.Split(m.chunks, "|")
	d := &LineData{English: m.en}
	for _, c := range parts {
		d.Chunks = append(d.Chunks, parseNotation(c))
	}
	d.AcceptedOrders = m.orders
	d.Quiz = Quiz{Question: m.q, Options: m.opts, Answer: m.ans, Explanation: m.why}
	if err := checkLine(d); err != nil {
		panic(err)
	}
	return d
}
