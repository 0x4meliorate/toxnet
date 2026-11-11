; Cryptography, all from scratch:
;   SHA-512, Salsa20 / HSalsa20 / XSalsa20, Poly1305, X25519,
;   and NaCl crypto_box (curve25519-xsalsa20-poly1305) built from them.

section .text

global sha512, hsalsa20, poly1305, x25519, box_keypair, box_beforenm, box_seal, box_open, rand_bytes

; ============================================================ SHA-512

; sha512_block(rcx = state[8 qwords], rdx = 128-byte block)
sha512_block:
%assign FR 656
    PROLOG FR
    mov [LOC(640)], rcx
    lea rdi, [LOC(0)]              ; W[80]
    xor ebp, ebp
.w16:
    mov rax, [rdx + rbp*8]
    bswap rax
    mov [rdi + rbp*8], rax
    inc ebp
    cmp ebp, 16
    jb .w16
.wexp:
    mov rax, [rdi + rbp*8 - 15*8]  ; s0
    mov rbx, rax
    ror rbx, 1
    mov rcx, rax
    ror rcx, 8
    xor rbx, rcx
    shr rax, 7
    xor rbx, rax
    mov rax, [rdi + rbp*8 - 2*8]   ; s1
    mov rcx, rax
    ror rcx, 19
    mov rsi, rax
    ror rsi, 61
    xor rcx, rsi
    shr rax, 6
    xor rcx, rax
    add rbx, rcx
    add rbx, [rdi + rbp*8 - 7*8]
    add rbx, [rdi + rbp*8 - 16*8]
    mov [rdi + rbp*8], rbx
    inc ebp
    cmp ebp, 80
    jb .wexp

    mov rax, [LOC(640)]
    mov r8,  [rax]
    mov r9,  [rax+8]
    mov r10, [rax+16]
    mov r11, [rax+24]
    mov r12, [rax+32]
    mov r13, [rax+40]
    mov r14, [rax+48]
    mov r15, [rax+56]
    lea rsi, [rel K512]
    xor ebp, ebp
.round:
    ; T1 = h + S1(e) + Ch(e,f,g) + K + W
    mov rax, r12
    ror rax, 14
    mov rbx, r12
    ror rbx, 18
    xor rax, rbx
    ror rbx, 23
    xor rax, rbx
    mov rbx, r12
    and rbx, r13
    mov rcx, r12
    not rcx
    and rcx, r14
    xor rbx, rcx
    add rax, rbx
    add rax, r15
    add rax, [rsi + rbp*8]
    add rax, [rdi + rbp*8]
    ; T2 = S0(a) + Maj(a,b,c)
    mov rbx, r8
    ror rbx, 28
    mov rcx, r8
    ror rcx, 34
    xor rbx, rcx
    ror rcx, 5
    xor rbx, rcx
    mov rcx, r8
    and rcx, r9
    mov rdx, r8
    and rdx, r10
    xor rcx, rdx
    mov rdx, r9
    and rdx, r10
    xor rcx, rdx
    add rbx, rcx
    mov r15, r14
    mov r14, r13
    mov r13, r12
    lea r12, [r11 + rax]
    mov r11, r10
    mov r10, r9
    mov r9, r8
    lea r8, [rax + rbx]
    inc ebp
    cmp ebp, 80
    jb .round

    mov rax, [LOC(640)]
    add [rax], r8
    add [rax+8], r9
    add [rax+16], r10
    add [rax+24], r11
    add [rax+32], r12
    add [rax+40], r13
    add [rax+48], r14
    add [rax+56], r15
    EPILOG FR

; sha512(rcx = data, rdx = len, r8 = out[64])
sha512:
%assign FR 208
    PROLOG FR
    mov rsi, rcx
    mov r12, rdx
    mov r13, r8
    lea rcx, [LOC(0)]
    lea rdx, [rel SHA512_IV]
    mov r8d, 64
    call mem_cpy
    mov r14, r12
.blocks:
    cmp r14, 128
    jb .final
    lea rcx, [LOC(0)]
    mov rdx, rsi
    call sha512_block
    add rsi, 128
    sub r14, 128
    jmp .blocks
