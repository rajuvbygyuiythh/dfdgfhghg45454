@echo off
setlocal EnableExtensions EnableDelayedExpansion
title ChitChat Bot - Run
cd /d "%~dp0"
set "ROOT=%CD%"
set "BIN=%ROOT%\build\bin\chitchatui.exe"
set "CLI=%ROOT%\build\bin\crushfling.exe"
set "MODE=%~1"
if "%MODE%"=="" set "MODE=gui"
if /i "%MODE%"=="help" goto :help
if /i "%MODE%"=="cli" goto :cli

:gui
if not exist "%BIN%" goto :missing
echo   Starting ChitChat Bot dashboard...
echo   data folder: %ROOT%\data
start "" /D "%ROOT%" "%BIN%"
goto :end

:missing
echo.
echo   build\bin\chitchatui.exe was not found.
echo.
echo   Option 1: the release zip already contains it - extract the WHOLE zip
echo             and double-click run.bat again (do not move the exe alone).
echo   Option 2: build it now with install.bat (installs Go/Wails/WebView2
echo             if needed, 5-15 minutes on the first run).
echo.
choice /C YN /N /M "  Build it now? [Y/N] "
if errorlevel 2 goto :end
call "%ROOT%\install.bat" build
if not exist "%BIN%" (
    echo   [ERROR] Build did not produce build\bin\chitchatui.exe. See build\install.log
    pause
    goto :end
)
start "" /D "%ROOT%" "%BIN%"
goto :end

:cli
if not exist "%CLI%" (
    echo   build\bin\crushfling.exe was not found. Run install.bat first.
    pause
    goto :end
)
shift
set "ARGS="
:cli_args
if "%~1"=="" goto :cli_run
set "ARGS=%ARGS% "%~1""
shift
goto :cli_args
:cli_run
"%CLI%" %ARGS%
goto :end

:help
echo.
echo   run.bat            start the dashboard (window opens)
echo   run.bat cli ...    run the CrushFling command-line worker
echo                      examples:  run.bat cli --check
echo                                 run.bat cli --count 1 --email mailtm -v
echo   run.bat help       this text
echo.
goto :end

:end
endlocal
exit /b 0
