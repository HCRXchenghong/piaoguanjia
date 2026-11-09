@echo off
setlocal
cd /d "%~dp0web"

echo.
echo   Building frontend (Vue 3 + Arco Design Vue) ...
echo.

set "TEMP=%CD%\.tmp"
set "TMP=%CD%\.tmp"
if not exist ".tmp" mkdir ".tmp"

if exist "node_modules" goto build
echo   Installing dependencies from npmmirror ...
call npm install --registry=https://registry.npmmirror.com --no-audit --no-fund
if errorlevel 1 goto failed

:build
call npm run build
if errorlevel 1 goto failed

echo.
echo   Done. Output: web\dist
echo.
pause
exit /b 0

:failed
echo.
echo   Build failed. Check the messages above.
echo.
pause
exit /b 1
