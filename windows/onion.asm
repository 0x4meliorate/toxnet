; Onion: paths through a TCP relay + 2 nodes, announcing ourselves,
; searching for friends, and onion data packets (DHT key, friend request).

section .text

; target_ptr(ecx = index) -> rax
target_ptr:
    lea rax, [rel targets]
    imul rcx, rcx, TARGET_size
    add rax, rcx
    ret

; path_ptr(ecx = index) -> rax
path_ptr:
    lea rax, [rel paths]
    imul rcx, rcx, PATH_size
    add rax, rcx
    ret

; friend_ptr(ecx = index) -> rax
friend_ptr:
    lea rax, [rel friends]
    imul rcx, rcx, FRIEND_size
    add rax, rcx
    ret

; ipport_pack(rcx = out19, rdx = NODE*)   IPv4 onion ip_port encoding
ipport_pack:
    mov byte [rcx], 2
    mov eax, [rdx + NODE.ip]
    mov [rcx + 1], eax
    xor eax, eax
    mov [rcx + 5], rax
    mov [rcx + 9], eax
    mov [rcx + 13], eax
    mov ax, [rdx + NODE.port]
    mov [rcx + 17], ax
    ret

; path_ok(ecx = index, edx = gen) -> rax = 1 if usable
path_ok:
%assign FR 0
    PROLOG FR
    mov esi, edx
    call path_ptr
    mov rbx, rax
    xor edi, edi
    cmp byte [rbx + PATH.valid], 0
    je .ret
    cmp [rbx + PATH.gen], esi
    jne .ret
    cmp byte [rbx + PATH.fails], 3
    jae .ret
    mov rax, [rel g_now]
    sub rax, [rbx + PATH.created]
    cmp rax, 900000
    jae .ret
    movzx ecx, byte [rbx + PATH.relay]
    call relay_ptr
    cmp dword [rax + RELAY.state], RS_READY
    jne .ret
    mov edi, 1
.ret:
    mov eax, edi
    EPILOG FR

; path_avoids(rcx = PATH*, rdx = pk) -> rax = 1 if no hop of the path is pk
path_avoids:
%assign FR 0
    PROLOG FR
    mov rbx, rcx
    mov rsi, rdx
    xor edi, edi
    lea rcx, [rbx + PATH.n2]
    mov rdx, rsi
    mov r8d, 32
    call mem_eq
    test eax, eax
    jnz .ret
    lea rcx, [rbx + PATH.n3]
    mov rdx, rsi
    mov r8d, 32
    call mem_eq
    test eax, eax
    jnz .ret
    movzx ecx, byte [rbx + PATH.relay]
    call relay_ptr
    lea rcx, [rax + RELAY.node]
    mov rdx, rsi
    mov r8d, 32
    call mem_eq
    test eax, eax
    jnz .ret
    mov edi, 1
.ret:
    mov eax, edi
    EPILOG FR

; pick_bootstrap(rcx = exclude pk 1, rdx = exclude pk 2, r8 = exclude pk 3) -> rax = NODE* or 0
pick_bootstrap:
%assign FR 0
    PROLOG FR
    mov r12, rcx
    mov r13, rdx
    mov r14, r8
    mov r15d, 30                ; attempts
.try:
    mov ecx, BOOTSTRAP_COUNT
    call rand_below
    cmp rax, -1
    je .none
    imul rax, rax, NODE_size
    lea rbx, [rel bootstrap_nodes]
    add rbx, rax
    mov rcx, rbx
    mov rdx, r12
    mov r8d, 32
    call mem_eq
    test eax, eax
    jnz .again
    mov rcx, rbx
    mov rdx, r13
    mov r8d, 32
    call mem_eq
    test eax, eax
    jnz .again
    mov rcx, rbx
    mov rdx, r14
    mov r8d, 32
    call mem_eq
    test eax, eax
    jnz .again
    mov rax, rbx
    jmp .ret
.again:
    dec r15d
    jnz .try
.none:
    xor eax, eax
.ret:
    EPILOG FR

; path_new(ecx = slot, rdx = destination pk) -> rax = 1 ok
path_new:
%assign FR 64
    PROLOG FR
    mov r12d, ecx
    mov r13, rdx
    ; choose a random ready relay
    xor esi, esi
    lea rbx, [rel relays]
    xor edi, edi
.cnt:
    cmp dword [rbx + RELAY.state], RS_READY
    jne .cn
    inc esi
.cn:
    add rbx, RELAY_size
    inc edi
    cmp edi, MAX_RELAYS
    jb .cnt
    xor eax, eax
    test esi, esi
    jz .ret
    mov ecx, esi
    call rand_below
    cmp rax, -1
    je .none
    mov r14d, eax               ; which ready relay
    lea rbx, [rel relays]
    xor edi, edi
