package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	bm "github.com/charmbracelet/wish/bubbletea"
	lm "github.com/charmbracelet/wish/logging"
	"github.com/muesli/termenv"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	serverBind    = ""
	serverPort    = 2229
	serverKeyPath = ""
	showVersion   = false
)

var (
	Version = "dev"
	RootCmd = &cobra.Command{
		Use:  "typedeck",
		Long: "typedeck — typioca with a memory: it keeps every key and drills your weak spots",
		RunE: func(cmd *cobra.Command, args []string) error {
			if showVersion {
				fmt.Println("typedeck ", Version)
				return nil
			} else {
				termenv.SetWindowTitle("typedeck")
				defer println("bye!")

				termWidth, termHeight, _ := term.GetSize(int(os.Stdin.Fd()))
				p := tea.NewProgram(
					initialModel(
						termenv.ColorProfile(),
						termenv.ForegroundColor(),
						termWidth,
						termHeight,
					),
					tea.WithAltScreen(),
				)

				_, err := p.Run()
				return err
			}
		},
	}
	serveCmd = &cobra.Command{
		Use:   "serve",
		Short: "Serve the typioca server",
		Long:  "serve starts the typioca SSH server.",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := wish.NewServer(
				wish.WithAddress(fmt.Sprintf("%s:%d", serverBind, serverPort)),
				wish.WithHostKeyPath(serverKeyPath),
				wish.WithMiddleware(
					lm.Middleware(),
					bm.Middleware(
						func(s ssh.Session) (tea.Model, []tea.ProgramOption) {
							pty, _, active := s.Pty()

							if !active {
								wish.Fatal(s, fmt.Errorf("not a tty"))
								return nil, nil
							}

							return initialModel(
									termenv.ANSI256,
									termenv.ANSIWhite,
									pty.Window.Width,
									pty.Window.Height,
								),
								[]tea.ProgramOption{tea.WithAltScreen()}
						}),
				),
			)

			if err != nil {
				return err
			}

			done := make(chan os.Signal, 1)
			signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

			log.Printf("Starting server on %s:%d", serverBind, serverPort)
			go func() {
				if err := s.ListenAndServe(); err != nil {
					log.Fatalln(err)
				}
			}()

			<-done

			log.Printf("Stopping SSH server on %s:%d", serverBind, serverPort)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer func() { cancel() }()
			if err := s.Shutdown(ctx); err != nil {
				return err
			}

			return nil
		},
	}
)

var importTitle, importStart = "", ""

var importCmd = &cobra.Command{
	Use:   "import <file | url | gutenberg number>...",
	Short: "Add a book to type through",
	Long:  "import cleans plain text (a file, a URL, or a Project Gutenberg ebook number) into a book.\nIt shows up in the timer run's word lists; every run starts at the bookmark.",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		for _, src := range args {
			book, err := ImportBook(src, importTitle, importStart)
			if err != nil {
				return err
			}
			fmt.Printf("%s (%s): %d characters, %d pages\n", book.Title, book.Author, book.Chars, book.Chars/bookPage+1)
		}
		return nil
	},
}

var booksCmd = &cobra.Command{
	Use:   "books",
	Short: "List the books and how far you are",
	Run: func(cmd *cobra.Command, args []string) {
		for _, b := range Books() {
			fmt.Println(b.show())
		}
	},
}

var catalogCmd = &cobra.Command{
	Use:   "catalog",
	Short: "Download Gutenberg's catalog now (the finder does it by itself once a month)",
	RunE: func(cmd *cobra.Command, args []string) error {
		cat, err := FetchCatalog()
		if err != nil {
			return err
		}
		fmt.Printf("%d English books, %d categories, %d ranked\n", len(cat.Entries), len(cat.Cats), cat.count(catPopular))
		return nil
	},
}

func init() {
	importCmd.Flags().StringVarP(&importTitle, "title", "t", "", "title, when the text does not carry one")
	importCmd.Flags().StringVarP(&importStart, "start", "s", "", "a phrase of the text: the book begins there, front matter is left out")
	RootCmd.AddCommand(importCmd, booksCmd, catalogCmd)
	serveCmd.Flags().StringVarP(&serverKeyPath, "key", "k", "typioca", "path to the server key")
	serveCmd.Flags().StringVarP(&serverBind, "bind", "b", "", "address to bind on")
	serveCmd.Flags().IntVarP(&serverPort, "port", "p", 2229, "port to serve on")
	RootCmd.Flags().BoolVarP(&showVersion, "version", "v", false, "show typioca version")
	RootCmd.AddCommand(serveCmd)
}
