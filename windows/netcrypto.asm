; net_crypto: cookie / handshake / encrypted data with reliable delivery,
; plus the friend_connection and Messenger packets on top of it.

PKT_ALIVE        equ 16
PKT_SHARE_RELAYS equ 17
PKT_ONLINE       equ 24
PKT_OFFLINE      equ 25
PKT_NICKNAME     equ 48
PKT_USERSTATUS   equ 50
PKT_MESSAGE      equ 64
PKT_ACTION       equ 65

section .text

; conn_by_peer(rcx = DHT pk) -> rax = FRIEND* with a connection to that DHT key, or 0
conn_by_peer:
%assign FR 0
    PROLOG FR
    mov rsi, rcx
    lea rbx, [rel friends]
    xor edi, edi
.l:
    cmp byte [rbx + FRIEND.used], 0
    je .n
    cmp byte [rbx + FRIEND.cstatus], CS_NONE
    je .n
    lea rcx, [rbx + FRIEND.peerdht]
    mov rdx, rsi
    mov r8d, 32
    call mem_eq
    test eax, eax
    jz .n
    mov rax, rbx
    jmp .ret
.n:
    add rbx, FRIEND_size
    inc edi
    cmp edi, MAX_FRIENDS
    jb .l
    xor eax, eax
.ret:
    EPILOG FR

; request_routes_all(rcx = pk): ask every ready relay to route us to pk
request_routes_all:
%assign FR 0
    PROLOG FR
    mov rsi, rcx
    lea rbx, [rel relays]
    xor edi, edi
.l:
    mov rcx, rbx
    mov rdx, rsi
    call relay_request_route    ; no-op unless the relay is ready
    add rbx, RELAY_size
    inc edi
    cmp edi, MAX_RELAYS
    jb .l
    EPILOG FR

; send_to_peer(rcx = DHT pk, rdx = packet, r8 = len) -> rax = 1 if sent
;   routed when possible; cookie/handshake packets fall back to out-of-band
;   (toxcore drops out-of-band data packets, so those are not sent that way)
send_to_peer:
%assign FR 0
    PROLOG FR
    mov rsi, rcx
    mov r12, rdx
    mov r13, r8
    mov rcx, rsi
    call any_route
    test rax, rax
    jz .oob
    mov rcx, rax
    mov rdx, rsi
    mov r8, r12
    mov r9, r13
    call relay_send_to
    mov eax, 1
    jmp .ret
.oob:
    xor r14d, r14d
    cmp byte [r12], 0x1b
    je .done
    lea rbx, [rel relays]
    xor edi, edi
.l:
    cmp dword [rbx + RELAY.state], RS_READY
    jne .n
    mov rcx, rbx
    mov rdx, rsi
    mov r8, r12
    mov r9, r13
    call relay_send_to
    mov r14d, 1
.n:
    add rbx, RELAY_size
    inc edi
    cmp edi, MAX_RELAYS
    jb .l
.done:
    mov eax, r14d
.ret:
    EPILOG FR

; make_cookie(rcx = out112, rdx = real pk, r8 = DHT pk) -> 0 / -1
make_cookie:
%assign FR 80
    PROLOG FR
    mov rdi, rcx
    mov rsi, rdx
    mov r12, r8
    call unix_time
    mov [LOC(0)], rax
    lea rcx, [LOC(8)]
    mov rdx, rsi
    mov r8d, 32
    call mem_cpy
    lea rcx, [LOC(40)]
    mov rdx, r12
    mov r8d, 32
    call mem_cpy
    mov rcx, rdi
    mov edx, 24
    call rand_bytes
    test eax, eax
    jnz .ret
    lea rcx, [rdi + 24]
    lea rdx, [rel g_cookiekey]
    mov r8, rdi
    lea r9, [LOC(0)]
    mov qword [rsp+32], 72
    call box_seal
    xor eax, eax
.ret:
    EPILOG FR

; open_cookie(rcx = cookie112) -> rax = 1 ok (g_ck_real / g_ck_dht filled)
open_cookie:
%assign FR 80
    PROLOG FR
    mov rsi, rcx
    lea rcx, [LOC(0)]
    lea rdx, [rel g_cookiekey]
    mov r8, rsi
    lea r9, [rsi + 24]
    mov qword [rsp+32], 88
    mov qword [rsp+40], 72
    call box_open
    xor ebx, ebx
    cmp rax, 72
    jne .ret
    call unix_time
    mov rcx, [LOC(0)]
    cmp rcx, rax
    ja .ret
    add rcx, 15
    cmp rcx, rax
    jb .ret
    lea rcx, [rel g_ck_real]
    lea rdx, [LOC(8)]
    mov r8d, 64                 ; real pk then DHT pk
    call mem_cpy
    mov ebx, 1
.ret:
    mov eax, ebx
    EPILOG FR

