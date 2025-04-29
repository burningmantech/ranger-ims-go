package web

import "embed"

// Are you hitting a compilation error here, because one of the
// files below cannot be found?
//
// Please run `bin/fetch_client_deps.sh`, as you need to have
// these files loaded in your filesystem in order to compile.

//go:embed static
//go:embed static/ext/bootstrap.min.css
//go:embed static/ext/bootstrap.bundle.min.js
//go:embed static/ext/jquery-3.1.0.min.js
//go:embed static/ext/dataTables.min.js
//go:embed static/ext/dataTables.bootstrap5.min.js
//go:embed static/ext/dataTables.bootstrap5.min.css
var StaticFS embed.FS
