package cmd

// keylight: an indicator above the text that lights for an instant on every key, green when it was the right
// key, red when it was not, so a slip is seen without looking down. The press itself redraws the frame; a single
// tea.Tick puts the light out again. A tick carries the press it belongs to, so a late one never dims a newer
// press. The zero value is ready to use.

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const keyLightFor = 120 * time.Millisecond

var (
	keyLightRight = lipgloss.NewStyle().Background(lipgloss.Color("2")).Foreground(lipgloss.Color("0")).Bold(true).Padding(0, 1)
	keyLightWrong = lipgloss.NewStyle().Background(lipgloss.Color("1")).Foreground(lipgloss.Color("0")).Bold(true).Padding(0, 1)
	keyLightOff   = lipgloss.NewStyle().Width(3)
)

type keylight struct {
	label string
	wrong bool
	lit   bool
	press int // counts presses; a dim message names the press it was scheduled for
}

type keyDimMsg struct{ press int }

func (k *keylight) on(r rune, wrong bool) {
	k.label, k.wrong, k.lit = keyLabel(r), wrong, true
	k.press++
}

// dimLater: the command that puts this press's light out.
func (k keylight) dimLater() tea.Cmd {
	if !k.lit {
		return nil
	}
	press := k.press
	return tea.Tick(keyLightFor, func(time.Time) tea.Msg { return keyDimMsg{press} })
}

func (k *keylight) dim(msg keyDimMsg) {
	if msg.press == k.press {
		k.lit = false
	}
}

func keyLabel(r rune) string {
	switch r {
	case ' ':
		return "␣"
	case '\b':
		return "⌫"
	case '\n':
		return "⏎"
	}
	return string(r)
}

func (k keylight) View() string {
	switch {
	case !k.lit:
		return keyLightOff.Render("")
	case k.wrong:
		return keyLightWrong.Render(k.label)
	}
	return keyLightRight.Render(k.label)
}

// withKeylight applies fn to the key light of a run state; other states pass through untouched.
func withKeylight(s State, fn func(*keylight)) State {
	switch st := s.(type) {
	case TimerBasedTest:
		fn(&st.base.struck)
		return st
	case WordCountBasedTest:
		fn(&st.base.struck)
		return st
	case SentenceCountBasedTest:
		fn(&st.base.struck)
		return st
	}
	return s
}
