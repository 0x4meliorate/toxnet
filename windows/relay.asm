; TCP relay client: non-blocking sockets driven by select().

WSAEWOULDBLOCK equ 10035
FIONBIO        equ 0x8004667E

section .text

; relay_ptr(ecx = index) -> rax
relay_ptr:
    lea rax, [rel relays]
    imul rcx, rcx, RELAY_size
    add rax, rcx
    ret

; relay_add(rcx = NODE* whose .port is the TCP port) -> rax = index or -1
relay_add:
%assign FR 0
    PROLOG FR
    mov rsi, rcx
    mov rax, -1
    cmp dword [rsi + NODE.ip], 0
    je .ret
    cmp word [rsi + NODE.port], 0
    je .ret
    lea rbx, [rel relays]
    xor edi, edi
.find:
    cmp dword [rbx + RELAY.state], RS_UNUSED
    je .next
    lea rcx, [rbx + RELAY.node + NODE.pk]
    mov rdx, rsi
    mov r8d, 32
    call mem_eq
    test eax, eax
    jnz .found
.next:
    add rbx, RELAY_size
    inc edi
    cmp edi, MAX_RELAYS
    jb .find
    lea rbx, [rel relays]
    xor edi, edi
.free:
    cmp dword [rbx + RELAY.state], RS_UNUSED
    je .take
    add rbx, RELAY_size
    inc edi
    cmp edi, MAX_RELAYS
    jb .free
    mov rax, -1
    jmp .ret
.take:
    lea rcx, [rbx + RELAY.node]
    mov rdx, rsi
    mov r8d, NODE_size
    call mem_cpy
    mov dword [rbx + RELAY.state], RS_IDLE
    mov qword [rbx + RELAY.tstate], 0
.found:
    mov eax, edi
.ret:
    EPILOG FR

; relay_close(rcx = relay)
relay_close:
%assign FR 0
    PROLOG FR
    mov rbx, rcx
    cmp dword [rbx + RELAY.state], RS_CONNECT
    jb .reset
    mov rcx, [rbx + RELAY.sock]
    call WINAPI(closesocket)
.reset:
    mov dword [rbx + RELAY.state], RS_IDLE
    mov rax, [rel g_now]
    mov [rbx + RELAY.tstate], rax
    mov dword [rbx + RELAY.rxlen], 0
    mov dword [rbx + RELAY.txlen], 0
    lea rcx, [rbx + RELAY.routes]
    mov edx, ROUTE_size * MAX_ROUTES
    call mem_zero
    EPILOG FR

; relay_connect(rcx = relay): start a non-blocking connect
relay_connect:
%assign FR 32
    PROLOG FR
    mov rbx, rcx
    mov rax, [rel g_now]
    mov [rbx + RELAY.tstate], rax
    mov ecx, 2                  ; AF_INET
    mov edx, 1                  ; SOCK_STREAM
    mov r8d, 6                  ; IPPROTO_TCP
    call WINAPI(socket)
    cmp rax, -1
    je .ret
    mov [rbx + RELAY.sock], rax
    mov dword [LOC(16)], 1
    mov rcx, rax
    mov edx, FIONBIO
    lea r8, [LOC(16)]
    call WINAPI(ioctlsocket)
    ; sockaddr_in
    mov qword [LOC(0)], 0
    mov qword [LOC(8)], 0
    mov word [LOC(0)], 2
    mov ax, [rbx + RELAY.node + NODE.port]
    mov [LOC(2)], ax
    mov eax, [rbx + RELAY.node + NODE.ip]
    mov [LOC(4)], eax
    mov rcx, [rbx + RELAY.sock]
    lea rdx, [LOC(0)]
    mov r8d, 16
    call WINAPI(connect)
    test eax, eax
    jz .ok
    call WINAPI(WSAGetLastError)
    cmp eax, WSAEWOULDBLOCK
    je .ok
    mov rcx, [rbx + RELAY.sock]
    call WINAPI(closesocket)
    jmp .ret
.ok:
    mov dword [rbx + RELAY.state], RS_CONNECT
.ret:
    EPILOG FR