.sel:
    cmp dword [rbx + RELAY.state], RS_READY
    jne .sn
    test r14d, r14d
    jz .got
    dec r14d
.sn:
    add rbx, RELAY_size
    inc edi
    jmp .sel
.got:
    mov r14d, edi               ; relay index
    mov r15, rbx
    lea rcx, [rbx + RELAY.node]
    mov rdx, r13
    mov r8, r13
    call pick_bootstrap
    test rax, rax
    jz .ret
    mov rsi, rax                ; n2
    lea rcx, [r15 + RELAY.node]
    mov rdx, r13
    mov r8, rsi
    call pick_bootstrap
    test rax, rax
    jz .ret
    mov rdi, rax                ; n3
    mov ecx, r12d
    call path_ptr
    mov rbx, rax
    mov byte [rbx + PATH.valid], 0
    lea rcx, [rbx + PATH.n2]
    mov rdx, rsi
    mov r8d, NODE_size
    call mem_cpy
    lea rcx, [rbx + PATH.n3]
    mov rdx, rdi
    mov r8d, NODE_size
    call mem_cpy
    lea rcx, [rbx + PATH.pk2]
    lea rdx, [LOC(0)]
    call box_keypair
    test eax, eax
    jnz .none
    lea rcx, [rbx + PATH.k2]
    lea rdx, [rbx + PATH.n2]
    lea r8, [LOC(0)]
    call box_beforenm
    lea rcx, [rbx + PATH.pk3]
    lea rdx, [LOC(0)]
    call box_keypair
    test eax, eax
    jnz .none
    lea rcx, [rbx + PATH.k3]
    lea rdx, [rbx + PATH.n3]
    lea r8, [LOC(0)]
    call box_beforenm
    mov [rbx + PATH.relay], r14b
    mov byte [rbx + PATH.fails], 0
    mov byte [rbx + PATH.valid], 1
    mov rax, [rel g_now]
    mov [rbx + PATH.created], rax
    inc dword [rel g_pathgen]
    mov eax, [rel g_pathgen]
    mov [rbx + PATH.gen], eax
    mov eax, 1
    jmp .ret
.none:
    xor eax, eax
.ret:
    EPILOG FR

; path_get(rcx = destination pk) -> rax = path index or -1
path_get:
%assign FR 16
    PROLOG FR
    mov r12, rcx
    xor r13d, r13d              ; candidate bitmask
    xor r14d, r14d              ; candidates
    xor r15d, r15d              ; valid paths
    xor edi, edi
.scan:
    mov ecx, edi
    call path_ptr
    mov rbx, rax
    mov ecx, edi
    mov edx, [rbx + PATH.gen]
    call path_ok
    test eax, eax
    jz .sn
    inc r15d
    mov rcx, rbx
    mov rdx, r12
    call path_avoids
    test eax, eax
    jz .sn
    bts r13d, edi
    inc r14d
.sn:
    inc edi
    cmp edi, MAX_PATHS
    jb .scan
    test r14d, r14d
    jz .create
    cmp r15d, MAX_PATHS
    jae .pick
    mov ecx, 4
    call rand_below
    cmp rax, -1
    je .none
    test eax, eax
    jnz .pick
.create:
    ; slot: first unusable path, else round robin
    xor edi, edi
.free:
    mov ecx, edi
    call path_ptr
    mov ecx, edi
    mov edx, [rax + PATH.gen]
    call path_ok
    test eax, eax
    jz .make
    inc edi
    cmp edi, MAX_PATHS
    jb .free
    mov edi, [rel g_pathnext]
    inc dword [rel g_pathnext]
    and edi, MAX_PATHS - 1
.make:
    btr r13d, edi               ; that slot is no longer a candidate
    jnc .mk
    dec r14d
.mk:
    mov ecx, edi
    mov rdx, r12
    call path_new
    test eax, eax
    jz .fallback
    mov eax, edi
    jmp .ret
.fallback:
    test r14d, r14d
    jz .none
.pick:
    mov ecx, r14d
    call rand_below
    cmp rax, -1
    je .none
    mov esi, eax
    xor edi, edi
.pl:
    bt r13d, edi
    jnc .pn
    test esi, esi
    jz .chosen
    dec esi
.pn:
    inc edi
    cmp edi, MAX_PATHS
    jb .pl
.none:
    mov rax, -1
    jmp .ret
.chosen:
    mov eax, edi
.ret:
    EPILOG FR

