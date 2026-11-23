// Command winprep is the WinPrep executable. Started without arguments it
// opens the interactive menu; with arguments it behaves as a conventional
// command-line tool. Both interfaces run the same application services.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/developerasaad/winprep/internal/app"
	"github.com/developerasaad/winprep/internal/cli"
	"github.com/developerasaad/winprep/internal/tui"
)

func main() {
	a := app.New()
	ctx := context.Background()

	args := os.Args[1:]
	if len(args) > 0 {
		io := cli.IO{
			Out:         os.Stdout,
			Err:         os.Stderr,
			In:          os.Stdin,
			Interactive: isTerminal(os.Stdin),
		}
		os.Exit(cli.Run(ctx, a, io, args))
	}

	// No arguments: the interactive interface. It needs a real console on
	// both ends; anything else (a pipe, a service context) gets a clear
	// message and the CLI instead of a broken menu.
	if !isTerminal(os.Stdin) || !isTerminal(os.Stdout) {
		fmt.Fprintln(os.Stderr, "WinPrep needs an interactive terminal for its menu.")
		fmt.Fprintln(os.Stderr, "Use the commands instead: winprep status, winprep persist, winprep check,")
		fmt.Fprintln(os.Stderr, "winprep clean. Run \"winprep help\" for details.")
		os.Exit(cli.ExitUsage)
	}

	if err := tui.Run(a, os.Stdin, os.Stdout); err != nil {
		// The console is interactive but cannot host the full-screen
		// interface (unusual terminal, restricted escape-sequence support).
		// The numbered fallback menu covers that case.
		fmt.Fprintln(os.Stderr, "The full-screen interface could not start:", err)
		fmt.Fprintln(os.Stderr, "Continuing with the simplified menu.")
		fmt.Fprintln(os.Stderr)
		os.Exit(tui.RunFallback(ctx, a, os.Stdin, os.Stdout))
	}
}

// isTerminal reports whether f is attached to a character device (a console).
func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
