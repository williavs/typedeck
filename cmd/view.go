package cmd

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/guptarohit/asciigraph"
	"github.com/muesli/reflow/indent"
)

var lineLenLimit int

// narrow: under 52 columns (the deck's panel is 34 x 20) labels are shortened so nothing runs off the edge.
var narrow bool

func label(long, short string) string {
	if narrow {
		return short
	}
	return long
}

// fit cuts a line, colours and all, to n columns.
func fit(s string, n int) string {
	return ansi.Truncate(s, max(n, 0), "~")
}

// listName keeps a word list's name short enough for its menu line on a narrow window.
func listName(name string) string {
	if narrow {
		return shorten(name, 12)
	}
	return name
}

var minLineLen int = 5
var maxLineLen int = 40
var resultsStyle = lipgloss.NewStyle().
	Align(lipgloss.Center).
	PaddingTop(1).
	PaddingBottom(1).
	PaddingLeft(5).
	PaddingRight(5)

func wrapWithCursor(shouldWrap bool, line string, stringStyle StringStyle) string {
	cursor := " "
	cursorClose := " "
	if shouldWrap {
		cursor = style(">", stringStyle)
		cursorClose = style("<", stringStyle)
	}

	return fmt.Sprintf("%s %s%s", cursor, line, cursorClose)
}

func renderSelectionWindow[T any](
	maxAmtToShow, cursor, prevSelectionAmt int,
	cursorWidgetStyle StringStyle,
	selections []T,
	lineRenderer func(elem T, isCursorOnLine bool) string,
) string {
	internalCursorPos := cursor - prevSelectionAmt
	total := len(selections)
	upperInc := int(math.Min(math.Max(float64(maxAmtToShow), float64(internalCursorPos)), float64(total-1)))
	lowerInc := int(math.Max(float64(upperInc-maxAmtToShow), float64(0)))
	var view string

	cursorWidget := fmt.Sprintf("  [%d-%d:%d]", lowerInc+1, upperInc+1, total)
	view += style(cursorWidget, cursorWidgetStyle)
	view += "\n"

	selectionsToShow := selections[lowerInc : upperInc+1]
	for idx, elem := range selectionsToShow {
		view += lineRenderer(elem, idx+lowerInc+prevSelectionAmt == cursor)
	}

	return view
}