; onion_send(ecx = path index, rdx = destination NODE*, r8 = data, r9 = len)
;   -> 1 queued / 0 failed
onion_send:
%assign FR 32
    PROLOG FR
    mov r12, rdx
    mov r13, r8
    mov r14, r9
    call path_ptr
    mov rbx, rax
    lea rcx, [LOC(0)]
    mov edx, 24
    call rand_bytes
    test eax, eax
    jnz .bad
    ; step1 = ip_port(dest) || data
    lea rsi, [rel buf_s1]
    mov rcx, rsi
    mov rdx, r12
    call ipport_pack
    lea rcx, [rsi + 19]
    mov rdx, r13
    mov r8, r14
    call mem_cpy
    ; step2 = ip_port(n3) || pk3 || seal(k3, step1)
    lea rdi, [rel buf_s2]
    mov rcx, rdi
    lea rdx, [rbx + PATH.n3]
    call ipport_pack
    lea rcx, [rdi + 19]
    lea rdx, [rbx + PATH.pk3]
    mov r8d, 32
    call mem_cpy
    lea rcx, [rdi + 51]
    lea rdx, [rbx + PATH.k3]
    lea r8, [LOC(0)]
    mov r9, rsi
    lea rax, [r14 + 19]
    mov [rsp+32], rax
    call box_seal
    lea r15, [rax + 51]         ; step2 length
    ; packet = [8] nonce ip_port(n2) pk2 seal(k2, step2)
    lea rsi, [rel buf_op]
    mov byte [rsi], 8
    lea rcx, [rsi + 1]
    lea rdx, [LOC(0)]
    mov r8d, 24
    call mem_cpy
    lea rcx, [rsi + 25]
    lea rdx, [rbx + PATH.n2]
    call ipport_pack
    lea rcx, [rsi + 44]
    lea rdx, [rbx + PATH.pk2]
    mov r8d, 32
    call mem_cpy
    lea rcx, [rsi + 76]
    lea rdx, [rbx + PATH.k2]
    lea r8, [LOC(0)]
    mov r9, rdi
    mov [rsp+32], r15
    call box_seal
    lea r15, [rax + 76]
    movzx ecx, byte [rbx + PATH.relay]
    call relay_ptr
    mov rcx, rax
    mov rdx, rsi
    mov r8, r15
    call relay_send
    jmp .ret
.bad:
    xor eax, eax
.ret:
    EPILOG FR

; is_public_ip(rcx = NODE*) -> rax = 1 if the IPv4 address looks public
is_public_ip:
    movzx eax, byte [rcx + NODE.ip]
    movzx edx, byte [rcx + NODE.ip + 1]
    test eax, eax
    jz .no
    cmp eax, 10
    je .no
    cmp eax, 127
    je .no
    cmp eax, 224
    jae .no
    cmp eax, 192
    jne .n172
    cmp edx, 168
    je .no
.n172:
    cmp eax, 172
    jne .n169
    and edx, 0xf0
    cmp edx, 16
    je .no
.n169:
    cmp eax, 169
    jne .yes
    cmp edx, 254
    je .no
.yes:
    cmp word [rcx + NODE.port], 0
    je .no
    mov eax, 1
    ret
.no:
    xor eax, eax
    ret

; target_add(ecx = target index, rdx = NODE*): keep the 8 closest nodes
target_add:
%assign FR 0
    PROLOG FR
    mov rsi, rdx
    call target_ptr
    mov rbx, rax
    mov rcx, rsi
    call is_public_ip
    test eax, eax
    jz .ret
    lea r12, [rbx + TARGET.nodes]
    xor r13d, r13d              ; free slot (ptr) or 0
    xor r14d, r14d              ; farthest used (ptr) or 0
    xor edi, edi
.l:
    mov rax, rdi
    imul rax, rax, ONODE_size
    lea r15, [r12 + rax]
    cmp byte [r15 + ONODE.used], 0
    jne .used
    test r13, r13
    jnz .n
    mov r13, r15
    jmp .n
.used:
    mov rcx, r15
    mov rdx, rsi
    mov r8d, 32
    call mem_eq
    test eax, eax
    jnz .ret                    ; already known
    test r14, r14
    jz .far
    mov rcx, rbx                ; is the current farthest closer than this one?
    mov rdx, r14
    mov r8, r15
    call closer
    test eax, eax
    jz .n
.far:
    mov r14, r15
.n:
    inc edi
    cmp edi, ONION_NODES
    jb .l
    test r13, r13
    jnz .fill
    mov rcx, rbx                ; full: replace the farthest if the new node is closer
    mov rdx, rsi
    mov r8, r14
    call closer
    test eax, eax
    jz .ret
    mov r13, r14