; make_handshake(rcx = friend, rdx = their cookie) -> 0 / -1
; Build the handshake into friend.temp; never leave a failed packet sendable.
make_handshake:
%assign FR 288
    PROLOG FR
    mov rbx, rcx
    mov rsi, rdx
    ; plain = sent nonce | session pk | sha512(their cookie) | our cookie   (232 bytes)
    lea rcx, [LOC(0)]
    lea rdx, [rbx + FRIEND.sentnonce]
    mov r8d, 24
    call mem_cpy
    lea rcx, [LOC(24)]
    lea rdx, [rbx + FRIEND.sesspk]
    mov r8d, 32
    call mem_cpy
    mov rcx, rsi
    mov edx, 112
    lea r8, [LOC(56)]
    call sha512
    lea rcx, [LOC(120)]
    lea rdx, [rbx + FRIEND.realpk]
    lea r8, [rbx + FRIEND.peerdht]
    call make_cookie
    test eax, eax
    jnz .bad
    lea rcx, [LOC(240)]
    lea rdx, [rbx + FRIEND.realpk]
    lea r8, [rel g_realsk]
    call box_beforenm
    ; packet = [0x1a] their cookie, nonce, seal
    lea rdi, [rbx + FRIEND.temp]
    mov byte [rdi], 0x1a
    lea rcx, [rdi + 1]
    mov rdx, rsi
    mov r8d, 112
    call mem_cpy
    lea rcx, [rdi + 113]
    mov edx, 24
    call rand_bytes
    test eax, eax
    jnz .bad
    lea rcx, [rdi + 137]
    lea rdx, [LOC(240)]
    lea r8, [rdi + 113]
    lea r9, [LOC(0)]
    mov qword [rsp+32], 232
    call box_seal
    mov dword [rbx + FRIEND.templen], 385
    xor eax, eax
    jmp .ret
.bad:
    mov dword [rbx + FRIEND.templen], 0
    mov rax, -1
.ret:
    EPILOG FR

; conn_new(rcx = friend, edx = 1 to send the cookie request now) -> 0 / -1
conn_new:
%assign FR 96
    PROLOG FR
    mov rbx, rcx
    mov r12d, edx
    mov byte [rbx + FRIEND.cstatus], CS_COOKIE
    lea rcx, [rbx + FRIEND.sesspk]
    lea rdx, [rbx + FRIEND.sesssk]
    call box_keypair
    test eax, eax
    jnz .bad
    lea rcx, [rbx + FRIEND.sentnonce]
    mov edx, 24
    call rand_bytes
    test eax, eax
    jnz .bad
    lea rcx, [rbx + FRIEND.dhtshared]
    lea rdx, [rbx + FRIEND.peerdht]
    lea r8, [rel g_dhtsk]
    call box_beforenm
    lea rcx, [rbx + FRIEND.cookienum]
    mov edx, 8
    call rand_bytes
    test eax, eax
    jnz .bad
    xor eax, eax
    mov [rbx + FRIEND.sendstart], eax
    mov [rbx + FRIEND.sendend], eax
    mov [rbx + FRIEND.recvstart], eax
    mov [rbx + FRIEND.recvend], eax
    mov [rbx + FRIEND.temptries], eax
    mov [rbx + FRIEND.tempsent], rax
    mov [rbx + FRIEND.lastrequest], rax
    mov [rbx + FRIEND.lastalive], rax
    mov [rbx + FRIEND.lastalivesent], rax
    mov [rbx + FRIEND.lastshare], rax
    ; clear the receive window
    lea rdi, [rbx + FRIEND.rbuf]
    xor ecx, ecx
.clr:
    mov dword [rdi + RPKT.present], 0
    add rdi, RPKT_size
    inc ecx
    cmp ecx, WINDOW
    jb .clr
    ; cookie request = [0x18] our DHT pk, nonce, seal(real pk | 32 zero | cookie number)
    lea rcx, [LOC(0)]
    mov edx, 72
    call mem_zero
    lea rcx, [LOC(0)]
    lea rdx, [rel g_realpk]
    mov r8d, 32
    call mem_cpy
    mov rax, [rbx + FRIEND.cookienum]
    mov [LOC(64)], rax
    lea rdi, [rbx + FRIEND.temp]
    mov byte [rdi], 0x18
    lea rcx, [rdi + 1]
    lea rdx, [rel g_dhtpk]
    mov r8d, 32
    call mem_cpy
    lea rcx, [rdi + 33]
    mov edx, 24
    call rand_bytes
    test eax, eax
    jnz .bad
    lea rcx, [rdi + 57]
    lea rdx, [rbx + FRIEND.dhtshared]
    lea r8, [rdi + 33]
    lea r9, [LOC(0)]
    mov qword [rsp+32], 72
    call box_seal
    mov dword [rbx + FRIEND.templen], 145
    test r12d, r12d
    jz .ok
    mov rcx, rbx
    call send_temp
.ok:
    xor eax, eax
    jmp .ret
.bad:
    mov byte [rbx + FRIEND.cstatus], CS_NONE
    mov byte [rbx + FRIEND.online], 0
    mov dword [rbx + FRIEND.templen], 0
    lea rcx, [rbx + FRIEND.sesspk]
    mov edx, FRIEND.req - FRIEND.sesspk
    call mem_zero
    mov rax, -1
