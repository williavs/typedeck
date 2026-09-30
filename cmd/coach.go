package cmd

// The coach: what typioca never kept. Every keystroke of every run is appended to keys.csv (the raw record, for
// deep analysis off the deck) and folded into coach.json, a small aggregate per key and per letter pair that fades
// with age. The aggregate names the weak spots, and the "Weak spots" word list is drawn from them.
// Progress lives in the DATA dir: typioca kept its history in ~/.cache, which cleaners are free to wipe.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/williavs/typedeck/cmd/words"
)

const (
	coachDecay   = 0.95 // per run: old evidence fades, half-life about 14 runs
	coachMinN    = 5    // evidence a key or pair needs before it can be called weak
	coachPauseMs = 2000 // a longer gap is a pause, not a slow key
	coachFocus   = 8    // how many weak keys and pairs steer the "Weak spots" list
)

type stroke struct {
	Pos      int // index into the text
	Exp, Got rune
	Ms       int // since the previous key, 0 = first key or after a pause
}

type tally struct{ N, Miss, MsSum, MsN float64 }

type coachData struct {
	Runs   int
	Keys   map[string]*tally
	Pairs  map[string]*tally
	scores map[string]float64 // weak keys and pairs, built on demand, dropped when the tallies change
}

type runRecord struct {
	At    int64 // unix seconds
	Test  string
	Words string
	Wpm   int
	Acc   float64
	Secs  float64
	Keys  int
}

type weakSpot struct {
	What    string
	MissPct float64
	Ms      float64
	Score   float64
}

// ponytail: package state, one typist per process. The ssh server mode would share it; give each session its own
// coach if that mode ever matters.
var (
	coach *coachData
	run   struct {
		strokes []stroke
		last    time.Time
		active  time.Duration // time spent typing: every gap between keys, a pause counts as coachPauseMs at most
		plotted time.Duration // `active` when the speed curve got its last point
	}
)

func init() {
	words.Weigh = func(w string) float64 { return getCoach().weigh(w) }
}

func getDataPath() string {
	dir := os.Getenv("XDG_DATA_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".local", "share")
	}
	dir = filepath.Join(dir, "typedeck")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		panic(err)
	}
	return dir
}

// writeFileAtomic writes beside the target and renames over it: a rocker switched off mid-write leaves the old
// file, never half of a new one.
func writeFileAtomic(path string, write func(io.Writer) error) error {
	tmp := path + ".tmp"
	fh, err := os.Create(tmp)
	if err != nil {
		return err
	}
	err = write(fh)
	if err == nil {
		err = fh.Sync()
	}
	if cerr := fh.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

func writeJSONAtomic(path string, v any) error {
	return writeFileAtomic(path, func(w io.Writer) error {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "\t")
		return enc.Encode(v)
	})
}

func getCoach() *coachData {
	if coach == nil {
		coach = &coachData{}
		if raw, err := os.ReadFile(filepath.Join(getDataPath(), "coach.json")); err == nil {
			json.Unmarshal(raw, coach) // a damaged file costs the aggregate, keys.csv still has every key
		}
		if coach.Keys == nil {
			coach.Keys = map[string]*tally{}
		}
		if coach.Pairs == nil {
			coach.Pairs = map[string]*tally{}
		}
	}
	return coach
}

func coachStart() {
	run.strokes, run.last, run.active, run.plotted = run.strokes[:0], time.Time{}, 0, 0
}

func coachRecord(pos int, exp, got rune, now time.Time) {
	ms := 0
	if !run.last.IsZero() {
		gap := now.Sub(run.last)
		if gap.Milliseconds() < coachPauseMs {
			ms = int(gap.Milliseconds())
		}
		run.active += min(gap, coachPauseMs*time.Millisecond)
	}
	run.last = now
	run.strokes = append(run.strokes, stroke{Pos: pos, Exp: exp, Got: got, Ms: ms})
}

func bump(m map[string]*tally, key string, miss bool, ms int) {
	t := m[key]
	if t == nil {
		t = &tally{}
		m[key] = t
	}
	t.N++
	if miss {
		t.Miss++
	} else if ms > 0 {
		t.MsSum += float64(ms)
		t.MsN++
	}
}

func (c *coachData) absorb(strokes []stroke) {
	for _, m := range []map[string]*tally{c.Keys, c.Pairs} {
		for k, t := range m {
			t.N, t.Miss, t.MsSum, t.MsN = t.N*coachDecay, t.Miss*coachDecay, t.MsSum*coachDecay, t.MsN*coachDecay
			if t.N < 0.05 {
				delete(m, k)
			}
		}
	}
	for i, s := range strokes {
		// timing only counts in flow: straight after the previous letter, which was right
		flow := i > 0 && strokes[i-1].Pos == s.Pos-1 && strokes[i-1].Got == strokes[i-1].Exp
		ms := 0
		if flow {
			ms = s.Ms
		}
		bump(c.Keys, string(s.Exp), s.Got != s.Exp, ms)
		if flow && s.Exp != ' ' && strokes[i-1].Exp != ' ' {
			bump(c.Pairs, string(strokes[i-1].Exp)+string(s.Exp), s.Got != s.Exp, ms)
		}
	}
	c.Runs++
	c.scores = nil
}

