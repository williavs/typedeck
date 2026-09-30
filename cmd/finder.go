package cmd

// Finder: pick a category, then a book, and it joins the collection. Two screens, one bubbles list that narrows as
// you type. The first run opens here: a home screen of stats means nothing before there is a book.

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	finderWidth = 60
	finderRows  = 200 // matches kept per search; nobody scrolls further, they type another letter
)

var (
	finderTitle  = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true)
	finderFaint  = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	finderAccent = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	finderError  = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	finderFrame  = lipgloss.NewStyle().PaddingLeft(2)
)

type finderStage int

const (
	pickCategory finderStage = iota
	pickBook
)

// row is what both lists show: a name and a fainter detail on the left, a note on the right.
type row struct {
	category Category
	entry    Entry
	name     string
	detail   string
	note     string
}

func (r row) FilterValue() string { return r.name }

// rowDelegate renders one line per row, the cursor's in the accent colour.
type rowDelegate struct{ width int }

func (d rowDelegate) Height() int                         { return 1 }
func (d rowDelegate) Spacing() int                        { return 0 }
func (d rowDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }
func (d rowDelegate) Render(w io.Writer, m list.Model, i int, item list.Item) {
	r := item.(row)
	cursor, nameStyle := "  ", lipgloss.NewStyle()
	if i == m.Index() {
		cursor, nameStyle = finderAccent.Render("> "), finderAccent
	}
	note := finderFaint.Render(r.note)
	room := d.width - lipgloss.Width(cursor) - lipgloss.Width(note) - 1
	name, detail := r.fit(room) // cut as plain text, then coloured: a cut inside an escape code bleeds
	left := nameStyle.Render(name) + finderFaint.Render(detail)
	gap := max(1, room-lipgloss.Width(left))
	fmt.Fprint(w, cursor+left+strings.Repeat(" ", gap)+note)
}

// fit shortens "name detail" to room characters and hands the two parts back.
func (r row) fit(room int) (name, detail string) {
	plain := r.name
	if r.detail != "" {
		plain += " " + r.detail
	}
	cut := []rune(shorten(plain, room))
	if n := len([]rune(r.name)); len(cut) > n {
		return string(cut[:n]), string(cut[n:])
	}
	return string(cut), ""
}

type finderKeys struct{ up, down, choose, back key.Binding }

var finderKeyMap = finderKeys{
	up:     key.NewBinding(key.WithKeys("up"), key.WithHelp("↑↓", "move")),
	down:   key.NewBinding(key.WithKeys("down")),
	choose: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "choose")),
	back:   key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
}

type Finder struct {
	stage    finderStage
	catalog  *Catalog
	category Category
	owned    map[int]bool
	input    textinput.Model
	list     list.Model
	spin     spinner.Model
	help     help.Model
	busy     string // what is being waited on; "" = nothing
	err      error
	added    string // the last title that joined the collection
}

type catalogMsg struct {
	catalog *Catalog
	err     error
}

type importedMsg struct {
	entry Entry
	err   error
}

var loadedCatalog *Catalog // one per process

func initFinder() (Finder, tea.Cmd) {
	f := Finder{owned: ownedGutenbergIDs(), input: newFinderInput(), list: newFinderList(), spin: spinner.New(spinner.WithSpinner(spinner.Dot)), help: help.New()}
	if loadedCatalog != nil {
		f.setCatalog(loadedCatalog)
		return f, nil
	}
	f.busy = "reading the catalog"
	if _, err := readCatalog(); err != nil {
		f.busy = "downloading Gutenberg's catalog, 21 MB, once a month"
	}
	return f, tea.Batch(f.spin.Tick, loadCatalog)
}

func loadCatalog() tea.Msg {
	c, err := LoadCatalog()
	return catalogMsg{c, err}
}

func ownedGutenbergIDs() map[int]bool {
	owned := map[int]bool{}
	for _, b := range Books() {
		if id, err := strconv.Atoi(b.Source); err == nil {
			owned[id] = true
		}
	}
	return owned
}

func newFinderInput() textinput.Model {
	in := textinput.New()
	in.Prompt = "/ "
	in.PromptStyle, in.Cursor.Style = finderAccent, finderAccent
	in.PlaceholderStyle = finderFaint
	in.Focus()
	return in
}

func newFinderList() list.Model {
	l := list.New(nil, rowDelegate{width: finderWidth}, finderWidth, 10)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetShowPagination(false)
	l.SetFilteringEnabled(false)
	l.DisableQuitKeybindings()
	l.KeyMap = list.KeyMap{CursorUp: finderKeyMap.up, CursorDown: finderKeyMap.down, // the letters go to the search box
		NextPage: key.NewBinding(key.WithKeys("pgdown", "right")), PrevPage: key.NewBinding(key.WithKeys("pgup", "left"))}
	return l
}

func (f *Finder) setCatalog(c *Catalog) {
	loadedCatalog, f.catalog, f.busy = c, c, ""
	f.showCategories()
}

func (f *Finder) showCategories() {
	f.stage = pickCategory
	f.input.SetValue("")
	f.input.Placeholder = "type to narrow the categories"
	f.fill()
}

func (f *Finder) showBooks(c Category) {
	f.stage, f.category = pickBook, c
	f.input.SetValue("")
	f.input.Placeholder = "type a title or an author"
	f.fill()
}

