@echo off
setlocal
set CGO_ENABLED=1
where gcc >nul 2>nul
if errorlevel 1 (
  echo race-test requires a C compiler on PATH because Go race builds need cgo.
  exit /b 1
)
cd /d "%~dp0"
go test -race ./controlplane/... -count=1 -timeout=120s
