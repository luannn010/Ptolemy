@echo off
REM Run the Vosk voice catcher. Puts libvosk.dll (+ support DLLs) and
REM libportaudio.dll on PATH, and points VOSK_MODEL_PATH at the local model.
REM Pass extra args through, e.g.  run-vosk.bat -listen-only
cd /d D:\Ptolemy
set PATH=D:\Ptolemy\vosk-lib;C:\msys64\mingw64\bin;%PATH%
set VOSK_MODEL_PATH=D:\Ptolemy\.state\vosk-model
if "%WORKER_BASE_URL%"=="" set WORKER_BASE_URL=http://127.0.0.1:8080
REM Brain (llama.cpp) runs inside WSL on 127.0.0.1:1089, which Windows cannot
REM reach directly. Start the WSL relay first (in WSL: python3 brain-relay.py),
REM then this Windows port 18089 is forwarded to it. See brain-relay.py.
if "%BRAIN_BASE_URL%"=="" set BRAIN_BASE_URL=http://127.0.0.1:18089
if "%BRAIN_MODEL%"=="" set BRAIN_MODEL=Qwen3.5-4B-Q4_K_M.gguf
bin\ptolemy-voice-vosk.exe %*
