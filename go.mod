module github.com/luannn010/ptolemy

go 1.25.0

require (
	github.com/go-chi/chi/v5 v5.2.5
	github.com/google/uuid v1.6.0
	github.com/joho/godotenv v1.5.1
	github.com/rs/zerolog v1.35.1
	modernc.org/sqlite v1.49.1
)

require (
	github.com/alphacep/vosk-api/go v0.3.50
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/gordonklaus/portaudio v0.0.0-20260203164431-765aa7dfa631
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	golang.org/x/sys v0.42.0 // indirect
	modernc.org/libc v1.72.0 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)

// Use a local copy of the Vosk Go wrapper whose #cgo directives point at the
// repo's vosk-lib/ directory. This avoids setting CGO_CPPFLAGS/CGO_LDFLAGS as
// global env vars, which would leak into runtime/cgo and break the build under
// MinGW (-Werror). See third_party/vosk-go/vendor-notes.md.
replace github.com/alphacep/vosk-api/go => ./third_party/vosk-go
