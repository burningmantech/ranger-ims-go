package api

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestExtensionByType(t *testing.T) {
	t.Parallel()
	assert.Equal(t, ".jpg", extensionByType("image/jpg"))
	assert.Equal(t, ".png", extensionByType("image/png"))
	assert.Equal(t, ".txt", extensionByType("text/plain"))
	assert.Equal(t, ".htm", extensionByType("text/html"))
	assert.Equal(t, "", extensionByType("notta/mime"))
}