.ret:
    EPILOG FR

; send_temp(rcx = friend): (re)send the pending cookie request / handshake
send_temp:
%assign FR 0
    PROLOG FR
    mov rbx, rcx
    mov rax, [rel g_now]
    mov [rbx + FRIEND.tempsent], rax
    inc dword [rbx + FRIEND.temptries]
    mov r8d, [rbx + FRIEND.templen]
    test r8d, r8d
    jz .ret
    lea rcx, [rbx + FRIEND.peerdht]
    lea rdx, [rbx + FRIEND.temp]
    call send_to_peer
.ret:
    EPILOG FR

; kill_conn(rcx = friend)
kill_conn:
%assign FR 16
    PROLOG FR
    mov rbx, rcx
    cmp byte [rbx + FRIEND.cstatus], CS_NONE
    je .ret
    cmp byte [rbx + FRIEND.cstatus], CS_NOTCONF
    jb .off
    mov byte [LOC(0)], 2        ; PACKET_ID_KILL
    mov rcx, rbx
    mov edx, [rbx + FRIEND.recvstart]
    mov r8d, [rbx + FRIEND.sendend]
    lea r9, [LOC(0)]
    mov qword [rsp+32], 1
    call send_data_packet
.off:
    mov byte [rbx + FRIEND.cstatus], CS_NONE
    mov byte [rbx + FRIEND.online], 0
    mov rcx, rbx
    call file_break             ; close files and drop transfers for this friend
.ret:
    EPILOG FR

; set_friend_dht(rcx = friend, rdx = DHT pk): (re)connect using a new DHT key
set_friend_dht:
%assign FR 0
    PROLOG FR
    mov rbx, rcx
    mov rsi, rdx
    cmp byte [rbx + FRIEND.hasdht], 0
    je .set
    cmp byte [rbx + FRIEND.cstatus], CS_NONE
    je .set
    lea rcx, [rbx + FRIEND.dhtpk]
    mov rdx, rsi
    mov r8d, 32
    call mem_eq
    test eax, eax
    jnz .ret
.set:
    mov rcx, rbx
    call kill_conn
    lea rcx, [rbx + FRIEND.dhtpk]
    mov rdx, rsi
    mov r8d, 32
    call mem_cpy
    lea rcx, [rbx + FRIEND.peerdht]
    mov rdx, rsi
    mov r8d, 32
    call mem_cpy
    mov byte [rbx + FRIEND.hasdht], 1
    lea rcx, [rbx + FRIEND.dhtpk]
    call request_routes_all
    mov rcx, rbx
    mov edx, 1
    call conn_new
.ret:
    EPILOG FR

; peer_route_online(rcx = DHT pk): a relay just connected us to this peer
peer_route_online:
%assign FR 0
    PROLOG FR
    call conn_by_peer
    test rax, rax
    jz .ret
    mov rbx, rax
    mov rcx, rbx
    cmp byte [rbx + FRIEND.cstatus], CS_ESTAB
    je .estab
    call send_temp
    jmp .ret
.estab:
    call resend_unacked
.ret:
    EPILOG FR

; handle_crypto(rcx = relay, rdx = sender DHT pk, r8 = packet, r9 = len)
handle_crypto:
%assign FR 336
    PROLOG FR
    mov r15, rcx
    mov rsi, rdx
    mov rdi, r8
    mov r12, r9
    test r12, r12
    jz .ret
    movzx eax, byte [rdi]
    cmp eax, 0x18
    je .cookiereq
    cmp eax, 0x19
    je .cookieresp
    cmp eax, 0x1a
    je .handshake
    cmp eax, 0x1b
    je .data
    jmp .ret

.cookiereq:                     ; answer anyone: [0x19] nonce seal(cookie | number)
    cmp r12, 145
    jne .ret
    lea rcx, [rdi + 1]
    mov rdx, rsi
    mov r8d, 32
    call mem_eq
    test eax, eax
    jz .ret
    lea rcx, [LOC(0)]           ; shared
    mov rdx, rsi
    lea r8, [rel g_dhtsk]
    call box_beforenm
    lea rcx, [LOC(32)]          ; request plain (72)
    lea rdx, [LOC(0)]
    lea r8, [rdi + 33]
    lea r9, [rdi + 57]
    mov qword [rsp+32], 88
    mov qword [rsp+40], 72
    call box_open
    cmp rax, 72
    jne .ret
    lea rcx, [LOC(112)]         ; response plain = cookie(112) | number(8)
    lea rdx, [LOC(32)]
    mov r8, rsi
    call make_cookie
    test eax, eax
    jnz .ret
    mov rax, [LOC(32 + 64)]
    mov [LOC(224)], rax
    lea rbx, [rel buf_hs]
    mov byte [rbx], 0x19
    lea rcx, [rbx + 1]
    mov edx, 24
    call rand_bytes
    test eax, eax
    jnz .ret
    lea rcx, [rbx + 25]
    lea rdx, [LOC(0)]
    lea r8, [rbx + 1]
    lea r9, [LOC(112)]
    mov qword [rsp+32], 120
    call box_seal
    mov rcx, r15
    mov rdx, rsi
    mov r8, rbx
    mov r9d, 161
    call relay_send_to
    jmp .ret