.final:
    lea rcx, [LOC(64)]
    mov edx, 128
    call mem_zero
    lea rcx, [LOC(64)]
    mov rdx, rsi
    mov r8, r14
    call mem_cpy
    mov byte [LOC(64) + r14], 0x80
    cmp r14, 112
    jb .lenfit
    lea rcx, [LOC(0)]
    lea rdx, [LOC(64)]
    call sha512_block
    lea rcx, [LOC(64)]
    mov edx, 128
    call mem_zero
.lenfit:
    mov rax, r12
    shl rax, 3
    bswap rax
    mov [LOC(64+120)], rax
    lea rcx, [LOC(0)]
    lea rdx, [LOC(64)]
    call sha512_block
    xor ecx, ecx
.out:
    mov rax, [LOC(0) + rcx*8]
    bswap rax
    mov [r13 + rcx*8], rax
    inc ecx
    cmp ecx, 8
    jb .out
    EPILOG FR

; ============================================================ Salsa20

%define SX(i) LOC(i*4)
%macro QR 4
    mov eax, [SX(%1)]
    add eax, [SX(%4)]
    rol eax, 7
    xor [SX(%2)], eax
    mov eax, [SX(%2)]
    add eax, [SX(%1)]
    rol eax, 9
    xor [SX(%3)], eax
    mov eax, [SX(%3)]
    add eax, [SX(%2)]
    rol eax, 13
    xor [SX(%4)], eax
    mov eax, [SX(%4)]
    add eax, [SX(%3)]
    rol eax, 18
    xor [SX(%1)], eax
%endmacro

; salsa_core(rcx = out, rdx = key32, r8 = in16, r9 = mode)
;   mode 0: Salsa20 block, writes 64 bytes
;   mode 1: HSalsa20, writes 32 bytes
salsa_core:
%assign FR 128
    PROLOG FR
    mov rdi, rcx
    mov r12, r9
    ; state copy at LOC(64): constants, key, input
    mov dword [LOC(64+0)],  0x61707865
    mov dword [LOC(64+20)], 0x3320646e
    mov dword [LOC(64+40)], 0x79622d32
    mov dword [LOC(64+60)], 0x6b206574
    mov rax, [rdx]
    mov [LOC(64+4)], rax
    mov rax, [rdx+8]
    mov [LOC(64+12)], rax
    mov rax, [rdx+16]
    mov [LOC(64+44)], rax
    mov rax, [rdx+24]
    mov [LOC(64+52)], rax
    mov rax, [r8]
    mov [LOC(64+24)], rax
    mov rax, [r8+8]
    mov [LOC(64+32)], rax
    ; working copy x = state
    xor ecx, ecx
.cp:
    mov eax, [LOC(64) + rcx*4]
    mov [LOC(0) + rcx*4], eax
    inc ecx
    cmp ecx, 16
    jb .cp
    mov ebx, 10
.dround:
    QR 0, 4, 8, 12
    QR 5, 9, 13, 1
    QR 10, 14, 2, 6
    QR 15, 3, 7, 11
    QR 0, 1, 2, 3
    QR 5, 6, 7, 4
    QR 10, 11, 8, 9
    QR 15, 12, 13, 14
    dec ebx
    jnz .dround
    test r12, r12
    jnz .hsalsa
    xor ecx, ecx
.add:
    mov eax, [LOC(0) + rcx*4]
    add eax, [LOC(64) + rcx*4]
    mov [rdi + rcx*4], eax
    inc ecx
    cmp ecx, 16
    jb .add
    jmp .done
.hsalsa:
    mov eax, [SX(0)]
    mov [rdi], eax
    mov eax, [SX(5)]
    mov [rdi+4], eax
    mov eax, [SX(10)]
    mov [rdi+8], eax
    mov eax, [SX(15)]
    mov [rdi+12], eax
    mov eax, [SX(6)]
    mov [rdi+16], eax
    mov eax, [SX(7)]
    mov [rdi+20], eax
    mov eax, [SX(8)]
    mov [rdi+24], eax
    mov eax, [SX(9)]
    mov [rdi+28], eax
.done:
    EPILOG FR

; hsalsa20(rcx = out32, rdx = key32, r8 = in16)
hsalsa20:
    sub rsp, 40
    mov r9d, 1
    call salsa_core
    add rsp, 40
    ret

