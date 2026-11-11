; toxnet: a tiny Tox client library in pure x86-64 assembly (NASM, Windows).
;
; Connects to bootstrap nodes / TCP relays, announces itself and finds
; friends over the onion, sends friend requests, and exchanges messages over
; net_crypto. Keys exist only in memory. Uses only Win32 APIs (kernel32,
; ws2_32, bcrypt) - no C runtime. Single-threaded: call toxnet_iterate()
; in a loop. One Tox instance per process.
; Link runtime.obj as well as toxnet.obj. toxnet_init resolves the Win32 API
; table; toxnet_shutdown releases it. Other APIs require successful init.
;
; API (Win64 calling convention):
;   int  toxnet_init(const char *name)
;   void toxnet_address(char out[77])                    76 hex chars + NUL
;   int  toxnet_add_friend(const char *tox_id, const char *message)  -> friend or -1
;   void toxnet_iterate(void)                            runs ~50 ms
;   int  toxnet_friend_online(int friend)
;   int  toxnet_send_message(int friend, const char *msg, int len)   -> 0 / -1
;   int  toxnet_receive_message(char *buf, int buflen, int *friend)  -> len / -1
;   int  toxnet_friend_name(int friend, char *buf, int buflen)       -> len
;   void toxnet_shutdown(void)

default rel
bits 64

%include "macros.inc"
%include "structs.inc"

IMPORT GetTickCount64
IMPORT GetSystemTimeAsFileTime
IMPORT WriteFile
IMPORT Sleep
IMPORT BCryptGenRandom
IMPORT WSAStartup
IMPORT WSACleanup
IMPORT WSAGetLastError
IMPORT socket
IMPORT connect
IMPORT ioctlsocket
IMPORT select
IMPORT send
IMPORT recv
IMPORT closesocket
IMPORT CreateFileA
IMPORT GetFileSizeEx
IMPORT ReadFile
IMPORT CloseHandle
IMPORT SetFilePointerEx
extern runtime_init, runtime_shutdown

global toxnet_init, toxnet_address, toxnet_add_friend, toxnet_iterate
global toxnet_friend_online, toxnet_send_message, toxnet_receive_message
global toxnet_friend_name, toxnet_shutdown
global toxnet_file_send, toxnet_file_progress, toxnet_file_control
global toxnet_file_poll_offer, toxnet_file_accept

%include "util.asm"
%include "crypto.asm"
%include "relay.asm"
%include "onion.asm"
%include "netcrypto.asm"
%include "filexfer.asm"
%include "generated.inc"

ONION_DATA_FRIEND_REQ equ 32

section .text

global save_regs, restore_regs
SHARED_PROLOG

; int toxnet_init(rcx = name)
toxnet_init:
%assign FR 64
    PROLOG FR
    mov rsi, rcx
    mov rax, -1
    cmp byte [rel g_started], 0
    jne .ret                  ; don't replace an active instance
    call runtime_init
    test eax, eax
    jnz .bad
    mov ecx, 0x0202
    lea rdx, [rel wsadata]
    call WINAPI(WSAStartup)
    test eax, eax
    jnz .linkcleanup
    mov byte [rel g_started], 1
    ; nickname
    xor eax, eax
    test rsi, rsi
    jz .noname
    mov rcx, rsi
    call str_len
    cmp eax, 127
    jbe .nl
    mov eax, 127
.nl:
    mov [rel g_namelen], eax
    lea rcx, [rel g_name]
    mov rdx, rsi
    mov r8, rax
    call mem_cpy
.noname:
    ; fresh in-memory identity
    lea rcx, [rel g_realpk]
    lea rdx, [rel g_realsk]
    call box_keypair
    test eax, eax
    jnz .rngfail
    lea rcx, [rel g_dhtpk]
    lea rdx, [rel g_dhtsk]
    call box_keypair
    test eax, eax
    jnz .rngfail
    lea rcx, [rel g_onionpk]
    lea rdx, [rel g_onionsk]
    call box_keypair
    test eax, eax
    jnz .rngfail
    lea rcx, [rel g_nospam]
    mov edx, 4
    call rand_bytes
    test eax, eax
    jnz .rngfail
    lea rcx, [rel g_cookiekey]
    mov edx, 32
    call rand_bytes
    test eax, eax
    jnz .rngfail
    ; target 0 = ourselves (announce with our real keys)
    lea rbx, [rel targets]
    lea rcx, [rbx + TARGET.pk]
    lea rdx, [rel g_realpk]
    mov r8d, 32
    call mem_cpy
    lea rcx, [rbx + TARGET.sendpk]
    lea rdx, [rel g_realpk]
    mov r8d, 32
    call mem_cpy
    lea rcx, [rbx + TARGET.sendsk]
    lea rdx, [rel g_realsk]
    mov r8d, 32
    call mem_cpy
    mov byte [rbx + TARGET.isself], 1
    ; every built-in bootstrap node is also a TCP relay
    lea rsi, [rel bootstrap_nodes]
    xor edi, edi