.fill:
    mov rcx, r13
    mov edx, ONODE_size
    call mem_zero
    mov rcx, r13
    mov rdx, rsi
    mov r8d, NODE_size
    call mem_cpy
    mov byte [r13 + ONODE.used], 1
    mov byte [r13 + ONODE.path], 0xff
.ret:
    EPILOG FR

; announce_send(ecx = target index, edx = node slot)
announce_send:
%assign FR 160
    PROLOG FR
    mov r12d, ecx
    mov r13d, edx
    call target_ptr
    mov rbx, rax
    mov eax, r13d
    imul rax, rax, ONODE_size
    lea rsi, [rbx + TARGET.nodes + rax]
    mov rax, [rel g_now]
    mov [rsi + ONODE.lastsent], rax
    ; path: reuse the node's path (its ping id is bound to it) if still good
    movzx ecx, byte [rsi + ONODE.path]
    cmp ecx, 0xff
    je .newpath
    mov edx, [rsi + ONODE.pathgen]
    call path_ok
    test eax, eax
    jnz .havepath
.newpath:
    mov rcx, rsi
    call path_get
    cmp rax, -1
    je .ret
    mov [rsi + ONODE.path], al
    mov ecx, eax
    call path_ptr
    mov eax, [rax + PATH.gen]
    mov [rsi + ONODE.pathgen], eax
.havepath:
    ; pending entry: a free one, else the oldest
    lea rdi, [rel pending]
    mov r14, rdi
    mov r15, -1
    xor ecx, ecx
.pf:
    cmp byte [rdi + PEND.used], 0
    je .ptake
    mov rax, [rdi + PEND.sent]
    cmp rax, r15
    jae .pnext
    mov r15, rax
    mov r14, rdi
.pnext:
    add rdi, PEND_size
    inc ecx
    cmp ecx, MAX_PENDING
    jb .pf
    mov rdi, r14
.ptake:
    mov r14, rdi               ; keep the pending entry for failure cleanup
    mov byte [rdi + PEND.used], 1
    mov [rdi + PEND.target], r12b
    mov [rdi + PEND.slot], r13b
    mov al, [rsi + ONODE.path]
    mov [rdi + PEND.path], al
    mov eax, [rsi + ONODE.pathgen]
    mov [rdi + PEND.pathgen], eax
    mov rax, [rel g_now]
    mov [rdi + PEND.sent], rax
    lea rcx, [rdi + PEND.id]
    mov edx, 8
    call rand_bytes
    test eax, eax
    jnz .rngfail
    lea rcx, [rdi + PEND.nodepk]
    mov rdx, rsi
    mov r8d, 32
    call mem_cpy
    ; plain = ping_id | searched pk | data pk | sendback   (104 bytes at LOC(0))
    lea rcx, [LOC(0)]
    mov edx, 104
    call mem_zero
    cmp byte [rbx + TARGET.isself], 0
    je .nself
    lea rcx, [LOC(0)]
    lea rdx, [rsi + ONODE.pingid]
    mov r8d, 32
    call mem_cpy
    lea rcx, [LOC(64)]
    lea rdx, [rel g_onionpk]
    mov r8d, 32
    call mem_cpy
.nself:
    lea rcx, [LOC(32)]
    lea rdx, [rbx + TARGET.pk]
    mov r8d, 32
    call mem_cpy
    mov rax, [rdi + PEND.id]
    mov [LOC(96)], rax
    ; packet = [0x83] nonce sendpk seal(shared(node, sendsk), plain)
    lea rdi, [rel buf_ann]
    mov byte [rdi], 0x83
    lea rcx, [rdi + 1]
    mov edx, 24
    call rand_bytes
    test eax, eax
    jnz .rngfail
    lea rcx, [rdi + 25]
    lea rdx, [rbx + TARGET.sendpk]
    mov r8d, 32
    call mem_cpy
    lea rcx, [LOC(112)]
    mov rdx, rsi
    lea r8, [rbx + TARGET.sendsk]
    call box_beforenm
    lea rcx, [rdi + 57]
    lea rdx, [LOC(112)]
    lea r8, [rdi + 1]
    lea r9, [LOC(0)]
    mov qword [rsp+32], 104
    call box_seal
    movzx ecx, byte [rsi + ONODE.path]
    mov rdx, rsi
    mov r8, rdi
    lea r9, [rax + 57]
    call onion_send
    test eax, eax
    jnz .ret
.rngfail:
    mov byte [r14 + PEND.used], 0
.ret:
    EPILOG FR