; relay_begin_handshake(rcx = relay): queue the 128-byte client handshake
relay_begin_handshake:
%assign FR 160
    PROLOG FR
    mov rbx, rcx
    lea rcx, [LOC(0)]           ; temp pk at LOC(0), base nonce follows at LOC(32)
    lea rdx, [rbx + RELAY.tmpsk]
    call box_keypair
    test eax, eax
    jnz .rngfail
    lea rcx, [rbx + RELAY.snonce]
    mov edx, 24
    call rand_bytes
    test eax, eax
    jnz .rngfail
    lea rcx, [LOC(32)]
    lea rdx, [rbx + RELAY.snonce]
    mov r8d, 24
    call mem_cpy
    lea rcx, [rbx + RELAY.shared]
    lea rdx, [rbx + RELAY.node + NODE.pk]
    lea r8, [rel g_dhtsk]
    call box_beforenm
    lea rdi, [rbx + RELAY.tx]
    mov rcx, rdi
    lea rdx, [rel g_dhtpk]
    mov r8d, 32
    call mem_cpy
    lea rcx, [rdi + 32]
    mov edx, 24
    call rand_bytes
    test eax, eax
    jnz .rngfail
    lea rcx, [rdi + 56]
    lea rdx, [rbx + RELAY.shared]
    lea r8, [rdi + 32]
    lea r9, [LOC(0)]
    mov qword [rsp+32], 56
    call box_seal
    mov dword [rbx + RELAY.txlen], 128
    mov dword [rbx + RELAY.state], RS_HANDSHK
    mov rax, [rel g_now]
    mov [rbx + RELAY.tstate], rax
    jmp .ret
.rngfail:
    mov rcx, rbx
    call relay_close
.ret:
    EPILOG FR

; relay_finish_handshake(rcx = relay) -> rax = 1 ok, 0 failed (relay closed)
relay_finish_handshake:
%assign FR 96
    PROLOG FR
    mov rbx, rcx
    lea rdi, [rbx + RELAY.rx]
    lea rcx, [LOC(0)]
    lea rdx, [rbx + RELAY.shared]
    mov r8, rdi
    lea r9, [rdi + 24]
    mov qword [rsp+32], 72
    mov qword [rsp+40], 56
    call box_open
    cmp rax, 56
    je .ok
.bad:
    mov rcx, rbx
    call relay_close
    xor eax, eax
    jmp .ret
.ok:
    lea rcx, [rbx + RELAY.shared]
    lea rdx, [LOC(0)]
    lea r8, [rbx + RELAY.tmpsk]
    call box_beforenm
    lea rcx, [rbx + RELAY.rnonce]
    lea rdx, [LOC(32)]
    mov r8d, 24
    call mem_cpy
    ; consume 96 bytes
    mov r8d, [rbx + RELAY.rxlen]
    sub r8d, 96
    mov [rbx + RELAY.rxlen], r8d
    mov rcx, rdi
    lea rdx, [rdi + 96]
    call mem_cpy
    mov dword [rbx + RELAY.state], RS_READY
    mov rax, [rel g_now]
    mov [rbx + RELAY.tstate], rax
    ; ping confirms the connection with the server
    mov byte [LOC(64)], 4
    lea rcx, [LOC(65)]
    mov edx, 8
    call rand_bytes
    test eax, eax
    jnz .bad
    mov rcx, rbx
    lea rdx, [LOC(64)]
    mov r8d, 9
    call relay_send
    ; route to every friend whose DHT key we know
    lea rsi, [rel friends]
    xor edi, edi
.fr:
    cmp byte [rsi + FRIEND.used], 0
    je .frn
    cmp byte [rsi + FRIEND.hasdht], 0
    je .frn
    mov rcx, rbx
    lea rdx, [rsi + FRIEND.dhtpk]
    call relay_request_route
.frn:
    add rsi, FRIEND_size
    inc edi
    cmp edi, MAX_FRIENDS
    jb .fr
    mov eax, 1
.ret:
    EPILOG FR

; relay_send(rcx = relay, rdx = payload, r8 = len) -> rax = 1 queued, 0 dropped
relay_send:
%assign FR 0
    PROLOG FR
    mov rbx, rcx
    mov rsi, rdx
    mov r12, r8
    xor eax, eax
    cmp dword [rbx + RELAY.state], RS_READY
    jne .ret
    mov edi, [rbx + RELAY.txlen]
    lea rax, [rdi + r12 + 18]
    cmp rax, TX_SIZE
    mov eax, 0
    ja .ret
    lea r13, [rbx + RELAY.tx]
    add r13, rdi                ; frame start
    lea rcx, [r13 + 2]
    lea rdx, [rbx + RELAY.shared]
    lea r8, [rbx + RELAY.snonce]
    mov r9, rsi
    mov [rsp+32], r12
    call box_seal
    lea eax, [r12 + 16]
    xchg al, ah
    mov [r13], ax
    lea rcx, [rbx + RELAY.snonce]
    mov edx, 1
    call inc_nonce
    lea eax, [rdi + r12 + 18]
    mov [rbx + RELAY.txlen], eax
    mov eax, 1
