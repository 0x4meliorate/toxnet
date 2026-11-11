; Documented run-time linking for the single-threaded client/library.
; This is a normal PE object: the Windows loader supplies LoadLibraryExW,
; GetProcAddress and FreeLibrary through its import address table.
; Call runtime_init before any WINAPI call, and runtime_shutdown only after
; sockets/files and WinSock are shut down. Repeated init/shutdown is harmless.

default rel
bits 64
%include "macros.inc"

; No static imports. runtime_boot (below) resolves LoadLibraryExW,
; GetProcAddress, FreeLibrary and ExitProcess straight from kernel32's export
; table via the PEB, so this image has an empty import table.
extern save_regs, restore_regs
global runtime_init, runtime_shutdown

section .text
; runtime_init() -> 0 / -1. No API slots are usable after a failed init.
runtime_init:
%assign FR 0
    PROLOG FR
    cmp byte [rel runtime_ready], 0
    jne .ok
    call runtime_boot               ; resolve the four kernel32 bootstrap APIs
    test eax, eax
    jnz .failed
%macro MODULE 2
    lea rcx, [rel dll_name_%1]
    xor edx, edx
    mov r8d, 0x800             ; LOAD_LIBRARY_SEARCH_SYSTEM32
    call [rel p_LoadLibraryExW]
    test rax, rax
    jz .failed
    mov [rel module_%1], rax
    mov rbx, rax
%endmacro
%macro API 1
    mov rcx, rbx
    lea rdx, [rel api_name_%1]
    call [rel p_GetProcAddress]
    test rax, rax
    jz .failed
    mov [rel winapi_%1], rax
%endmacro
%include "imports.inc"
%unmacro MODULE 2
%unmacro API 1
    mov byte [rel runtime_ready], 1
.ok:
    xor eax, eax
    jmp .ret
.failed:
    call runtime_shutdown
    mov rax, -1
.ret:
    EPILOG FR

; Drop only the module references acquired here, including partial init.
runtime_shutdown:
%assign FR 0
    PROLOG FR
    mov byte [rel runtime_ready], 0
    lea rdi, [rel runtime_api_table]
    mov ecx, (runtime_api_end - runtime_api_table) / 8
    xor eax, eax
    cld
    rep stosq
    lea rbx, [rel runtime_modules]
    mov esi, (runtime_modules_end - runtime_modules) / 8
.next:
    dec esi
    mov rcx, [rbx + rsi*8]
    mov qword [rbx + rsi*8], 0
    test rcx, rcx
    jz .skip
    call [rel p_FreeLibrary]
.skip:
    test esi, esi
    jnz .next
    EPILOG FR

section .rdata
%macro MODULE 2
    align 2
dll_name_%1: dw __utf16__(%2), 0
%endmacro
%macro API 1
    %defstr %%name %1
api_name_%1: db %%name, 0
%endmacro
%include "imports.inc"
%unmacro MODULE 2
%unmacro API 1

section .bss
alignb 8
runtime_modules:
%macro MODULE 2
module_%1: resq 1
%endmacro
%macro API 1
%endmacro
%include "imports.inc"
%unmacro MODULE 2
%unmacro API 1
runtime_modules_end:

runtime_api_table:
%macro MODULE 2
%endmacro
%macro API 1
    global winapi_%1
winapi_%1: resq 1
%endmacro
%include "imports.inc"
%unmacro MODULE 2
%unmacro API 1
runtime_api_end:
runtime_ready: resb 1

; ---------------------------------------------------------------------------
; Loader-independent bootstrap. Instead of relying on the Windows loader to
; populate an import address table, we locate the already-mapped kernel32
; through the PEB module list and read LoadLibraryExW, GetProcAddress,
; FreeLibrary and ExitProcess straight out of its export directory. The image
; then has no import table at all. Everything here is RIP-relative, so it
; stays position-independent.
; ---------------------------------------------------------------------------
section .text

; runtime_boot() -> eax = 0 on success, -1 on failure. Resolves the four
; kernel32 bootstrap APIs from the PEB into p_* and is safe to call repeatedly.
global runtime_boot
runtime_boot:
%assign FR 0
    PROLOG FR
    cmp byte [rel runtime_booted], 0
    jne .ok                         ; already resolved
    mov rax, [gs:0x60]              ; TEB -> PEB
    mov rax, [rax + 0x18]           ; PEB -> Ldr (PEB_LDR_DATA*)
    lea rdi, [rax + 0x10]           ; &InLoadOrderModuleList (list head)
    mov rsi, [rdi]                  ; first module entry
.walk:
    cmp rsi, rdi                    ; wrapped back to the head -> not found
    je .fail
    mov rbx, [rsi + 0x30]           ; LDR_DATA_TABLE_ENTRY.DllBase
    test rbx, rbx
    jz .next
    mov rcx, [rsi + 0x60]           ; BaseDllName.Buffer (UTF-16)
    test rcx, rcx
    jz .next
    lea rdx, [rel name_kernel32]
    call wstr_ieq_ascii             ; case-insensitive name match
    test eax, eax
    jnz .found
.next:
    mov rsi, [rsi]                  ; InLoadOrderLinks.Flink -> next entry
    jmp .walk
