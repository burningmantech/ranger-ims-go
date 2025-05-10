package format_test

import (
	"github.com/burningmantech/ranger-ims-go/lib/format"
	"github.com/stretchr/testify/assert"
	"math"
	"testing"
)

const ki = 1024

func TestHumanByteSize(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "invalid", format.HumanByteSize(-100))
	assert.Equal(t, "0 B", format.HumanByteSize(0))
	assert.Equal(t, "2 B", format.HumanByteSize(2))
	assert.Equal(t, "1.95 KiB", format.HumanByteSize(2_000))
	assert.Equal(t, "1.91 MiB", format.HumanByteSize(2_000_000))
	assert.Equal(t, "1.86 GiB", format.HumanByteSize(2_000_000_000))
	assert.Equal(t, "1.82 TiB", format.HumanByteSize(2_000_000_000_000))
	assert.Equal(t, "1.78 PiB", format.HumanByteSize(2_000_000_000_000_000))
	assert.Equal(t, "1.73 EiB", format.HumanByteSize(2_000_000_000_000_000_000))

	assert.Equal(t, "567 B", format.HumanByteSize(567))
	assert.Equal(t, "567 KiB", format.HumanByteSize(567*ki))
	assert.Equal(t, "567 MiB", format.HumanByteSize(567*ki*ki))
	assert.Equal(t, "567 GiB", format.HumanByteSize(567*ki*ki*ki))
	assert.Equal(t, "567 TiB", format.HumanByteSize(567*ki*ki*ki*ki))
	assert.Equal(t, "567 PiB", format.HumanByteSize(567*ki*ki*ki*ki*ki))
	assert.Equal(t, "8 EiB", format.HumanByteSize(math.MaxInt64))
}
