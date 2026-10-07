package cmd

// keystrip: the last keys struck, newest on the right, shown above the text so a wrong key is seen without
// looking down at the keyboard. It rides the keypress that already redraws the frame: no timer, no goroutine,
// nothing to wait on. The zero value is ready to use.

import "strings"

const keystripLen = 12

type struck struct {
	label string
	wrong bool
}

type keystrip struct {
	recent []struck
}

func (k *keystrip) push(r rune, wrong bool) {
	k.recent = append(k.recent, struck{label: keyLabel(r), wrong: wrong})
	if len(k.recent) > keystripLen {
		k.recent = k.recent[len(k.recent)-keystripLen:]
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

// View: faint history, the newest key bright, every wrong key in the mistake colour.
func (k keystrip) View(styles Styles) string {
	if len(k.recent) == 0 {
		return ""
	}
	parts := make([]string, len(k.recent))
	for i, s := range k.recent {
		switch {
		case s.wrong:
			parts[i] = style(s.label, styles.mistakes)
		case i == len(k.recent)-1:
			parts[i] = style(s.label, styles.correct)
		default:
			parts[i] = style(s.label, styles.toEnter)
		}
	}
	return strings.Join(parts, " ")
}