// fill puts the rows that match the search box into the list.
func (f *Finder) fill() {
	var rows []list.Item
	if f.stage == pickCategory {
		q := strings.ToLower(f.input.Value())
		for i := range f.catalog.categoryCount() {
			c := Category(i)
			if name := f.catalog.categoryName(c); strings.Contains(strings.ToLower(name), q) {
				rows = append(rows, row{category: c, name: name, note: strconv.Itoa(f.catalog.count(c))})
			}
		}
	} else {
		for _, e := range f.catalog.search(f.category, f.input.Value(), finderRows) {
			rows = append(rows, row{entry: e, name: e.Title, detail: e.Author, note: f.bookNote(e)})
		}
	}
	f.list.SetItems(rows)
	f.list.Select(0)
}

func (f Finder) bookNote(e Entry) string {
	switch {
	case f.owned[e.ID]:
		return "yours"
	case e.Rank > 0 && f.category != catPopular:
		return "top " + strconv.Itoa((e.Rank+99)/100*100)
	}
	return ""
}

func (f Finder) handle(msg tea.Msg) (State, tea.Cmd) {
	switch msg := msg.(type) {
	case catalogMsg:
		if msg.err != nil {
			f.busy, f.err = "", fmt.Errorf("catalog: %w", msg.err)
			return f, nil
		}
		f.setCatalog(msg.catalog)
		return f, nil
	case importedMsg:
		f.busy, f.err = "", msg.err
		if msg.err == nil {
			f.owned[msg.entry.ID], f.added = true, msg.entry.Title
			f.fill()
		}
		return f, nil
	case spinner.TickMsg:
		if f.busy == "" {
			return f, nil
		}
		var cmd tea.Cmd
		f.spin, cmd = f.spin.Update(msg)
		return f, cmd
	case tea.KeyMsg:
		return f.handleKey(msg)
	}
	return f, nil
}

func (f Finder) handleKey(msg tea.KeyMsg) (State, tea.Cmd) {
	if f.catalog == nil || f.busy != "" {
		return f, nil
	}
	f.err, f.added = nil, ""
	switch {
	case key.Matches(msg, finderKeyMap.choose):
		return f.choose()
	case key.Matches(msg, f.list.KeyMap.CursorUp, f.list.KeyMap.CursorDown, f.list.KeyMap.NextPage, f.list.KeyMap.PrevPage):
		var cmd tea.Cmd
		f.list, cmd = f.list.Update(msg)
		return f, cmd
	}
	before := f.input.Value()
	var cmd tea.Cmd
	f.input, cmd = f.input.Update(msg)
	if f.input.Value() != before {
		f.fill()
	}
	return f, cmd
}

// back is what esc does: a step up, or out.
func (f Finder) back() (State, bool) {
	if f.stage == pickBook {
		f.showCategories()
		return f, true
	}
	return f, false
}

func (f Finder) choose() (State, tea.Cmd) {
	r, ok := f.list.SelectedItem().(row)
	if !ok {
		return f, nil
	}
	if f.stage == pickCategory {
		f.showBooks(r.category)
		return f, nil
	}
	if f.owned[r.entry.ID] {
		f.added = r.entry.Title
		return f, nil
	}
	f.busy = "adding " + r.entry.Title
	return f, tea.Batch(f.spin.Tick, func() tea.Msg {
		_, err := ImportBook(strconv.Itoa(r.entry.ID), r.entry.Title, "")
		return importedMsg{r.entry, err}
	})
}

func (m model) finderView(f Finder) string {
	width := min(m.width-4, finderWidth)
	body := []string{f.headerLine(width), f.input.View()}
	if status := f.statusLine(width); status != "" {
		body = append(body, "", status)
	} else if f.catalog != nil {
		l := f.list
		l.SetSize(width, max(1, m.height-len(body)-2))
		l.SetDelegate(rowDelegate{width: width})
		body = append(body, l.View())
	}
	page := lipgloss.JoinVertical(lipgloss.Left, strings.Join(body, "\n"), "", f.help.ShortHelpView(f.helpKeys()))
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Top, finderFrame.Width(width+2).Render(page))
}

func (f Finder) headerLine(width int) string {
	head := finderTitle.Render("find a book")
	switch {
	case f.stage == pickBook:
		head += finderFaint.Render("  " + shorten(f.catalog.categoryName(f.category), width-14))
	case f.catalog != nil:
		head += finderFaint.Render(fmt.Sprintf("  %d books", len(f.catalog.Entries)))
	}
	return head
}

func (f Finder) statusLine(width int) string {
	switch {
	case f.busy != "":
		return f.spin.View() + finderFaint.Render(shorten(f.busy, width-4))
	case f.err != nil:
		return finderError.Render(shorten(f.err.Error(), width)) + "\n" + finderFaint.Render("enter to try again")
	case f.added != "":
		return finderAccent.Render("in your books: ") + shorten(f.added, width-16) + "\n" + finderFaint.Render("esc esc, then enter on it to type")
	}
	return ""
}

func (f Finder) helpKeys() []key.Binding {
	choose := finderKeyMap.choose
	choose.SetHelp("enter", map[finderStage]string{pickCategory: "open", pickBook: "add"}[f.stage])
	back := finderKeyMap.back
	back.SetHelp("esc", map[finderStage]string{pickCategory: "home", pickBook: "categories"}[f.stage])
	return []key.Binding{finderKeyMap.up, choose, back}
}
