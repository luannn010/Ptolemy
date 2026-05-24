@echo off
REM Run the Vosk voice catcher. Puts libvosk.dll (+ support DLLs) and
REM libportaudio.dll on PATH, and points VOSK_MODEL_PATH at the local model.
REM Pass extra args through, e.g.  run-vosk.bat -listen-only
cd /d D:\Ptolemy
set PATH=D:\Ptolemy\vosk-lib;C:\msys64\mingw64\bin;%PATH%
set VOSK_MODEL_PATH=D:\Ptolemy\.state\vosk-model
if "%WORKER_BASE_URL%"=="" set WORKER_BASE_URL=http://127.0.0.1:8080
if "%BRAIN_BASE_URL%"=="" set BRAIN_BASE_URL=http://127.0.0.1:1089
if "%BRAIN_MODEL%"=="" set BRAIN_MODEL=qwen3.5:4b
bin\ptolemy-voice-vosk.exe %*
