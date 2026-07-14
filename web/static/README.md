# Generated JavaScript

TypeScript is the source of truth for frontend code. The js files in this dir
are transpiled from it by `tsgo`, they are **not** checked in, and you should
not edit them directly.

Instead, make your changes to the TypeScript in

```
web/typescript
```

then regenerate:

```shell
make generate
```

`make build`, `make run/live` and the Docker build all run the generators too,
as does CI, so there's nothing to check in. Note that `tsgo` is also the
TypeScript *type checker*, so a type error in `web/typescript` fails the build.

The hand-written assets here (`style.css`, `logos/`) are checked in as normal.
The third-party client libraries under `ext/` aren't checked in either; they're
fetched by `bin/fetchbuilddeps`.
