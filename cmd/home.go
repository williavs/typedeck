package cmd

// Home: the first screen, built around one typist instead of typioca's list of test types. One key continues the
// book; the drills are one key each; the numbers on it are his. typioca's own menu lives on under "More".

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/williavs/typedeck/cmd/words"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const drillTime = 30 * time.Second

type homeItem int

const (
	homeBook homeItem = iota
	homeWeak
	homeSymbols
	homeWords
	homeProgress
	homeMore
)

type Home struct {
	menu   MainMenu
	items  []homeItem
	cursor int
	books  []Book // the one typed in last comes first
	book   int
	runs   []runRecord
}

func initHome() Home {
	h := Home{menu: initMainMenu(), books: Books(), runs: readRuns()}
	sort.SliceStable(h.books, func(a, b int) bool { return h.books[a].Last > h.books[b].Last })
	if len(h.books) > 0 {
		h.items = append(h.items, homeBook)
	}
	h.items = append(h.items, homeWeak, homeSymbols, homeWords, homeProgress, homeMore)
	return h
}

// drill starts a timer run on one word list without touching what typioca's menu remembers.
func (h Home) drill(listKey string, length time.Duration) State {
	settings, ok := h.menu.selections[0].(TimerBasedTestSettings)
	if !ok {
		return h
	}
	found := false
	for i, sel := range settings.wordListSelections {
		if sel.generatorKey == listKey {
			settings.wordListCursor, found = i, true
		}
	}
	if !found { // the list was switched off in Config
		return h
	}
	for i, t := range settings.timeSelections {
		if t == length {
			settings.timeCursor = i
		}
	}
	return initTimerBasedTest(settings, h.menu)
}

func (h Home) handleInput(msg tea.Msg) State {
	key, isKey := msg.(tea.KeyMsg)
	if !isKey {
		return h
	}
	onBook := h.items[h.cursor] == homeBook
	switch key.String() {
	case "up", "k":
		h.cursor = (h.cursor + len(h.items) - 1) % len(h.items)
	case "down", "j":
		h.cursor = (h.cursor + 1) % len(h.items)
	case "left", "h":
		if onBook {
			h.book = (h.book + len(h.books) - 1) % len(h.books)
		}
	case "right", "l":
		if onBook {
			h.book = (h.book + 1) % len(h.books)
		}
	case "enter", " ":
		switch h.items[h.cursor] {
		case homeBook:
			return h.drill(bookKeyPrefix+h.books[h.book].Slug, 0)
		case homeWeak:
			return h.drill(words.WeakSpots, drillTime)
		case homeSymbols:
			return h.drill(words.CodeSymbols, drillTime)
		case homeWords:
			return h.drill("Common words", drillTime)
		case homeProgress:
			return ProgressView{mainMenu: h.menu, runs: h.runs}
		case homeMore:
			return h.menu
		}
	}
	return h
}

func day(unix int64) string { return time.Unix(unix, 0).Local().Format("2006-01-02") }

// habit: minutes typed today, and how many days in a row ending today (or yesterday: today is not lost yet).
func habit(runs []runRecord, now time.Time) (minutesToday float64, streak int) {
	typed := map[string]bool{}
	today := now.Local().Format("2006-01-02")
	for _, r := range runs {
		typed[day(r.At)] = true
		if day(r.At) == today {
			minutesToday += r.Secs / 60
		}
	}
	at := now
	if !typed[today] {
		at = at.AddDate(0, 0, -1)
	}
	for typed[at.Local().Format("2006-01-02")] {
		streak++
		at = at.AddDate(0, 0, -1)
	}
	return minutesToday, streak
}

func bar(share float64, width int) string {
	filled := int(share*float64(width) + 0.5)
	if share > 0 && filled == 0 {
		filled = 1
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

func (m model) homeView(h Home) string {
	width := min(m.width-4, 46)
	s := m.styles
	faint := func(t string) string { return style(t, s.toEnter) }
	row := func(item homeItem, left, right string) string {
		gap := width - 2 - lipgloss.Width(left) - lipgloss.Width(right)
		line := left + strings.Repeat(" ", max(gap, 1)) + right
		if h.items[h.cursor] == item {
			return style("> ", s.runningTimer) + line
		}
		return "  " + line
	}

	minutes, streak := habit(h.runs, time.Now())
	head := "  " + style("typedeck", s.faintGreen)
	switch {
	case len(h.runs) == 0:
		head += faint("  first run")
	case streak > 1:
		head += faint(fmt.Sprintf("  %d days in a row, %.0f min today", streak, minutes))
	default:
		head += faint(fmt.Sprintf("  %.0f min today, %s", minutes, plural(len(h.runs), "run")))
	}

	lines := []string{head, ""}
	if len(h.books) > 0 {
		b := h.books[h.book]
		share := float64(b.Pos) / float64(b.Chars)
		title := shorten(b.Title, width-12)
		if len(h.books) > 1 {
			title += faint(fmt.Sprintf("  %d/%d", h.book+1, len(h.books)))
		}
		lines = append(lines,
			row(homeBook, title, style(fmt.Sprintf("%.2f%%", 100*share), s.runningTimer)),
			"  "+style(bar(share, 16), s.greener)+faint(fmt.Sprintf(" page %d of %d", b.Pos/bookPage+1, b.Chars/bookPage+1)),
			"")
	}

	c := getCoach()
	var spots []string
	for _, w := range append(weak(c.Keys, 3), weak(c.Pairs, 2)...) {
		spots = append(spots, strings.ReplaceAll(w.What, " ", "␣"))
	}
	weakLine := faint("after a few runs")
	if len(spots) > 0 {
		weakLine = style(strings.Join(spots, " "), s.greener)
	}
	trend := faint("no runs yet")
	if len(h.runs) > 0 {
		recent := h.runs
		if len(recent) > 10 {
			recent = recent[len(recent)-10:]
		}
		wpm, acc := avgRuns(recent)
		trend = style(fmt.Sprintf("%.0f wpm  %.1f%%", wpm, acc), s.greener)
	}
	lines = append(lines,
		row(homeWeak, "Weak spots", weakLine),
		row(homeSymbols, "Symbols", faint("[ ] ( ) = 0-9")),
		row(homeWords, "Words", faint("common, 30s")),
		"",
		row(homeProgress, "Progress", trend),
		row(homeMore, "More", faint("timers, lists, config")),
	)
	if m.height >= 14 {
		lines = append(lines, "", faint("  enter start, esc quit"))
	}
	block := lipgloss.NewStyle().Width(width).Align(lipgloss.Left).Render(strings.Join(lines, "\n"))
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, block)
}

func plural(n int, what string) string {
	if n == 1 {
		return "1 " + what
	}
	return fmt.Sprintf("%d %ss", n, what)
}
