package termview

type Renderer interface {
	Name() string
	Render(input string, cols int, rows int) (string, error)
}

func Render(input string, cols int, rows int) (string, string, error) {
	renderer := New()
	output, err := renderer.Render(input, cols, rows)
	if err != nil {
		return input, renderer.Name(), err
	}
	return output, renderer.Name(), nil
}