; onion_tick(): expire requests, refill and query nodes for every target
onion_tick:
%assign FR 16
    PROLOG FR
    mov r15, [rel g_now]
    ; expire pending requests
    lea rbx, [rel pending]
    xor edi, edi
.exp:
    cmp byte [rbx + PEND.used], 0
    je .expn
    mov rax, r15
    sub rax, [rbx + PEND.sent]
    cmp rax, 6000
    jb .expn
    mov byte [rbx + PEND.used], 0
    movzx ecx, byte [rbx + PEND.path]
    cmp ecx, MAX_PATHS
    jae .nodefail
    call path_ptr
    mov ecx, [rbx + PEND.pathgen]
    cmp [rax + PATH.gen], ecx
    jne .nodefail
    inc byte [rax + PATH.fails]
.nodefail:
    movzx ecx, byte [rbx + PEND.target]
    call target_ptr
    movzx ecx, byte [rbx + PEND.slot]
    imul rcx, rcx, ONODE_size
    lea rsi, [rax + TARGET.nodes + rcx]
    cmp byte [rsi + ONODE.used], 0
    je .expn
    mov rcx, rsi
    lea rdx, [rbx + PEND.nodepk]
    mov r8d, 32
    call mem_eq
    test eax, eax
    jz .expn
    inc byte [rsi + ONODE.unans]
    cmp byte [rsi + ONODE.unans], 3
    jb .expn
    mov byte [rsi + ONODE.used], 0
.expn:
    add rbx, PEND_size
    inc edi
    cmp edi, MAX_PENDING
    jb .exp
    ; targets
    xor r12d, r12d
.tgt:
    test r12d, r12d
    jz .active
    lea ecx, [r12 - 1]
    call friend_ptr
    cmp byte [rax + FRIEND.used], 0
    je .tnext
.active:
    mov ecx, r12d
    call target_ptr
    mov rbx, rax
    ; count used nodes, refill from the bootstrap list if needed
    xor esi, esi
    xor edi, edi
.cu:
    mov rax, rdi
    imul rax, rax, ONODE_size
    cmp byte [rbx + TARGET.nodes + rax + ONODE.used], 0
    je .cun
    inc esi
.cun:
    inc edi
    cmp edi, ONION_NODES
    jb .cu
    cmp esi, ONION_NODES
    jae .query
    lea r13, [rel bootstrap_nodes]
    xor edi, edi
.bs:
    mov ecx, r12d
    mov rdx, r13
    call target_add
    add r13, NODE_size
    inc edi
    cmp edi, BOOTSTRAP_COUNT
    jb .bs
.query:
    xor edi, edi
.q:
    mov rax, rdi
    imul rax, rax, ONODE_size
    lea rsi, [rbx + TARGET.nodes + rax]
    cmp byte [rsi + ONODE.used], 0
    je .qn
    mov ecx, 15000
    cmp qword [rsi + ONODE.lastrecv], 0
    je .fastq
    cmp byte [rbx + TARGET.isself], 0
    je .slow
    cmp byte [rsi + ONODE.stored], 2
    je .slow
.fastq:
    mov ecx, 3000
.slow:
    mov rax, r15
    sub rax, [rsi + ONODE.lastsent]
    cmp rax, rcx
    jb .qn
    mov ecx, r12d
    mov edx, edi
    call announce_send
.qn:
    inc edi
    cmp edi, ONION_NODES
    jb .q
.tnext:
    inc r12d
    cmp r12d, MAX_TARGETS
    jb .tgt
    EPILOG FR

; onion_response(rcx = data, rdx = len)  (from TCP_PACKET_ONION_RESPONSE)
onion_response:
    cmp byte [rcx], 0x84
    je handle_announce_response
    cmp byte [rcx], 0x86
    je handle_onion_data
    ret

; handle_announce_response(rcx = data, rdx = len)
handle_announce_response:
%assign FR 96
    PROLOG FR
    mov rsi, rcx
    mov r12, rdx
    cmp r12, 1 + 8 + 24 + 1 + 32 + 16
    jb .ret
    mov rax, [rsi + 1]          ; sendback id
    lea rdi, [rel pending]
    xor ecx, ecx
.f:
    cmp byte [rdi + PEND.used], 0
    je .fn
    cmp [rdi + PEND.id], rax
    je .found
.fn:
    add rdi, PEND_size
    inc ecx
    cmp ecx, MAX_PENDING
    jb .f
    jmp .ret
