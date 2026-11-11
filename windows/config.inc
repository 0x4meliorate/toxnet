; Settings for remote.asm. Edit, then run build.bat and remote.exe.

; Tox ID (76 hex characters) of the device you control this PC FROM.
; remote.exe accepts commands ONLY from this ID - it is the authentication,
; so treat its secret key like an any private key.
%define FRIEND_ID              "B4CE07F8744E345F9F96E52709A9C0BD8D6A1D1748B86DAAF4695B0D4008EE6A4807D1996D75"

; Our nickname shown to the owner (the web UI shows an OS logo from this).
%define NAME                   "toxnet-windows"

; How often to check for newly received commands.
%define POLL_INTERVAL_MS       1000