; salsa_xor(rcx = buf, rdx = len, r8 = key32, r9 = nonce8): XOR buf with Salsa20 stream
salsa_xor:
%assign FR 96
    PROLOG FR
    mov rsi, rcx
    mov r12, rdx
    mov r13, r8
    mov rax, [r9]
    mov [LOC(0)], rax           ; nonce
    mov qword [LOC(8)], 0       ; block counter
.blk:
    test r12, r12
    jz .done
    lea rcx, [LOC(16)]          ; 64-byte keystream
    mov rdx, r13
    lea r8, [LOC(0)]
    xor r9d, r9d
    call salsa_core
    mov rcx, r12
    cmp rcx, 64
    jbe .n
    mov ecx, 64
.n:
    xor edx, edx
.x:
    mov al, [LOC(16) + rdx]
    xor [rsi + rdx], al
    inc edx
    cmp edx, ecx
    jb .x
    add rsi, rcx
    sub r12, rcx
    inc qword [LOC(8)]
    jmp .blk
.done:
    EPILOG FR

; ============================================================ Poly1305

; poly1305(rcx = out16, rdx = msg, r8 = len, r9 = key32)
poly1305:
%assign FR 64
    PROLOG FR
    mov [LOC(0)], rcx
    mov [LOC(8)], r9
    mov rbx, rdx                ; msg
    mov r15, r8                 ; remaining
    mov r12, [r9]
    mov rax, 0x0ffffffc0fffffff
    and r12, rax                ; r0
    mov r13, [r9+8]
    mov rax, 0x0ffffffc0ffffffc
    and r13, rax                ; r1
    mov r14, r13
    shr r14, 2
    add r14, r13                ; s1 = r1 + r1/4
    xor esi, esi                ; h0
    xor edi, edi                ; h1
    xor ebp, ebp                ; h2
.blk:
    test r15, r15
    jz .final
    cmp r15, 16
    jb .partial
    add rsi, [rbx]
    adc rdi, [rbx+8]
    adc rbp, 1
    add rbx, 16
    sub r15, 16
    jmp .mul
.partial:
    mov qword [LOC(16)], 0
    mov qword [LOC(24)], 0
    xor ecx, ecx
.pc:
    mov al, [rbx + rcx]
    mov [LOC(16) + rcx], al
    inc ecx
    cmp rcx, r15
    jb .pc
    mov byte [LOC(16) + rcx], 1
    add rsi, [LOC(16)]
    adc rdi, [LOC(24)]
    adc rbp, 0
    xor r15d, r15d
.mul:
    ; d0 = h0*r0 + h1*s1 -> r9:r8
    mov rax, rsi
    mul r12
    mov r8, rax
    mov r9, rdx
    mov rax, rdi
    mul r14
    add r8, rax
    adc r9, rdx
    ; d1 = h0*r1 + h1*r0 + h2*s1 + d0.hi -> r11:r10
    mov rax, rsi
    mul r13
    mov r10, rax
    mov r11, rdx
    mov rax, rdi
    mul r12
    add r10, rax
    adc r11, rdx
    mov rax, rbp
    imul rax, r14
    add r10, rax
    adc r11, 0
    add r10, r9
    adc r11, 0
    ; d2 = h2*r0 + d1.hi
    mov rax, rbp
    imul rax, r12
    add rax, r11
    mov rsi, r8
    mov rdi, r10
    mov rbp, rax
    and rbp, 3
    mov rcx, rax
    shr rax, 2
    and rcx, -4
    add rax, rcx                ; (d2 >> 2) * 5
    add rsi, rax
    adc rdi, 0
    adc rbp, 0
    jmp .blk
.final:
    mov r8, rsi
    add r8, 5
    mov r9, rdi
    adc r9, 0
    mov r10, rbp
    adc r10, 0
    shr r10, 2
    jz .keep
    mov rsi, r8
    mov rdi, r9
.keep:
    mov rax, [LOC(8)]
    add rsi, [rax+16]
    adc rdi, [rax+24]
    mov rax, [LOC(0)]
    mov [rax], rsi
    mov [rax+8], rdi
    EPILOG FR

; ============================================================ Field arithmetic mod 2^255-19
; Elements are 4 little-endian qwords, kept < 2^256 (not always fully reduced).