func (m model) View() string {
	var s string

	termWidth, termHeight := m.width, m.height
	narrow = termWidth < 52

	reactiveLimit := (termWidth * 6) / 10
	if narrow {
		reactiveLimit = termWidth - 4
	}
	lineLenLimit = int(math.Min(float64(maxLineLen), math.Max(float64(minLineLen), float64(reactiveLimit))))

	switch state := m.state.(type) {
	case Home:
		return m.homeView(state)

	case Finder:
		return m.finderView(state)

	case MainMenu:
		typioca := style("  typedeck", m.styles.faintGreen)
		typioca = lipgloss.NewStyle().PaddingBottom(1).Render(typioca)

		var choices []string
		choiceStyle := lipgloss.NewStyle().PaddingTop(1)
		for i, choice := range state.selections {
			choiceShow := choice.show(m.styles)
			if !choice.Enabled() {
				choiceShow = style(dropAnsiCodes(choiceShow), m.styles.toEnter)
			}

			choiceShow = wrapWithCursor(state.cursor == i, choiceShow, m.styles.runningTimer)
			choiceShow = choiceStyle.Render(choiceShow)
			choices = append(choices, choiceShow)
		}

		joined := lipgloss.JoinVertical(lipgloss.Left, append([]string{typioca}, choices...)...)
		s = lipgloss.NewStyle().Align(lipgloss.Left).Render(joined)

		return lipgloss.Place(termWidth, termHeight, lipgloss.Center, lipgloss.Center, s)

	case ConfigView:
		absolutePad := longestStringLen(names(state.config.WordLists)) + 2
		var view string
		sectionMaxAmountToShow := m.height / 7
		compact := narrow || termHeight < 30 // typioca's padded layout needs 28 rows
		// compact: no blank line under the headers, names cut to the room left, and the two scrolling lists share
		// whatever height is left after everything else
		gap, help := "\n\n", "s sync/delete, e enable/disable, esc to menu"
		name := func(n string) string { return n }
		if compact {
			absolutePad = min(absolutePad, termWidth-12)
			sectionMaxAmountToShow = max(1, (termHeight-9-len(state.config.EmbededWordLists))/2-1)
			gap, help = "\n", "s sync, e on/off, esc menu"
			name = func(n string) string { return shorten(n, absolutePad-1) }
		}

		view += "Config" + gap

		wordlistHeader := fmt.Sprintf("%s%*s%s/%s"+gap, "  wordlist", absolutePad-11, " ", "synced", "enabled")
		if compact {
			wordlistHeader = fmt.Sprintf("  %-*s%s\n", absolutePad, "wordlist", "sync on")
		}
		view += wordlistHeader

		accumulatedLength := len(state.config.EmbededWordLists)
		for idx, elem := range state.config.EmbededWordLists {
			var enabled string
			if elem.Enabled {
				enabled = "x"
			} else {
				enabled = " "
			}
			enabled = style(enabled, m.styles.greener)

			toPad := absolutePad - len(name(elem.Name))
			line := fmt.Sprintf("%s%*s     [%s] ", style(name(elem.Name), m.styles.greener), toPad, "", enabled)

			view += wrapWithCursor(idx == state.cursor, line, m.styles.runningTimer)
			view += "\n"
		}
		view += "\n"

		view += renderSelectionWindow(
			sectionMaxAmountToShow,
			state.cursor,
			accumulatedLength,
			m.styles.toEnter,
			state.config.WordLists,
			func(elem WordList, isCursorOnLine bool) string {
				var synced string
				if elem.synced {
					synced = "x"
				} else {
					synced = " "
				}
				if !elem.isLocal {
					synced = style(synced, m.styles.greener)
				} else {
					synced = style(synced, m.styles.toEnter)
				}

				var enabled string
				if elem.Enabled {
					enabled = "x"
				} else {
					enabled = " "
				}

				if !elem.isLocal {
					enabled = style(enabled, m.styles.greener)
				} else {
					enabled = style(enabled, m.styles.toEnter)
				}

				toPad := absolutePad - len(name(elem.Name))
				line := fmt.Sprintf("%s%*s[%s]  [%s] ", style(name(elem.Name), m.styles.greener), toPad, "", synced, enabled)
				if !elem.syncOK {
					line = style(dropAnsiCodes(line), m.styles.mistakes)
				}

				lineContents := wrapWithCursor(isCursorOnLine, line, m.styles.runningTimer)
				lineContents += "\n"

				return lineContents
			},
		)
		view += "\n"
		accumulatedLength += len(state.config.WordLists)

		layoutListHeader := fmt.Sprintf("%s%*s%s/%s"+gap, "  layouts", absolutePad-11, " ", "synced", "enabled")
		if compact {
			layoutListHeader = fmt.Sprintf("  %-*s%s\n", absolutePad, "layouts", "sync on")
		} else {
			view += "\n"
		}
		view += layoutListHeader
		view += renderSelectionWindow(
			sectionMaxAmountToShow,
			state.cursor,
			accumulatedLength,
			m.styles.toEnter,
			state.config.LayoutFiles,
			func(elem LayoutFile, isCursorOnLine bool) string {
				var synced string
				if elem.synced {
					synced = "x"
				} else {
					synced = " "
				}

				var enabled string
				if elem.Name == state.config.Layout.Name {
					enabled = "x"
				} else {
					enabled = " "
				}

				toPad := absolutePad - len(name(elem.Name))
				line := fmt.Sprintf("%s%*s[%s]  [%s] ", style(name(elem.Name), m.styles.greener), toPad, "", synced, enabled)

				if elem.Name == "Qwerty" {
					line = fmt.Sprintf("%s%*s     [%s] ", style(name(elem.Name), m.styles.greener), toPad, "", enabled)
				}

				lineContent := wrapWithCursor(isCursorOnLine, line, m.styles.runningTimer)
				lineContent += "\n"
				return lineContent
			},
		)
		view += "\n"
		accumulatedLength += len(state.config.LayoutFiles)

		help = style(help, m.styles.toEnter)
		if !compact {
			help = lipgloss.NewStyle().Align(lipgloss.Center).Padding(1).Render(help)
		}
		view = lipgloss.NewStyle().Align(lipgloss.Left).Render(view)

		all := lipgloss.JoinVertical(lipgloss.Center, view, help)

		return lipgloss.Place(termWidth, termHeight, lipgloss.Center, lipgloss.Center, all)

	case TimerBasedTestResults:
		s = m.resultsView(state.results, state.wpmEachSecond, "words: ")

	case WordCountTestResults:
		s = m.resultsView(state.results, state.wpmEachSecond, fmt.Sprintf("cnt: %s words: ", style(strconv.Itoa(state.wordCnt), m.styles.greener)))

	case SentenceCountTestResults:
		s = m.resultsView(state.results, state.wpmEachSecond, fmt.Sprintf("cnt: %s sentences: ", style(strconv.Itoa(state.sentenceCnt), m.styles.greener)))

	case ProgressView:
		s = m.progressView(state)

	case TimerBasedTest:
		clock := state.timer.timer.View()
		if state.book != nil { // no countdown: how far into the book, moving as you type
			at := *state.book
			at.Pos = (at.Pos + len(state.base.inputBuffer)) % at.Chars
			clock = at.progress()
		}
		return m.testView(&state.base, clock, state.timer.isRunning, hint(state.book != nil))

	case WordCountBasedTest:
		return m.testView(&state.base, state.stopwatch.stopwatch.View(), state.stopwatch.isRunning, hint(false))

	case SentenceCountBasedTest:
		return m.testView(&state.base, state.stopwatch.stopwatch.View(), state.stopwatch.isRunning, hint(false))
	}

	return s
}

