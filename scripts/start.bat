@echo off
REM ===========================================
REM Password Manager - Windows Startup Scripts
REM ===========================================

setlocal enabledelayedexpansion

set "PROJECT_DIR=%~dp0.."
set "SERVER_BIN=%PROJECT_DIR%\pwman-server.exe"
set "TAURI_BIN=%PROJECT_DIR%\src-tauri\target\release\pwman.exe"

REM ===========================================
REM Build Everything
REM ===========================================

:build-all
echo Building Go server...
cd /d "%PROJECT_DIR%"
go build -o pwman-server.exe .\cmd\server
if errorlevel 1 (
    echo Failed to build server
    exit /b 1
)

echo Building Tauri app...
call npm run tauri build
if errorlevel 1 (
    echo Failed to build Tauri app
    exit /b 1
)

echo Build complete!
echo Server: %SERVER_BIN%
echo App: %TAURI_BIN%
goto :eof

REM ===========================================
REM Start Server
REM ===========================================

:start-server
if not exist "%SERVER_BIN%" (
    echo Server not found. Run 'build-all' first.
    exit /b 1
)

echo Starting Go API server on port 18475...
cd /d "%PROJECT_DIR%"
start /b "" "%SERVER_BIN%"
goto :eof

REM ===========================================
REM Start App
REM ===========================================

:start-app
if not exist "%TAURI_BIN%" (
    echo App not found. Run 'build-all' first.
    exit /b 1
)

echo Starting Password Manager app...
start "" "%TAURI_BIN%"
goto :eof

REM ===========================================
REM Start Full Stack
REM ===========================================

:start-full
call :start-server
timeout /t 2 /nobreak >nul
call :start-app
goto :eof

REM ===========================================
REM Main
REM ===========================================

if "%~1"=="" goto help
if "%~1"=="build-all" goto build-all
if "%~1"=="start-server" goto start-server
if "%~1"=="start-app" goto start-app
if "%~1"=="start-full" goto start-full
if "%~1"=="help" goto help

echo Unknown command: %~1
echo.

:help
echo Password Manager - Startup Scripts
echo.
echo Usage: scripts\start.bat [command]
echo.
echo Commands:
echo   build-all      Build everything (server + app)
echo   start-server   Run Go API server
echo   start-app      Run desktop app
echo   start-full     Run both server + app
echo   help           Show this help
echo.
echo Examples:
echo   scripts\start.bat build-all
echo   scripts\start.bat start-full

:end