; fe_add(rcx = out, rdx = a, r8 = b)
fe_add:
    mov rax, [rdx]
    mov r9, [rdx+8]
    mov r10, [rdx+16]
    mov r11, [rdx+24]
    add rax, [r8]
    adc r9, [r8+8]
    adc r10, [r8+16]
    adc r11, [r8+24]
    mov edx, 0
    mov r8d, 38
    cmovc rdx, r8
    add rax, rdx
    adc r9, 0
    adc r10, 0
    adc r11, 0
    mov edx, 0
    cmovc rdx, r8
    add rax, rdx
    mov [rcx], rax
    mov [rcx+8], r9
    mov [rcx+16], r10
    mov [rcx+24], r11
    ret

; fe_sub(rcx = out, rdx = a, r8 = b)
fe_sub:
    mov rax, [rdx]
    mov r9, [rdx+8]
    mov r10, [rdx+16]
    mov r11, [rdx+24]
    sub rax, [r8]
    sbb r9, [r8+8]
    sbb r10, [r8+16]
    sbb r11, [r8+24]
    mov edx, 0
    mov r8d, 38
    cmovc rdx, r8
    sub rax, rdx
    sbb r9, 0
    sbb r10, 0
    sbb r11, 0
    mov edx, 0
    cmovc rdx, r8
    sub rax, rdx
    mov [rcx], rax
    mov [rcx+8], r9
    mov [rcx+16], r10
    mov [rcx+24], r11
    ret

; fe_mul(rcx = out, rdx = a, r8 = b)  (out may alias a or b)
fe_mul:
%assign FR 64
    PROLOG FR
    mov rdi, rcx
    mov rsi, rdx
    mov rbx, r8
    ; product t[0..7] in r8..r15
    xor r8d, r8d
    xor r9d, r9d
    xor r10d, r10d
    xor r11d, r11d
    xor r12d, r12d
    xor r13d, r13d
    xor r14d, r14d
    xor r15d, r15d
%macro MULROW 5   ; a-index, then the four t registers t[i], t[i+1], t[i+2], t[i+3]; carry into next
    mov rcx, [rsi + %1*8]
    xor ebp, ebp                ; carry
    mov rax, rcx
    mul qword [rbx]
    add rax, rbp
    adc rdx, 0
    add %2, rax
    adc rdx, 0
    mov rbp, rdx
    mov rax, rcx
    mul qword [rbx+8]
    add rax, rbp
    adc rdx, 0
    add %3, rax
    adc rdx, 0
    mov rbp, rdx
    mov rax, rcx
    mul qword [rbx+16]
    add rax, rbp
    adc rdx, 0
    add %4, rax
    adc rdx, 0
    mov rbp, rdx
    mov rax, rcx
    mul qword [rbx+24]
    add rax, rbp
    adc rdx, 0
    add %5, rax
    adc rdx, 0
%endmacro
    MULROW 0, r8, r9, r10, r11
    mov r12, rdx
    MULROW 1, r9, r10, r11, r12
    mov r13, rdx
    MULROW 2, r10, r11, r12, r13
    mov r14, rdx
    MULROW 3, r11, r12, r13, r14
    mov r15, rdx
    ; reduce: t[0..3] + 38 * t[4..7]
    mov ecx, 38
    mov rax, r12
    mul rcx
    add r8, rax
    adc rdx, 0
    mov rbp, rdx
    mov rax, r13
    mul rcx
    add rax, rbp
    adc rdx, 0
    add r9, rax
    adc rdx, 0
    mov rbp, rdx
    mov rax, r14
    mul rcx
    add rax, rbp
    adc rdx, 0
    add r10, rax
    adc rdx, 0
    mov rbp, rdx
    mov rax, r15
    mul rcx
    add rax, rbp
    adc rdx, 0
    add r11, rax
    adc rdx, 0
    imul rdx, rdx, 38
    add r8, rdx
    adc r9, 0
    adc r10, 0
    adc r11, 0
    mov edx, 0
    cmovc rdx, rcx
    add r8, rdx
    mov [rdi], r8
    mov [rdi+8], r9
    mov [rdi+16], r10
    mov [rdi+24], r11
    EPILOG FR