.cookieresp:
    cmp r12, 161
    jne .ret
    mov rcx, rsi
    call conn_by_peer
    test rax, rax
    jz .ret
    mov rbx, rax
    cmp byte [rbx + FRIEND.cstatus], CS_COOKIE
    jne .ret
    lea rcx, [LOC(0)]
    lea rdx, [rbx + FRIEND.dhtshared]
    lea r8, [rdi + 1]
    lea r9, [rdi + 25]
    mov qword [rsp+32], 136
    mov qword [rsp+40], 120
    call box_open
    cmp rax, 120
    jne .ret
    mov rax, [LOC(112)]
    cmp rax, [rbx + FRIEND.cookienum]
    jne .ret
    mov rcx, rbx
    lea rdx, [LOC(0)]
    call make_handshake
    test eax, eax
    jnz .connfail
    mov byte [rbx + FRIEND.cstatus], CS_HSSENT
    mov dword [rbx + FRIEND.temptries], 0
    mov rcx, rbx
    call send_temp
    jmp .ret

.handshake:
    cmp r12, 385
    jne .ret
    lea rcx, [rdi + 1]
    call open_cookie
    test eax, eax
    jz .ret
    lea rcx, [rel g_ck_real]
    call friend_by_realpk
    cmp rax, -1
    je .ret
    mov ecx, eax
    call friend_ptr
    mov rbx, rax
    lea rcx, [LOC(0)]
    lea rdx, [rel g_ck_real]
    lea r8, [rel g_realsk]
    call box_beforenm
    lea rcx, [LOC(32)]          ; plain (232)
    lea rdx, [LOC(0)]
    lea r8, [rdi + 113]
    lea r9, [rdi + 137]
    mov qword [rsp+32], 248
    mov qword [rsp+40], 232
    call box_open
    cmp rax, 232
    jne .ret
    lea rcx, [rdi + 1]
    mov edx, 112
    lea r8, [LOC(264)]
    call sha512
    lea rcx, [LOC(264)]
    lea rdx, [LOC(32 + 56)]
    mov r8d, 64
    call mem_eq
    test eax, eax
    jz .ret
    ; new connection if we have none, or the peer's DHT key changed
    cmp byte [rbx + FRIEND.cstatus], CS_NONE
    je .hsnew
    lea rcx, [rbx + FRIEND.peerdht]
    lea rdx, [rel g_ck_dht]
    mov r8d, 32
    call mem_eq
    test eax, eax
    jnz .hshave
.hsnew:
    mov rcx, rbx
    call kill_conn
    lea rcx, [rbx + FRIEND.dhtpk]
    lea rdx, [rel g_ck_dht]
    mov r8d, 32
    call mem_cpy
    lea rcx, [rbx + FRIEND.peerdht]
    lea rdx, [rel g_ck_dht]
    mov r8d, 32
    call mem_cpy
    mov byte [rbx + FRIEND.hasdht], 1
    lea rcx, [rbx + FRIEND.dhtpk]
    call request_routes_all
    mov rcx, rbx
    xor edx, edx
    call conn_new
    test eax, eax
    jnz .ret
.hshave:
    cmp byte [rbx + FRIEND.cstatus], CS_ESTAB
    je .ret
    lea rcx, [rbx + FRIEND.recvnonce]
    lea rdx, [LOC(32)]
    mov r8d, 24
    call mem_cpy
    lea rcx, [rbx + FRIEND.shared]
    lea rdx, [LOC(32 + 24)]
    lea r8, [rbx + FRIEND.sesssk]
    call box_beforenm
    movzx r13d, byte [rbx + FRIEND.cstatus]
    mov rcx, rbx
    lea rdx, [LOC(32 + 120)]
    call make_handshake
    test eax, eax
    jnz .connfail
    mov byte [rbx + FRIEND.cstatus], CS_NOTCONF
    mov dword [rbx + FRIEND.temptries], 0
    cmp r13d, CS_COOKIE
    jne .ret
    mov rcx, rbx
    call send_temp
    jmp .ret

.data:
    mov rcx, rsi
    call conn_by_peer
    test rax, rax
    jz .ret
    cmp byte [rax + FRIEND.cstatus], CS_NOTCONF
    jb .ret
    mov rcx, rax
    mov rdx, rdi
    mov r8, r12
    call handle_data
    jmp .ret
.connfail:
    mov rcx, rbx
    call kill_conn
.ret:
    EPILOG FR