.bs:
    lea rcx, [LOC(0)]
    mov rdx, rsi
    mov r8d, NODE_size
    call mem_cpy
    mov ax, [rsi + NODE.tport]
    mov [LOC(0) + NODE.port], ax
    lea rcx, [LOC(0)]
    call relay_add
    add rsi, NODE_size
    inc edi
    cmp edi, BOOTSTRAP_COUNT
    jb .bs
    call now_ms
    mov [rel g_now], rax
    xor eax, eax
    jmp .ret
.rngfail:
    ; No relays or target keys have been installed yet. Balance WSAStartup
    ; and discard every piece of the incomplete identity before returning.
    lea rcx, [rel g_realpk]
    mov edx, g_namelen - g_realpk
    call mem_zero
    call WINAPI(WSACleanup)
    mov byte [rel g_started], 0
.linkcleanup:
    call runtime_shutdown
.bad:
    mov rax, -1
.ret:
    EPILOG FR

; void toxnet_address(rcx = out[77])
toxnet_address:
%assign FR 48
    PROLOG FR
    mov rdi, rcx
    lea rcx, [LOC(0)]
    lea rdx, [rel g_realpk]
    mov r8d, 32
    call mem_cpy
    mov eax, [rel g_nospam]
    mov [LOC(32)], eax
    lea rcx, [LOC(0)]
    call address_checksum
    mov [LOC(36)], ax
    mov rcx, rdi
    lea rdx, [LOC(0)]
    mov r8d, 38
    call hex_encode
    mov byte [rdi + 76], 0
    EPILOG FR

; address_checksum(rcx = 36 bytes) -> ax (2 checksum bytes, little-endian order)
address_checksum:
    xor r9d, r9d                ; even bytes
    xor r10d, r10d              ; odd bytes
    xor edx, edx
.l:
    xor r9b, [rcx + rdx]
    xor r10b, [rcx + rdx + 1]
    add edx, 2
    cmp edx, 36
    jb .l
    mov eax, r10d
    shl eax, 8
    or eax, r9d
    ret

; int toxnet_add_friend(rcx = tox id hex, rdx = request message)
toxnet_add_friend:
%assign FR 64
    PROLOG FR
    mov rsi, rcx
    mov r12, rdx
    mov rcx, rsi
    call str_len
    cmp rax, 76
    jb .bad
    lea rcx, [LOC(0)]
    mov rdx, rsi
    mov r8d, 38
    call hex_decode
    test eax, eax
    jz .bad
    lea rcx, [LOC(0)]
    call address_checksum
    cmp ax, [LOC(36)]
    jne .bad
    lea rcx, [LOC(0)]
    lea rdx, [rel g_realpk]
    mov r8d, 32
    call mem_eq
    test eax, eax
    jnz .bad
    lea rcx, [LOC(0)]
    call friend_by_realpk
    cmp rax, -1
    jne .bad
    ; free slot
    lea rbx, [rel friends]
    xor edi, edi
.f:
    cmp byte [rbx + FRIEND.used], 0
    je .take
    add rbx, FRIEND_size
    inc edi
    cmp edi, MAX_FRIENDS
    jb .f
    jmp .bad
.take:
    mov rcx, rbx
    mov edx, FRIEND_size
    call mem_zero
    mov byte [rbx + FRIEND.used], 1
    mov byte [rbx + FRIEND.sendreq], 1
    lea rcx, [rbx + FRIEND.realpk]
    lea rdx, [LOC(0)]
    mov r8d, 32
    call mem_cpy
    mov eax, [LOC(32)]
    mov [rbx + FRIEND.nospam], eax
    ; request message (default "Hi", max 255 bytes)
    test r12, r12
    jz .defmsg
    mov rcx, r12
    call str_len
    test eax, eax
    jz .defmsg
    cmp eax, 255
    jbe .ml
    mov eax, 255
.ml:
    mov [rbx + FRIEND.reqlen], eax
    lea rcx, [rbx + FRIEND.req]
    mov rdx, r12
    mov r8, rax
    call mem_cpy
    jmp .target
.defmsg:
    mov word [rbx + FRIEND.req], 'Hi'
    mov dword [rbx + FRIEND.reqlen], 2
.target:
    lea ecx, [rdi + 1]
    call target_ptr
    mov rsi, rax
    mov rcx, rsi
    mov edx, TARGET_size
    call mem_zero
    mov rcx, rsi
    lea rdx, [rbx + FRIEND.realpk]
    mov r8d, 32
    call mem_cpy
    lea rcx, [rsi + TARGET.sendpk]
    lea rdx, [rsi + TARGET.sendsk]
    call box_keypair
    test eax, eax
    jnz .rngfail
    mov eax, edi
    jmp .ret
