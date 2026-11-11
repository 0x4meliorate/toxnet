; Common macros (Win64 calling convention).
;
; Every function saves all callee-saved registers, so rbx, rbp, rsi, rdi and
; r12-r15 are free to use inside it. Locals live at [rsp+64 ...]; [rsp+0..31]
; is shadow space for callees and [rsp+32..63] holds outgoing args 5-8.
;
;   myfunc:
;   %assign FR 64          ; bytes of locals, multiple of 16
;       PROLOG FR
;       ...
;       EPILOG FR

; The register saves are shared code (save_regs / restore_regs, defined once
; with SHARED_PROLOG) to keep the binary small. rax is clobbered on entry.
%macro PROLOG 1
    call save_regs
    sub rsp, %1 + 72
%endmacro

%macro EPILOG 1
    add rsp, %1 + 72
    jmp restore_regs
%endmacro

%macro SHARED_PROLOG 0
save_regs:                      ; pushes the 8 callee-saved registers, returns
    pop rax
    push rbp
    push rbx
    push rsi
    push rdi
    push r12
    push r13
    push r14
    push r15
    jmp rax
restore_regs:                   ; pops them and returns from the caller's function
    pop r15
    pop r14
    pop r13
    pop r12
    pop rdi
    pop rsi
    pop rbx
    pop rbp
    ret
%endmacro

%define LOC(x) rsp + 64 + (x)
; 5th/6th incoming argument (valid after PROLOG FR)
%define ARG5 rsp + FR + 176
%define ARG6 rsp + FR + 184

; Runtime Win32 API table, initialized by runtime_init before use.
%macro IMPORT 1
    extern winapi_%1
%endmacro
%define WINAPI(n) [rel winapi_ %+ n]

; Documented loader APIs and the early-failure exit remain normal PE imports.
%macro STATIC_IMPORT 1
    extern __imp_%1
%endmacro
%define STATIC_WINAPI(n) [rel __imp_ %+ n]

; memory helpers usable inside PROLOG functions (clobber rdi, rsi, rcx)
%macro COPY 3     ; dst, src, len
    lea rdi, %1
    lea rsi, %2
    mov ecx, %3
    rep movsb
%endmacro
