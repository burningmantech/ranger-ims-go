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

package web

import "embed"

// Are you hitting a compilation error here, because one of the
// files below cannot be found?
//
// Please run `go run bin/fetchbuilddeps/fetchbuilddeps.go`,
// as you need to have these files loaded in your filesystem in
// order to compile.

//go:embed static
//go:embed static/ext/bootstrap.min.css
//go:embed static/ext/bootstrap.bundle.min.js
//go:embed static/ext/jquery.min.js
//go:embed static/ext/dataTables.min.js
//go:embed static/ext/dataTables.bootstrap5.min.js
//go:embed static/ext/dataTables.bootstrap5.min.css
//go:embed static/ext/flatpickr.min.css
//go:embed static/ext/flatpickr.min.js
var StaticFS embed.FS
