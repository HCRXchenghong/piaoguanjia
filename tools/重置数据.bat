@echo off
setlocal
cd /d "%~dp0"

echo.
echo   ============================================
echo     Reset all data (users / invoices / files)
echo   ============================================
echo.
echo   This will DELETE every account, invoice and attachment.
set /p CONFIRM=Type YES to continue: 
if /i not "%CONFIRM%"=="YES" goto cancelled

if not defined INVOICE_MYSQL_HOME set "INVOICE_MYSQL_HOME=D:\dev\mysql"
set "MYSQL=%INVOICE_MYSQL_HOME%\bin\mysql.exe"

"%MYSQL%" -u invoice -p"Invoice@2026" -h 127.0.0.1 invoice -e "SET FOREIGN_KEY_CHECKS=0; TRUNCATE TABLE files; TRUNCATE TABLE invoices; TRUNCATE TABLE pending_uploads; TRUNCATE TABLE users; TRUNCATE TABLE audit_logs; TRUNCATE TABLE settings; SET FOREIGN_KEY_CHECKS=1;"
if errorlevel 1 goto failed

if exist "..\server\data\files" rd /s /q "..\server\data\files"
if exist "..\server\data\thumbs" rd /s /q "..\server\data\thumbs"
if exist "..\server\data\tmp" rd /s /q "..\server\data\tmp"
mkdir "..\server\data\files" 2>nul
mkdir "..\server\data\thumbs" 2>nul
mkdir "..\server\data\tmp" 2>nul

echo.
echo   Done. Restart the service and open the page to create the first admin.
echo.
pause
exit /b 0

:cancelled
echo   Cancelled.
pause
exit /b 0

:failed
echo.
echo   [!] Reset failed. Check MySQL is running and credentials are correct.
pause
exit /b 1
