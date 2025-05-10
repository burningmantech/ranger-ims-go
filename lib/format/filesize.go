package format

import (
	"fmt"
)

const (
	kib = 1 << 10
	mib = 1 << 20
	gib = 1 << 30
	tib = 1 << 40
	pib = 1 << 50
	eib = 1 << 60
)

func HumanByteSize(numBytes int64) string {
	if numBytes < 0 {
		return "invalid"
	}
	if numBytes < kib {
		return fmt.Sprintf("%d B", numBytes)
	}
	if numBytes < mib {
		return fmt.Sprintf("%.3g KiB", float64(numBytes)/float64(kib))
	}
	if numBytes < gib {
		return fmt.Sprintf("%.3g MiB", float64(numBytes)/float64(mib))
	}
	if numBytes < tib {
		return fmt.Sprintf("%.3g GiB", float64(numBytes)/float64(gib))
	}
	if numBytes < pib {
		return fmt.Sprintf("%.3g TiB", float64(numBytes)/float64(tib))
	}
	if numBytes < eib {
		return fmt.Sprintf("%.3g PiB", float64(numBytes)/float64(pib))
	}
	return fmt.Sprintf("%.3g EiB", float64(numBytes)/float64(eib))
}