.found:
    movzx r13d, byte [rdi + PEND.target]
    mov ecx, r13d
    call target_ptr
    mov rbx, rax
    movzx eax, byte [rdi + PEND.slot]
    imul rax, rax, ONODE_size
    lea r14, [rbx + TARGET.nodes + rax]
    cmp byte [r14 + ONODE.used], 0
    je .ret
    mov rcx, r14
    lea rdx, [rdi + PEND.nodepk]
    mov r8d, 32
    call mem_eq
    test eax, eax
    jz .ret
    lea rcx, [LOC(0)]
    mov rdx, r14
    lea r8, [rbx + TARGET.sendsk]
    call box_beforenm
    lea rcx, [rel onion_plain]
    lea rdx, [LOC(0)]
    lea r8, [rsi + 9]
    lea r9, [rsi + 33]
    lea rax, [r12 - 33]
    mov [rsp+32], rax
    mov qword [rsp+40], 2048    ; onion_plain capacity
    call box_open
    cmp rax, 33
    jl .ret
    mov r15, rax                ; plaintext length
    mov byte [rdi + PEND.used], 0
    ; the request made it: path and node are fine
    movzx ecx, byte [rdi + PEND.path]
    cmp ecx, MAX_PATHS
    jae .pok
    call path_ptr
    mov ecx, [rdi + PEND.pathgen]
    cmp [rax + PATH.gen], ecx
    jne .pok
    mov byte [rax + PATH.fails], 0
.pok:
    mov byte [r14 + ONODE.unans], 0
    mov rax, [rel g_now]
    mov [r14 + ONODE.lastrecv], rax
    lea rsi, [rel onion_plain]
    movzx eax, byte [rsi]       ; is_stored
    cmp byte [rbx + TARGET.isself], 0
    je .friend
    mov [r14 + ONODE.stored], al
    mov r12d, eax
    lea rcx, [r14 + ONODE.pingid]
    lea rdx, [rsi + 1]
    mov r8d, 32
    call mem_cpy
    cmp r12d, 2
    je .fastreset
    test r12d, r12d
    jnz .nodes
    cmp byte [r14 + ONODE.fast], 3
    jae .nodes
    inc byte [r14 + ONODE.fast]
    mov ecx, r13d               ; re-announce right away with the new ping id
    movzx edx, byte [rdi + PEND.slot]
    call announce_send
    jmp .nodes
.fastreset:
    mov byte [r14 + ONODE.fast], 0
    jmp .nodes
.friend:
    cmp eax, 1
    jne .notstored
    mov byte [r14 + ONODE.stored], 1
    lea rcx, [r14 + ONODE.datapk]
    lea rdx, [rsi + 1]
    mov r8d, 32
    call mem_cpy
    jmp .nodes
.notstored:
    mov byte [r14 + ONODE.stored], 0
.nodes:
    ; closer nodes follow at plain + 33
    lea rsi, [rel onion_plain + 33]
    lea r12, [r15 - 33]
.nl:
    test r12, r12
    jle .ret
    movzx eax, byte [rsi]
    cmp eax, 2
    je .v4
    cmp eax, 130
    je .skip4
    cmp eax, 10
    je .skip6
    cmp eax, 138
    je .skip6
    jmp .ret
.v4:
    cmp r12, 39
    jl .ret
    lea rcx, [LOC(32)]          ; NODE
    lea rdx, [rsi + 7]
    mov r8d, 32
    call mem_cpy
    mov eax, [rsi + 1]
    mov [LOC(32) + NODE.ip], eax
    mov ax, [rsi + 5]
    mov [LOC(32) + NODE.port], ax
    mov ecx, r13d
    lea rdx, [LOC(32)]
    call target_add
.skip4:
    add rsi, 39
    sub r12, 39
    jmp .nl
.skip6:
    add rsi, 51
    sub r12, 51
    jmp .nl
.ret:
    EPILOG FR

; send_onion_data(ecx = friend index, rdx = data, r8 = len) -> rax = nodes sent to
send_onion_data:
%assign FR 128
    PROLOG FR
    mov r12d, ecx
    mov rsi, rdx
    mov r13, r8
    xor r15d, r15d              ; sent count, including early RNG failures
    call friend_ptr
    mov rbx, rax
    lea rcx, [LOC(0)]           ; nonce
    mov edx, 24
    call rand_bytes
    test eax, eax
    jnz .done
    ; inner = our real pk || seal(shared(friend real pk, our real sk), data)
    lea rcx, [LOC(32)]
    lea rdx, [rbx + FRIEND.realpk]
    lea r8, [rel g_realsk]
    call box_beforenm
    lea rdi, [rel buf_inner]
    mov rcx, rdi
    lea rdx, [rel g_realpk]
    mov r8d, 32
    call mem_cpy
    lea rcx, [rdi + 32]
    lea rdx, [LOC(32)]
    lea r8, [LOC(0)]
    mov r9, rsi
    mov [rsp+32], r13
    call box_seal
    lea r13, [rax + 32]         ; inner length
    lea ecx, [r12 + 1]
    call target_ptr
    lea r14, [rax + TARGET.nodes]
    xor edi, edi
