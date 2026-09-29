package cmd

// Books: any plain text becomes a run you finish over months. `typedeck import` cleans a file, a URL or a Project
// Gutenberg number into the data dir; the book then shows up as a word list of the timer run, every run starts at
// the bookmark and moves it by what was typed.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	bookKeyPrefix = "book:"
	bookChunk     = 4000 // characters handed to a run at a time; a run that gets near the end is handed more
	bookPage      = 1500 // characters in a "page", for the progress line
)

type Book struct {
	Title   string
	Author  string
	Slug    string
	Source  string
	Chars   int // runes in the cleaned text
	Pos     int // the bookmark: next rune to type
	Laps    int // times finished
	Runs    int
	Typed   int   // characters typed in this book, all laps
	Started int64 // unix seconds of the import
	Last    int64 // unix seconds of the last run in it: the home screen offers that book first
}

func booksDir() string {
	dir := filepath.Join(getDataPath(), "books")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		panic(err)
	}
	return dir
}

var (
	gutenbergStart = regexp.MustCompile(`(?m)^\*\*\* ?START OF (THE|THIS) PROJECT GUTENBERG EBOOK.*$`)
	gutenbergEnd   = regexp.MustCompile(`(?m)^\*\*\* ?END OF (THE|THIS) PROJECT GUTENBERG EBOOK.*$`)
	headerField    = regexp.MustCompile(`(?m)^(Title|Author): (.+)$`)
	bracketNote    = regexp.MustCompile(`\[(Illustration|Transcriber|Footnote)[^\]]*\]`)
	notSlug        = regexp.MustCompile(`[^a-z0-9]+`)
)

// Everything a keyboard cannot type is folded to what it can, the rest is dropped.
var typeable = strings.NewReplacer(
	"“", `"`, "”", `"`, "„", `"`, "«", `"`, "»", `"`,
	"‘", "'", "’", "'", "‚", "'", "′", "'",
	"—", "--", "–", "-", "‒", "-", "‐", "-", "‑", "-", "…", "...",
	" ", " ", " ", " ", " ", " ", " ", " ", " ", " ", "\xef\xbb\xbf", "",
	"é", "e", "è", "e", "ê", "e", "ë", "e", "á", "a", "à", "a", "â", "a", "ä", "a",
	"í", "i", "ì", "i", "î", "i", "ï", "i", "ó", "o", "ò", "o", "ô", "o", "ö", "o",
	"ú", "u", "ù", "u", "û", "u", "ü", "u", "ñ", "n", "ç", "c", "ß", "ss",
	"É", "E", "È", "E", "À", "A", "Ñ", "N", "Ç", "C", "æ", "ae", "œ", "oe",
)

// cleanBook turns raw text into one line of typeable characters: licence text cut, hard-wrapped lines rejoined,
// typographic characters folded to their keyboard form.
func cleanBook(raw string) (title, author, text string) {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	head := raw
	if at := gutenbergStart.FindStringIndex(raw); at != nil {
		head, raw = raw[:at[0]], raw[at[1]:]
		if end := gutenbergEnd.FindStringIndex(raw); end != nil {
			raw = raw[:end[0]]
		}
		for _, f := range headerField.FindAllStringSubmatch(head, -1) {
			if f[1] == "Title" && title == "" {
				title = strings.TrimSpace(f[2])
			} else if f[1] == "Author" && author == "" {
				author = strings.TrimSpace(f[2])
			}
		}
	}
	raw = typeable.Replace(bracketNote.ReplaceAllString(raw, ""))
	raw = strings.ReplaceAll(raw, "_", "") // _italics_

	var out strings.Builder
	space := true // no leading space, runs of whitespace become one
	for _, r := range raw {
		switch {
		case r == ' ' || r == '\n' || r == '\t':
			if !space {
				out.WriteByte(' ')
				space = true
			}
		case r > 32 && r < 127:
			out.WriteRune(r)
			space = false
		}
	}
	return title, author, strings.TrimSpace(out.String()) + " " // the closing space lets the last word wrap to the first
}

func fetchBook(src string) (string, error) {
	if src != "" && strings.Trim(src, "0123456789") == "" { // a bare number is a Project Gutenberg ebook
		src = fmt.Sprintf("https://www.gutenberg.org/cache/epub/%s/pg%s.txt", src, src)
	}
	if !strings.HasPrefix(src, "http://") && !strings.HasPrefix(src, "https://") {
		raw, err := os.ReadFile(src)
		return string(raw), err
	}
	client := http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(src)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s: %s", src, resp.Status)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	return string(raw), err
}

