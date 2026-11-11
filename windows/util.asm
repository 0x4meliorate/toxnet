; Small helpers: memory, hex, nonces, time, console output.

section .text

global mem_cpy, mem_zero

; mem_cpy(rcx = dst, rdx = src, r8 = len)   (memmove-safe for dst < src)
mem_cpy:
    push rdi
    push rsi
    mov rdi, rcx
    mov rsi, rdx
    mov rcx, r8
    cld
    rep movsb
    pop rsi
    pop rdi
    ret

; mem_zero(rcx = dst, rdx = len)
mem_zero:
    push rdi
    mov rdi, rcx
    mov rcx, rdx
    xor eax, eax
    cld
    rep stosb
    pop rdi
    ret

; mem_eq(rcx = a, rdx = b, r8 = len) -> rax = 1 if equal
mem_eq:
    xor eax, eax
    test r8, r8
    jz .eq
.l:
    mov r9b, [rcx]
    cmp r9b, [rdx]
    jne .ne
    inc rcx
    inc rdx
    dec r8
    jnz .l
.eq:
    mov eax, 1
    ret
.ne:
    xor eax, eax
    ret

; inc_nonce(rcx = nonce24, edx = amount): big-endian add
inc_nonce:
    mov r8d, edx
    mov r9d, 23
.l:
    movzx eax, byte [rcx + r9]
    mov r10d, r8d
    and r10d, 0xff
    add eax, r10d
    mov [rcx + r9], al
    shr r8d, 8
    shr eax, 8
    add r8d, eax                ; carry into the remaining amount
    dec r9d
    jns .l
    ret

; closer(rcx = target, rdx = a, r8 = b) -> rax = 1 if a is strictly closer to target (XOR metric)
closer:
    xor r9d, r9d
.l:
    mov al, [rdx + r9]
    xor al, [rcx + r9]
    mov r10b, [r8 + r9]
    xor r10b, [rcx + r9]
    cmp al, r10b
    jb .yes
    ja .no
    inc r9d
    cmp r9d, 32
    jb .l
.no:
    xor eax, eax
    ret
.yes:
    mov eax, 1
    ret

; now_ms() -> rax
now_ms:
    sub rsp, 40
    call WINAPI(GetTickCount64)
    add rsp, 40
    ret

; unix_time() -> rax (seconds)
unix_time:
    sub rsp, 56
    lea rcx, [rsp+32]
    call WINAPI(GetSystemTimeAsFileTime)
    mov rax, [rsp+32]
    mov rcx, 116444736000000000
    sub rax, rcx
    xor edx, edx
    mov rcx, 10000000
    div rcx
    add rsp, 56
    ret

; rand_below(ecx = n) -> rax in [0, n), or -1 on RNG failure / n == 0
rand_below:
%assign FR 16
    PROLOG FR
    mov ebx, ecx
    test ebx, ebx
    jz .bad
    lea rcx, [LOC(0)]
    mov edx, 4
    call rand_bytes
    test eax, eax
    jnz .bad
    mov eax, [LOC(0)]
    xor edx, edx
    div ebx
    mov eax, edx
    jmp .ret
.bad:
    mov rax, -1
.ret:
    EPILOG FR

; hex_decode(rcx = dst, rdx = hex string, r8 = byte count) -> rax = 1 ok, 0 bad
hex_decode:
    push rbx
.l:
    test r8, r8
    jz .ok
    movzx eax, byte [rdx]
    call .nib
    jc .bad
    mov ebx, eax
    shl ebx, 4
    movzx eax, byte [rdx+1]
    call .nib
    jc .bad
    or eax, ebx
    mov [rcx], al
    inc rcx
    add rdx, 2
    dec r8
    jmp .l
.ok:
    mov eax, 1
    pop rbx
    ret
.bad:
    xor eax, eax
    pop rbx
    ret
.nib:                           ; al = char -> eax nibble, CF on error
    cmp al, '0'
    jb .nerr
    cmp al, '9'
    jbe .dig
    or al, 0x20
    cmp al, 'a'
    jb .nerr
    cmp al, 'f'
    ja .nerr
    sub al, 'a' - 10
    clc
    ret
.dig:
    sub al, '0'
    clc
    ret
.nerr:
    stc
    ret

; hex_encode(rcx = dst, rdx = src, r8 = byte count): uppercase, no terminator
hex_encode:
    lea r9, [rel hexdigits]
.l:
    test r8, r8
    jz .done
    movzx eax, byte [rdx]
    mov r10d, eax
    shr eax, 4
    mov al, [r9 + rax]
    mov [rcx], al
    and r10d, 15
    mov al, [r9 + r10]
    mov [rcx+1], al
    add rcx, 2
    inc rdx
    dec r8
    jmp .l
.done:
    ret

; str_len(rcx) -> rax
str_len:
    xor eax, eax
.l:
    cmp byte [rcx + rax], 0
    je .d
    inc rax
    jmp .l
.d:
    ret

section .text
hexdigits:   db "0123456789ABCDEF"
