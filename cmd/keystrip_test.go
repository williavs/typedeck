package cmd

import (
	"strings"
	"testing"
)

func TestKeystripShowsTheLastKeysAndTheirMistakes(t *testing.T) {
	var k keystrip // zero value
	if k.View(plainStyles()) != "" {
		t.Fatal("nothing struck, nothing shown")
	}
	for _, r := range "abcdefghijklmnop" {
		k.push(r, false)
	}
	k.push(' ', false)
	k.push('x', true)
	k.push('\b', false)
	if len(k.recent) != keystripLen {
		t.Fatalf("keeps the last %d, has %d", keystripLen, len(k.recent))
	}
	if got := k.View(plainStyles()); !strings.HasSuffix(got, "␣ x ⌫") || strings.HasPrefix(got, "a") || !strings.HasPrefix(got, "h") {
		t.Fatalf("strip = %q", got)
	}
	if !k.recent[keystripLen-2].wrong || k.recent[keystripLen-1].wrong {
		t.Fatal("the x was wrong, the backspace was not")
	}
}

func plainStyles() Styles {
	return Styles{correct: plainStyle, toEnter: plainStyle, mistakes: plainStyle}
}
