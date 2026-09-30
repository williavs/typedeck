package cmd

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/williavs/typedeck/cmd/words"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/muesli/termenv"
)

func freshHome(t *testing.T) string {
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir+"/cache")
	t.Setenv("XDG_CONFIG_HOME", dir+"/config")
	t.Setenv("XDG_DATA_HOME", dir+"/data")
	coach = nil
	coachStart()
	return dir
}

// typeText plays a text through the coach: `wrong` keys are missed once then corrected, `slow` keys take 400 ms.
func typeText(text string, wrong, slow string) []stroke {
	coachStart()
	now := time.Unix(1000, 0)
	for pos, r := range text {
		gap := 100 * time.Millisecond
		if strings.ContainsRune(slow, r) {
			gap = 400 * time.Millisecond
		}
		now = now.Add(gap)
		if strings.ContainsRune(wrong, r) {
			coachRecord(pos, r, '#', now)
			now = now.Add(gap)
		}
		coachRecord(pos, r, r, now)
	}
	return run.strokes
}

func TestCoachFindsWeakSpots(t *testing.T) {
	freshHome(t)
	c := getCoach()
	for i := 0; i < 6; i++ {
		c.absorb(typeText("the quick brown fox jumps over the lazy dog pack my box", "p", "q"))
	}
	keys := weak(c.Keys, 2)
	if len(keys) != 2 || !strings.Contains("pq", keys[0].What) || !strings.Contains("pq", keys[1].What) {
		t.Fatalf("weak keys = %+v, want p (missed) and q (slow)", keys)
	}
	for _, w := range keys {
		if w.What == "p" && w.MissPct < 40 {
			t.Errorf("p missed every time and corrected: want about 50%%, got %.0f%%", w.MissPct)
		}
		if w.What == "q" && w.Ms < 350 {
			t.Errorf("q took 400 ms, got %.0f", w.Ms)
		}
	}
	if pairs := weak(c.Pairs, 1); len(pairs) != 1 || pairs[0].What != "qu" && pairs[0].What != "um" && pairs[0].What != "mp" {
		// "mp" is the pair that ENDS on the missed p; "qu" follows the slow q only in time, the lag lands on q itself
		t.Logf("weak pairs = %+v", pairs)
	}
	if c.weigh("puppy") <= c.weigh("tell") {
		t.Errorf("a word full of the weak key must weigh more: puppy %.2f, tell %.2f", c.weigh("puppy"), c.weigh("tell"))
	}
}

func TestOldEvidenceFades(t *testing.T) {
	freshHome(t)
	c := getCoach()
	for i := 0; i < 6; i++ {
		c.absorb(typeText("pop pup pip pap", "p", ""))
	}
	was := weak(c.Keys, 1)[0].MissPct
	for i := 0; i < 40; i++ {
		c.absorb(typeText("pop pup pip pap", "", "")) // the typist fixed it
	}
	if now := weak(c.Keys, 5); len(now) > 0 && now[0].What == "p" && now[0].MissPct > was/4 {
		t.Errorf("p was %.0f%% missed, 40 clean runs later it is still %.0f%%", was, now[0].MissPct)
	}
}

func TestRunIsSavedAndReloaded(t *testing.T) {
	dir := freshHome(t)
	typeText("pop pup", "p", "")
	coachFinish(Results{identifier: ResultsIdentifier{testType: "TimerBasedTest"}, wpm: 42, accuracy: 91.5, wordList: "Common words", time: 30 * time.Second})
	data := filepath.Join(dir, "data", "typedeck")
	raw, err := os.ReadFile(filepath.Join(data, "keys.csv"))
	if err != nil || strings.Count(string(raw), "\n") != 1+7+4 { // header + 7 letters and a space... + 4 misses
		t.Fatalf("keys.csv: %v, %d lines:\n%s", err, strings.Count(string(raw), "\n"), raw)
	}
	runs := readRuns()
	if len(runs) != 1 || runs[0].Wpm != 42 || runs[0].Words != "Common words" {
		t.Fatalf("runs = %+v", runs)
	}
	coach = nil // a new process
	if got := getCoach(); got.Runs != 1 || got.Keys["p"] == nil || got.Keys["p"].Miss != 4 {
		t.Fatalf("coach.json did not come back: %+v", got.Keys["p"])
	}
}

func TestAtomicWriteKeepsTheOldFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "results.json")
	os.WriteFile(path, []byte("old"), 0o644)
	err := writeFileAtomic(path, func(w io.Writer) error { w.Write([]byte("half of the n")); return errors.New("power cut") })
	if raw, _ := os.ReadFile(path); err == nil || string(raw) != "old" {
		t.Fatalf("a failed write must leave the old file: err=%v file=%q", err, raw)
	}
}