.l:
    cmp byte [r14 + ONODE.used], 0
    je .n
    cmp byte [r14 + ONODE.stored], 1
    jne .n
    mov rcx, r14
    call path_get
    cmp rax, -1
    je .n
    mov ebp, eax
    lea rcx, [LOC(64)]          ; temp pk
    lea rdx, [LOC(96)]          ; temp sk (overwritten by shared key below)
    call box_keypair
    test eax, eax
    jnz .done
    lea rcx, [LOC(32)]
    lea rdx, [r14 + ONODE.datapk]
    lea r8, [LOC(96)]
    call box_beforenm
    ; request = [0x85] friend real pk, nonce, temp pk, seal(inner)
    lea rsi, [rel buf_req]
    mov byte [rsi], 0x85
    lea rcx, [rsi + 1]
    lea rdx, [rbx + FRIEND.realpk]
    mov r8d, 32
    call mem_cpy
    lea rcx, [rsi + 33]
    lea rdx, [LOC(0)]
    mov r8d, 24
    call mem_cpy
    lea rcx, [rsi + 57]
    lea rdx, [LOC(64)]
    mov r8d, 32
    call mem_cpy
    lea rcx, [rsi + 89]
    lea rdx, [LOC(32)]
    lea r8, [LOC(0)]
    lea r9, [rel buf_inner]
    mov [rsp+32], r13
    call box_seal
    mov ecx, ebp
    mov rdx, r14
    mov r8, rsi
    lea r9, [rax + 89]
    call onion_send
    test eax, eax
    jz .n
    inc r15d
.n:
    add r14, ONODE_size
    inc edi
    cmp edi, ONION_NODES
    jb .l
.done:
    mov eax, r15d
    EPILOG FR

; send_dhtpk(ecx = friend index) -> rax = nodes sent to
;   [156][no_replay BE64][our DHT pk][up to 3 TCP relays]
send_dhtpk:
%assign FR 0
    PROLOG FR
    mov r12d, ecx
    call unix_time
    mov rcx, [rel g_noreplay]
    cmp rax, rcx
    ja .t
    lea rax, [rcx + 1]
.t:
    mov [rel g_noreplay], rax
    lea rdi, [rel buf_dhtpk]
    mov byte [rdi], 156
    bswap rax
    mov [rdi + 1], rax
    lea rcx, [rdi + 9]
    lea rdx, [rel g_dhtpk]
    mov r8d, 32
    call mem_cpy
    lea rcx, [rdi + 41]
    mov edx, 3
    call pack_relays
    lea r8, [rax + 41]
    mov ecx, r12d
    mov rdx, rdi
    call send_onion_data
    EPILOG FR

; pack_relays(rcx = out, edx = max) -> rax = bytes written
;   connected relays as packed TCP nodes, relays routed to nobody first doesn't matter
pack_relays:
%assign FR 0
    PROLOG FR
    mov rdi, rcx
    mov r12d, edx
    xor r13d, r13d              ; written
    xor r14d, r14d              ; count
    lea rbx, [rel relays]
    xor esi, esi
.l:
    cmp r14d, r12d
    jae .done
    cmp dword [rbx + RELAY.state], RS_READY
    jne .n
    lea r15, [rdi + r13]
    mov byte [r15], 130         ; TCP IPv4
    mov eax, [rbx + RELAY.node + NODE.ip]
    mov [r15 + 1], eax
    mov ax, [rbx + RELAY.node + NODE.port]
    mov [r15 + 5], ax
    lea rcx, [r15 + 7]
    lea rdx, [rbx + RELAY.node]
    mov r8d, 32
    call mem_cpy
    add r13d, 39
    inc r14d
.n:
    add rbx, RELAY_size
    inc esi
    cmp esi, MAX_RELAYS
    jb .l
.done:
    mov eax, r13d
    EPILOG FR

; add_tcp_nodes(rcx = packed nodes, rdx = len, r8 = friend or 0)
;   connect to the TCP relays listed (and route to the friend through them)
add_tcp_nodes:
%assign FR 48
    PROLOG FR
    mov rsi, rcx
    mov r12, rdx
    mov r13, r8
