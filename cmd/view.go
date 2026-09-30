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
	"github.com/guptarohit/asciigraph"
	"github.com/muesli/reflow/indent"
)

var lineLenLimit int

// narrow: under 52 columns (the deck's panel is 45 x 15) labels are shortened so nothing runs off the edge.
var narrow bool

func label(long, short string) string {
	if narrow {
		return short
	}
	return long
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

		header := "Config\n\n"
		view += header

		wordlistHeader := fmt.Sprintf("%s%*s%s/%s\n\n", "  wordlist", absolutePad-11, " ", "synced", "enabled")
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

			toPad := absolutePad - len(elem.Name)
			line := fmt.Sprintf("%s%*s     [%s] ", style(elem.Name, m.styles.greener), toPad, "", enabled)

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

				toPad := absolutePad - len(elem.Name)
				line := fmt.Sprintf("%s%*s[%s]  [%s] ", style(elem.Name, m.styles.greener), toPad, "", synced, enabled)
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

		view += "\n"
		layoutListHeader := fmt.Sprintf("%s%*s%s/%s\n\n", "  layouts", absolutePad-11, " ", "synced", "enabled")
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

				toPad := absolutePad - len(elem.Name)
				line := fmt.Sprintf("%s%*s[%s]  [%s] ", style(elem.Name, m.styles.greener), toPad, "", synced, enabled)

				if elem.Name == "Qwerty" {
					line = fmt.Sprintf("%s%*s     [%s] ", style(elem.Name, m.styles.greener), toPad, "", enabled)
				}

				lineContent := wrapWithCursor(isCursorOnLine, line, m.styles.runningTimer)
				lineContent += "\n"
				return lineContent
			},
		)
		view += "\n"
		accumulatedLength += len(state.config.LayoutFiles)

		help := style("s sync/delete, e enable/disable, esc to menu", m.styles.toEnter)
		help = lipgloss.NewStyle().Align(lipgloss.Center).Padding(1).Render(help)
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
		var coloredTimer string
		if state.timer.isRunning {
			coloredTimer = style(clock, m.styles.runningTimer)
		} else {
			coloredTimer = style(clock, m.styles.stoppedTimer)
		}

		linesAroundCursor, avgLineLen := state.base.window(lineLenLimit, m.styles)
		s += positionVerticaly(termHeight)
		indentBy := uint(math.Max(0, float64(termWidth/2-avgLineLen/2)))

		s += m.indent(coloredTimer, indentBy) + "\n\n" + m.indent(linesAroundCursor, indentBy)

		if !state.timer.isRunning {
			s += "\n\n\n"
			s += lipgloss.PlaceHorizontal(termWidth, lipgloss.Center, style(hint(state.book != nil), m.styles.toEnter))
		}

	case WordCountBasedTest:
		var coloredStopwatch string
		if state.stopwatch.isRunning {
			coloredStopwatch = style(state.stopwatch.stopwatch.View(), m.styles.runningTimer)
		} else {
			coloredStopwatch = style(state.stopwatch.stopwatch.View(), m.styles.stoppedTimer)
		}

		linesAroundCursor, avgLineLen := state.base.window(lineLenLimit, m.styles)
		s += positionVerticaly(termHeight)
		indentBy := uint(math.Max(0, float64(termWidth/2-avgLineLen/2)))

		s += m.indent(coloredStopwatch, indentBy) + "\n\n" + m.indent(linesAroundCursor, indentBy)

		if !state.stopwatch.isRunning {
			s += "\n\n\n"
			s += lipgloss.PlaceHorizontal(termWidth, lipgloss.Center, style("ctrl+r restart, esc menu", m.styles.toEnter))
		}

	case SentenceCountBasedTest:
		var coloredStopwatch string
		if state.stopwatch.isRunning {
			coloredStopwatch = style(state.stopwatch.stopwatch.View(), m.styles.runningTimer)
		} else {
			coloredStopwatch = style(state.stopwatch.stopwatch.View(), m.styles.stoppedTimer)
		}

		linesAroundCursor, avgLineLen := state.base.window(lineLenLimit, m.styles)
		indentBy := uint(math.Max(0, float64(termWidth/2-avgLineLen/2)))

		s += positionVerticaly(termHeight)
		s += m.indent(coloredStopwatch, indentBy) + "\n\n" + m.indent(linesAroundCursor, indentBy)

		if !state.stopwatch.isRunning {
			s += "\n\n\n"
			s += lipgloss.PlaceHorizontal(termWidth, lipgloss.Center, style("ctrl+r restart, esc menu", m.styles.toEnter))
		}

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

func positionVerticaly(termHeight int) string {
	var acc strings.Builder

	for i := 0; i < termHeight/2-3; i++ {
		acc.WriteRune('\n')
	}

	return acc.String()
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
		wordListSelection = selection.wordListSelections[selection.wordListCursor].name
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
		wordListSelection = selection.wordListSelections[selection.wordListCursor].name
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
		wordListSelection = selection.sentenceListSelections[selection.sentenceListCursor].name
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

// resultsView is the one result screen. Below 20 rows (the deck is 15) it drops the padding and shortens the
// plot so the coach's lines still fit.
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
	plotWidth := len(dropAnsiCodes(stats)) - 2

	var block string
	if m.height < 20 {
		block = lipgloss.JoinVertical(lipgloss.Center, wpm, plotWpms(plotData, plotWidth, 3, 0), stats, wordsLine, coachPanel(m.styles), help)
	} else {
		block = lipgloss.JoinVertical(lipgloss.Center, resultsStyle.Padding(1).Render(wpm), plotWpms(plotData, plotWidth, 5, 1),
			resultsStyle.Padding(0).Render(stats), resultsStyle.Render(wordsLine), coachPanel(m.styles), resultsStyle.Render(help))
	}
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, block)
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
		trend += fmt.Sprintf("  before: %s wpm %s%%", style(fmt.Sprintf("%.0f", bWpm), m.styles.greener), style(fmt.Sprintf("%.1f", bAcc), m.styles.greener))
	}
	shown := runs
	if len(shown) > 40 {
		shown = shown[len(shown)-40:]
	}
	plot := make([]float64, len(shown))
	for i, r := range shown {
		plot[i] = float64(r.Wpm)
	}
	height := 5
	if m.height < 20 {
		height = 3
	}
	block := lipgloss.JoinVertical(lipgloss.Center,
		"Progress  "+style(plural(len(runs), "run"), m.styles.greener),
		plotWpms(plot, min(40, m.width-10), height, 0), trend, coachPanel(m.styles), m.bookLines(3), style("esc menu", m.styles.toEnter))
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
func (base *TestBase) window(limit int, styles Styles) (string, int) {
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

	low, high := cursorLine-1, cursorLine+2 // the line above, the cursor's, the line below
	if cursorLine == 0 {
		low, high = 0, 3
	}
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
		lines = append(lines, style(b.show(), m.styles.greener))
	}
	return strings.Join(lines, "\n")
}
