package cmd

import (
	"strings"
	"testing"
)

func TestKeylightFlashesPerPressAndIgnoresStaleDims(t *testing.T) {
	var k keylight // zero value
	if k.dimLater() != nil || strings.TrimSpace(k.View()) != "" {
		t.Fatal("nothing pressed: dark, nothing to put out")
	}
	k.on('a', false)
	first := keyDimMsg{k.press}
	if !k.lit || k.wrong || !strings.Contains(k.View(), "a") || k.dimLater() == nil {
		t.Fatalf("a right key lights up: %+v", k)
	}
	k.on('x', true) // the next key lands before the first light went out
	k.dim(first)
	if !k.lit || !k.wrong || !strings.Contains(k.View(), "x") {
		t.Fatalf("a stale dim must not put out the newer press: %+v", k)
	}
	k.dim(keyDimMsg{k.press})
	if k.lit || strings.TrimSpace(k.View()) != "" {
		t.Fatalf("its own dim puts it out: %+v", k)
	}
	k.on(' ', false)
	k.on('\b', false)
	if k.label != "⌫" {
		t.Fatalf("label %q", k.label)
	}
}
