package cmd

import (
	"strings"
	"testing"
)

func TestKeylightShowsTheLastKeyUntilTheNext(t *testing.T) {
	var k keylight // zero value
	if strings.TrimSpace(k.View()) != "" {
		t.Fatal("nothing pressed: dark")
	}
	k.on('a', false)
	if !k.lit || k.wrong || !strings.Contains(k.View(), "a") {
		t.Fatalf("a right key lights up: %+v", k)
	}
	k.on('x', true)
	if !k.wrong || !strings.Contains(k.View(), "x") || strings.Contains(k.View(), "a") {
		t.Fatalf("the next key replaces it: %+v", k)
	}
	k.on('\b', false)
	if k.label != "⌫" || k.wrong {
		t.Fatalf("backspace: %+v", k)
	}
}