; fe_mul121665(rcx = out, rdx = a)
fe_mul121665:
%assign FR 0
    PROLOG FR
    mov rdi, rcx
    mov rsi, rdx
    mov ebx, 121665
    mov rax, [rsi]
    mul rbx
    mov r8, rax
    mov r12, rdx
    mov rax, [rsi+8]
    mul rbx
    add rax, r12
    adc rdx, 0
    mov r9, rax
    mov r12, rdx
    mov rax, [rsi+16]
    mul rbx
    add rax, r12
    adc rdx, 0
    mov r10, rax
    mov r12, rdx
    mov rax, [rsi+24]
    mul rbx
    add rax, r12
    adc rdx, 0
    mov r11, rax
    imul rdx, rdx, 38
    add r8, rdx
    adc r9, 0
    adc r10, 0
    adc r11, 0
    mov edx, 0
    mov ecx, 38
    cmovc rdx, rcx
    add r8, rdx
    mov [rdi], r8
    mov [rdi+8], r9
    mov [rdi+16], r10
    mov [rdi+24], r11
    EPILOG FR

; fe_cswap(rcx = a, rdx = b, r8 = swap bit)
fe_cswap:
    neg r8                      ; 0 or all ones
    xor r9d, r9d
.l:
    mov rax, [rcx + r9*8]
    mov r10, [rdx + r9*8]
    mov r11, rax
    xor r11, r10
    and r11, r8
    xor rax, r11
    xor r10, r11
    mov [rcx + r9*8], rax
    mov [rdx + r9*8], r10
    inc r9
    cmp r9, 4
    jb .l
    ret

; fe_inv(rcx = out, rdx = z): z^(p-2)
fe_inv:
%assign FR 64
    PROLOG FR
    mov rdi, rcx
    mov rsi, rdx
    lea rcx, [LOC(0)]           ; r = z (copy, so out may alias z)
    mov rdx, rsi
    mov r8d, 32
    call mem_cpy
    lea rcx, [LOC(32)]          ; result = 1
    mov edx, 32
    call mem_zero
    mov qword [LOC(32)], 1
    mov ebx, 254
.l:
    lea rcx, [LOC(32)]
    lea rdx, [LOC(32)]
    lea r8, [LOC(32)]
    call fe_mul
    cmp ebx, 2
    je .skip
    cmp ebx, 4
    je .skip
    lea rcx, [LOC(32)]
    lea rdx, [LOC(32)]
    lea r8, [LOC(0)]
    call fe_mul
.skip:
    dec ebx
    jns .l
    mov rcx, rdi
    lea rdx, [LOC(32)]
    mov r8d, 32
    call mem_cpy
    EPILOG FR

; fe_tobytes(rcx = out32, rdx = a): canonical little-endian encoding
fe_tobytes:
    mov r8, [rdx]
    mov r9, [rdx+8]
    mov r10, [rdx+16]
    mov r11, [rdx+24]
    ; fold bit 255 twice
%rep 2
    mov rax, r11
    shr rax, 63
    btr r11, 63
    imul rax, rax, 19
    add r8, rax
    adc r9, 0
    adc r10, 0
    adc r11, 0
%endrep
    ; if x + 19 >= 2^255 then x -= p
    mov rax, r8
    add rax, 19
    mov rdx, r9
    adc rdx, 0
    push rbx
    mov rbx, r10
    adc rbx, 0
    push rdi
    mov rdi, r11
    adc rdi, 0
    bt rdi, 63
    jnc .store
    btr rdi, 63
    mov r8, rax
    mov r9, rdx
    mov r10, rbx
    mov r11, rdi
.store:
    pop rdi
    pop rbx
    mov [rcx], r8
    mov [rcx+8], r9
    mov [rcx+16], r10
    mov [rcx+24], r11
    ret

; x25519(rcx = out32, rdx = scalar32, r8 = point32)
%define FE_X1 LOC(32)
%define FE_X2 LOC(64)
%define FE_Z2 LOC(96)
%define FE_X3 LOC(128)
%define FE_Z3 LOC(160)
%define FE_A  LOC(192)
%define FE_AA LOC(224)
%define FE_B  LOC(256)
%define FE_BB LOC(288)
%define FE_E  LOC(320)
%define FE_C  LOC(352)
%define FE_D  LOC(384)
%define FE_DA LOC(416)
%define FE_CB LOC(448)
%define FE_T  LOC(480)
%macro FCALL 4
    lea rcx, [%2]
    lea rdx, [%3]
    lea r8, [%4]
    call %1
