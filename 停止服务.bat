@echo off
echo.
echo   Stopping Invoice Hub and MySQL ...
taskkill /IM invoice-server.exe /F >nul 2>&1
taskkill /IM mysqld.exe /F >nul 2>&1
echo   Done.
echo.
timeout /t 2 /nobreak >nul
