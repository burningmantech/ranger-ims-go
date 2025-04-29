package template

import (
	"os"
	"testing"
)

func TestRoot(t *testing.T) {
	Root("Dev").Render(t.Context(), os.Stdout)
}
