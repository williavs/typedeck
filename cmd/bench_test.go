package cmd

import (
	"os"
	"testing"

	"github.com/muesli/termenv"
)

// A frame in the middle of a 30 s run, on the deck's 53 x 15 terminal. This is what every keypress and every
// timer tick costs.
func midRunModel(tb testing.TB) model {
	dir := tb.TempDir()
	os.Setenv("XDG_CACHE_HOME", dir+"/cache")
	os.Setenv("XDG_CONFIG_HOME", dir+"/config")
	os.Setenv("XDG_DATA_HOME", dir+"/data")
	m := initialModel(termenv.ANSI256, termenv.ANSIWhite, 53, 15)
	menu := m.state.(Home).menu
	test := initTimerBasedTest(menu.selections[0].(TimerBasedTestSettings), menu)
	for i := 0; i < 400; i++ { // 400 keys in, every 20th one wrong
		r := test.base.wordsToEnter[i]
		if i%20 == 7 {
			r = '#'
		}
		test.base.inputBuffer = append(test.base.inputBuffer, r)
		if r == '#' {
			test.base.mistakes.mistakesAt[i] = true
		}
	}
	test.base.cursor = len(test.base.inputBuffer)
	m.state = test
	return m
}

func BenchmarkFrame(b *testing.B) {
	m := midRunModel(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.View()
	}
}
