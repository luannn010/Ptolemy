@echo off
REM Build the Vosk voice catcher on Windows with MinGW on PATH.
REM Intentionally does NOT set CGO_CPPFLAGS / CGO_LDFLAGS: the Vosk header/lib
REM paths come from third_party/vosk-go's package-scoped #cgo directives, so they
REM never leak into runtime/cgo. See third_party/vosk-go/vendor-notes.md.
cd /d D:\Ptolemy
set PATH=C:\msys64\mingw64\bin;%PATH%
set CGO_ENABLED=1
set CGO_CPPFLAGS=
set CGO_CFLAGS=
set CGO_LDFLAGS=
"C:\Program Files\Go\bin\go.exe" build -tags vosk -o bin\ptolemy-voice-vosk.exe .\cmd\ptolemy-voice
echo BUILD_EXIT=%ERRORLEVEL%