.ret:
    EPILOG FR

; relay_flush(rcx = relay)
relay_flush:
%assign FR 0
    PROLOG FR
    mov rbx, rcx
    mov r8d, [rbx + RELAY.txlen]
    test r8d, r8d
    jz .ret
    mov rcx, [rbx + RELAY.sock]
    lea rdx, [rbx + RELAY.tx]
    xor r9d, r9d
    call WINAPI(send)
    cmp eax, 0
    jg .sent
    call WINAPI(WSAGetLastError)
    cmp eax, WSAEWOULDBLOCK
    je .ret
    mov rcx, rbx
    call relay_close
    jmp .ret
.sent:
    mov eax, eax
    mov r8d, [rbx + RELAY.txlen]
    sub r8d, eax
    mov [rbx + RELAY.txlen], r8d
    lea rcx, [rbx + RELAY.tx]
    lea rdx, [rcx + rax]
    call mem_cpy
.ret:
    EPILOG FR

; relay_recv(rcx = relay): read what's available and process frames
relay_recv:
%assign FR 0
    PROLOG FR
    mov rbx, rcx
    mov eax, [rbx + RELAY.rxlen]
    mov r8d, RX_SIZE
    sub r8d, eax
    jz .process
    mov rcx, [rbx + RELAY.sock]
    lea rdx, [rbx + RELAY.rx + rax]
    xor r9d, r9d
    call WINAPI(recv)
    cmp eax, 0
    jg .got
    jl .err
    mov rcx, rbx                ; connection closed by server
    call relay_close
    jmp .ret
.err:
    call WINAPI(WSAGetLastError)
    cmp eax, WSAEWOULDBLOCK
    je .ret
    mov rcx, rbx
    call relay_close
    jmp .ret
.got:
    add [rbx + RELAY.rxlen], eax
.process:
    cmp dword [rbx + RELAY.state], RS_HANDSHK
    jne .frames
    cmp dword [rbx + RELAY.rxlen], 96
    jb .ret
    mov rcx, rbx
    call relay_finish_handshake
    test eax, eax
    jz .ret
.frames:
    cmp dword [rbx + RELAY.state], RS_READY
    jne .ret
    mov eax, [rbx + RELAY.rxlen]
    cmp eax, 2
    jb .ret
    movzx esi, word [rbx + RELAY.rx]
    rol si, 8                   ; big-endian length
    movzx esi, si
    cmp esi, 16
    jbe .bad
    cmp esi, 4096
    ja .bad
    lea ecx, [esi + 2]
    cmp eax, ecx
    jb .ret
    lea rcx, [rel frame_plain]
    lea rdx, [rbx + RELAY.shared]
    lea r8, [rbx + RELAY.rnonce]
    lea r9, [rbx + RELAY.rx + 2]
    mov [rsp+32], rsi
    mov qword [rsp+40], 4160     ; frame_plain capacity
    call box_open
    cmp rax, -1
    je .bad
    mov r12, rax                ; plaintext length
    lea rcx, [rbx + RELAY.rnonce]
    mov edx, 1
    call inc_nonce
    ; consume the frame before dispatching
    mov r8d, [rbx + RELAY.rxlen]
    sub r8d, esi
    sub r8d, 2
    mov [rbx + RELAY.rxlen], r8d
    lea rcx, [rbx + RELAY.rx]
    lea rdx, [rcx + rsi + 2]
    call mem_cpy
    test r12, r12
    jz .frames
    mov rcx, rbx
    lea rdx, [rel frame_plain]
    mov r8, r12
    call relay_dispatch
    jmp .frames
.bad:
    mov rcx, rbx
    call relay_close
.ret:
    EPILOG FR

; route_find(rcx = relay, rdx = pk) -> rax = ROUTE* or 0
route_find:
%assign FR 0
    PROLOG FR
    lea rbx, [rcx + RELAY.routes]
    mov rsi, rdx
    xor edi, edi