%endmacro
x25519:
%assign FR 528
    PROLOG FR
    mov [LOC(512)], rcx
    mov rsi, rdx
    mov rdi, r8
    ; clamp scalar into LOC(0)
    lea rcx, [LOC(0)]
    mov rdx, rsi
    mov r8d, 32
    call mem_cpy
    and byte [LOC(0)], 248
    and byte [LOC(31)], 127
    or byte [LOC(31)], 64
    ; x1 = u (top bit cleared)
    lea rcx, [FE_X1]
    mov rdx, rdi
    mov r8d, 32
    call mem_cpy
    btr qword [FE_X1 + 24], 63
    ; x2 = 1, z2 = 0, x3 = u, z3 = 1
    lea rcx, [FE_X2]
    mov edx, 128
    call mem_zero
    mov qword [FE_X2], 1
    mov qword [FE_Z3], 1
    lea rcx, [FE_X3]
    lea rdx, [FE_X1]
    mov r8d, 32
    call mem_cpy
    xor r12d, r12d              ; swap
    mov ebx, 254
.ladder:
    mov eax, ebx
    shr eax, 3
    movzx r13d, byte [LOC(0) + rax]
    mov ecx, ebx
    and ecx, 7
    shr r13d, cl
    and r13d, 1                 ; k_t
    xor r12d, r13d
    lea rcx, [FE_X2]
    lea rdx, [FE_X3]
    mov r8, r12
    call fe_cswap
    lea rcx, [FE_Z2]
    lea rdx, [FE_Z3]
    mov r8, r12
    call fe_cswap
    mov r12d, r13d
    FCALL fe_add, FE_A, FE_X2, FE_Z2
    FCALL fe_mul, FE_AA, FE_A, FE_A
    FCALL fe_sub, FE_B, FE_X2, FE_Z2
    FCALL fe_mul, FE_BB, FE_B, FE_B
    FCALL fe_sub, FE_E, FE_AA, FE_BB
    FCALL fe_add, FE_C, FE_X3, FE_Z3
    FCALL fe_sub, FE_D, FE_X3, FE_Z3
    FCALL fe_mul, FE_DA, FE_D, FE_A
    FCALL fe_mul, FE_CB, FE_C, FE_B
    FCALL fe_add, FE_T, FE_DA, FE_CB
    FCALL fe_mul, FE_X3, FE_T, FE_T
    FCALL fe_sub, FE_T, FE_DA, FE_CB
    FCALL fe_mul, FE_T, FE_T, FE_T
    FCALL fe_mul, FE_Z3, FE_X1, FE_T
    FCALL fe_mul, FE_X2, FE_AA, FE_BB
    lea rcx, [FE_T]
    lea rdx, [FE_E]
    call fe_mul121665
    FCALL fe_add, FE_T, FE_AA, FE_T
    FCALL fe_mul, FE_Z2, FE_E, FE_T
    dec ebx
    jns .ladder
    lea rcx, [FE_X2]
    lea rdx, [FE_X3]
    mov r8, r12
    call fe_cswap
    lea rcx, [FE_Z2]
    lea rdx, [FE_Z3]
    mov r8, r12
    call fe_cswap
    lea rcx, [FE_T]
    lea rdx, [FE_Z2]
    call fe_inv
    FCALL fe_mul, FE_T, FE_X2, FE_T
    mov rcx, [LOC(512)]
    lea rdx, [FE_T]
    call fe_tobytes
    EPILOG FR

; ============================================================ NaCl box

; rand_bytes(rcx = dst, rdx = len) -> 0 / -1 (destination cleared on failure)
rand_bytes:
%assign FR 0
    PROLOG FR
    mov rdi, rcx
    mov rsi, rdx
    mov r8, rdx
    mov rdx, rcx
    xor ecx, ecx
    mov r9d, 2                  ; BCRYPT_USE_SYSTEM_PREFERRED_RNG
    call WINAPI(BCryptGenRandom)
    test eax, eax
    js .bad
    xor eax, eax
    jmp .ret
.bad:
    mov rcx, rdi
    mov rdx, rsi
    call mem_zero
    mov rax, -1
.ret:
    EPILOG FR

