package cmd

import (
	"strings"
	"testing"
)

const catalogFixture = `Text#,Type,Issued,Title,Language,Authors,Subjects,LoCC,Bookshelves
67138,Text,2022-01-10,The Sun Also Rises,en,"Hemingway, Ernest, 1899-1961",Fiction,PS,Category: Novels; Category: American Literature
2701,Text,2001-07-01,"Moby Dick; Or, The Whale",en,"Melville, Herman, 1819-1891",Whales,PS,Category: Novels; Category: Adventure
1727,Text,1999-04-01,The Odyssey,en,"Homer, 750? BC-650? BC; Butler, Samuel, 1835-1902",Epic,PA,Category: Poetry
99,Sound,2003-01-01,Some audio,en,Nobody,,,Category: Novels
100,Text,2003-01-01,Un livre,fr,"Verne, Jules, 1828-1905",,,Category: Novels
101,Text,2003-01-01,"Zed
a second line",en,Anonymous,,,
`

const topFixture = `<h2 id="books-last1">Top 1000 EBooks yesterday</h2><ol><li><a href="/ebooks/1727">x</a></li></ol>
<h2 id="books-last30">Top 1000 EBooks last 30 days</h2><ol><li><a href="/ebooks/2701">Moby Dick</a></li><li><a href="/ebooks/67138">Sun</a></li><li><a href="/ebooks/2701">dup</a></li></ol>
<h2 id="authors-last30">authors</h2><ol><li><a href="/ebooks/1727">not a book rank</a></li></ol>`

func TestCatalog(t *testing.T) {
	rank := parseTop(topFixture)
	if rank[2701] != 1 || rank[67138] != 2 || rank[1727] != 0 || len(rank) != 2 {
		t.Fatalf("ranks %v", rank)
	}
	cat, err := parseCatalog(catalogFixture, rank)
	if err != nil {
		t.Fatal(err)
	}
	if len(cat.Entries) != 4 { // the sound file and the French book are out
		t.Fatalf("entries %+v", cat.Entries)
	}
	if got := []string{cat.Entries[0].Title, cat.Entries[1].Title, cat.Entries[2].Title, cat.Entries[3].Title}; strings.Join(got, "|") != "Moby Dick; Or, The Whale|The Sun Also Rises|The Odyssey|Zed" {
		t.Fatalf("order %v", got)
	}
	if strings.Join(cat.Cats, "|") != "Adventure|American Literature|Novels|Poetry" {
		t.Fatalf("cats %v", cat.Cats)
	}
	if cat.categoryName(catFirst+2) != "Novels" || cat.count(catFirst+2) != 2 || cat.count(catPopular) != 2 || cat.count(catAll) != 4 {
		t.Fatalf("category counts wrong: novels %d popular %d all %d", cat.count(catFirst+2), cat.count(catPopular), cat.count(catAll))
	}
	if hits := cat.search(catAll, "hemingway sun", 10); len(hits) != 1 || hits[0].ID != 67138 || hits[0].Author != "Ernest Hemingway" {
		t.Fatalf("search %+v", hits)
	}
	if hits := cat.search(catFirst+2, "odyssey", 10); len(hits) != 0 { // The Odyssey is poetry, not a novel
		t.Fatalf("category filter %+v", hits)
	}
	if a := firstAuthor("Homer, 750? BC-650? BC; Butler, Samuel, 1835-1902"); a != "Homer" {
		t.Fatalf("author %q", a)
	}
}

func TestSkipFrontMatter(t *testing.T) {
	book := "THE BOOK\n\nCONTENTS\n\nCHAPTER I. The start\nCHAPTER II. More\n\n" + strings.Repeat("preface words ", 20) +
		"\n\nCHAPTER I.\n\nIt was a dark night. " + strings.Repeat("and so on ", 400)
	got := skipFrontMatter(book)
	if !strings.HasPrefix(got, "CHAPTER I.\n\nIt was a dark night") {
		t.Fatalf("starts at %q", got[:40])
	}
	if plain := "no headings here " + strings.Repeat("x ", 100); skipFrontMatter(plain) != plain {
		t.Fatal("a book without chapters must be untouched")
	}
	stories := "MEN WITHOUT WOMEN\n\nBy Ernest Hemingway\n\nNew York 1927\n\nCopyright, 1926, by Ernest Hemingway\n\nTHE UNDEFEATED\n\n" +
		"Manuel Garcia climbed the stairs.\n\n" + strings.Repeat("more prose here. ", 40)
	if got := skipFrontMatter(stories); !strings.HasPrefix(got, "Manuel Garcia") {
		t.Fatalf("no chapters: start at the first prose paragraph, got %q", got[:30])
	}
	dialogue := "A TALE\n\n\"Come in,\" she said.\n\n" + strings.Repeat("more prose here. ", 40)
	if got := skipFrontMatter(dialogue); !strings.HasPrefix(got, "\"Come in,\"") {
		t.Fatalf("a short opening line of prose must be kept, got %q", got[:20])
	}
	if s := "Part in which nothing\nPART I\nreal" + strings.Repeat(" pad", 200); !strings.HasPrefix(skipFrontMatter(s), "PART I\nreal") {
		t.Fatal("PART I not found")
	}
}