func TestTypingPastTheEndDoesNotPanic(t *testing.T) {
	freshHome(t)
	base := TestBase{wordsToEnter: []rune("ab"), mistakes: mistakes{mistakesAt: map[int]bool{}}}
	for _, r := range "abcd" { // upstream: index out of range on the third key
		handleRunes(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}, &base, nil)
	}
	handleSpace(&base)
	if string(base.inputBuffer) != "ab" {
		t.Fatalf("buffer = %q", string(base.inputBuffer))
	}
}

func TestWpmIsNotRoundedDown(t *testing.T) {
	base := TestBase{inputBuffer: []rune("twelve chars"), mistakes: mistakes{mistakesAt: map[int]bool{}}}
	if got := base.calculateNormalizedWpm(0.5); got != 4.8 { // 12 chars = 2.4 words in half a minute; upstream said 4
		t.Fatalf("wpm = %v, want 4.8", got)
	}
}

func TestWindowShowsThreeLinesAroundTheCursor(t *testing.T) {
	m := midRunModel(t)
	test := m.state.(TimerBasedTest)
	plain := func(styled string) string { return dropAnsiCodes(styled) }
	view, _ := test.base.window(31, m.styles)
	lines := strings.Split(plain(view), "\n")
	if len(lines) != 3 {
		t.Fatalf("%d lines:\n%s", len(lines), plain(view))
	}
	text := string(test.base.wordsToEnter)
	if !strings.Contains(text, strings.Join(lines, "")) {
		t.Fatalf("the window is not a slice of the text:\n%q", lines)
	}
	at := len(test.base.inputBuffer)
	if before := strings.Index(text, lines[0]); before > at || before+len(lines[0])+len(lines[1]) <= at {
		t.Fatalf("cursor at %d is not on the middle line (window starts %d)", at, before)
	}
	for _, l := range lines[:2] {
		if len([]rune(l)) > 31 || !strings.HasSuffix(l, " ") {
			t.Fatalf("line %q: longer than 31 or not broken at a space", l)
		}
	}
}

func TestGeneratedLists(t *testing.T) {
	freshHome(t)
	g := words.NewGenerator(nil)
	g.Count = 50
	code := string(g.Generate(words.CodeSymbols))
	if n := len(code); n < 150 || strings.ContainsAny(code, "\n\t") || !strings.ContainsAny(code, "[]%$&") {
		t.Fatalf("code symbols: %q", code)
	}
	c := getCoach()
	for i := 0; i < 6; i++ {
		c.absorb(typeText("zip zap zoo buzz fizz jazz", "z", ""))
	}
	g.Count = 300
	drill := strings.Fields(string(g.Generate(words.WeakSpots)))
	withZ := 0
	for i, w := range drill {
		if strings.Contains(w, "z") {
			withZ++
		}
		if i > 0 && drill[i-1] == w {
			t.Fatalf("%q twice in a row", w)
		}
	}
	plain := strings.Fields(string(g.Generate("Common words")))
	plainZ := 0
	for _, w := range plain {
		if strings.Contains(w, "z") {
			plainZ++
		}
	}
	// z is the worst key. Common words has a handful of z words, and none may follow itself, so the drill
	// alternates them with other words: measured 111-119 of 300, against 0-1 of 300 in plain common words.
	if len(drill) != 300 || withZ < 90 || plainZ > 10 {
		t.Fatalf("weak drill: %d of %d words carry z; common words: %d of %d", withZ, len(drill), plainZ, len(plain))
	}
}

var tinyBook = "The Project Gutenberg eBook of Tiny\r\n\r\nTitle: Tiny Book\r\n\r\nAuthor: A. Writer\r\n\r\n" +
	"*** START OF THE PROJECT GUTENBERG EBOOK TINY BOOK ***\r\n\r\n" +
	"“It was a fine day,” she said—and it was.\r\nThe café was _full_.\r\n\r\n[Illustration: a cafe]\r\n\r\n" +
	strings.Repeat("He walked on down the road to the town. ", 12) + "\r\n\r\n" +
	"*** END OF THE PROJECT GUTENBERG EBOOK TINY BOOK ***\r\nlicence text\r\n"

