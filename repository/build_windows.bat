@echo off
setlocal
set GOOS=windows
set GOARCH=amd64
set CGO_ENABLED=0
go build -trimpath -ldflags="-s -w -H windowsgui" -o dareluna_patcher.exe .
if errorlevel 1 (
  echo Build failed. Please install Go 1.22 or later.
  exit /b 1
)
echo Built dareluna_patcher.exe
endlocal
