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

// Package template holds the web UI's HTML templates. The .templ files here are
// the source of truth; templ compiles each one to a matching _templ.go.
//
// Are you hitting compilation errors here, or seeing "undefined" errors for
// things this package is supposed to provide? None of the generated code in
// this repo is checked in, so you need to produce it first:
//
//	make generate
//
// This file and the .templ files are the only hand-written ones in this package.
// It exists so that the package resolves even before the generators have run.
// Without it, Go reports the missing directory as a missing *module* and
// suggests a `go get` that cannot work.
package template
