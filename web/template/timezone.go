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

package template

import "context"

type timeZoneKey struct{}

// WithTimeZone sets the IANA time zone that rendered pages tell the web UI
// to show and take dates and times in.
func WithTimeZone(ctx context.Context, timeZone string) context.Context {
	return context.WithValue(ctx, timeZoneKey{}, timeZone)
}

func timeZone(ctx context.Context) string {
	tz, _ := ctx.Value(timeZoneKey{}).(string)
	return tz
}
