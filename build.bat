@echo off
rem Builds Toxnet: the self-contained web UI, for Windows (Toxnet.exe) and Linux
rem (Toxnet). The three remote shells are built, copied into web\assets, and
rem embedded into each Toxnet binary (go:embed). Run from the project root.
rem Requires Go, plus NASM + ld (for the Windows remote shells; see windows\build.bat).
setlocal
cd /d "%~dp0"
set GOARCH=amd64
set CGO_ENABLED=0

echo [1/4] Building Windows remote shells (remote.exe, remote.bin)...
call windows\build.bat || goto :err
cd /d "%~dp0"

echo [2/4] Building Linux remote shell (toxnet-linux)...
set GOOS=linux
go build -o linux\toxnet-linux .\linux || goto :err

echo [3/4] Collecting assets to embed...
if not exist web\assets mkdir web\assets
copy /y windows\remote.exe web\assets\toxnet-windows.exe >nul || goto :err
copy /y windows\remote.bin web\assets\toxnet-windows.bin >nul || goto :err
copy /y linux\toxnet-linux web\assets\toxnet-linux >nul || goto :err

echo [4/4] Building the Toxnet web UI (Windows + Linux)...
set GOOS=windows
go build -o Toxnet.exe .\web || goto :err
set GOOS=linux
go build -o Toxnet .\web || goto :err

echo.
echo Done. Toxnet.exe (Windows) and Toxnet (Linux) are in the project root.
echo Run either from any folder; it writes toxnet.save, toxnet.db and downloads\ there.
goto :eof

:err
echo BUILD FAILED
exit /b 1