// testView is every run's screen: the struck keys, the clock, the lines around the cursor, and the hint while
// stopped. On a narrow window (the deck is 34 x 20) a line wraps at nearly the full width, and a tall one shows
// five lines instead of three, since narrow lines hold fewer words.
func (m model) testView(base *TestBase, clock string, running bool, hint string) string {
	if running {
		clock = style(clock, m.styles.runningTimer)
	} else {
		clock = style(clock, m.styles.stoppedTimer)
	}
	rows := 3
	if narrow && m.height >= 18 {
		rows = 5
	}
	lines, avg := base.window(lineLenLimit, rows, m.styles)
	indentBy := uint(max(0, m.width/2-avg/2))
	s := positionVerticaly(min(m.height/2-3, m.height-rows-8)) +
		lipgloss.PlaceHorizontal(m.width, lipgloss.Center, base.struck.View()) + "\n\n" + m.indent(clock, indentBy) + "\n\n" + m.indent(lines, indentBy)
	if !running {
		s += "\n\n\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, style(hint, m.styles.toEnter))
	}
	return s
}

func plusIfPositive(f float64) string {
	if f > 0.0 {
		return "+"
	} else {
		return ""
	}
}

func positionVerticaly(lines int) string {
	return strings.Repeat("\n", max(0, lines))
}

func plotWpms(wpms []float64, width, height, padding int) string {
	wpmGraph := asciigraph.Plot(
		wpms,
		asciigraph.Precision(0),
		asciigraph.Height(height),
		asciigraph.Width(width),
		asciigraph.CaptionColor(2),
		asciigraph.LabelColor(2),
	)

	return lipgloss.NewStyle().Padding(padding).Render(wpmGraph)
}

func (selection TimerBasedTestSettings) show(styles Styles) string {
	var wordListSelection string
	if selection.enabled {
		wordListSelection = listName(selection.wordListSelections[selection.wordListCursor].name)
	} else {
		wordListSelection = "no wordlist enabled"
	}

	timeSelection := selection.timeSelections[selection.timeCursor].String()
	if selection.book() != nil {
		timeSelection = "untimed"
	}
	selections := []string{timeSelection, wordListSelection}
	selectionsStr := showSelections(selections, selection.cursor, styles)
	return fmt.Sprintf("%s %s", label("Timer run", "Timer"), selectionsStr)
}

func (selection WordCountBasedTestSettings) show(styles Styles) string {
	var wordListSelection string
	if selection.enabled {
		wordListSelection = listName(selection.wordListSelections[selection.wordListCursor].name)
	} else {
		wordListSelection = "no wordlist enabled"
	}

	selections := []string{fmt.Sprint(selection.wordCountSelections[selection.wordCountCursor]), wordListSelection}
	selectionsStr := showSelections(selections, selection.cursor, styles)
	return fmt.Sprintf("%s %s", label("Word count run", "Words"), selectionsStr)
}

