; Runtime DLL/API manifest. MODULE and API are supplied by runtime.asm.
; Names are ordinary Windows export names; lookup uses GetProcAddress.
MODULE kernel32, "kernel32.dll"
API GetTickCount64
API GetSystemTimeAsFileTime
API WriteFile
API Sleep
API CreateFileA
API GetFileSizeEx
API ReadFile
API CloseHandle
API SetFilePointerEx
API CreatePipe
API CreateProcessA
API PeekNamedPipe
API SetHandleInformation

MODULE ws2_32, "ws2_32.dll"
API WSAStartup
API WSACleanup
API WSAGetLastError
API socket
API connect
API ioctlsocket
API select
API send
API recv
API closesocket

MODULE bcrypt, "bcrypt.dll"
API BCryptGenRandom