.rngfail:
    mov rcx, rsi
    mov edx, TARGET_size
    call mem_zero
    mov rcx, rbx
    mov edx, FRIEND_size
    call mem_zero
.bad:
    mov rax, -1
.ret:
    EPILOG FR

; friends_tick(): friend requests, DHT key announcements, connections
friends_tick:
%assign FR 0
    PROLOG FR
    mov r15, [rel g_now]
    lea rbx, [rel friends]
    xor edi, edi
.l:
    cmp byte [rbx + FRIEND.used], 0
    je .n
    ; found = some onion node holds the friend's announcement
    lea ecx, [rdi + 1]
    call target_ptr
    lea rsi, [rax + TARGET.nodes]
    xor r12d, r12d
    xor ecx, ecx
.fnd:
    cmp byte [rsi + ONODE.used], 0
    je .fn
    cmp byte [rsi + ONODE.stored], 1
    jne .fn
    mov r12d, 1
.fn:
    add rsi, ONODE_size
    inc ecx
    cmp ecx, ONION_NODES
    jb .fnd
    test r12d, r12d
    jz .conn
    ; friend request until they accept
    cmp byte [rbx + FRIEND.sendreq], 0
    je .dht
    cmp byte [rbx + FRIEND.everonline], 0
    jne .dht
    mov rax, r15
    sub rax, [rbx + FRIEND.lastreqsent]
    cmp rax, 20000
    jb .dht
    lea rsi, [rel buf_freq]
    mov byte [rsi], ONION_DATA_FRIEND_REQ
    mov eax, [rbx + FRIEND.nospam]
    mov [rsi + 1], eax
    lea rcx, [rsi + 5]
    lea rdx, [rbx + FRIEND.req]
    mov r8d, [rbx + FRIEND.reqlen]
    call mem_cpy
    mov r8d, [rbx + FRIEND.reqlen]
    add r8d, 5
    mov ecx, edi
    mov rdx, rsi
    call send_onion_data
    test eax, eax
    jz .dht
    mov [rbx + FRIEND.lastreqsent], r15
.dht:
    ; our DHT key + relays: every 10 s while offline, every minute while online
    mov ecx, 10000
    cmp byte [rbx + FRIEND.online], 0
    je .di
    mov ecx, 60000
.di:
    mov rax, r15
    sub rax, [rbx + FRIEND.lastdhtpk]
    cmp rax, rcx
    jb .conn
    mov ecx, edi
    call send_dhtpk
    test eax, eax
    jz .conn
    mov [rbx + FRIEND.lastdhtpk], r15
.conn:
    mov rcx, rbx
    call crypto_tick
.n:
    add rbx, FRIEND_size
    inc edi
    cmp edi, MAX_FRIENDS
    jb .l
    EPILOG FR

; void toxnet_iterate(void)
toxnet_iterate:
%assign FR 0
    PROLOG FR
    call now_ms
    mov [rel g_now], rax
    call relays_tick
    call file_pump_all          ; send file chunks every iterate, not only at 250ms
    call now_ms
    mov [rel g_now], rax
    sub rax, [rel g_lasttick]
    cmp rax, 250
    jb .flush
    mov rax, [rel g_now]
    mov [rel g_lasttick], rax
    call onion_tick
    call friends_tick
.flush:
    call relays_flush_all
    EPILOG FR

; int toxnet_friend_online(ecx = friend)
toxnet_friend_online:
    xor eax, eax
    cmp ecx, MAX_FRIENDS
    jae .r
    sub rsp, 40
    call friend_ptr
    add rsp, 40
    movzx eax, byte [rax + FRIEND.online]
.r:
    ret

; int toxnet_send_message(ecx = friend, rdx = msg, r8d = len) -> 0 / -1
toxnet_send_message:
%assign FR 1392
    PROLOG FR
    mov rsi, rdx
    mov r12d, r8d
    mov rax, -1
    cmp ecx, MAX_FRIENDS
    jae .ret
    test r12d, r12d
    jz .ret
    cmp r12d, MAX_DATA - 1
    ja .ret
    call friend_ptr
    mov rbx, rax
    mov rax, -1
    cmp byte [rbx + FRIEND.used], 0
    je .ret
    cmp byte [rbx + FRIEND.online], 0
    je .ret
    cmp byte [rbx + FRIEND.cstatus], CS_ESTAB
    jne .ret
    mov byte [LOC(0)], PKT_MESSAGE
    lea rcx, [LOC(1)]
    mov rdx, rsi
    mov r8d, r12d
    call mem_cpy
    mov rcx, rbx
    lea rdx, [LOC(0)]
    lea r8, [r12 + 1]
    call send_lossless
    mov r13, rax                ; 0 or -1
    call relays_flush_all
    mov rax, r13
