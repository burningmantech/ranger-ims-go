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
	"regexp"
	"strconv"
	"strings"
)

// Patterns for the pieces of a Black Rock City address. All are used within
// case-insensitive expressions.
const (
	// e.g. "7", "7:00", "0:15", or colonless "813" and "1015".
	radialPat = `(?:(\d{1,2})(?::(\d{1,2}))?|(\d{3,4}))`
	// e.g. "E", "esp", "Esplanade".
	concentricPat = `([A-L]|ESPLANADE|ESPL|ESP)\.?`
	// distance from the Man, e.g. "2500'", "2500 ft", "500".
	distancePat = `(\d{1,5})\s*('|’|FT\.?|FEET)?`
	// e.g. "&", " + ", ", ", " and ".
	separatorPat = `(?:\s*[&+/,]\s*|\s+AND\s+|\s+)`
)

var (
	radialConcentricRE = regexp.MustCompile(`(?i)^` + radialPat + separatorPat + concentricPat + `$`)
	concentricRadialRE = regexp.MustCompile(`(?i)^` + concentricPat + separatorPat + radialPat + `$`)
	radialDistanceRE   = regexp.MustCompile(`(?i)^` + radialPat + separatorPat + distancePat + `$`)
)

// NormalizeAddress rewrites a Black Rock City address into its canonical form:
// "7:00 & E" for a radial/concentric intersection (in whichever order the
// author wrote it), or "12:00 2500'" for an open-playa address given as a
// radial and a distance from the Man. Anything that doesn't parse with
// confidence, such as "behind the porta potties", is returned unchanged.
func NormalizeAddress(address string) string {
	trimmed := strings.TrimSpace(address)

	if m := radialConcentricRE.FindStringSubmatch(trimmed); m != nil {
		if radial, ok := formatRadial(m[1], m[2], m[3]); ok {
			return radial + " & " + formatConcentric(m[4])
		}
	}
	if m := concentricRadialRE.FindStringSubmatch(trimmed); m != nil {
		if radial, ok := formatRadial(m[2], m[3], m[4]); ok {
			return formatConcentric(m[1]) + " & " + radial
		}
	}
	if m := radialDistanceRE.FindStringSubmatch(trimmed); m != nil {
		radial, radialOK := formatRadial(m[1], m[2], m[3])
		distance, distanceOK := formatDistance(m[4], m[5])
		if radialOK && distanceOK {
			return radial + " " + distance
		}
	}

	return address
}

// formatRadial takes either an hour and minute or a colonless radial such as
// "813", exactly one of which the radial pattern captures.
func formatRadial(hour, minute, colonless string) (string, bool) {
	if colonless != "" {
		hour, minute = colonless[:len(colonless)-2], colonless[len(colonless)-2:]
	}
	h, err := strconv.Atoi(hour)
	if err != nil || h > 12 {
		return "", false
	}
	m := 0
	if minute != "" {
		m, err = strconv.Atoi(minute)
		if err != nil || m > 59 {
			return "", false
		}
	}
	return fmt.Sprintf("%d:%02d", h, m), true
}

func formatConcentric(street string) string {
	if len(street) == 1 {
		return strings.ToUpper(street)
	}
	return "Esplanade"
}

func formatDistance(feet, unit string) (string, bool) {
	// Without a unit, a short number is more likely a typo than a distance,
	// so leave those alone rather than guessing.
	if unit == "" && len(feet) < 3 {
		return "", false
	}
	f, err := strconv.Atoi(feet)
	if err != nil || f == 0 {
		return "", false
	}
	return strconv.Itoa(f) + "'", true
}
