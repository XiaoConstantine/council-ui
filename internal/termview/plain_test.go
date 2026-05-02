package termview

import "testing"

func TestRenderUsesAvailableRenderer(t *testing.T) {
	output, name, err := Render("hello", 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	if output != "hello" {
		t.Fatalf("output = %q, want hello", output)
	}
	if name == "" {
		t.Fatal("renderer name is empty")
	}
}