func (selection SentenceCountBasedTestSettings) show(styles Styles) string {
	var wordListSelection string
	if selection.enabled {
		wordListSelection = listName(selection.sentenceListSelections[selection.sentenceListCursor].name)
	} else {
		wordListSelection = "no wordlist enabled"
	}
	selections := []string{fmt.Sprint(selection.sentenceCountSelections[selection.sentenceCountCursor]), wordListSelection}
	selectionsStr := showSelections(selections, selection.cursor, styles)
	return fmt.Sprintf("%s %s", label("Sentence count run", "Sentences"), selectionsStr)
}

func (selection ProgressViewSelection) show(styles Styles) string {
	return "Progress "
}

func (selection ConfigViewSelection) show(styles Styles) string {
	return "Config "
}

func showSelections(selections []string, cursor int, styles Styles) string {
	var selectionsStr string
	for i, option := range selections {
		if i+1 == cursor {
			selectionsStr += "[" + style(option, styles.runningTimer) + "]"
		} else {
			selectionsStr += "[" + style(option, styles.greener) + "]"
		}
		selectionsStr += " "
	}
	return selectionsStr
}

var ansiCodes = regexp.MustCompile("\x1b\\[[0-9;]*m") // was compiled again on every call, several times a frame

func dropAnsiCodes(colored string) string {
	return ansiCodes.ReplaceAllString(colored, "")
}

func (m model) indent(block string, indentBy uint) string {
	indentedBlock := indent.String(block, indentBy) // this crashes on small windows

	return indentedBlock
}

func style(str string, style StringStyle) string {
	return style(str).String()
}

// resultsView is the one result screen. When the padded layout does not fit the window it drops the padding and
// shortens the plot so the coach's lines still fit.
func (m model) resultsView(r Results, wpmEachSecond []float64, wordsLabel string) string {
	wpm := "wpm: " + style(strconv.Itoa(r.wpm), m.styles.runningTimer)
	stats := fmt.Sprintf("%s %s %s %s",
		label("accuracy: ", "acc: ")+style(fmt.Sprintf("%.1f", r.accuracy), m.styles.greener),
		label("Δavg: ", "Δ: ")+style(fmt.Sprintf("%s%.0f%%", plusIfPositive(r.deltaWpm), math.Min(r.deltaWpm, 100.0)), m.styles.greener),
		"raw: "+style(strconv.Itoa(r.rawWpm), m.styles.greener),
		label("time: ", "")+style(r.time.Round(time.Second).String(), m.styles.greener))
	if narrow {
		wordsLabel = ""
	}
	wordsLine := wordsLabel + style(r.wordList, m.styles.greener)
	help := style("enter again, esc menu", m.styles.toEnter)
	plotData := append(append([]float64{}, wpmEachSecond...), float64(r.wpm))
	plotWidth := min(lipgloss.Width(stats)-2, m.width-8)
	stats, wordsLine = fit(stats, m.width), fit(wordsLine, m.width)
	coach := coachPanel(m.styles, m.width)

	block := lipgloss.JoinVertical(lipgloss.Center, resultsStyle.Padding(1).Render(wpm), plotWpms(plotData, plotWidth, 5, 1),
		resultsStyle.Padding(0).Render(stats), resultsStyle.Render(wordsLine), coach, resultsStyle.Render(help))
	if !m.fits(block) {
		block = lipgloss.JoinVertical(lipgloss.Center, wpm, plotWpms(plotData, plotWidth, 3, 0), stats, wordsLine, coach, help)
	}
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, block)
}

func (m model) fits(block string) bool {
	return lipgloss.Width(block) <= m.width && lipgloss.Height(block) <= m.height
}

