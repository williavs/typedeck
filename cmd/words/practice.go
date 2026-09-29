package words

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"
)

// Two generated word lists typioca never had.
const (
	WeakSpots   = "Weak spots"   // common words, the ones full of the typist's weak keys and pairs come first
	CodeSymbols = "Code symbols" // digits and punctuation in the shapes a programmer types them
)

// Weigh scores a word by how hard it works the typist's weak spots. The coach sets it; nil = every word is equal.
var Weigh func(word string) float64

// focusShare of a weak-spot drill comes from the words that hit a weak key or pair; the rest keeps it reading
// like language. Measured 09-28: weighting alone barely moved the mix (3 z in 200 words with z the worst key),
// because few words carry a rare letter. So the focus words are drawn WITH replacement and repeat.
const focusShare = 0.7

// weighted deals count words: focus words in proportion to their weight, the rest at random.
func weighted(pool []string, count int) []string {
	if len(pool) == 0 {
		return nil
	}
	var focus []string
	var upTo []float64 // running total of the focus weights
	total := 0.0
	for _, w := range pool {
		if Weigh == nil {
			break
		}
		if extra := Weigh(w) - 1; extra > 0 {
			total += extra
			focus = append(focus, w)
			upTo = append(upTo, total)
		}
	}

	out := make([]string, 0, count)
	for len(out) < count {
		word := pool[rand.Intn(len(pool))]
		if len(focus) > 0 && rand.Float64() < focusShare {
			word = focus[sort.SearchFloat64s(upTo, rand.Float64()*total)]
		}
		if n := len(out); n > 0 && out[n-1] == word && len(pool) > 1 {
			continue // never the same word twice in a row
		}
		out = append(out, word)
	}
	return out
}

var codeForms = []string{
	"x[%d]", "arr[%d] = %d", "(a + b) * %d", "$HOME", "%d%%", "a && b", "!ok", "i += %d", "f(x, y)", "#%d",
	"@user", "key: 'v'", "a == b", "n - %d", "~/src", "x ^ y", "a | b", "{%d, %d}", "p->q", "s = \"%d\"",
	"`cmd`", "a != b", "1/%d", "%d.%d", "<%d>", "v_%d", "ok?", "a; b", "\\n", "%d * %d", "[%d:%d]", "&v",
}

func codeSymbols(count int) []string {
	out := make([]string, count)
	for i := range out {
		form := codeForms[rand.Intn(len(codeForms))]
		args := make([]any, strings.Count(form, "%d"))
		for j := range args {
			args[j] = rand.Intn(100)
		}
		out[i] = fmt.Sprintf(form, args...)
	}
	return out
}
