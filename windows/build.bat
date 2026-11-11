@echo off
rem Builds remote.exe. runtime_boot resolves every Win32 API from the PEB, so
rem the image imports nothing and needs no link libraries. remote.bin is the
rem flat image: .text plus .rdata (the DLL/API name strings it references), so
rem the blob is self-contained apart from the zero-initialized .bss scratch.
setlocal
cd /d "%~dp0"

set NASM=nasm
where nasm >nul 2>nul || set NASM=C:\nasm\nasm.exe
set LD=ld
where ld >nul 2>nul || set LD=C:\msys64\mingw64\bin\ld.exe
set OBJCOPY=objcopy
where objcopy >nul 2>nul || set OBJCOPY=C:\msys64\mingw64\bin\objcopy.exe

"%NASM%" -f win64 -o toxnet.obj toxnet.asm || exit /b 1
"%NASM%" -f win64 -o remote.obj remote.asm || exit /b 1
"%NASM%" -f win64 -o runtime.obj runtime.asm || exit /b 1
"%LD%" -s -o remote.exe remote.obj toxnet.obj runtime.obj -e main --subsystem console || exit /b 1
"%OBJCOPY%" -O binary -j .text -j .rdata remote.exe remote.bin || exit /b 1
del toxnet.obj remote.obj runtime.obj
