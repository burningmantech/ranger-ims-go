//
// See the file COPYRIGHT for copyright information.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

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
