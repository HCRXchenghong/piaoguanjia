@echo off
setlocal
cd /d "%~dp0"

if exist "D:\dev\go\bin\go.exe" (
  set "GO=D:\dev\go\bin\go.exe"
) else (
  set "GO=go"
)

echo.
echo   Building backend with: %GO%
echo.

"%GO%" version
if errorlevel 1 (
  echo   [!] Go not found. Install Go 1.23+ or set the path in this file.
  pause
  exit /b 1
)

cd server
if not exist "bin" mkdir "bin"
"%GO%" build -o bin\invoice-server.exe ./cmd/server
if errorlevel 1 (
  echo.
  echo   Build failed.
  pause
  exit /b 1
)

echo.
echo   Done. Output: server\bin\invoice-server.exe
echo.
pause
exit /b 0