; send_data_packet(rcx = friend, edx = ack, r8d = number, r9 = payload, [5] = len) -> rax = 1 sent
send_data_packet:
%assign FR 32
    PROLOG FR
    mov rbx, rcx
    mov r12d, edx
    mov r13d, r8d
    mov rsi, r9
    mov r14, [ARG5]
    lea rcx, [rbx + FRIEND.peerdht]
    call any_route
    test rax, rax
    jz .ret                     ; rax = 0
    ; plain = ack BE32 | number BE32 | zero padding | payload
    lea rdi, [rel nc_plain]
    mov eax, r12d
    bswap eax
    mov [rdi], eax
    mov eax, r13d
    bswap eax
    mov [rdi + 4], eax
    mov eax, MAX_DATA
    sub rax, r14
    and eax, 7
    mov r15, rax                ; padding
    lea rcx, [rdi + 8]
    mov rdx, r15
    call mem_zero
    lea rcx, [rdi + 8 + r15]
    mov rdx, rsi
    mov r8, r14
    call mem_cpy
    lea r14, [r14 + r15 + 8]    ; plain length
    lea rsi, [rel nc_pkt]
    mov byte [rsi], 0x1b
    mov ax, [rbx + FRIEND.sentnonce + 22]
    mov [rsi + 1], ax
    lea rcx, [rsi + 3]
    lea rdx, [rbx + FRIEND.shared]
    lea r8, [rbx + FRIEND.sentnonce]
    mov r9, rdi
    mov [rsp+32], r14
    call box_seal
    lea r14, [rax + 3]
    lea rcx, [rbx + FRIEND.sentnonce]
    mov edx, 1
    call inc_nonce
    lea rcx, [rbx + FRIEND.peerdht]
    mov rdx, rsi
    mov r8, r14
    call send_to_peer
.ret:
    EPILOG FR

; send_lossless(rcx = friend, rdx = payload, r8 = len) -> rax = 0 queued, -1 window full
send_lossless:
%assign FR 0
    PROLOG FR
    mov rbx, rcx
    mov rsi, rdx
    mov r12, r8
    mov eax, [rbx + FRIEND.sendend]
    sub eax, [rbx + FRIEND.sendstart]
    cmp eax, WINDOW
    mov rax, -1
    jae .ret
    mov r13d, [rbx + FRIEND.sendend]
    mov eax, r13d
    and eax, WINDOW - 1
    imul rax, rax, SPKT_size
    lea rdi, [rbx + FRIEND.sbuf + rax]
    mov [rdi + SPKT.len], r12d
    mov qword [rdi + SPKT.sent], 0
    lea rcx, [rdi + SPKT.data]
    mov rdx, rsi
    mov r8, r12
    call mem_cpy
    inc dword [rbx + FRIEND.sendend]
    mov rcx, rbx
    mov edx, [rbx + FRIEND.recvstart]
    mov r8d, r13d
    lea r9, [rdi + SPKT.data]
    mov [rsp+32], r12
    call send_data_packet
    test eax, eax
    jz .q
    mov rax, [rel g_now]
    mov [rdi + SPKT.sent], rax
.q:
    xor eax, eax
.ret:
    EPILOG FR

; resend_unacked(rcx = friend): resend reliable packets not acknowledged within 2 s
resend_unacked:
%assign FR 0
    PROLOG FR
    mov rbx, rcx
    mov r12d, [rbx + FRIEND.sendstart]
    xor r13d, r13d              ; sent this round
.l:
    cmp r12d, [rbx + FRIEND.sendend]
    je .ret
    cmp r13d, 32
    jae .ret
    mov eax, r12d
    and eax, WINDOW - 1
    imul rax, rax, SPKT_size
    lea rdi, [rbx + FRIEND.sbuf + rax]
    mov rax, [rel g_now]
    sub rax, [rdi + SPKT.sent]
    cmp rax, 2000
    jb .n
    mov rcx, rbx
    mov edx, [rbx + FRIEND.recvstart]
    mov r8d, r12d
    lea r9, [rdi + SPKT.data]
    mov eax, [rdi + SPKT.len]
    mov [rsp+32], rax
    call send_data_packet
    test eax, eax
    jz .ret
    mov rax, [rel g_now]
    mov [rdi + SPKT.sent], rax
    inc r13d
.n:
    inc r12d
    jmp .l
.ret:
    EPILOG FR

; send_request(rcx = friend): PACKET_ID_REQUEST listing reliable packets we're missing
send_request:
%assign FR 160
    PROLOG FR
    mov rbx, rcx
    mov rax, [rel g_now]
    mov [rbx + FRIEND.lastrequest], rax
    mov byte [LOC(0)], 1
    mov r13d, 1                 ; length
    mov r14d, 1                 ; n
    mov r12d, [rbx + FRIEND.recvstart]
.l:
    cmp r12d, [rbx + FRIEND.recvend]
    je .send
    cmp r13d, 128
    jae .send
    mov eax, r12d
    and eax, WINDOW - 1
    imul rax, rax, RPKT_size
    cmp dword [rbx + FRIEND.rbuf + rax + RPKT.present], 0
    jne .have
    mov [LOC(0) + r13], r14b
    inc r13d
    xor r14d, r14d
    jmp .nx
