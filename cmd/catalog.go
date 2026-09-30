package cmd

// Catalog: every book Project Gutenberg has, from their own catalog file (pg_catalog.csv, ~21 MB), plus their
// "top 1000 of the last 30 days" page for a popularity rank. Official, free, no key. Gutendex would have been
// simpler but sat behind a 403/503 wall when tried (2026-09-30). Parsed once into a compact gzip under the data
// dir and refreshed after 30 days.

import (
	"compress/gzip"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	catalogURL    = "https://www.gutenberg.org/cache/epub/feeds/pg_catalog.csv"
	topURL        = "https://www.gutenberg.org/browse/scores/top1000.php"
	catalogMaxAge = 30 * 24 * time.Hour
)

type Entry struct {
	ID     int
	Title  string
	Author string
	Rank   int     // 1 = most downloaded in the last 30 days, 0 = not in the top 1000
	Cats   []uint8 // indices into Catalog.Cats
	key    string  // lower-cased title and author, what a search matches
}

type Catalog struct {
	Cats    []string // Gutenberg's "Category:" shelves, sorted by name
	Entries []Entry  // ranked books first in rank order, then everything else by title
	Fetched time.Time
}

func catalogPath() string { return filepath.Join(getDataPath(), "catalog.json.gz") }

// parseCatalog keeps the English texts. Titles keep their first line ("Moby Dick; Or, The Whale"), authors become
// "First Last" without the dates ("Hemingway, Ernest, 1899-1961" -> "Ernest Hemingway").
func parseCatalog(csvText string, rank map[int]int) (*Catalog, error) {
	r := csv.NewReader(strings.NewReader(csvText))
	r.LazyQuotes = true
	head, err := r.Read()
	if err != nil {
		return nil, err
	}
	col := map[string]int{}
	for i, h := range head {
		col[h] = i
	}
	for _, need := range []string{"Text#", "Type", "Title", "Language", "Authors", "Bookshelves"} {
		if _, ok := col[need]; !ok {
			return nil, fmt.Errorf("catalog: no %q column", need)
		}
	}
	catIndex := map[string]uint8{}
	cat := &Catalog{Fetched: time.Now()}
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if rec[col["Type"]] != "Text" || rec[col["Language"]] != "en" {
			continue
		}
		id, err := strconv.Atoi(rec[col["Text#"]])
		if err != nil {
			continue
		}
		e := Entry{ID: id, Title: firstLine(rec[col["Title"]]), Author: firstAuthor(rec[col["Authors"]]), Rank: rank[id]}
		for _, shelf := range strings.Split(rec[col["Bookshelves"]], ";") {
			shelf = strings.TrimSpace(shelf)
			if !strings.HasPrefix(shelf, "Category: ") {
				continue
			}
			name := strings.TrimPrefix(shelf, "Category: ")
			idx, ok := catIndex[name]
			if !ok {
				if len(cat.Cats) == 255 {
					continue
				}
				idx = uint8(len(cat.Cats))
				catIndex[name] = idx
				cat.Cats = append(cat.Cats, name)
			}
			e.Cats = append(e.Cats, idx)
		}
		cat.Entries = append(cat.Entries, e)
	}
	// categories in name order; entries were indexed in discovery order, so remap
	order := make([]uint8, len(cat.Cats))
	names := slices.Clone(cat.Cats)
	slices.Sort(names)
	for i, n := range names {
		order[catIndex[n]] = uint8(i)
	}
	cat.Cats = names
	for i := range cat.Entries {
		for j, c := range cat.Entries[i].Cats {
			cat.Entries[i].Cats[j] = order[c]
		}
	}
	cat.sortEntries()
	return cat, nil
}

func (c *Catalog) sortEntries() {
	slices.SortStableFunc(c.Entries, func(a, b Entry) int {
		switch {
		case (a.Rank > 0) != (b.Rank > 0):
			if a.Rank > 0 {
				return -1
			}
			return 1
		case a.Rank > 0:
			return a.Rank - b.Rank
		}
		return strings.Compare(strings.ToLower(a.Title), strings.ToLower(b.Title))
	})
	for i := range c.Entries {
		c.Entries[i].key = strings.ToLower(c.Entries[i].Title + " " + c.Entries[i].Author)
	}
}

func firstLine(s string) string {
	if at := strings.IndexByte(s, '\n'); at >= 0 {
		s = s[:at]
	}
	return strings.Join(strings.Fields(s), " ")
}