func (m model) progressView(state ProgressView) string {
	runs := state.runs
	if len(runs) == 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center,
			"Progress\n\n"+style("no runs saved yet", m.styles.toEnter)+"\n"+m.bookLines(5)+"\n"+style("esc menu", m.styles.toEnter))
	}
	recent, before := runs, []runRecord(nil)
	if len(runs) > 10 {
		recent, before = runs[len(runs)-10:], runs[:len(runs)-10]
		if len(before) > 10 {
			before = before[len(before)-10:]
		}
	}
	wpm, acc := avgRuns(recent)
	trend := fmt.Sprintf(label("last %d: %s wpm %s%%", "%d: %s wpm %s%%"), len(recent), style(fmt.Sprintf("%.0f", wpm), m.styles.runningTimer), style(fmt.Sprintf("%.1f", acc), m.styles.greener))
	if len(before) > 0 {
		bWpm, bAcc := avgRuns(before)
		trend += fmt.Sprintf(label("  before: %s wpm %s%%", "  was %s %s%%"), style(fmt.Sprintf("%.0f", bWpm), m.styles.greener), style(fmt.Sprintf("%.1f", bAcc), m.styles.greener))
	}
	shown := runs
	if len(shown) > 40 {
		shown = shown[len(shown)-40:]
	}
	plot := make([]float64, len(shown))
	for i, r := range shown {
		plot[i] = float64(r.Wpm)
	}
	var block string
	for _, height := range []int{5, 3} {
		block = lipgloss.JoinVertical(lipgloss.Center,
			"Progress  "+style(plural(len(runs), "run"), m.styles.greener),
			plotWpms(plot, min(40, m.width-10), height, 0), fit(trend, m.width), coachPanel(m.styles, m.width), m.bookLines(3), style("esc menu", m.styles.toEnter))
		if m.fits(block) {
			break
		}
	}
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, block)
}

// lineStarts wraps greedily at spaces and returns the index where each line begins. The space stays at the end
// of its line, as it always did.
func lineStarts(text []rune, limit int) []int {
	starts := []int{0}
	lineStart, wordStart := 0, 0
	for i, r := range text {
		if r != ' ' && i+1 != len(text) {
			continue
		}
		// the word text[wordStart..i] ends here
		if i+1-lineStart > limit && wordStart > lineStart {
			starts = append(starts, wordStart)
			lineStart = wordStart
		}
		wordStart = i + 1
	}
	return starts
}

// window draws the lines around the cursor and nothing else. typioca styled every typed character on its own,
// wrapped all 300 words and stripped the colours back off, on every key and every timer tick, to show three lines.
func (base *TestBase) window(limit, rows int, styles Styles) (string, int) {
	text := base.wordsToEnter
	starts := lineStarts(text, limit)
	end := func(line int) int {
		if line+1 < len(starts) {
			return starts[line+1]
		}
		return len(text)
	}
	at := len(base.inputBuffer)
	cursorLine := sort.Search(len(starts), func(i int) bool { return starts[i] > at }) - 1

	low := max(0, cursorLine-1) // the line above, the cursor's, the rest below
	high := low + rows
	if high > len(starts) {
		high = len(starts)
	}

	var out strings.Builder
	for line := low; line < high; line++ {
		if line > low {
			out.WriteByte('\n')
		}
		for i := starts[line]; i < end(line); {
			kind := base.kindAt(i, at)
			j := i + 1
			for j < end(line) && base.kindAt(j, at) == kind {
				j++
			}
			out.WriteString(style(string(text[i:j]), [...]StringStyle{styles.correct, styles.mistakes, styles.cursor, styles.toEnter}[kind]))
			i = j
		}
	}

	avg := len(text)
	if len(starts) > 1 { // the last line is left out: it may be short and would skew the centring
		avg = starts[len(starts)-1] / (len(starts) - 1)
	}
	return out.String(), avg
}

// kindAt: 0 typed right, 1 typed wrong, 2 the cursor, 3 still to type
func (base *TestBase) kindAt(i, at int) int {
	switch {
	case i == at:
		return 2
	case i > at:
		return 3
	case base.mistakes.mistakesAt[i]:
		return 1
	}
	return 0
}

func (m model) bookLines(limit int) string {
	var lines []string
	for i, b := range Books() {
		if i == limit {
			break
		}
		line := b.show()
		if len(line) > m.width { // the days left go first, then the title
			line = shorten(b.brief(), m.width)
		}
		lines = append(lines, style(line, m.styles.greener))
	}
	return strings.Join(lines, "\n")
}
