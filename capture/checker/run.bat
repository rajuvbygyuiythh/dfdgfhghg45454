@echo off
setlocal
cd /d "%~dp0"
title Dizkover Checker
color 0B
chcp 65001 >nul
set "PYTHONUTF8=1"
set "PYTHONIOENCODING=utf-8"

rem ---- Find Python (python, then the py launcher) --------------------------
set "PY=python"
python --version >nul 2>&1
if not errorlevel 1 goto :python_ready
set "PY=py -3"
py -3 --version >nul 2>&1
if not errorlevel 1 goto :python_ready
set "PY="

:python_ready
if not exist "data" mkdir "data"

:menu
cls
echo.
echo ==================================================
echo    Dizkover Checker 2.1
echo ==================================================
if defined PY (
    echo    Python : %PY%
) else (
    echo    Python : NOT FOUND - run the installer
)
if exist "data\ac.txt" (
    echo    Accounts: data\ac.txt
) else (
    echo    Accounts: data\ac.txt (missing)
)
echo.
echo    [1] Run check (headless)
echo    [2] Run check (visible browser)
echo    [3] Test proxies only
echo    [4] Run self-test (no browser)
echo    [5] View logs.txt
echo    [6] View valid.txt
echo    [7] View invalid.txt
echo    [8] Edit accounts (data\ac.txt)
echo    [9] Edit proxies (data\proxy.txt)
echo   [10] Run installer
echo   [11] Real-browser self-test (local replica)
echo    [0] Exit
echo.
set /p "pick=Select: "

if "%pick%"=="1" goto :run_headless
if "%pick%"=="2" goto :run_visible
if "%pick%"=="3" goto :test_proxies
if "%pick%"=="4" goto :selftest
if "%pick%"=="5" goto :view_logs
if "%pick%"=="6" goto :view_valid
if "%pick%"=="7" goto :view_invalid
if "%pick%"=="8" goto :edit_accounts
if "%pick%"=="9" goto :edit_proxies
if "%pick%"=="10" goto :installer
if "%pick%"=="11" goto :selftest_live
if "%pick%"=="0" goto :end
goto :menu

rem ---- Actions -------------------------------------------------------------

:run_headless
call :require_python
if errorlevel 1 goto :pause_menu
call :ensure_deps
if errorlevel 1 goto :pause_menu
call :ensure_accounts
if errorlevel 1 goto :pause_menu
cls
echo [*] Running headless...
echo.
%PY% main.py --headless
if errorlevel 2 (
    echo.
    echo [!] The check could not start (see the message above).
    start "" notepad "data\ac.txt"
)
goto :pause_menu

:run_visible
call :require_python
if errorlevel 1 goto :pause_menu
call :ensure_deps
if errorlevel 1 goto :pause_menu
call :ensure_accounts
if errorlevel 1 goto :pause_menu
cls
echo [*] Running with a visible browser...
echo.
%PY% main.py --visible
if errorlevel 2 (
    echo.
    echo [!] The check could not start (see the message above).
    start "" notepad "data\ac.txt"
)
goto :pause_menu

:test_proxies
call :require_python
if errorlevel 1 goto :pause_menu
call :ensure_deps
if errorlevel 1 goto :pause_menu
if not exist "data\proxy.txt" (
    copy /Y "proxy.example.txt" "data\proxy.txt" >nul
    echo [!] data\proxy.txt was missing - a template was created.
    start "" notepad "data\proxy.txt"
    goto :pause_menu
)
cls
echo [*] Testing proxies (requests + real Chromium)...
echo.
%PY% main.py --test-proxies
goto :pause_menu

:selftest
call :require_python
if errorlevel 1 goto :pause_menu
cls
echo [*] Running browser-free self-test...
echo.
%PY% -m unittest -v test_core.py test_flow.py
goto :pause_menu

:view_logs
call :view_file "data\logs.txt"
goto :menu

:view_valid
call :view_file "data\valid.txt"
goto :menu

:view_invalid
call :view_file "data\invalid.txt"
goto :menu

:edit_accounts
if not exist "data\ac.txt" copy /Y "ac.example.txt" "data\ac.txt" >nul
start "" notepad "data\ac.txt"
goto :menu

:edit_proxies
if not exist "data\proxy.txt" copy /Y "proxy.example.txt" "data\proxy.txt" >nul
start "" notepad "data\proxy.txt"
goto :menu

:installer
call install.bat
goto :menu

:selftest_live
call :require_python
if errorlevel 1 goto :pause_menu
call :ensure_deps
if errorlevel 1 goto :pause_menu
cls
echo [*] Real-browser self-test against a local replica (no internet)...
echo.
%PY% selftest_live.py
goto :pause_menu

rem ---- Helpers -------------------------------------------------------------

:require_python
if defined PY exit /b 0
color 0C
echo.
echo [X] Python was not found.
echo     Run option 10 [installer] after installing Python 3.9+.
echo     https://www.python.org/downloads/
exit /b 1

:ensure_deps
%PY% -c "import playwright" >nul 2>&1
if not errorlevel 1 exit /b 0
color 0E
echo.
echo [!] Playwright is not installed - running the installer first...
echo.
call install.bat
%PY% -c "import playwright" >nul 2>&1
if not errorlevel 1 exit /b 0
color 0C
echo [X] Playwright is still missing. Run install.bat manually.
exit /b 1

:ensure_accounts
if not exist "data\ac.txt" (
    copy /Y "ac.example.txt" "data\ac.txt" >nul
    echo.
    echo [!] data\ac.txt was missing. A template was created.
    echo     Add one account per line in this format: username:password
    start "" notepad "data\ac.txt"
    exit /b 1
)
exit /b 0

:view_file
if not exist "%~1" (
    echo.
    echo [!] %~1 not found or empty.
    exit /b 0
)
cls
echo ======== %~1 ========
echo.
more "%~1"
echo.
echo ======================
exit /b 0

:pause_menu
echo.
pause
goto :menu

:end
exit /b 0