.l:
    cmp byte [rbx + ROUTE.used], 0
    je .n
    mov rcx, rbx
    mov rdx, rsi
    mov r8d, 32
    call mem_eq
    test eax, eax
    jz .n
    mov rax, rbx
    jmp .ret
.n:
    add rbx, ROUTE_size
    inc edi
    cmp edi, MAX_ROUTES
    jb .l
    xor eax, eax
.ret:
    EPILOG FR

; route_by_id(rcx = relay, edx = connection id) -> rax = ROUTE* or 0
route_by_id:
    lea rax, [rcx + RELAY.routes]
    mov r8d, MAX_ROUTES
.l:
    cmp byte [rax + ROUTE.used], 0
    je .n
    cmp [rax + ROUTE.id], dl
    je .ret
.n:
    add rax, ROUTE_size
    dec r8d
    jnz .l
    xor eax, eax
.ret:
    ret

; route_alloc(rcx = relay, rdx = pk) -> rax = new ROUTE* (pk set) or 0
route_alloc:
%assign FR 0
    PROLOG FR
    mov rsi, rdx
    lea rbx, [rcx + RELAY.routes]
    xor edi, edi
.l:
    cmp byte [rbx + ROUTE.used], 0
    je .take
    add rbx, ROUTE_size
    inc edi
    cmp edi, MAX_ROUTES
    jb .l
    xor eax, eax
    jmp .ret
.take:
    mov rcx, rbx
    mov rdx, rsi
    mov r8d, 32
    call mem_cpy
    mov byte [rbx + ROUTE.used], 1
    mov byte [rbx + ROUTE.id], 0
    mov byte [rbx + ROUTE.online], 0
    mov rax, rbx
.ret:
    EPILOG FR

; relay_request_route(rcx = relay, rdx = pk)
relay_request_route:
%assign FR 48
    PROLOG FR
    mov rbx, rcx
    mov rsi, rdx
    cmp dword [rbx + RELAY.state], RS_READY
    jne .ret
    mov rcx, rbx
    mov rdx, rsi
    call route_find
    test rax, rax
    jnz .ret
    mov rcx, rbx
    mov rdx, rsi
    call route_alloc
    test rax, rax
    jz .ret
    mov byte [LOC(0)], 0        ; TCP_PACKET_ROUTING_REQUEST
    lea rcx, [LOC(1)]
    mov rdx, rsi
    mov r8d, 32
    call mem_cpy
    mov rcx, rbx
    lea rdx, [LOC(0)]
    mov r8d, 33
    call relay_send
.ret:
    EPILOG FR

; relay_online_to(rcx = relay, rdx = pk) -> rax = 1 if a routed connection is online
relay_online_to:
%assign FR 0
    PROLOG FR
    xor ebx, ebx
    cmp dword [rcx + RELAY.state], RS_READY
    jne .ret
    call route_find
    test rax, rax
    jz .ret
    movzx ebx, byte [rax + ROUTE.online]
.ret:
    mov eax, ebx
    EPILOG FR

; any_route(rcx = pk) -> rax = relay with an online route to pk, or 0
any_route:
%assign FR 0
    PROLOG FR
    mov rsi, rcx
    lea rbx, [rel relays]
    xor edi, edi
.l:
    mov rcx, rbx
    mov rdx, rsi
    call relay_online_to
    test eax, eax
    jz .n
    mov rax, rbx
    jmp .ret
.n:
    add rbx, RELAY_size
    inc edi
    cmp edi, MAX_RELAYS
    jb .l
    xor eax, eax
.ret:
    EPILOG FR

; relay_send_to(rcx = relay, rdx = pk, r8 = data, r9 = len):
;   routed if the connection to pk is online, otherwise out-of-band
relay_send_to:
%assign FR 0
    PROLOG FR
    mov rbx, rcx
    mov rsi, rdx
    mov r12, r8
    mov r13, r9
    cmp r13, 2048
    ja .ret
    lea rdi, [rel relay_out]
    mov rcx, rbx
    mov rdx, rsi
    call route_find
    test rax, rax
    jz .oob
    cmp byte [rax + ROUTE.online], 0
    je .oob
    mov al, [rax + ROUTE.id]
    mov [rdi], al
    lea rcx, [rdi + 1]
    mov rdx, r12
    mov r8, r13
    call mem_cpy
    lea r14, [r13 + 1]
    jmp .send