var hasDigit = regexp.MustCompile(`[0-9]`)

func firstAuthor(s string) string {
	if at := strings.Index(s, ";"); at >= 0 {
		s = s[:at]
	}
	var parts []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" && !hasDigit.MatchString(p) {
			parts = append(parts, p)
		}
	}
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	default:
		return parts[1] + " " + parts[0]
	}
}

var topLink = regexp.MustCompile(`href="/ebooks/(\d+)"`)

// parseTop reads the "last 30 days" section of Gutenberg's top-1000 page: id -> rank.
func parseTop(html string) map[int]int {
	rank := map[int]int{}
	at := strings.Index(html, `id="books-last30"`)
	if at < 0 {
		return rank
	}
	html = html[at:]
	if end := strings.Index(html[1:], "<h2"); end >= 0 {
		html = html[:end+1]
	}
	for _, m := range topLink.FindAllStringSubmatch(html, -1) {
		id, _ := strconv.Atoi(m[1])
		if _, seen := rank[id]; !seen {
			rank[id] = len(rank) + 1
		}
	}
	return rank
}

func fetchText(url string, limit int64) (string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	// gutenberg.org's gzip stream for the top-1000 page ends without a valid trailer and reads as "unexpected EOF"
	// through Go's transparent decompression. Plain bytes arrive intact.
	req.Header.Set("Accept-Encoding", "identity")
	client := http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s: %s", url, resp.Status)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	return string(raw), err
}

// FetchCatalog downloads both files, parses them and saves the compact form.
func FetchCatalog() (*Catalog, error) {
	csvText, err := fetchText(catalogURL, 128<<20)
	if err != nil {
		return nil, err
	}
	top, err := fetchText(topURL, 8<<20)
	if err != nil {
		return nil, err
	}
	cat, err := parseCatalog(csvText, parseTop(top))
	if err != nil {
		return nil, err
	}
	err = writeFileAtomic(catalogPath(), func(w io.Writer) error {
		gz := gzip.NewWriter(w)
		if err := json.NewEncoder(gz).Encode(cat); err != nil {
			return err
		}
		return gz.Close()
	})
	return cat, err
}

func readCatalog() (*Catalog, error) {
	fh, err := os.Open(catalogPath())
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	gz, err := gzip.NewReader(fh)
	if err != nil {
		return nil, err
	}
	var cat Catalog
	if err := json.NewDecoder(gz).Decode(&cat); err != nil {
		return nil, err
	}
	cat.sortEntries()
	return &cat, nil
}

// LoadCatalog: the saved copy while it is fresh, otherwise a download. A stale copy still serves when the
// download fails (the deck's WiFi is not to be trusted).
func LoadCatalog() (*Catalog, error) {
	saved, err := readCatalog()
	if err == nil && time.Since(saved.Fetched) < catalogMaxAge {
		return saved, nil
	}
	fresh, ferr := FetchCatalog()
	if ferr == nil {
		return fresh, nil
	}
	if err == nil {
		return saved, nil
	}
	return nil, ferr
}

// Category: two pseudo-categories in front of Gutenberg's own shelves.
type Category int

const (
	catPopular Category = iota
	catAll
	catFirst // the first Gutenberg shelf; Category(catFirst + i) is Cats[i]
)

func (c *Catalog) categoryName(i Category) string {
	switch i {
	case catPopular:
		return "Popular"
	case catAll:
		return "All"
	}
	return c.Cats[i-catFirst]
}

func (c *Catalog) categoryCount() int { return len(c.Cats) + int(catFirst) }

func (e Entry) inCategory(i Category) bool {
	switch i {
	case catPopular:
		return e.Rank > 0
	case catAll:
		return true
	}
	return slices.Contains(e.Cats, uint8(i-catFirst))
}

// search: the entries of one category whose title or author contain every word of query, at most limit.
func (c *Catalog) search(category Category, query string, limit int) []Entry {
	words := strings.Fields(strings.ToLower(query))
	var out []Entry
	for _, e := range c.Entries {
		if !e.inCategory(category) {
			continue
		}
		hit := true
		for _, w := range words {
			if !strings.Contains(e.key, w) {
				hit = false
				break
			}
		}
		if hit {
			out = append(out, e)
			if len(out) == limit {
				break
			}
		}
	}
	return out
}

func (c *Catalog) count(category Category) int {
	n := 0
	for _, e := range c.Entries {
		if e.inCategory(category) {
			n++
		}
	}
	return n
}
