package tui

import (
	"context"
	"errors"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/developerasaad/winprep/internal/app"
)

// ErrNoTerminal is reported when the console cannot host an interactive
// interface at all.
var ErrNoTerminal = errors.New("no interactive terminal is attached")

// Run starts the interactive interface on the given console. It returns an
// error only when the terminal cannot be driven; normal exits are nil.
func Run(a *app.App, in *os.File, out *os.File) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := NewModel(a, ctx)
	p := tea.NewProgram(m,
		tea.WithContext(ctx),
		tea.WithInput(in),
		tea.WithOutput(out),
	)
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("starting the interactive interface: %w", err)
	}
	return nil
}