.oob:
    mov byte [rdi], 6           ; TCP_PACKET_OOB_SEND
    lea rcx, [rdi + 1]
    mov rdx, rsi
    mov r8d, 32
    call mem_cpy
    lea rcx, [rdi + 33]
    mov rdx, r12
    mov r8, r13
    call mem_cpy
    lea r14, [r13 + 33]
.send:
    mov rcx, rbx
    mov rdx, rdi
    mov r8, r14
    call relay_send
.ret:
    EPILOG FR

; relay_dispatch(rcx = relay, rdx = plaintext, r8 = len)
relay_dispatch:
%assign FR 16
    PROLOG FR
    mov rbx, rcx
    mov rsi, rdx
    mov r12, r8
    movzx eax, byte [rsi]
    cmp eax, 1
    je .routing
    cmp eax, 2
    je .connnotify
    cmp eax, 3
    je .disconn
    cmp eax, 4
    je .ping
    cmp eax, 7
    je .oob
    cmp eax, 9
    je .onion
    cmp eax, 16
    jae .data
    jmp .ret
.routing:                       ; [1][id][pk]
    cmp r12, 34
    jne .ret
    cmp byte [rsi + 1], 0
    je .ret
    mov rcx, rbx
    lea rdx, [rsi + 2]
    call route_find
    test rax, rax
    jnz .setid
    mov rcx, rbx
    lea rdx, [rsi + 2]
    call route_alloc
    test rax, rax
    jz .ret
.setid:
    mov cl, [rsi + 1]
    mov [rax + ROUTE.id], cl
    jmp .ret
.connnotify:
    cmp r12, 2
    jne .ret
    mov rcx, rbx
    movzx edx, byte [rsi + 1]
    call route_by_id
    test rax, rax
    jz .ret
    mov byte [rax + ROUTE.online], 1
    mov rdi, rax
    mov rcx, rdi
    call peer_route_online
    jmp .ret
.disconn:
    cmp r12, 2
    jne .ret
    mov rcx, rbx
    movzx edx, byte [rsi + 1]
    call route_by_id
    test rax, rax
    jz .ret
    mov byte [rax + ROUTE.online], 0
    mov rdi, rax
    jmp .ret
.ping:
    cmp r12, 9
    jne .ret
    mov rax, [rsi + 1]
    mov byte [LOC(0)], 5
    mov [LOC(1)], rax
    mov rcx, rbx
    lea rdx, [LOC(0)]
    mov r8d, 9
    call relay_send
    jmp .ret
.oob:                           ; [7][sender pk][data]
    cmp r12, 33
    jbe .ret
    mov rcx, rbx
    lea rdx, [rsi + 1]
    lea r8, [rsi + 33]
    lea r9, [r12 - 33]
    call handle_crypto
    jmp .ret
.onion:
    cmp r12, 1
    jbe .ret
    lea rcx, [rsi + 1]
    lea rdx, [r12 - 1]
    call onion_response
    jmp .ret
.data:
    cmp r12, 1
    jbe .ret
    mov rcx, rbx
    movzx edx, byte [rsi]
    call route_by_id
    test rax, rax
    jz .ret
    mov rcx, rbx
    mov rdx, rax
    lea r8, [rsi + 1]
    lea r9, [r12 - 1]
    call handle_crypto
.ret:
    EPILOG FR

; fd_isset(rcx = fd_set, rdx = socket) -> rax = 1 if present
fd_isset:
    mov r8d, [rcx]
    lea r9, [rcx + 8]
.l:
    test r8d, r8d
    jz .no
    cmp [r9], rdx
    je .yes
    add r9, 8
    dec r8d
    jmp .l
.yes:
    mov eax, 1
    ret
.no:
    xor eax, eax
    ret

; fd_add(rcx = fd_set, rdx = socket)
fd_add:
    mov eax, [rcx]
    mov [rcx + 8 + rax*8], rdx
    inc dword [rcx]
    ret

; relays_tick(): connect, select (up to 50 ms), read, write
relays_tick:
%assign FR 32
    PROLOG FR
    mov r15, [rel g_now]
    ; count active relays
    xor r12d, r12d
    lea rbx, [rel relays]
    xor edi, edi
.cnt:
    cmp dword [rbx + RELAY.state], RS_CONNECT
    jb .cntn
    inc r12d