// weak ranks keys or pairs by missed share (weighted 3x) plus how far they lag the typist's own average speed.
func weak(m map[string]*tally, n int) []weakSpot {
	var msSum, msN float64
	for _, t := range m {
		msSum += t.MsSum
		msN += t.MsN
	}
	var out []weakSpot
	for k, t := range m {
		if t.N < coachMinN {
			continue
		}
		w := weakSpot{What: k, MissPct: 100 * t.Miss / t.N}
		slow := 0.0
		if t.MsN > 0 && msN > 0 {
			w.Ms = t.MsSum / t.MsN
			slow = math.Max(0, w.Ms/(msSum/msN)-1)
		}
		w.Score = 3*t.Miss/t.N + slow
		if w.Score >= 0.05 {
			out = append(out, w)
		}
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].Score != out[b].Score {
			return out[a].Score > out[b].Score
		}
		return out[a].What < out[b].What
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}

func (c *coachData) weigh(word string) float64 {
	if c.scores == nil {
		c.scores = map[string]float64{}
		for _, w := range weak(c.Keys, coachFocus) {
			c.scores[w.What] = w.Score
		}
		for _, w := range weak(c.Pairs, coachFocus) {
			c.scores[w.What] = 2 * w.Score // a pair is rarer than a key: hitting one is worth more
		}
	}
	runes := []rune(word)
	if len(runes) == 0 {
		return 1
	}
	sum := 0.0
	for i, r := range runes {
		sum += c.scores[string(r)]
		if i > 0 {
			sum += c.scores[string(runes[i-1])+string(r)]
		}
	}
	return 1 + 8*sum/float64(len(runes))
}

func appendLine(path, header, line string) error {
	_, statErr := os.Stat(path)
	fh, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer fh.Close()
	if os.IsNotExist(statErr) && header != "" {
		line = header + line
	}
	_, err = fh.WriteString(line)
	return err
}

// coachFinish closes a run: aggregate, raw log, run list. A failed write must never cost the run on screen.
func coachFinish(r Results) {
	c := getCoach()
	c.absorb(run.strokes)
	dir := getDataPath()
	now := time.Now().Unix()

	var raw strings.Builder
	for _, s := range run.strokes {
		fmt.Fprintf(&raw, "%d,%d,%d,%d,%d\n", now, s.Pos, s.Exp, s.Got, s.Ms)
	}
	rec, _ := json.Marshal(runRecord{
		At: now, Test: r.identifier.testType, Words: r.wordList, Wpm: r.wpm, Acc: r.accuracy,
		Secs: r.time.Seconds(), Keys: len(run.strokes),
	})
	for _, err := range []error{
		writeJSONAtomic(filepath.Join(dir, "coach.json"), c),
		appendLine(filepath.Join(dir, "keys.csv"), "run_unix,pos,expected,typed,ms_since_previous\n", raw.String()),
		appendLine(filepath.Join(dir, "runs.jsonl"), "", string(rec)+"\n"),
	} {
		if err != nil {
			fmt.Fprintln(os.Stderr, "typedeck: progress not saved:", err)
		}
	}
	coachStart()
}

func readRuns() []runRecord {
	fh, err := os.Open(filepath.Join(getDataPath(), "runs.jsonl"))
	if err != nil {
		return nil
	}
	defer fh.Close()
	var out []runRecord
	for sc := bufio.NewScanner(fh); sc.Scan(); {
		var r runRecord
		if json.Unmarshal(sc.Bytes(), &r) == nil {
			out = append(out, r)
		}
	}
	return out
}

func showSpot(w weakSpot, base float64) string {
	what := strings.ReplaceAll(w.What, " ", "␣")
	slow := 0.0
	if base > 0 && w.Ms > 0 {
		slow = w.Ms/base - 1
	}
	if 3*w.MissPct/100 >= slow {
		return fmt.Sprintf("%s %.0f%%", what, w.MissPct)
	}
	return fmt.Sprintf("%s %.0fms", what, w.Ms)
}

// coachPanel is the two lines under every result: the worst keys and the worst pairs, each with its reason
// (share missed, or how slow).
func coachPanel(styles Styles) string {
	c := getCoach()
	var msSum, msN float64
	for _, t := range c.Keys {
		msSum += t.MsSum
		msN += t.MsN
	}
	base := 0.0
	if msN > 0 {
		base = msSum / msN
	}
	line := func(label string, spots []weakSpot) string {
		if len(spots) == 0 {
			return label + style("none yet", styles.toEnter)
		}
		var parts []string
		for _, w := range spots {
			parts = append(parts, style(showSpot(w, base), styles.greener))
		}
		return label + strings.Join(parts, "  ")
	}
	return line("weak keys: ", weak(c.Keys, 4)) + "\n" + line("weak pairs: ", weak(c.Pairs, 3))
}

func avgRuns(runs []runRecord) (wpm, acc float64) {
	if len(runs) == 0 {
		return 0, 0
	}
	for _, r := range runs {
		wpm += float64(r.Wpm)
		acc += r.Acc
	}
	return wpm / float64(len(runs)), acc / float64(len(runs))
}
