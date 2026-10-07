package cmd

// keylight: an indicator above the text showing the last key struck, green when it was the right key, red when
// it was not, so a slip is seen without looking down. It stays on until the next key replaces it. The press
// itself redraws the frame: no timer, nothing to wait on. The zero value is dark and ready to use.

import "github.com/charmbracelet/lipgloss"

var (
	keyLightRight = lipgloss.NewStyle().Background(lipgloss.Color("2")).Foreground(lipgloss.Color("0")).Bold(true).Padding(0, 1)
	keyLightWrong = lipgloss.NewStyle().Background(lipgloss.Color("1")).Foreground(lipgloss.Color("0")).Bold(true).Padding(0, 1)
	keyLightOff   = lipgloss.NewStyle().Width(3)
)

type keylight struct {
	label string
	wrong bool
	lit   bool
}

func (k *keylight) on(r rune, wrong bool) {
	k.label, k.wrong, k.lit = keyLabel(r), wrong, true
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
