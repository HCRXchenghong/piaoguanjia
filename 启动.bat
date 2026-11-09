@echo off
setlocal
cd /d "%~dp0"

if not defined INVOICE_MYSQL_HOME set "INVOICE_MYSQL_HOME=D:\dev\mysql"
set "MYSQLD=%INVOICE_MYSQL_HOME%\bin\mysqld.exe"
set "MYSQLINI=%INVOICE_MYSQL_HOME%\my.ini"

echo.
echo   ==========================================
echo     Invoice Hub - Invoice Management
echo   ==========================================
echo.

powershell -NoProfile -Command "if (Get-NetTCPConnection -LocalPort 3306 -State Listen -ErrorAction SilentlyContinue) { exit 0 } else { exit 1 }" >nul 2>&1
if not errorlevel 1 goto mysql_ready

if exist "%MYSQLD%" goto start_mysql
echo   [!] MySQL not found at "%INVOICE_MYSQL_HOME%"
echo       Edit INVOICE_MYSQL_HOME in this file, or start MySQL manually.
echo.
pause
exit /b 1

:start_mysql
echo   [1/2] Starting MySQL on port 3306 ...
start "MySQL 8.0" /min "%MYSQLD%" --defaults-file="%MYSQLINI%" --console
timeout /t 6 /nobreak >nul
goto server

:mysql_ready
echo   [1/2] MySQL is already running.

:server
if not exist "server\bin\invoice-server.exe" goto no_server
if not exist "web\dist\index.html" echo   [!] web\dist missing - run build-web.bat first.

echo   [2/2] Starting service on http://127.0.0.1:8686
echo.
start "" http://127.0.0.1:8686
server\bin\invoice-server.exe -addr :8686 -web web\dist -data server\data

echo.
echo   Service stopped.
pause
exit /b 0

:no_server
echo   [!] server\bin\invoice-server.exe not found.
echo       Build it with:  cd server  ^&^&  go build -o bin\invoice-server.exe ./cmd/server
echo.
pause
exit /b 1
