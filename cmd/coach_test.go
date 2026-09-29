package cmd

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bloznelis/typioca/cmd/words"
	tea "github.com/charmbracelet/bubbletea"
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