.have:
    cmp r14d, 255
    jne .nx
    mov byte [LOC(0) + r13], 0
    inc r13d
    xor r14d, r14d
.nx:
    inc r14d
    inc r12d
    jmp .l
.send:
    mov rcx, rbx
    mov edx, [rbx + FRIEND.recvstart]
    mov r8d, [rbx + FRIEND.sendend]
    lea r9, [LOC(0)]
    mov [rsp+32], r13
    call send_data_packet
    EPILOG FR

; handle_request(rcx = friend, rdx = request, r8 = len): resend what the peer is missing
handle_request:
%assign FR 0
    PROLOG FR
    mov rbx, rcx
    lea rsi, [rdx + 1]
    lea r12, [r8 - 1]
    mov r13d, 1                 ; n
    mov r14d, [rbx + FRIEND.sendstart]
.l:
    test r12, r12
    jz .ret
    cmp r14d, [rbx + FRIEND.sendend]
    je .ret
    movzx eax, byte [rsi]
    cmp r13d, eax
    jne .next
    ; packet r14d requested
    mov eax, r14d
    and eax, WINDOW - 1
    imul rax, rax, SPKT_size
    lea rdi, [rbx + FRIEND.sbuf + rax]
    mov rax, [rel g_now]
    sub rax, [rdi + SPKT.sent]
    cmp rax, 1000
    jb .skip
    mov rcx, rbx
    mov edx, [rbx + FRIEND.recvstart]
    mov r8d, r14d
    lea r9, [rdi + SPKT.data]
    mov eax, [rdi + SPKT.len]
    mov [rsp+32], rax
    call send_data_packet
    mov rax, [rel g_now]
    mov [rdi + SPKT.sent], rax
.skip:
    inc rsi
    dec r12
    xor r13d, r13d
.next:
    cmp r13d, 255
    jne .inc
    mov r13d, 1
    test r12, r12
    jz .ret
    cmp byte [rsi], 0
    jne .ret
    inc rsi
    dec r12
    jmp .adv
.inc:
    inc r13d
.adv:
    inc r14d
    jmp .l
.ret:
    EPILOG FR

; share_relays(rcx = friend): PACKET_ID_SHARE_RELAYS with up to 3 relays
share_relays:
%assign FR 160
    PROLOG FR
    mov rbx, rcx
    mov rax, [rel g_now]
    mov [rbx + FRIEND.lastshare], rax
    mov byte [LOC(0)], PKT_SHARE_RELAYS
    lea rcx, [LOC(1)]
    mov edx, 3
    call pack_relays
    test eax, eax
    jz .ret
    lea r8, [rax + 1]
    mov rcx, rbx
    lea rdx, [LOC(0)]
    call send_lossless
.ret:
    EPILOG FR

; on_established(rcx = friend): first data packet arrived
on_established:
%assign FR 160
    PROLOG FR
    mov rbx, rcx
    mov byte [rbx + FRIEND.cstatus], CS_ESTAB
    mov dword [rbx + FRIEND.templen], 0
    mov rax, [rel g_now]
    mov [rbx + FRIEND.lastalive], rax
    mov [rbx + FRIEND.lastalivesent], rax
    mov [rbx + FRIEND.lastrouted], rax
    mov byte [LOC(0)], PKT_ONLINE
    mov rcx, rbx
    lea rdx, [LOC(0)]
    mov r8d, 1
    call send_lossless
    ; qTox only shows a friend as online after receiving their user status
    mov byte [LOC(0)], PKT_USERSTATUS
    mov byte [LOC(1)], 0
    mov rcx, rbx
    lea rdx, [LOC(0)]
    mov r8d, 2
    call send_lossless
    mov byte [LOC(0)], PKT_NICKNAME
    lea rcx, [LOC(1)]
    lea rdx, [rel g_name]
    mov r8d, [rel g_namelen]
    call mem_cpy
    mov r8d, [rel g_namelen]
    inc r8d
    mov rcx, rbx
    lea rdx, [LOC(0)]
    call send_lossless
    mov byte [LOC(0)], PKT_ALIVE
    mov rcx, rbx
    lea rdx, [LOC(0)]
    mov r8d, 1
    call send_lossless
    mov rcx, rbx
    call share_relays
    EPILOG FR

; handle_data(rcx = friend, rdx = packet, r8 = len)
handle_data:
%assign FR 32
    PROLOG FR
    mov rbx, rcx
    mov rsi, rdx
    mov r12, r8
    cmp r12, 1 + 2 + 16
    jbe .ret
    ; nonce = recv nonce + (packet number - our base number)
    movzx eax, word [rsi + 1]
    rol ax, 8
    movzx ecx, word [rbx + FRIEND.recvnonce + 22]
    rol cx, 8
    sub ax, cx
    movzx r13d, ax              ; diff
    lea rcx, [LOC(0)]
    lea rdx, [rbx + FRIEND.recvnonce]
    mov r8d, 24
    call mem_cpy
    lea rcx, [LOC(0)]
    mov edx, r13d
    call inc_nonce
    lea rcx, [rel data_plain]
    lea rdx, [rbx + FRIEND.shared]
    lea r8, [LOC(0)]
    lea r9, [rsi + 3]
    lea rax, [r12 - 3]
    mov [rsp+32], rax
    mov qword [rsp+40], 2048    ; data_plain capacity
    call box_open
    cmp rax, 8
    jle .ret
    mov r12, rax
    cmp r13d, 2 * 21845
    jbe .nonceok
    lea rcx, [rbx + FRIEND.recvnonce]
    mov edx, 21845
    call inc_nonce
