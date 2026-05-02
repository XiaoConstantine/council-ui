//go:build libghostty

package termview

import libghostty "github.com/mitchellh/go-libghostty"

type libghosttyRenderer struct{}

func New() Renderer {
	return libghosttyRenderer{}
}

func (libghosttyRenderer) Name() string {
	return "go-libghostty"
}

func (libghosttyRenderer) Render(input string, cols int, rows int) (string, error) {
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}

	term, err := libghostty.NewTerminal(libghostty.WithSize(uint16(cols), uint16(rows)))
	if err != nil {
		return "", err
	}
	defer term.Close()

	term.VTWrite([]byte(input))

	formatter, err := libghostty.NewFormatter(
		term,
		libghostty.WithFormatterFormat(libghostty.FormatterFormatPlain),
		libghostty.WithFormatterTrim(false),
	)
	if err != nil {
		return "", err
	}
	defer formatter.Close()

	return formatter.FormatString()
}
