package words

import (
	"fmt"
	"math"
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

// weighted draws count words without replacement, heavier words first more often (Efraimidis-Spirakis).
func weighted(pool []string, count int) []string {
	type pick struct {
		word string
		key  float64
	}
	picks := make([]pick, len(pool))
	for i, w := range pool {
		weight := 1.0
		if Weigh != nil {
			weight = math.Max(Weigh(w), 0.01)
		}
		picks[i] = pick{w, math.Pow(rand.Float64(), 1/weight)}
	}
	sort.Slice(picks, func(a, b int) bool { return picks[a].key > picks[b].key })

	out := make([]string, 0, count)
	for i := 0; i < count && i < len(picks); i++ {
		out = append(out, picks[i].word)
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