.ret:
    EPILOG FR

; int toxnet_receive_message(rcx = buf, edx = buflen, r8 = int *friend) -> len / -1
toxnet_receive_message:
%assign FR 0
    PROLOG FR
    mov rdi, rcx
    mov r12d, edx
    mov r13, r8
    mov rax, -1
    cmp dword [rel g_incount], 0
    je .ret
    mov eax, [rel g_inhead]
    and eax, INBOX_SIZE - 1
    imul rax, rax, INMSG_size
    lea rsi, [rel inbox]
    add rsi, rax
    mov eax, [rsi + INMSG.friend]
    test r13, r13
    jz .nof
    mov [r13], eax
.nof:
    mov ebx, [rsi + INMSG.len]
    cmp ebx, r12d
    jbe .cp
    mov ebx, r12d
.cp:
    mov rcx, rdi
    lea rdx, [rsi + INMSG.text]
    mov r8d, ebx
    call mem_cpy
    inc dword [rel g_inhead]
    dec dword [rel g_incount]
    mov eax, ebx
.ret:
    EPILOG FR

; int toxnet_friend_name(ecx = friend, rdx = buf, r8d = buflen) -> len
toxnet_friend_name:
%assign FR 0
    PROLOG FR
    mov rdi, rdx
    mov r12d, r8d
    xor eax, eax
    cmp ecx, MAX_FRIENDS
    jae .ret
    call friend_ptr
    mov rsi, rax
    mov ebx, [rsi + FRIEND.namelen]
    cmp ebx, r12d
    jbe .cp
    mov ebx, r12d
.cp:
    mov rcx, rdi
    lea rdx, [rsi + FRIEND.name]
    mov r8d, ebx
    call mem_cpy
    mov eax, ebx
.ret:
    EPILOG FR

; void toxnet_shutdown(void)
toxnet_shutdown:
%assign FR 0
    PROLOG FR
    cmp byte [rel g_started], 0
    je .ret
    lea rbx, [rel relays]
    xor edi, edi
.l:
    cmp dword [rbx + RELAY.state], RS_CONNECT
    jb .n
    mov rcx, [rbx + RELAY.sock]
    call WINAPI(closesocket)
    mov dword [rbx + RELAY.state], RS_IDLE
.n:
    add rbx, RELAY_size
    inc edi
    cmp edi, MAX_RELAYS
    jb .l
    lea rbx, [rel friends]
    xor edi, edi
.files:
    mov rcx, rbx
    call file_break
    add rbx, FRIEND_size
    inc edi
    cmp edi, MAX_FRIENDS
    jb .files
    call WINAPI(WSACleanup)
    ; Clear all per-instance keys/state so a subsequent init starts fresh.
    lea rcx, [rel crypto_scratch]
    mov edx, state_end - crypto_scratch
    call mem_zero
    call runtime_shutdown
.ret:
    EPILOG FR

section .bss
alignb 16
g_realpk:    resb 32
g_realsk:    resb 32
g_dhtpk:     resb 32
g_dhtsk:     resb 32
g_onionpk:   resb 32
g_onionsk:   resb 32
g_cookiekey: resb 32
g_ck_real:   resb 32        ; filled by open_cookie
g_ck_dht:    resb 32
g_nospam:    resb 4
g_namelen:   resd 1
g_pathgen:   resd 1
g_pathnext:  resd 1
g_inhead:    resd 1
g_incount:   resd 1
g_now:       resq 1
g_lasttick:  resq 1
g_noreplay:  resq 1
g_name:      resb 128
wsadata:     resb 512
fds_rd:      resb 8 + 8 * 64
fds_wr:      resb 8 + 8 * 64
fds_ex:      resb 8 + 8 * 64
frame_plain: resb 4160
onion_plain: resb 2048
onion_plain2: resb 2048
data_plain:  resb 2048
relay_out:   resb 2112
buf_s1:      resb 2048
buf_s2:      resb 2112
buf_op:      resb 2176
buf_ann:     resb 256
buf_inner:   resb 1600
buf_req:     resb 1700
buf_dhtpk:   resb 256
buf_hs:      resb 256
buf_freq:    resb 320
nc_plain:    resb 1536
nc_pkt:      resb 1536
alignb 16
relays:      resb RELAY_size * MAX_RELAYS
paths:       resb PATH_size * MAX_PATHS
targets:     resb TARGET_size * MAX_TARGETS
pending:     resb PEND_size * MAX_PENDING
friends:     resb FRIEND_size * MAX_FRIENDS
inbox:       resb INMSG_size * INBOX_SIZE
offers:      resb OFFER_size * OFFER_SIZE
g_started:   resb 1
state_end:
