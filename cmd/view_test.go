package cmd

import (
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// TestScreensFitTheDeck renders every screen at the deck's 34 x 20 and at a laptop's 80 x 24: a line wider than
// the window wraps or is cut, a screen taller than it scrolls its top away.
func TestScreensFitTheDeck(t *testing.T) {
	freshHome(t)
	typeText("the quick brown fox jumps over the lazy dog, but why? because 6 boys typed by the gap", "bgy6", "tb")
	menu := initMainMenu()
	var runs []runRecord
	for i := range 12 {
		runs = append(runs, runRecord{At: int64(1000 + i), Wpm: 20 + 7*i, Acc: 91.5})
	}
	results := Results{wpm: 123, accuracy: 100, deltaWpm: 100, rawWpm: 130, time: 95 * time.Second, wordList: "Frankenstein sentences"}
	screens := map[string]State{
		"home":     initHome(),
		"menu":     menu,
		"config":   initConfigView(menu.config, menu),
		"progress": ProgressView{mainMenu: menu, runs: runs},
		"results":  TimerBasedTestResults{results: results, wpmEachSecond: []float64{90, 110, 130, 120}, mainMenu: menu},
		"timer":    initTimerBasedTest(menu.selections[0].(TimerBasedTestSettings), menu),
		"words":    initWordCountBasedTest(menu.selections[1].(WordCountBasedTestSettings), menu),
	}
	for _, size := range [][2]int{{34, 20}, {80, 24}} {
		m := initialModel(termenv.ANSI256, termenv.ANSIWhite, size[0], size[1])
		for name, state := range screens {
			m.state = state
			view := m.View()
			if w, h := lipgloss.Width(view), lipgloss.Height(view); w > size[0] || h > size[1] {
				t.Errorf("%s at %dx%d is %dx%d:\n%s", name, size[0], size[1], w, h, view)
			}
		}
	}
}
