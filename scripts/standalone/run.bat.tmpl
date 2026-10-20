@echo off
setlocal EnableExtensions
title CrushFling Auto Register
cd /d "%~dp0"

if not exist "CrushFling.exe" goto :missing
echo   Starting CrushFling...
start "" /D "%~dp0" "%~dp0CrushFling.exe"
goto :end

:missing
echo.
echo   CrushFling.exe was not found in this folder.
echo.
echo   Option 1: extract the WHOLE zip and double-click run.bat again.
echo   Option 2: build it now with install.bat (needs Go 1.25+ and internet,
echo             first run downloads dependencies, 3-10 minutes).
echo.
choice /C YN /N /M "  Build it now? [Y/N] "
if errorlevel 2 goto :end
call "%~dp0install.bat"
if not exist "CrushFling.exe" (
    echo   [ERROR] Build did not produce CrushFling.exe - see build.log
    pause
    goto :end
)
start "" /D "%~dp0" "%~dp0CrushFling.exe"
goto :end

:end
endlocal
exit /b 0
