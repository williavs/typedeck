<h1 align=center> typedeck </h1>

<p align=center> A typing trainer for one typist, forked from <a href="https://github.com/bloznelis/typioca">typioca</a>. </p>

## Install

```
go install github.com/williavs/typedeck@latest
```

Installs to `$GOBIN` (`$HOME/go/bin` by default). Or from a checkout: `go build -o typedeck .`

Data lives in `~/.local/share/typedeck/` (`$XDG_DATA_HOME/typedeck`). Copy that directory to carry your
progress to another machine.

## What it is

A fork of typioca for one typist who wants to get better over months, built to run on a 53 x 15 terminal
(Pi Zero 2 W + 2.2" PiTFT) as well as a desktop. Same look, same keys. What changed:

- **A home screen of its own.** One key continues the book at the bookmark. Below it: a 30 s drill on your weak
  spots (named on the row), symbols, common words, your trend, and typioca's own menu under More.
  The header counts your days in a row and the minutes typed today.
- **It remembers.** Every key of every run goes to `keys.csv` (expected, typed, milliseconds since the last key);
  `coach.json` keeps a fading tally per key and per letter pair; `runs.jsonl` lists every run.
  All of it lives in `~/.local/share/typedeck/` - upstream kept its history in `~/.cache`.
- **Weak spots.** Every result names the worst keys and pairs with the reason (share missed, or how slow).
  The `Weak spots` word list deals common words full of them. `Code symbols` drills digits and punctuation.
- **Books.** `typedeck import <file | url | gutenberg number> [--start "first words"] [--title T]` cleans any plain
  text into a book. It appears in the timer run's word lists, every run starts at the bookmark, `typedeck books`
  and the Progress screen show how far you are.
- **Nothing is lost.** Esc backs out one level instead of quitting the program; stopping a timer run early keeps
  the result, the keys and the bookmark. Files are written beside the target and renamed over it.
- **Bugs fixed:** keys arriving in one read were dropped except the last; typing past the end of a timer run's
  text panicked; the config was rewritten to disk on every message in the menu; WPM was rounded down by integer
  division; a config path collision (`XDG_CACHE_HOME == XDG_CONFIG_HOME`) panicked at start.
- **Speed:** a frame drew all 300 words to show three lines. On the Pi Zero 2 W: 17.5 ms before, 0.41 ms after
  (`go test ./cmd -bench Frame`).

Tests: `go test ./...`. Pi build: `GOOS=linux GOARCH=arm GOARM=7 go build -o execs/typedeck-armv7 .`

## Custom wordlists
1. Create your word list in a new line separated manner:
```
custom
words
are
the
best
```
or in the same JSON format as the official ones:
```json
{
  "metadata" : {
    "name" : "My words",
    "size" : 5,
    "packagedAt" : "1970-01-01T00:00:00Z",
    "version" : 1
  },
  "words": [ "custom", "words", "are", "the", "best" ]
}
```
2. Place your configuration to platform specific location:

| Platform | **User configuration**                                                                     |
|----------|--------------------------------------------------------------------------------------------|
| Windows  | `%APPDATA%\typedeck\typedeck.conf` or `C:\Users\%USER%\AppData\Roaming\typedeck\typedeck.conf` |
| Linux    | `$XDG_CONFIG_HOME/typedeck/typedeck.conf` or `$HOME/.config/typedeck/typedeck.conf`            |
| macOS    | `$HOME/Library/Application Support/typedeck/typedeck.conf`                                   |

Config example (it is [TOML](https://github.com/toml-lang/toml)):
```toml
[[words]]
  name      = "Best hits '22"
  enabled   = false
  sentences = false
  path      = "/home/words/best-hits-22.json"
[[words]]
  name      = "Even better hits '23"
  enabled   = true
  sentences = false
  path      = "/home/words/better-hits-23.json"
```
3. Use your words!
![ship it](https://user-images.githubusercontent.com/33397865/176735281-5c2b34cb-5b19-43c1-9954-92c0583c4cc5.png)

**Note:** Notice that custom wordlist controls are greyed-out, personal configuration must be handled via the file only.


### Acknowledgments
typioca by [Lukas Bloznelis](https://github.com/bloznelis/typioca), MIT. Built with
[bubbletea](https://github.com/charmbracelet/bubbletea).
