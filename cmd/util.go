package cmd

func longestStringLen(strings []string) int {
	var longest int
	for _, elem := range strings {
		length := len(elem)
		if len(elem) > longest {
			longest = length
		}
	}

	return longest
}

func names(wordList []WordList) []string {
	var acc []string

	for _, elem := range wordList {
		acc = append(acc, elem.Name)
	}

	return acc
}

func dropLastRune(runes []rune) []rune {
	le := len(runes)
	if le != 0 {
		return runes[:le-1]
	} else {
		return runes
	}
}