// ImportBook reads src (file, URL or Gutenberg number) and stores the cleaned book. Importing a book again
// refreshes its text and keeps the bookmark. start, when given, is a phrase of the text: everything before its
// first appearance (title page, contents, preface) is left out.
func ImportBook(src, title, start string) (Book, error) {
	raw, err := fetchBook(src)
	if err != nil {
		return Book{}, err
	}
	foundTitle, author, text := cleanBook(raw)
	if start != "" {
		at := strings.Index(text, start)
		if at < 0 {
			return Book{}, fmt.Errorf("%s: the phrase %q is not in the text", src, start)
		}
		text = text[at:]
	}
	if title == "" {
		title = foundTitle
	}
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))
	}
	chars := len([]rune(text))
	if chars < 200 {
		return Book{}, fmt.Errorf("%s: only %d typeable characters, this is not a book (plain text only, no epub/pdf)", src, chars)
	}
	book := Book{Title: title, Author: author, Source: src, Chars: chars, Started: time.Now().Unix(),
		Slug: strings.Trim(notSlug.ReplaceAllString(strings.ToLower(title), "-"), "-")}
	if old, ok := loadBook(book.Slug); ok {
		book.Pos, book.Laps, book.Runs, book.Typed, book.Started, book.Last = old.Pos%chars, old.Laps, old.Runs, old.Typed, old.Started, old.Last
	}
	if err := os.WriteFile(filepath.Join(booksDir(), book.Slug+".txt"), []byte(text), 0o644); err != nil {
		return Book{}, err
	}
	delete(bookTexts, book.Slug)
	return book, saveBook(book)
}

func saveBook(b Book) error { return writeJSONAtomic(filepath.Join(booksDir(), b.Slug+".json"), b) }

func loadBook(slug string) (Book, bool) {
	var b Book
	raw, err := os.ReadFile(filepath.Join(booksDir(), slug+".json"))
	return b, err == nil && json.Unmarshal(raw, &b) == nil && b.Chars > 0
}

func Books() []Book {
	var out []Book
	names, _ := filepath.Glob(filepath.Join(booksDir(), "*.json"))
	for _, n := range names {
		if b, ok := loadBook(strings.TrimSuffix(filepath.Base(n), ".json")); ok {
			out = append(out, b)
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Title < out[b].Title })
	return out
}

var bookTexts = map[string][]rune{}

func bookText(slug string) []rune {
	if t, ok := bookTexts[slug]; ok {
		return t
	}
	raw, err := os.ReadFile(filepath.Join(booksDir(), slug+".txt"))
	if err != nil {
		panic(err) // the menu listed a book whose text is gone
	}
	bookTexts[slug] = []rune(string(raw))
	return bookTexts[slug]
}

// bookSlice is n characters of the book from `from`, running over the end into the beginning.
func bookSlice(slug string, from, n int) []rune {
	text := bookText(slug)
	out := make([]rune, n)
	for i := range out {
		out[i] = text[(from+i)%len(text)]
	}
	return out
}

func bookSlug(generatorKey string) (string, bool) {
	return strings.CutPrefix(generatorKey, bookKeyPrefix)
}

// bookAdvance moves the bookmark by what was typed, back to the start of the word the run ended in.
func bookAdvance(slug string, typed int) Book {
	b, ok := loadBook(slug)
	if !ok {
		return b
	}
	text := bookText(slug)
	for typed > 0 && text[(b.Pos+typed-1)%len(text)] != ' ' {
		typed--
	}
	b.Pos += typed
	b.Typed += typed
	b.Runs++
	b.Last = time.Now().Unix()
	b.Laps += b.Pos / b.Chars
	b.Pos %= b.Chars
	if err := saveBook(b); err != nil {
		fmt.Fprintln(os.Stderr, "typedeck: bookmark not saved:", err)
	}
	return b
}

// progress: "1.42%  page 4 of 240"
func (b Book) progress() string {
	return fmt.Sprintf("%.2f%%  page %d of %d", 100*float64(b.Pos)/float64(b.Chars), b.Pos/bookPage+1, b.Chars/bookPage+1)
}

func hint(bookRun bool) string {
	if bookRun {
		return "type. esc stops and saves"
	}
	return "ctrl+r restart, esc menu"
}

// brief fits under a result on the deck's 45 columns: "The Sun Also Rises 1.42% p4/240"
func (b Book) brief() string {
	return fmt.Sprintf("%s %.2f%% p%d/%d", shorten(b.Title, 22), 100*float64(b.Pos)/float64(b.Chars), b.Pos/bookPage+1, b.Chars/bookPage+1)
}

// show: "The Sun Also Rises 1.42% p4/240 ~90d"; the days left come from the pace since the import.
func (b Book) show() string {
	line := fmt.Sprintf("%s %.2f%% p%d/%d", shorten(b.Title, 20), 100*float64(b.Pos)/float64(b.Chars), b.Pos/bookPage+1, b.Chars/bookPage+1)
	days := float64(time.Now().Unix()-b.Started)/86400 + 1
	if perDay := float64(b.Typed) / days; perDay > 0 {
		line += fmt.Sprintf(" ~%.0fd", float64(b.Chars-b.Pos)/perDay)
	}
	if b.Laps > 0 {
		line += fmt.Sprintf(" x%d", b.Laps)
	}
	return line
}

func shorten(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "~"
	}
	return s
}

func bookSelections() []WordsSelection {
	var out []WordsSelection
	for _, b := range Books() {
		out = append(out, WordsSelection{name: shorten(b.Title, 22), generatorKey: bookKeyPrefix + b.Slug})
	}
	return out
}