.nonceok:
    lea rsi, [rel data_plain]
    ; acknowledgement: everything before ack was received by the peer
    mov eax, [rsi]
    bswap eax
    mov ecx, eax
    sub ecx, [rbx + FRIEND.sendstart]
    mov edx, [rbx + FRIEND.sendend]
    sub edx, [rbx + FRIEND.sendstart]
    cmp ecx, edx
    ja .noack
    mov [rbx + FRIEND.sendstart], eax
.noack:
    mov r14d, [rsi + 4]
    bswap r14d                  ; packet number
    add rsi, 8
    sub r12, 8
.pad:
    test r12, r12
    jz .ret
    cmp byte [rsi], 0
    jne .padded
    inc rsi
    dec r12
    jmp .pad
.padded:
    cmp byte [rsi], 2           ; PACKET_ID_KILL
    jne .alive
    mov byte [rbx + FRIEND.cstatus], CS_COOKIE   ; below NOTCONF: no kill sent back
    mov rcx, rbx
    call kill_conn
    jmp .ret
.alive:
    cmp byte [rbx + FRIEND.cstatus], CS_NOTCONF
    jne .type
    mov rcx, rbx
    call on_established
.type:
    movzx eax, byte [rsi]
    cmp eax, 1
    je .request
    cmp eax, 16
    jb .ret
    cmp eax, 191
    ja .ret
    ; reliable packet: store in the window, deliver in order
    mov eax, r14d
    sub eax, [rbx + FRIEND.recvstart]
    cmp eax, WINDOW
    jae .ret
    mov eax, r14d
    and eax, WINDOW - 1
    imul rax, rax, RPKT_size
    lea rdi, [rbx + FRIEND.rbuf + rax]
    cmp r12, 1376
    ja .ret
    mov dword [rdi + RPKT.present], 1
    mov [rdi + RPKT.len], r12d
    lea rcx, [rdi + RPKT.data]
    mov rdx, rsi
    mov r8, r12
    call mem_cpy
    lea eax, [r14 + 1]
    mov ecx, eax
    sub ecx, [rbx + FRIEND.recvstart]
    mov edx, [rbx + FRIEND.recvend]
    sub edx, [rbx + FRIEND.recvstart]
    cmp ecx, edx
    jbe .deliver
    mov [rbx + FRIEND.recvend], eax
.deliver:
    mov eax, [rbx + FRIEND.recvstart]
    and eax, WINDOW - 1
    imul rax, rax, RPKT_size
    lea rdi, [rbx + FRIEND.rbuf + rax]
    cmp dword [rdi + RPKT.present], 0
    je .ret
    mov dword [rdi + RPKT.present], 0
    inc dword [rbx + FRIEND.recvstart]
    mov rcx, rbx
    lea rdx, [rdi + RPKT.data]
    mov r8d, [rdi + RPKT.len]
    call handle_friend_packet
    cmp byte [rbx + FRIEND.cstatus], CS_ESTAB
    je .deliver
    jmp .ret
.request:
    mov rcx, rbx
    mov rdx, rsi
    mov r8, r12
    call handle_request
.ret:
    EPILOG FR

; handle_friend_packet(rcx = friend, rdx = data, r8 = len)
handle_friend_packet:
%assign FR 0
    PROLOG FR
    mov rbx, rcx
    mov rsi, rdx
    mov r12, r8
    movzx eax, byte [rsi]
    cmp eax, PKT_ALIVE
    je .alive
    cmp eax, PKT_SHARE_RELAYS
    je .share
    cmp eax, PKT_ONLINE
    je .online
    cmp eax, PKT_OFFLINE
    je .offline
    cmp eax, PKT_NICKNAME
    je .nick
    cmp eax, PKT_MESSAGE
    je .msg
    cmp eax, PKT_ACTION
    je .msg
    cmp eax, 80
    je .filereq
    cmp eax, 81
    je .filectl
    cmp eax, 82
    je .filedata
    jmp .ret
.alive:
    mov rax, [rel g_now]
    mov [rbx + FRIEND.lastalive], rax
    jmp .ret
.share:
    lea rcx, [rsi + 1]
    lea rdx, [r12 - 1]
    mov r8, rbx
    call add_tcp_nodes
    jmp .ret
.online:
    cmp byte [rbx + FRIEND.online], 0
    jne .ret
    mov byte [rbx + FRIEND.online], 1
    mov byte [rbx + FRIEND.everonline], 1
    jmp .ret
