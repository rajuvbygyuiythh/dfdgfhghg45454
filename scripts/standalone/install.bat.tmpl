@echo off
REM Builds the standalone CrushFling app on Windows.
REM   install.bat          install Go if missing, then build
REM   install.bat build    only build (Go must be present)
setlocal EnableExtensions
title CrushFling - install and build
cd /d "%~dp0"
set "LOG=%~dp0build.log"
echo [%date% %time%] install.bat %~1 >> "%LOG%"

set "PATH=%ProgramFiles%\Go\bin;%LOCALAPPDATA%\Programs\Go\bin;%USERPROFILE%\go\bin;%PATH%"

where go >nul 2>&1
if errorlevel 1 (
    echo   [..] Go not found - installing Go 1.25 via winget ...
    winget install --id GoLang.Go -e --silent --accept-package-agreements --accept-source-agreements >> "%LOG%" 2>&1
    set "PATH=%ProgramFiles%\Go\bin;%LOCALAPPDATA%\Programs\Go\bin;%USERPROFILE%\go\bin;%PATH%"
)
where go >nul 2>&1
if errorlevel 1 (
    echo   [ERROR] Go is required. Install it from https://go.dev/dl and run install.bat again.
    echo.
    pause
    exit /b 1
)
for /f "tokens=3" %%v in ('go version') do echo   [OK] Go %%v

echo   [..] go mod tidy   ^(downloads the dependencies^)
go mod tidy >> "%LOG%" 2>&1
if errorlevel 1 goto :failed

set "CGO_ENABLED=0"
set "GOFLAGS=-mod=mod"
echo   [..] Building CrushFling.exe ...
go build -trimpath -tags desktop,production -ldflags "-w -s -H windowsgui" -o "CrushFling.exe" . >> "%LOG%" 2>&1
if not exist "CrushFling.exe" goto :failed

echo   [..] Building the command-line worker ...
go build -trimpath -ldflags "-w -s" -o "crushfling-cli.exe" .\cmd\crushfling >> "%LOG%" 2>&1
if exist "crushfling-cli.exe" echo   [OK] crushfling-cli.exe built.

echo.
echo   ==================================================
echo    DONE.  CrushFling.exe is ready.
echo    Data folder: %~dp0data\   (bio.txt, pictures\, logs\)
echo    Start with run.bat
echo   ==================================================
echo.
choice /C YN /N /M "  Start CrushFling now? [Y/N] "
if errorlevel 2 goto :end
start "" /D "%~dp0" "%~dp0CrushFling.exe"
goto :end

:failed
echo   [ERROR] Build failed. See build.log for the compiler output.
echo.
pause
exit /b 1

:end
endlocal
exit /b 0
