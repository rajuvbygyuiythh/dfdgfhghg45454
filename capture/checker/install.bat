@echo off
setlocal
cd /d "%~dp0"
title Dizkover Checker - Installer
color 0A

echo.
echo ==================================================
echo    Dizkover Checker - Installer
echo ==================================================
echo.

rem ---- Find Python (python, then the py launcher) --------------------------
set "PY=python"
python --version >nul 2>&1
if not errorlevel 1 goto :python_ready

set "PY=py -3"
py -3 --version >nul 2>&1
if not errorlevel 1 goto :python_ready

set "PY="
color 0C
echo [X] Python was not found.
echo.
echo     1. Install Python 3.9 or newer from https://www.python.org/downloads/
echo     2. Tick "Add python.exe to PATH" during installation.
echo     3. Run install.bat again.
echo.
pause
exit /b 1

:python_ready
echo [OK] Python:
%PY% --version
echo.

rem ---- pip + packages -----------------------------------------------------
echo [*] Upgrading pip...
%PY% -m pip install --upgrade pip --disable-pip-version-check --quiet
if errorlevel 1 echo [!] pip upgrade failed - continuing with the current pip.

echo.
echo [*] Installing required packages...
%PY% -m pip install -r requirements.txt --disable-pip-version-check
if errorlevel 1 (
    color 0C
    echo.
    echo [X] Package installation failed.
    echo     Check the internet connection and run install.bat again.
    pause
    exit /b 1
)
echo [OK] Python packages installed.
echo.

rem ---- Chromium -----------------------------------------------------------
echo [*] Downloading Playwright Chromium (first time can take several minutes)...
%PY% -m playwright install chromium
if errorlevel 1 (
    color 0C
    echo.
    echo [X] Chromium download failed.
    echo     Check the internet connection and run install.bat again.
    pause
    exit /b 1
)
echo [OK] Chromium ready.
echo.

rem ---- data folder --------------------------------------------------------
if not exist "data" mkdir "data"
if not exist "data\ac.txt" (
    copy /Y "ac.example.txt" "data\ac.txt" >nul
    echo [OK] Created data\ac.txt - put your accounts in it.
)
if not exist "data\proxy.txt" (
    copy /Y "proxy.example.txt" "data\proxy.txt" >nul
    echo [OK] Created data\proxy.txt - optional.
)
if not exist "data\valid.txt" type nul > "data\valid.txt"
if not exist "data\invalid.txt" type nul > "data\invalid.txt"
if not exist "data\banned.txt" type nul > "data\banned.txt"
if not exist "data\errors.txt" type nul > "data\errors.txt"

echo.
echo ==================================================
echo    Installation complete.
echo    1. Edit data\ac.txt
echo    2. Start run.bat
echo ==================================================
echo.
pause
exit /b 0