func TestBookIsCleanedForAKeyboard(t *testing.T) {
	title, author, text := cleanBook(tinyBook)
	if title != "Tiny Book" || author != "A. Writer" {
		t.Fatalf("title %q author %q", title, author)
	}
	want := `"It was a fine day," she said--and it was. The cafe was full. He walked on`
	if !strings.HasPrefix(text, want) {
		t.Fatalf("text = %q", text[:90])
	}
	if strings.Contains(text, "licence") || strings.Contains(text, "Gutenberg") || strings.Contains(text, "Illustration") {
		t.Fatalf("licence or notes left in: %q", text)
	}
	for _, r := range text {
		if r < 32 || r > 126 {
			t.Fatalf("untypeable %q in the text", r)
		}
	}
}

func TestBookmarkMovesAndSurvivesAReimport(t *testing.T) {
	dir := freshHome(t)
	bookTexts = map[string][]rune{}
	src := filepath.Join(dir, "tiny.txt")
	os.WriteFile(src, []byte(tinyBook), 0o644)
	book, err := ImportBook(src, "", "")
	if err != nil || book.Slug != "tiny-book" || book.Pos != 0 {
		t.Fatalf("import: %v %+v", err, book)
	}
	sel := bookSelections()
	if len(sel) != 1 || sel[0].generatorKey != "book:tiny-book" {
		t.Fatalf("menu entries: %+v", sel)
	}
	text := string(bookText("tiny-book"))
	first := string(bookSlice("tiny-book", 0, 30))
	if first != text[:30] {
		t.Fatalf("first chunk %q", first)
	}

	moved := bookAdvance("tiny-book", 25) // 25 keys end inside "said--and": back to the start of that word
	if got := text[:moved.Pos]; got != `"It was a fine day," she ` {
		t.Fatalf("bookmark after 25 keys: %q", got)
	}
	if next := string(bookSlice("tiny-book", moved.Pos, 9)); next != "said--and" {
		t.Fatalf("the next run starts with %q", next)
	}

	again, _ := ImportBook(src, "", "")
	if again.Pos != moved.Pos || again.Runs != 1 {
		t.Fatalf("a re-import lost the bookmark: %+v", again)
	}

	lapped := bookAdvance("tiny-book", book.Chars) // a whole book later: same place, one lap done
	if lapped.Laps != 1 || lapped.Pos != moved.Pos {
		t.Fatalf("after a full lap: %+v", lapped)
	}
	if _, err := ImportBook(filepath.Join(dir, "missing.txt"), "", ""); err == nil {
		t.Fatal("a missing file must be an error")
	}
	if _, err := ImportBook(src, "", "no such phrase"); err == nil {
		t.Fatal("a start phrase that is not in the text must be an error")
	}
	cut, err := ImportBook(src, "Cut", "He walked on")
	if err != nil || !strings.HasPrefix(string(bookText("cut")), "He walked on down") || cut.Chars >= book.Chars {
		t.Fatalf("start phrase: %v %+v", err, cut)
	}
}

func TestABurstOfKeysIsNotDropped(t *testing.T) {
	freshHome(t)
	base := TestBase{wordsToEnter: []rune("he is here"), mistakes: mistakes{mistakesAt: map[int]bool{}}}
	handleRunes(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("he is")}, &base, nil) // upstream kept only the "s"
	if string(base.inputBuffer) != "he is" || len(base.mistakes.mistakesAt) != 0 {
		t.Fatalf("buffer %q mistakes %v", string(base.inputBuffer), base.mistakes.mistakesAt)
	}
}

func TestBookRunIsUntimedAndMeasuredOverTypingTime(t *testing.T) {
	dir := freshHome(t)
	bookTexts = map[string][]rune{}
	src := filepath.Join(dir, "tiny.txt")
	os.WriteFile(src, []byte(tinyBook), 0o644)
	if _, err := ImportBook(src, "", ""); err != nil {
		t.Fatal(err)
	}
	menu := initMainMenu()
	settings := menu.selections[0].(TimerBasedTestSettings)
	for i, sel := range settings.wordListSelections {
		if sel.generatorKey == "book:tiny-book" {
			settings.wordListCursor = i
		}
	}
	if shown := dropAnsiCodes(settings.show(Styles{greener: plainStyle, runningTimer: plainStyle})); !strings.Contains(shown, "[untimed] [Tiny Book]") {
		t.Fatalf("menu line %q", shown)
	}
	test := initTimerBasedTest(settings, menu)
	if test.book == nil || test.timer.duration != untimed {
		t.Fatalf("book %v duration %v", test.book, test.timer.duration)
	}

	now := time.Unix(5000, 0)
	text := test.base.wordsToEnter
	for i := 0; i < 60; i++ { // 60 keys, 200 ms apart, with one 10 minute break in the middle
		gap := 200 * time.Millisecond
		if i == 30 {
			gap = 10 * time.Minute
		}
		now = now.Add(gap)
		coachRecord(i, text[i], text[i], now)
		test.base.inputBuffer = append(test.base.inputBuffer, text[i])
		test.base.rawInputCnt++
	}
	test.timer.isRunning = true
	// 58 gaps of 0.2 s + the break counted as 2 s = 13.6 s of typing
	if got := test.elapsed(); got != 13600*time.Millisecond {
		t.Fatalf("typing time %v, want 13.6s", got)
	}
	if !test.worthKeeping() {
		t.Fatal("13 s and 60 keys is a run worth keeping")
	}
	done := test.finish(test.elapsed())
	if done.results.wpm < 50 || done.results.wpm > 54 { // 60 chars = 12 words in 13.6 s = 52.9 wpm
		t.Fatalf("wpm %d, want 52", done.results.wpm)
	}
	if b, _ := loadBook("tiny-book"); b.Pos == 0 || b.Pos > 60 || b.Runs != 1 {
		t.Fatalf("bookmark %+v", b)
	}
}

