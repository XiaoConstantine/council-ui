//go:build !libghostty

package termview

type plainRenderer struct{}

func New() Renderer {
	return plainRenderer{}
}

func (plainRenderer) Name() string {
	return "plain"
}

func (plainRenderer) Render(input string, _ int, _ int) (string, error) {
	return input, nil
}