.found:
    ; rbx = kernel32 base. Resolve each export from the table, storing each as
    ; soon as it is found so ExitProcess is usable even on a later failure.
    mov rcx, rbx
    lea rdx, [rel name_exitprocess]
    call find_export_ascii
    mov [rel p_ExitProcess], rax
    test rax, rax
    jz .fail
    mov rcx, rbx
    lea rdx, [rel name_getproc]
    call find_export_ascii
    mov [rel p_GetProcAddress], rax
    test rax, rax
    jz .fail
    mov rcx, rbx
    lea rdx, [rel name_loadlibrary]
    call find_export_ascii
    mov [rel p_LoadLibraryExW], rax
    test rax, rax
    jz .fail
    mov rcx, rbx
    lea rdx, [rel name_freelibrary]
    call find_export_ascii
    mov [rel p_FreeLibrary], rax
    test rax, rax
    jz .fail
    mov byte [rel runtime_booted], 1
.ok:
    xor eax, eax
    jmp .ret
.fail:
    mov eax, -1
.ret:
    EPILOG FR

; find_export_ascii(rcx = module base, rdx = ASCII name) -> rax = addr or 0.
; Walks the PE export directory by name. GetProcAddress is a normal (non-
; forwarded) kernel32 export, so export forwarders are not resolved here.
find_export_ascii:
    push rbx
    push rsi
    push rdi
    push r12
    push r13
    mov rbx, rcx                    ; base
    mov r12, rdx                    ; target name
    mov eax, [rbx + 0x3c]           ; e_lfanew
    mov eax, [rbx + rax + 0x88]     ; DataDirectory[Export].VirtualAddress
    test eax, eax
    jz .none
    lea rsi, [rbx + rax]            ; IMAGE_EXPORT_DIRECTORY*
    mov r13d, [rsi + 0x18]          ; NumberOfNames
    test r13d, r13d
    jz .none
    mov eax, [rsi + 0x20]           ; AddressOfNames RVA
    lea rdi, [rbx + rax]            ; -> array of dword name RVAs
    xor ecx, ecx                    ; i = 0
.loop:
    cmp ecx, r13d
    jae .none
    mov eax, [rdi + rcx*4]          ; name RVA
    lea rax, [rbx + rax]            ; -> ASCII name
    mov r8, rax
    mov r9, r12
.cmp:
    mov r10b, [r8]
    cmp r10b, [r9]
    jne .nextname
    test r10b, r10b
    jz .match                       ; both hit NUL together
    inc r8
    inc r9
    jmp .cmp
.nextname:
    inc ecx
    jmp .loop
.match:
    mov eax, [rsi + 0x24]           ; AddressOfNameOrdinals RVA
    lea rax, [rbx + rax]
    movzx eax, word [rax + rcx*2]   ; ordinal index for name i
    mov edx, [rsi + 0x1c]           ; AddressOfFunctions RVA
    lea rdx, [rbx + rdx]
    mov eax, [rdx + rax*4]          ; function RVA
    lea rax, [rbx + rax]            ; absolute function address
    jmp .ret
.none:
    xor eax, eax
.ret:
    pop r13
    pop r12
    pop rdi
    pop rsi
    pop rbx
    ret

; wstr_ieq_ascii(rcx = UTF-16 buffer, rdx = ASCII string) -> eax = 1 if equal
; over the ASCII range, case-insensitive; both must terminate together.
wstr_ieq_ascii:
    push rsi
    push rdi
    mov rsi, rcx
    mov rdi, rdx
.l:
    movzx eax, word [rsi]           ; wide char
    movzx ecx, byte [rdi]           ; ascii char
    test ecx, ecx
    jz .end
    cmp eax, 0x7f                   ; non-ASCII wide char can't match
    ja .no
    cmp al, 'A'
    jb .a1
    cmp al, 'Z'
    ja .a1
    add al, 0x20
.a1:
    cmp cl, 'A'
    jb .b1
    cmp cl, 'Z'
    ja .b1
    add cl, 0x20
.b1:
    cmp al, cl
    jne .no
    add rsi, 2
    inc rdi
    jmp .l
.end:
    test eax, eax                   ; ascii ended; wide must end too
    jnz .no
    mov eax, 1
    jmp .ret
.no:
    xor eax, eax
.ret:
    pop rdi
    pop rsi
    ret

section .rdata
; Both are ASCII targets: wstr_ieq_ascii widens the PEB's UTF-16 name as it
; compares, and find_export_ascii compares against ASCII export names.
name_kernel32:     db "kernel32.dll", 0
name_getproc:      db "GetProcAddress", 0
name_loadlibrary:  db "LoadLibraryExW", 0
name_freelibrary:  db "FreeLibrary", 0
name_exitprocess:  db "ExitProcess", 0

section .bss
    alignb 8
global p_GetProcAddress, p_LoadLibraryExW, p_FreeLibrary, p_ExitProcess
p_GetProcAddress: resq 1
p_LoadLibraryExW: resq 1
p_FreeLibrary:    resq 1
p_ExitProcess:    resq 1
runtime_booted:   resb 1