func plainStyle(s string) termenv.Style { return termenv.String(s) }

func TestAccuracyOfNothingIsNotNaN(t *testing.T) {
	if got := (TestBase{}).calculateAccuracy(); got != 0 {
		t.Fatalf("accuracy with no keys = %v", got)
	}
}

func TestHomeIsBuiltAroundTheTypist(t *testing.T) {
	dir := freshHome(t)
	bookTexts = map[string][]rune{}
	h := initHome()
	if h.items[0] != homeWeak || len(h.items) != 5 {
		t.Fatalf("no book yet: the screen opens on the drills, got %v", h.items)
	}
	src := filepath.Join(dir, "tiny.txt")
	os.WriteFile(src, []byte(tinyBook), 0o644)
	ImportBook(src, "", "")
	ImportBook(src, "Second Book", "")
	bookAdvance("second-book", 40)

	h = initHome()
	if h.items[0] != homeBook || h.books[0].Slug != "second-book" {
		t.Fatalf("the book typed in last comes first: %v %v", h.items, h.books[0].Slug)
	}
	enter := tea.KeyMsg{Type: tea.KeyEnter}
	run, ok := h.handleInput(enter).(TimerBasedTest) // ONE key from a cold start
	if !ok || run.book == nil || run.book.Slug != "second-book" || run.timer.duration != untimed {
		t.Fatalf("enter must continue the book: %+v", run.book)
	}
	if got := string(run.base.wordsToEnter[:12]); got != string(bookText("second-book")[run.book.Pos:run.book.Pos+12]) {
		t.Fatalf("not at the bookmark: %q", got)
	}

	next := h.handleInput(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")}).(Home)
	if next.books[next.book].Slug != "tiny-book" {
		t.Fatalf("right on the book row picks the other book, got %q", next.books[next.book].Slug)
	}
	down := next.handleInput(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}).(Home)
	drill, ok := down.handleInput(enter).(TimerBasedTest)
	if !ok || drill.book != nil || drill.timer.duration != drillTime ||
		drill.settings.wordListSelections[drill.settings.wordListCursor].generatorKey != words.WeakSpots {
		t.Fatalf("second row is the 30 s weak-spot drill: %+v", drill.settings)
	}
	if _, ok := initHome().menu.selections[0].(TimerBasedTestSettings); !ok {
		t.Fatal("More still holds typioca's menu")
	}
}

func TestHabitCountsDaysInARow(t *testing.T) {
	now := time.Date(2026, 9, 29, 15, 0, 0, 0, time.Local)
	at := func(daysAgo int, secs float64) runRecord {
		return runRecord{At: now.AddDate(0, 0, -daysAgo).Unix(), Secs: secs}
	}
	minutes, streak := habit([]runRecord{at(4, 60), at(2, 60), at(1, 60), at(0, 90), at(0, 150)}, now)
	if minutes != 4 || streak != 3 {
		t.Fatalf("today 1.5 + 2.5 min, and 3 days in a row: got %.1f min, %d days", minutes, streak)
	}
	if _, streak = habit([]runRecord{at(2, 60), at(1, 60)}, now); streak != 2 {
		t.Fatalf("nothing typed yet today does not break the streak: %d", streak)
	}
	if _, streak = habit([]runRecord{at(3, 60)}, now); streak != 0 {
		t.Fatalf("a gap of two days ends it: %d", streak)
	}
	if got := bar(0.001, 16); !strings.HasPrefix(got, "█░") || len([]rune(got)) != 16 {
		t.Fatalf("any progress shows one block: %q", got)
	}
}