.l:
    test r12, r12
    jle .ret
    movzx eax, byte [rsi]
    cmp eax, 130
    je .tcp4
    cmp eax, 2
    je .skip4
    cmp eax, 10
    je .skip6
    cmp eax, 138
    je .skip6
    jmp .ret
.tcp4:
    cmp r12, 39
    jl .ret
    lea rcx, [LOC(0)]
    lea rdx, [rsi + 7]
    mov r8d, 32
    call mem_cpy
    mov eax, [rsi + 1]
    mov [LOC(0) + NODE.ip], eax
    mov ax, [rsi + 5]
    mov [LOC(0) + NODE.port], ax
    lea rcx, [LOC(0)]
    call relay_add
    cmp rax, -1
    je .skip4
    test r13, r13
    jz .skip4
    cmp byte [r13 + FRIEND.hasdht], 0
    je .skip4
    mov ecx, eax
    call relay_ptr
    mov rcx, rax
    lea rdx, [r13 + FRIEND.dhtpk]
    call relay_request_route
.skip4:
    add rsi, 39
    sub r12, 39
    jmp .l
.skip6:
    add rsi, 51
    sub r12, 51
    jmp .l
.ret:
    EPILOG FR

; friend_by_realpk(rcx = pk) -> rax = friend index or -1
friend_by_realpk:
%assign FR 0
    PROLOG FR
    mov rsi, rcx
    lea rbx, [rel friends]
    xor edi, edi
.l:
    cmp byte [rbx + FRIEND.used], 0
    je .n
    lea rcx, [rbx + FRIEND.realpk]
    mov rdx, rsi
    mov r8d, 32
    call mem_eq
    test eax, eax
    jz .n
    mov eax, edi
    jmp .ret
.n:
    add rbx, FRIEND_size
    inc edi
    cmp edi, MAX_FRIENDS
    jb .l
    mov rax, -1
.ret:
    EPILOG FR

; handle_onion_data(rcx = data, rdx = len): [0x86][nonce][temp pk][enc]
handle_onion_data:
%assign FR 64
    PROLOG FR
    mov rsi, rcx
    mov r12, rdx
    cmp r12, 1 + 24 + 32 + 32 + 16 + 16
    jbe .ret
    lea rcx, [LOC(0)]
    lea rdx, [rsi + 25]
    lea r8, [rel g_onionsk]
    call box_beforenm
    lea rcx, [rel onion_plain]
    lea rdx, [LOC(0)]
    lea r8, [rsi + 1]
    lea r9, [rsi + 57]
    lea rax, [r12 - 57]
    mov [rsp+32], rax
    mov qword [rsp+40], 2048    ; onion_plain capacity
    call box_open
    cmp rax, 48
    jle .ret
    mov r13, rax
    ; inner layer: [sender real pk][enc with real keys]
    lea rcx, [LOC(0)]
    lea rdx, [rel onion_plain]
    lea r8, [rel g_realsk]
    call box_beforenm
    lea rcx, [rel onion_plain2]
    lea rdx, [LOC(0)]
    lea r8, [rsi + 1]
    lea r9, [rel onion_plain + 32]
    lea rax, [r13 - 32]
    mov [rsp+32], rax
    mov qword [rsp+40], 2048    ; onion_plain2 capacity
    call box_open
    cmp rax, 1
    jl .ret
    mov r13, rax
    lea rdi, [rel onion_plain2]
    cmp byte [rdi], 156
    jne .ret
    lea rcx, [rel onion_plain]
    call friend_by_realpk
    cmp rax, -1
    je .ret
    mov ecx, eax
    mov rdx, rdi
    mov r8, r13
    call handle_dhtpk
.ret:
    EPILOG FR

; handle_dhtpk(ecx = friend index, rdx = data, r8 = len)
handle_dhtpk:
%assign FR 0
    PROLOG FR
    mov r12d, ecx
    mov rsi, rdx
    mov r13, r8
    cmp r13, 41
    jb .ret
    call friend_ptr
    mov rbx, rax
    mov rax, [rsi + 1]
    bswap rax
    cmp rax, [rbx + FRIEND.lastnoreplay]
    jbe .ret
    mov [rbx + FRIEND.lastnoreplay], rax
    lea rcx, [rsi + 41]
    lea rdx, [r13 - 41]
    mov r8, rbx
    call add_tcp_nodes
    cmp byte [rbx + FRIEND.hasdht], 0
    je .new
    lea rcx, [rbx + FRIEND.dhtpk]
    lea rdx, [rsi + 9]
    mov r8d, 32
    call mem_eq
    test eax, eax
    jnz .ret
.new:
    mov rcx, rbx
    lea rdx, [rsi + 9]
    call set_friend_dht
.ret:
    EPILOG FR