.offline:
    mov byte [rbx + FRIEND.online], 0
    jmp .ret
.nick:
    lea rcx, [r12 - 1]
    cmp rcx, 128
    ja .ret
    mov [rbx + FRIEND.namelen], ecx
    mov r8, rcx
    lea rcx, [rbx + FRIEND.name]
    lea rdx, [rsi + 1]
    call mem_cpy
    jmp .ret
.msg:
    cmp r12, 1
    jbe .ret
    ; friend index from the pointer
    mov rax, rbx
    lea rcx, [rel friends]
    sub rax, rcx
    xor edx, edx
    mov ecx, FRIEND_size
    div rcx
    mov ecx, eax
    lea rdx, [rsi + 1]
    lea r8, [r12 - 1]
    call inbox_push
    jmp .ret
.filereq:
    mov rcx, rbx
    lea rdx, [rsi + 1]
    lea r8, [r12 - 1]
    call file_handle_sendreq
    jmp .ret
.filectl:
    mov rcx, rbx
    lea rdx, [rsi + 1]
    lea r8, [r12 - 1]
    call file_handle_control
    jmp .ret
.filedata:
    mov rcx, rbx
    lea rdx, [rsi + 1]
    lea r8, [r12 - 1]
    call file_handle_data
.ret:
    EPILOG FR

; inbox_push(ecx = friend index, rdx = text, r8 = len)
inbox_push:
%assign FR 0
    PROLOG FR
    mov r12d, ecx
    mov rsi, rdx
    mov r13, r8
    cmp r13, 1376
    jbe .lenok
    mov r13d, 1376
.lenok:
    mov eax, [rel g_incount]
    cmp eax, INBOX_SIZE
    jb .room
    inc dword [rel g_inhead]    ; full: drop the oldest
    dec dword [rel g_incount]
.room:
    mov eax, [rel g_inhead]
    add eax, [rel g_incount]
    and eax, INBOX_SIZE - 1
    imul rax, rax, INMSG_size
    lea rdi, [rel inbox]
    add rdi, rax
    mov [rdi + INMSG.friend], r12d
    mov [rdi + INMSG.len], r13d
    lea rcx, [rdi + INMSG.text]
    mov rdx, rsi
    mov r8, r13
    call mem_cpy
    inc dword [rel g_incount]
    EPILOG FR

; crypto_tick(rcx = friend)
crypto_tick:
%assign FR 16
    PROLOG FR
    mov rbx, rcx
    mov r15, [rel g_now]
    movzx eax, byte [rbx + FRIEND.cstatus]
    cmp eax, CS_NONE
    jne .have
    cmp byte [rbx + FRIEND.hasdht], 0
    je .ret
    mov rcx, rbx
    mov edx, 1
    call conn_new
    jmp .ret
.have:
    cmp eax, CS_ESTAB
    je .estab
    mov rax, r15
    sub rax, [rbx + FRIEND.tempsent]
    cmp rax, 1000
    jb .req
    cmp dword [rbx + FRIEND.temptries], 10
    jb .resend
    mov rcx, rbx                ; handshake stalled: start over
    call kill_conn
    jmp .ret
.resend:
    mov rcx, rbx
    call send_temp
.req:
    cmp byte [rbx + FRIEND.cstatus], CS_NOTCONF
    jne .ret
    mov rax, r15
    sub rax, [rbx + FRIEND.lastrequest]
    cmp rax, 1000
    jb .ret
    mov rcx, rbx
    call send_request
    jmp .ret
.estab:
    mov rax, r15
    sub rax, [rbx + FRIEND.lastrequest]
    cmp rax, 1000
    jb .noreq
    mov rcx, rbx
    call send_request
.noreq:
    mov rcx, rbx
    call resend_unacked
    lea rcx, [rbx + FRIEND.peerdht]
    call any_route
    test rax, rax
    jz .noroute
    mov [rbx + FRIEND.lastrouted], r15
    jmp .share
.noroute:
    mov rax, r15
    sub rax, [rbx + FRIEND.lastrouted]
    cmp rax, 20000
    jb .share
    mov rcx, rbx
    call kill_conn
    jmp .ret
.share:
    mov rax, r15
    sub rax, [rbx + FRIEND.lastshare]
    cmp rax, 120000
    jb .ping
    mov rcx, rbx
    call share_relays
.ping:
    mov rax, r15
    sub rax, [rbx + FRIEND.lastalivesent]
    cmp rax, 8000
    jb .timeout
    mov [rbx + FRIEND.lastalivesent], r15
    mov byte [LOC(0)], PKT_ALIVE
    mov rcx, rbx
    lea rdx, [LOC(0)]
    mov r8d, 1
    call send_lossless
.timeout:
    mov rax, r15
    sub rax, [rbx + FRIEND.lastalive]
    cmp rax, 32000
    jb .ret
    mov rcx, rbx
    call kill_conn
.ret:
    EPILOG FR