.cntn:
    add rbx, RELAY_size
    inc edi
    cmp edi, MAX_RELAYS
    jb .cnt
    ; start connections / time out stuck ones
    lea rbx, [rel relays]
    xor edi, edi
.conn:
    mov eax, [rbx + RELAY.state]
    cmp eax, RS_IDLE
    jne .tmo
    cmp r12d, MAX_CONNECTED
    jae .connn
    mov rax, [rbx + RELAY.tstate]
    test rax, rax
    jz .go
    mov rcx, r15
    sub rcx, rax
    cmp rcx, 10000
    jb .connn
.go:
    mov rcx, rbx
    call relay_connect
    inc r12d
    jmp .connn
.tmo:
    cmp eax, RS_CONNECT
    jb .connn
    cmp eax, RS_READY
    je .connn
    mov rcx, r15
    sub rcx, [rbx + RELAY.tstate]
    cmp rcx, 10000
    jb .connn
    mov rcx, rbx
    call relay_close
.connn:
    add rbx, RELAY_size
    inc edi
    cmp edi, MAX_RELAYS
    jb .conn
    ; build fd sets
    mov dword [rel fds_rd], 0
    mov dword [rel fds_wr], 0
    mov dword [rel fds_ex], 0
    xor r13d, r13d              ; total sockets
    lea rbx, [rel relays]
    xor edi, edi
.sets:
    mov eax, [rbx + RELAY.state]
    cmp eax, RS_CONNECT
    jb .setn
    inc r13d
    cmp eax, RS_CONNECT
    jne .rw
    lea rcx, [rel fds_wr]
    mov rdx, [rbx + RELAY.sock]
    call fd_add
    lea rcx, [rel fds_ex]
    mov rdx, [rbx + RELAY.sock]
    call fd_add
    jmp .setn
.rw:
    lea rcx, [rel fds_rd]
    mov rdx, [rbx + RELAY.sock]
    call fd_add
    cmp dword [rbx + RELAY.txlen], 0
    je .setn
    lea rcx, [rel fds_wr]
    mov rdx, [rbx + RELAY.sock]
    call fd_add
.setn:
    add rbx, RELAY_size
    inc edi
    cmp edi, MAX_RELAYS
    jb .sets
    test r13d, r13d
    jnz .select
    mov ecx, 50
    call WINAPI(Sleep)
    jmp .ret
.select:
    mov dword [LOC(0)], 0       ; timeval: 0 s
    mov dword [LOC(4)], 50000   ;          50 ms
    xor ecx, ecx
    lea rdx, [rel fds_rd]
    lea r8, [rel fds_wr]
    lea r9, [rel fds_ex]
    lea rax, [LOC(0)]
    mov [rsp+32], rax
    call WINAPI(select)
    cmp eax, 0
    jle .ret
    mov rax, [rel g_now]        ; unchanged; handlers use g_now
    lea rbx, [rel relays]
    xor edi, edi
.io:
    mov eax, [rbx + RELAY.state]
    cmp eax, RS_CONNECT
    jb .ion
    jne .rd
    lea rcx, [rel fds_wr]
    mov rdx, [rbx + RELAY.sock]
    call fd_isset
    test eax, eax
    jz .connex
    mov rcx, rbx
    call relay_begin_handshake
    jmp .wr
.connex:
    lea rcx, [rel fds_ex]
    mov rdx, [rbx + RELAY.sock]
    call fd_isset
    test eax, eax
    jz .ion
    mov rcx, rbx
    call relay_close
    jmp .ion
.rd:
    lea rcx, [rel fds_rd]
    mov rdx, [rbx + RELAY.sock]
    call fd_isset
    test eax, eax
    jz .wr
    mov rcx, rbx
    call relay_recv
.wr:
    cmp dword [rbx + RELAY.state], RS_HANDSHK
    jb .ion
    mov rcx, rbx
    call relay_flush
.ion:
    add rbx, RELAY_size
    inc edi
    cmp edi, MAX_RELAYS
    jb .io
.ret:
    EPILOG FR

; relays_flush_all(): push out anything queued during this iteration
relays_flush_all:
%assign FR 0
    PROLOG FR
    lea rbx, [rel relays]
    xor edi, edi
.l:
    cmp dword [rbx + RELAY.state], RS_HANDSHK
    jb .n
    mov rcx, rbx
    call relay_flush
.n:
    add rbx, RELAY_size
    inc edi
    cmp edi, MAX_RELAYS
    jb .l
    EPILOG FR
