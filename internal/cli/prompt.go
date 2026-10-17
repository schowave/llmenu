package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
)

// errInputEnded stops a dialog in accessible mode when stdin ends before all
// questions are answered; huh would otherwise take the defaults, and a retry
// loop would never end.
var errInputEnded = errors.New("input ended before all questions were answered")

// errBack: Esc left a dialog. It counts as an abort everywhere; only the
// setup list tells the two apart and returns to itself on Esc.
var errBack = fmt.Errorf("back: %w", huh.ErrUserAborted)

// accessible reports whether LLMENU_ACCESSIBLE is set (1, true). Then all
// dialogs run in huh's accessible mode: plain prompts, numbered options and
// one answer per line instead of a redrawn full-screen form. That is meant
// for screen readers, and it makes the dialogs scriptable, also without a
// terminal (answers on stdin, prompts on stderr).
func accessible() bool {
	on, _ := strconv.ParseBool(os.Getenv("LLMENU_ACCESSIBLE"))
	return on
}

// canPrompt reports whether a dialog may run: on a terminal, or when
// accessible mode was chosen explicitly. Otherwise scripts would hang on a
// prompt (ADR-0003), so commands abort instead.
func canPrompt() bool {
	return accessible() || isTerminal(os.Stdin)
}

// prompt is a huh field or a form, see asForm.
type prompt interface {
	Run() error
	RunAccessible(w io.Writer, r io.Reader) error
}

// runPrompt shows a dialog, in accessible mode line by line.
func runPrompt(p prompt) error {
	if !accessible() {
		if form := formOf(p); form != nil {
			return runForm(form)
		}
		return p.Run()
	}
	in := &lineReader{r: os.Stdin}
	var r io.Reader = in
	if isTerminal(os.Stdin) {
		// Hidden input reads the terminal directly and needs its descriptor.
		r = ttyLineReader{in}
	}
	err := p.RunAccessible(os.Stderr, r)
	// Separates the questions; answers from a pipe are not echoed.
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return err
	}
	if in.ended {
		return errInputEnded
	}
	return nil
}

// formOf returns the form a prompt runs in; a single field gets its own, as
// in huh.Run.
func formOf(p prompt) *huh.Form {
	switch p := p.(type) {
	case asForm:
		return p.Form
	case huh.Field:
		return huh.NewForm(huh.NewGroup(p)).WithShowHelp(false)
	}
	return nil
}

// runForm runs a form like huh's Form.Run, with Esc as errBack.
func runForm(form *huh.Form) error {
	form.SubmitCmd = tea.Quit
	form.CancelCmd = tea.Interrupt
	m, err := tea.NewProgram(&escForm{Form: form}, tea.WithOutput(os.Stderr), tea.WithReportFocus()).Run()
	switch {
	case m.(*escForm).back:
		return errBack
	case errors.Is(err, tea.ErrInterrupted) || form.State == huh.StateAborted:
		return huh.ErrUserAborted
	case err != nil:
		return fmt.Errorf("huh: %w", err)
	}
	return nil
}

// escForm leaves the form on Esc, unless the focused field uses Esc itself,
// e.g. to end filtering. Unlike Ctrl+C it ends the program cleanly, so the
// form disappears before the next dialog.
type escForm struct {
	*huh.Form
	back bool
}

func (m *escForm) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok && k.Type == tea.KeyEsc &&
		!key.Matches(k, m.GetFocusedField().KeyBinds()...) {
		m.back = true
		return m, tea.Quit
	}
	f, cmd := m.Form.Update(msg)
	m.Form = f.(*huh.Form)
	return m, cmd
}

func (m *escForm) View() string {
	if m.back {
		return ""
	}
	return m.Form.View()
}

// asForm lets runPrompt run a whole form.
type asForm struct{ *huh.Form }

func (f asForm) RunAccessible(w io.Writer, r io.Reader) error {
	return f.WithAccessible(true).WithOutput(w).WithInput(r).Run()
}

// secretEcho hides typed keys. In accessible mode without a terminal the key
// comes from a pipe, which nobody sees and which huh cannot read hidden.
func secretEcho() huh.EchoMode {
	if accessible() && !isTerminal(os.Stdin) {
		return huh.EchoModeNormal
	}
	return huh.EchoModePassword
}

// enterKeeps adapts a validator to accessible mode: there, Enter submits an
// empty line, which huh validates before replacing it with the current value.
func enterKeeps(current *string, validate func(string) error) func(string) error {
	return func(s string) error {
		if s == "" && accessible() {
			s = *current
		}
		return validate(s)
	}
}

// lineReader hands stdin to huh one byte per Read. huh starts a new
// bufio.Scanner for every question; reading more than one line at a time
// would swallow the answers to the following questions.
type lineReader struct {
	r       io.Reader
	partial bool // bytes read since the last newline
	ended   bool // stdin ended while a question waited for an answer
}

func (l *lineReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	n, err := l.r.Read(p[:1])
	if n == 1 {
		l.partial = p[0] != '\n'
	}
	if errors.Is(err, io.EOF) {
		// A last line without a newline is still an answer.
		if !l.partial {
			l.ended = true
		}
		l.partial = false
	}
	return n, err
}

type ttyLineReader struct{ *lineReader }

func (ttyLineReader) Fd() uintptr { return os.Stdin.Fd() }

// titled puts the description under the title in accessible mode, where huh
// shows only the title of an input or confirmation.
func titled(title, description string) string {
	if accessible() {
		return title + "\n" + description
	}
	return title
}