; box_keypair(rcx = pk32, rdx = sk32) -> 0 / -1 (both outputs cleared on failure)
box_keypair:
%assign FR 0
    PROLOG FR
    mov rdi, rcx
    mov rsi, rdx
    mov rcx, rsi
    mov edx, 32
    call rand_bytes
    test eax, eax
    jnz .bad
    mov rcx, rdi
    mov rdx, rsi
    lea r8, [rel basepoint9]
    call x25519
    xor eax, eax
    jmp .ret
.bad:
    mov rcx, rdi
    mov edx, 32
    call mem_zero
    mov rax, -1
.ret:
    EPILOG FR

; box_beforenm(rcx = shared32, rdx = peer pk, r8 = our sk)
box_beforenm:
%assign FR 32
    PROLOG FR
    mov rdi, rcx
    lea rcx, [LOC(0)]
    xchg rdx, r8                ; x25519(out, scalar=sk, point=pk)
    call x25519
    mov rcx, rdi
    lea rdx, [LOC(0)]
    lea r8, [rel zero16]
    call hsalsa20
    EPILOG FR

; box_seal(rcx = out, rdx = shared, r8 = nonce24, r9 = plain, [5] = len) -> rax = len + 16
; Output is MAC(16) || ciphertext. out may not overlap plain.
box_seal:
%assign FR 32
    PROLOG FR
    mov rdi, rcx
    mov rsi, rdx
    mov r12, r8
    mov r13, r9
    mov r14, [ARG5]
    ; subkey = hsalsa20(shared, nonce[0:16])
    lea rcx, [LOC(0)]
    mov rdx, rsi
    mov r8, r12
    call hsalsa20
    ; scratch = 32 zero bytes || plain
    lea rcx, [rel crypto_scratch]
    mov edx, 32
    call mem_zero
    lea rcx, [rel crypto_scratch + 32]
    mov rdx, r13
    mov r8, r14
    call mem_cpy
    lea rcx, [rel crypto_scratch]
    lea rdx, [r14 + 32]
    lea r8, [LOC(0)]
    lea r9, [r12 + 16]
    call salsa_xor
    ; ciphertext -> out+16, MAC over it with key = first 32 stream bytes
    lea rcx, [rdi + 16]
    lea rdx, [rel crypto_scratch + 32]
    mov r8, r14
    call mem_cpy
    mov rcx, rdi
    lea rdx, [rdi + 16]
    mov r8, r14
    lea r9, [rel crypto_scratch]
    call poly1305
    lea rax, [r14 + 16]
    EPILOG FR

; box_open(rcx = out, rdx = shared, r8 = nonce24, r9 = cipher,
;          [5] = cipher len, [6] = out capacity)
;   -> rax = plaintext length, or -1 if authentication or bounds checks fail
box_open:
%assign FR 64
    PROLOG FR
    mov rdi, rcx
    mov rsi, rdx
    mov r12, r8
    mov r13, r9
    mov r14, [ARG5]
    mov rax, -1
    cmp r14, 16
    jb .ret
    sub r14, 16                 ; plaintext length
    cmp r14, [ARG6]
    ja .ret
    cmp r14, 4096               ; leave room for the 32-byte stream prefix
    ja .ret
    lea rcx, [LOC(0)]
    mov rdx, rsi
    mov r8, r12
    call hsalsa20
    lea rcx, [rel crypto_scratch]
    mov edx, 32
    call mem_zero
    lea rcx, [rel crypto_scratch + 32]
    lea rdx, [r13 + 16]
    mov r8, r14
    call mem_cpy
    lea rcx, [rel crypto_scratch]
    lea rdx, [r14 + 32]
    lea r8, [LOC(0)]
    lea r9, [r12 + 16]
    call salsa_xor
    lea rcx, [LOC(32)]
    lea rdx, [r13 + 16]
    mov r8, r14
    lea r9, [rel crypto_scratch]
    call poly1305
    mov rax, [LOC(32)]
    mov rdx, [LOC(40)]
    xor rax, [r13]
    xor rdx, [r13 + 8]
    or rax, rdx
    mov rax, -1
    jnz .ret
    mov rcx, rdi
    lea rdx, [rel crypto_scratch + 32]
    mov r8, r14
    call mem_cpy
    mov rax, r14
.ret:
    EPILOG FR

section .text
align 16
basepoint9: db 9
            times 31 db 0
zero16:     times 32 db 0

section .bss
alignb 16
crypto_scratch: resb 4096 + 64
