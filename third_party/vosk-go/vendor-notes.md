# third_party/vosk-go

Local copy of [`github.com/alphacep/vosk-api/go`](https://github.com/alphacep/vosk-api)
**v0.3.50**, wired into the build via a `replace` directive in the root `go.mod`:

```
replace github.com/alphacep/vosk-api/go => ./third_party/vosk-go
```

## What changed vs upstream

Only the three `#cgo` lines in `vosk.go`. Upstream points them at
`${SRCDIR}/../src` (a directory that is not shipped inside the Go module). This
copy points them at the repo's own `vosk-lib/` directory:

```
// #cgo CPPFLAGS: -I ${SRCDIR}/../../vosk-lib
// #cgo !windows LDFLAGS: -L ${SRCDIR}/../../vosk-lib -lvosk -ldl -lpthread
// #cgo windows LDFLAGS: -L ${SRCDIR}/../../vosk-lib -lvosk -lpthread
```

`${SRCDIR}` is `third_party/vosk-go`, so `${SRCDIR}/../../vosk-lib` resolves to
`<repo-root>/vosk-lib`. This is machine-independent as long as `vosk-lib/` (the
unzipped `vosk-win64-*.zip`, containing `vosk_api.h` + `libvosk.lib` +
`libvosk.dll`) lives at the repo root.

## Why not just set CGO_CPPFLAGS / CGO_LDFLAGS?

Because those environment variables are applied to **every** cgo package the
build compiles — including Go's own `runtime/cgo`, which is built with
`-Wall -Werror`. The leaked `-I ...\vosk-lib` turns a warning into a fatal error,
so the build dies with:

```
runtime/cgo: ...\cgo.exe: exit status 2
```

before it ever reaches the Vosk code. (`go build -x` confirms the leaked `-I`
appears in the `-importpath runtime/cgo` compile step.) `#cgo` directives are
scoped to a single package, so they don't have this problem.

## Updating to a newer Vosk wrapper

1. Bump the version in the root `go.mod` `require` block.
2. Re-copy `vosk.go`, `doc.go`, `COPYING` from the new module version.
3. Re-apply the three `#cgo` line changes documented above.
4. Keep the `replace` directive.

## License

Upstream is licensed under Apache-2.0; see `COPYING`.
