; File transfers in assembly, wire-compatible with c-toxcore (and the pure-Go
; tox library's file.go), so a Go/qTox peer and this client exchange files.
;
; Three lossless packet types ride net_crypto, identical on the wire to toxcore:
;   80 SENDREQUEST  [slot][kind:4 BE][size:8 BE][file_id:32][name:0..255]
;   81 CONTROL      [dir][slot][type][payload]   type 0 resume 1 pause 2 kill 3 seek
;   82 DATA         [slot][bytes:0..1371]         position is implicit
;
; Unlike the Go library (which hands chunks to the application through a
; callback), this port does the file I/O itself with Win32: a sender is given a
; path, the library opens it and, as the reliable-send window drains, reads and
; sends the next chunk (file_tick, called from crypto_tick). A receiver accepts
; an offer with a destination path and the library writes each incoming chunk.
; The transfer is still pull-paced by the window, so a bulk file never starves
; messages or other transfers.

; Win32 constants.
GENERIC_READ     equ 0x80000000
GENERIC_WRITE    equ 0x40000000
FILE_SHARE_READ  equ 1
OPEN_EXISTING    equ 3
CREATE_ALWAYS    equ 2
FILE_ATTR_NORMAL equ 0x80
FILE_BEGIN       equ 0

; wire control types (toxcore Filecontrol)
FC_ACCEPT equ 0
FC_PAUSE  equ 1
FC_KILL   equ 2
FC_SEEK   equ 3

; ---- small leaf helpers (volatile regs only) ----

; ft_find(rcx = friend, edx = receiving 0/1, r8d = wire slot) -> rax = FT* or 0
ft_find:
    lea r9, [rcx + FRIEND.fsend]
    test edx, edx
    jz .have
    lea r9, [rcx + FRIEND.frecv]
.have:
    xor eax, eax
.loop:
    cmp byte [r9 + FT.status], FT_NONE
    je .next
    movzx r10d, byte [r9 + FT.slot]
    cmp r10d, r8d
    je .found
.next:
    add r9, FT_size
    inc eax
    cmp eax, MAX_FILES
    jb .loop
    xor eax, eax
    ret
.found:
    mov rax, r9
    ret

; ft_free_slot(rcx = table base) -> eax = free index or -1
ft_free_slot:
    xor eax, eax
.loop:
    mov r9, rax
    imul r9, r9, FT_size
    cmp byte [rcx + r9 + FT.status], FT_NONE
    je .found
    inc eax
    cmp eax, MAX_FILES
    jb .loop
    mov eax, -1
.found:
    ret

; friend_idx(rcx = friend ptr) -> eax = index
friend_idx:
    mov rax, rcx
    lea r9, [rel friends]
    sub rax, r9
    xor edx, edx
    mov r9d, FRIEND_size
    div r9
    ret

; ---- control packet sender ----

; file_send_ctrl(rcx = friend, edx = dir, r8d = slot, r9d = ctrl, [5] = data, [6] = dlen)
file_send_ctrl:
%assign FR 32
    PROLOG FR
    mov rbx, rcx
    mov byte [LOC(0)], 81           ; PACKET_ID_FILE_CONTROL
    mov [LOC(1)], dl
    mov [LOC(2)], r8b
    mov [LOC(3)], r9b
    mov r13d, 4
    mov r14, [ARG5]                 ; data ptr (may be 0)
    mov r15, [ARG6]                 ; data len
    test r14, r14
    jz .send
    test r15, r15
    jz .send
    lea rcx, [LOC(4)]
    mov rdx, r14
    mov r8, r15
    call mem_cpy
    lea r13, [r15 + 4]
.send:
    mov rcx, rbx
    lea rdx, [LOC(0)]
    mov r8, r13
    call send_lossless
    EPILOG FR

; ---- inbound handlers (rcx=friend, rdx=payload, r8=len), run from handle_data ----

; file_handle_sendreq: an incoming file offer. Register it and queue it to poll.
file_handle_sendreq:
%assign FR 32
    PROLOG FR
    mov rbx, rcx                    ; friend
    mov rsi, rdx                    ; payload
    mov r12, r8                     ; len
    cmp r12, 45                     ; 1 slot + 4 kind + 8 size + 32 id
    jb .ret
    lea rax, [r12 - 45]             ; name length
    cmp rax, 255
    ja .ret
    mov r14, rax                    ; name length
    movzx r13d, byte [rsi]          ; wire slot

    ; reject a duplicate offer for the same slot
    mov rcx, rbx
    mov edx, 1
    mov r8d, r13d
    call ft_find
    test rax, rax
    jnz .ret

    lea rcx, [rbx + FRIEND.frecv]
    call ft_free_slot
    cmp eax, -1
    je .ret
    mov r15d, eax                   ; recv table index
    imul rax, rax, FT_size
    lea rdi, [rbx + FRIEND.frecv + rax]  ; FT*

    mov rcx, rdi
    mov edx, FT_size
    call mem_zero
    mov byte [rdi + FT.status], FT_NOTACC
    mov byte [rdi + FT.slot], r13b
    mov rax, [rsi + 5]              ; size, big-endian on the wire
    bswap rax
    mov [rdi + FT.size], rax
    lea rcx, [rdi + FT.id]
    lea rdx, [rsi + 13]
    mov r8d, 32
    call mem_cpy

    ; push an offer: friend index, public filenum (index|0x10000), size, name
    mov rcx, rbx
    call friend_idx                 ; eax = friend index
    mov ebp, eax
    call offer_free                 ; rax = OFFER* or 0
    test rax, rax
    jz .ret
    mov rdi, rax
    mov dword [rdi + OFFER.used], 1
    mov [rdi + OFFER.friend], ebp
    mov eax, r15d
    or eax, 0x10000
    mov [rdi + OFFER.filenum], eax
    mov eax, r14d
    mov [rdi + OFFER.namelen], eax
    mov rax, [rsi + 5]
    bswap rax
    mov [rdi + OFFER.size], rax
    test r14, r14
    jz .ret
    lea rcx, [rdi + OFFER.name]
    lea rdx, [rsi + 45]
    mov r8, r14
    call mem_cpy
.ret:
    EPILOG FR

; file_handle_control
file_handle_control:
%assign FR 32
    PROLOG FR
    mov rbx, rcx
    mov rsi, rdx
    mov r12, r8
    cmp r12, 3
    jb .ret
    movzx r13d, byte [rsi]          ; dir (their inbound flag)
    movzx r14d, byte [rsi + 1]      ; slot
    movzx r15d, byte [rsi + 2]      ; control type
    ; outbound (our sending side) iff their dir==1
    xor ebp, ebp
    cmp r13d, 1
    jne .notout
    mov ebp, 1                      ; ebp = outbound(=our send)
.notout:
    ; find the transfer: our send table if outbound, else our recv table
    mov rcx, rbx
    mov edx, 1
    sub edx, ebp                    ; receiving = !outbound
    mov r8d, r14d
    call ft_find
    test rax, rax
    jnz .have
    ; unknown transfer: tell the peer to drop it too (unless it's a kill)
    cmp r15d, FC_KILL
    je .ret
    mov rcx, rbx
    mov edx, 1
    sub edx, ebp                    ; reply dir = receiving(!outbound)
    mov r8d, r14d
    mov r9d, FC_KILL
    mov qword [rsp + 32], 0         ; no payload
    mov qword [rsp + 40], 0
    call file_send_ctrl
    jmp .ret
.have:
    mov rdi, rax                    ; FT*
    cmp r15d, FC_ACCEPT
    je .accept
    cmp r15d, FC_PAUSE
    je .pause
    cmp r15d, FC_KILL
    je .kill
    cmp r15d, FC_SEEK
    je .seek
    jmp .ret
.accept:
    ; peer accepted a file we are sending -> start transferring
    test ebp, ebp
    jz .resume
    cmp byte [rdi + FT.status], FT_NOTACC
    jne .resume
    mov byte [rdi + FT.status], FT_XFER
    inc dword [rbx + FRIEND.numsending]
    jmp .ret
.resume:
    mov al, [rdi + FT.paused]
    test al, FPAUSE_OTHER
    jz .ret
    and al, ~FPAUSE_OTHER & 0xff
    mov [rdi + FT.paused], al
    jmp .ret
.pause:
    cmp byte [rdi + FT.status], FT_XFER
    jne .ret
    mov al, [rdi + FT.paused]
    test al, FPAUSE_OTHER
    jnz .ret
    or al, FPAUSE_OTHER
    mov [rdi + FT.paused], al
    jmp .ret
.kill:
    ; adjust numsending if this was one of our active sends
    test ebp, ebp
    jz .killclose
    cmp byte [rdi + FT.status], FT_XFER
    jb .killclose                   ; NONE/NOTACC
    mov eax, [rbx + FRIEND.numsending]
    test eax, eax
    jz .killclose
    dec dword [rbx + FRIEND.numsending]
.killclose:
    mov rcx, rdi
    call ft_close
    jmp .ret
.seek:
    ; only the sender honours a seek, before the transfer is accepted
    test ebp, ebp
    jz .ret
    cmp byte [rdi + FT.status], FT_NOTACC
    jne .ret
    cmp r12, 11                     ; 3 header + 8 position
    jb .ret
    mov rax, [rsi + 3]
    bswap rax                       ; seek position
    mov r13, rax
    mov rax, [rdi + FT.size]
    cmp rax, -1
    je .doseek                      ; stream: any position
    cmp r13, rax
    jae .ret
.doseek:
    mov [rdi + FT.transferred], r13
    mov rax, [rdi + FT.handle]
    test rax, rax
    jz .ret
    cmp rax, -1
    je .ret
    mov rcx, rax
    mov rdx, r13
    xor r8, r8
    mov r9d, FILE_BEGIN
    call WINAPI(SetFilePointerEx)
.ret:
    EPILOG FR

; file_handle_data: an incoming chunk; write it to the open destination file.
file_handle_data:
%assign FR 32
    PROLOG FR
    mov rbx, rcx
    mov rsi, rdx
    mov r12, r8
    cmp r12, 1
    jb .ret
    movzx r8d, byte [rsi]           ; slot
    mov rcx, rbx
    mov edx, 1                      ; receiving
    call ft_find
    test rax, rax
    jz .ret
    mov rdi, rax
    cmp byte [rdi + FT.status], FT_XFER
    jne .ret
    lea r13, [rsi + 1]              ; data
    lea r14, [r12 - 1]              ; wire data length (before clamp)

    mov r15, r14                    ; effective length to write
    mov rax, [rdi + FT.size]
    cmp rax, -1
    je .write                      ; stream: no clamp
    mov rdx, [rdi + FT.transferred]
    add rdx, r15
    cmp rdx, rax
    jbe .write
    mov r15, rax
    sub r15, [rdi + FT.transferred] ; clamp to remaining
.write:
    test r15, r15
    jz .checkdone
    mov rcx, [rdi + FT.handle]
    mov rdx, r13
    mov r8d, r15d
    lea r9, [LOC(0)]
    mov qword [rsp + 32], 0
    call WINAPI(WriteFile)
    add [rdi + FT.transferred], r15
.checkdone:
    ; end of transfer: zero-length packet, a short packet, or size reached
    test r14, r14
    jz .done
    cmp r14, MAX_FILEDATA
    jne .done
    mov rax, [rdi + FT.size]
    cmp rax, -1
    je .ret                        ; stream, full chunk -> more to come
    mov rdx, [rdi + FT.transferred]
    cmp rdx, rax
    jb .ret
.done:
    mov rcx, rdi
    call ft_close
.ret:
    EPILOG FR

; ---- the send pump ----

; file_tick(rcx = friend): read and send chunks for active outgoing transfers
; while the reliable-send window has room (mirrors toxcore do_reqchunk_filecb).
file_tick:
%assign FR 1408
    PROLOG FR
    mov rbx, rcx
    cmp dword [rbx + FRIEND.numsending], 0
    je .ret
    ; budget = WINDOW - inflight - reserve
    mov eax, [rbx + FRIEND.sendend]
    sub eax, [rbx + FRIEND.sendstart]
    mov r15d, WINDOW
    sub r15d, eax
    sub r15d, FILE_RESERVE          ; r15 = free window slots (signed)
    ; Cap one pass so we never queue more than the relay TX buffer can hold;
    ; file_pump_all runs every iterate, so the window still fills in a few passes.
    cmp r15d, FILE_MAXBURST
    jle .capped
    mov r15d, FILE_MAXBURST
.capped:

    xor r12d, r12d                  ; file index
.files:
    mov rax, r12
    imul rax, rax, FT_size
    lea rdi, [rbx + FRIEND.fsend + rax]  ; FT*
    movzx eax, byte [rdi + FT.status]
    cmp eax, FT_DONE
    je .finish
    cmp eax, FT_XFER
    jne .nextfile
    cmp byte [rdi + FT.paused], 0
    jne .nextfile
.chunk:
    cmp r15d, 0
    jle .nextfile
    ; remaining = size - transferred (unless stream)
    mov rax, [rdi + FT.size]
    cmp rax, -1
    je .readlen
    mov rdx, [rdi + FT.transferred]
    cmp rdx, rax
    jb .haveleft
    jmp .finish                     ; all bytes sent
.haveleft:
    sub rax, rdx                    ; remaining
    cmp rax, MAX_FILEDATA
    jb .len
.readlen:
    mov rax, MAX_FILEDATA
.len:
    mov r14d, eax                   ; chunk length to read
    ; ReadFile(handle, buf+2, len, &bytesRead, NULL)
    mov rcx, [rdi + FT.handle]
    lea rdx, [LOC(2)]
    mov r8d, r14d
    lea r9, [LOC(1384)]             ; bytesRead (DWORD)
    mov qword [rsp + 32], 0
    mov dword [LOC(1384)], 0
    call WINAPI(ReadFile)
    test eax, eax
    jz .finish                      ; read error -> finish/close
    mov r13d, [LOC(1384)]           ; bytes actually read
    test r13d, r13d
    jz .finish                      ; EOF
    ; build [82][slot][data] and queue it
    mov byte [LOC(0)], 82
    mov eax, r12d
    mov [LOC(1)], al                ; slot == index for our sends
    mov rcx, rbx
    lea rdx, [LOC(0)]
    lea r8, [r13 + 2]
    call send_lossless
    test eax, eax
    jnz .nextfile                   ; window full now: try again next tick
    add [rdi + FT.transferred], r13
    dec r15d
    cmp r13d, MAX_FILEDATA
    jb .finish                      ; short read: last chunk
    jmp .chunk
.finish:
    ; send the trailing zero-length packet once, then close and free the slot
    cmp byte [rdi + FT.finalsent], 1
    je .freeslot
    mov byte [LOC(0)], 82
    mov eax, r12d
    mov [LOC(1)], al
    mov rcx, rbx
    lea rdx, [LOC(0)]
    mov r8d, 2
    call send_lossless
    test eax, eax
    jnz .nextfile                   ; couldn't queue final: retry next tick
    mov byte [rdi + FT.finalsent], 1
.freeslot:
    mov eax, [rbx + FRIEND.numsending]
    test eax, eax
    jz .closed
    dec dword [rbx + FRIEND.numsending]
.closed:
    mov rcx, rdi
    call ft_close
.nextfile:
    inc r12d
    cmp r12d, MAX_FILES
    jb .files
.ret:
    EPILOG FR

; file_pump_all(): run the send pump for every established friend. Called every
; iterate (not only every 250 ms) so the reliable-send window is topped up in
; small increments as acknowledgements free it, keeping the link full without
; bursting a whole window into the relay's transmit buffer at once.
file_pump_all:
%assign FR 0
    PROLOG FR
    lea rbx, [rel friends]
    xor r12d, r12d
.l:
    cmp byte [rbx + FRIEND.used], 0
    je .next
    cmp byte [rbx + FRIEND.cstatus], CS_ESTAB
    jne .next
    cmp dword [rbx + FRIEND.numsending], 0
    je .next
    mov rcx, rbx
    call file_tick
.next:
    add rbx, FRIEND_size
    inc r12d
    cmp r12d, MAX_FRIENDS
    jb .l
    EPILOG FR

; file_break(rcx = friend): cancel all transfers (close files) on disconnect.
file_break:
%assign FR 0
    PROLOG FR
    mov rbx, rcx
    mov dword [rbx + FRIEND.numsending], 0
    xor r12d, r12d
.l:
    mov rax, r12
    imul rax, rax, FT_size
    lea rcx, [rbx + FRIEND.fsend + rax]
    call ft_close
    mov rax, r12
    imul rax, rax, FT_size
    lea rcx, [rbx + FRIEND.frecv + rax]
    call ft_close
    inc r12d
    cmp r12d, MAX_FILES
    jb .l
    EPILOG FR

; ft_close(rcx = FT*): close its file handle and reset the slot to FT_NONE.
ft_close:
%assign FR 0
    PROLOG FR
    mov rbx, rcx
    mov r12, [rbx + FT.handle]
    test r12, r12
    jz .z
    cmp r12, -1
    je .z
    mov rcx, r12
    call WINAPI(CloseHandle)
.z:
    mov qword [rbx + FT.handle], 0
    mov byte [rbx + FT.status], FT_NONE
    mov byte [rbx + FT.finalsent], 0
    mov byte [rbx + FT.paused], 0
    EPILOG FR

; offer_free() -> rax = free OFFER* or 0
offer_free:
    lea rax, [rel offers]
    xor r9d, r9d
.l:
    cmp dword [rax + OFFER.used], 0
    je .found
    add rax, OFFER_size
    inc r9d
    cmp r9d, OFFER_SIZE
    jb .l
    xor eax, eax
.found:
    ret

; ---- public API (Win64) ----

; int toxnet_file_send(ecx = friend, rdx = path) -> file number (0..) or -1
toxnet_file_send:
%assign FR 1408
    PROLOG FR
    mov rsi, rdx                    ; path
    mov rax, -1
    cmp ecx, MAX_FRIENDS
    jae .ret
    call friend_ptr
    mov rbx, rax
    mov rax, -1
    cmp byte [rbx + FRIEND.used], 0
    je .ret
    cmp byte [rbx + FRIEND.online], 0
    je .ret
    cmp byte [rbx + FRIEND.cstatus], CS_ESTAB
    jne .ret

    ; open the file for reading
    mov rcx, rsi
    mov edx, GENERIC_READ
    mov r8d, FILE_SHARE_READ
    xor r9, r9
    mov qword [rsp + 32], OPEN_EXISTING
    mov qword [rsp + 40], FILE_ATTR_NORMAL
    mov qword [rsp + 48], 0
    call WINAPI(CreateFileA)
    mov r14, rax                    ; handle
    mov rax, -1
    cmp r14, -1
    je .ret

    ; size
    mov rcx, r14
    lea rdx, [LOC(0)]
    mov qword [LOC(0)], 0
    call WINAPI(GetFileSizeEx)
    test eax, eax
    jz .closeerr
    mov r13, [LOC(0)]               ; file size

    lea rcx, [rbx + FRIEND.fsend]
    call ft_free_slot
    cmp eax, -1
    je .closeerr
    mov r12d, eax                   ; send slot/index
    mov rax, r12
    imul rax, rax, FT_size
    lea rdi, [rbx + FRIEND.fsend + rax]

    mov rcx, rdi
    mov edx, FT_size
    call mem_zero
    mov byte [rdi + FT.status], FT_NOTACC
    mov byte [rdi + FT.slot], r12b
    mov [rdi + FT.size], r13
    mov [rdi + FT.handle], r14
    lea rcx, [rdi + FT.id]
    mov edx, 32
    call rand_bytes
    test eax, eax
    jnz .rngfail

    ; basename of the path (after the last \ or /); rbp survives mem_cpy
    mov rbp, rsi                    ; name start, default = whole path
    mov rcx, rsi
.base:
    movzx eax, byte [rcx]
    test eax, eax
    jz .baseend
    cmp al, '\'
    je .sep
    cmp al, '/'
    jne .basenext
.sep:
    lea rbp, [rcx + 1]
.basenext:
    inc rcx
    jmp .base
.baseend:
    sub rcx, rbp                    ; name length
    cmp rcx, 255
    jbe .namelen
    mov ecx, 255
.namelen:
    mov r15, rcx                    ; name length (survives leaf mem_cpy)

    ; build SENDREQUEST [80][slot][kind:4 BE=0][size:8 BE][id:32][name]
    mov byte [LOC(0)], 80
    mov [LOC(1)], r12b
    mov dword [LOC(2)], 0           ; kind = FILEKIND_DATA, big-endian
    mov rax, r13
    bswap rax
    mov [LOC(6)], rax               ; size, big-endian
    lea rcx, [LOC(14)]
    lea rdx, [rdi + FT.id]
    mov r8d, 32
    call mem_cpy
    test r15, r15
    jz .reqsend
    lea rcx, [LOC(46)]
    mov rdx, rbp
    mov r8, r15
    call mem_cpy
.reqsend:
    mov rcx, rbx
    lea rdx, [LOC(0)]
    lea r8, [r15 + 46]              ; 46-byte header + name
    call send_lossless
    mov eax, r12d                   ; return the file number
    EPILOG FR
.rngfail:
    mov rcx, rdi
    mov edx, FT_size
    call mem_zero                  ; release the reserved slot as well as handle
.closeerr:
    mov rcx, r14
    call WINAPI(CloseHandle)
    mov rax, -1
    EPILOG FR
.ret:
    EPILOG FR

; int toxnet_file_progress(ecx = friend, edx = filenum, r8 = uint64* out) -> status
toxnet_file_progress:
%assign FR 0
    PROLOG FR
    mov r13, r8                     ; out ptr (may be 0)
    mov r14d, edx                   ; filenum
    xor eax, eax                    ; default: FT_NONE
    cmp ecx, MAX_FRIENDS
    jae .ret
    call friend_ptr
    mov rbx, rax
    mov edx, r14d
    test edx, 0x10000
    jz .send
    and edx, 0xffff
    cmp edx, MAX_FILES
    jae .ret
    mov rax, rdx
    imul rax, rax, FT_size
    lea rdi, [rbx + FRIEND.frecv + rax]
    jmp .got
.send:
    cmp edx, MAX_FILES
    jae .ret
    mov rax, rdx
    imul rax, rax, FT_size
    lea rdi, [rbx + FRIEND.fsend + rax]
.got:
    test r13, r13
    jz .nostore
    mov rax, [rdi + FT.transferred]
    mov [r13], rax
.nostore:
    movzx eax, byte [rdi + FT.status]
.ret:
    EPILOG FR

; int toxnet_file_control(ecx = friend, edx = filenum, r8d = control) -> 0 / -1
; control: 0 resume/accept-resume, 1 pause, 2 cancel
toxnet_file_control:
%assign FR 32
    PROLOG FR
    mov r14d, edx                   ; filenum
    mov r15d, r8d                   ; control
    mov rax, -1
    cmp ecx, MAX_FRIENDS
    jae .ret
    call friend_ptr
    mov rbx, rax
    mov edx, r14d
    xor ebp, ebp                    ; receiving flag
    test edx, 0x10000
    jz .snd
    mov ebp, 1
    and edx, 0xffff
.snd:
    cmp edx, MAX_FILES
    jae .reterr
    mov rax, rdx
    imul rax, rax, FT_size
    test ebp, ebp
    jz .sndtab
    lea rdi, [rbx + FRIEND.frecv + rax]
    jmp .havft
.sndtab:
    lea rdi, [rbx + FRIEND.fsend + rax]
.havft:
    cmp byte [rdi + FT.status], FT_NONE
    je .reterr
    movzx r12d, byte [rdi + FT.slot]
    cmp r15d, 2
    je .cancel
    cmp r15d, 1
    je .pause
    ; resume
    mov r9d, FC_ACCEPT
    and byte [rdi + FT.paused], ~FPAUSE_US & 0xff
    jmp .sendit
.pause:
    or byte [rdi + FT.paused], FPAUSE_US
    mov r9d, FC_PAUSE
    jmp .sendit
.cancel:
    mov r9d, FC_KILL
    test ebp, ebp
    jnz .sendkill
    cmp byte [rdi + FT.status], FT_XFER
    jb .sendkill
    mov eax, [rbx + FRIEND.numsending]
    test eax, eax
    jz .sendkill
    dec dword [rbx + FRIEND.numsending]
.sendkill:
    mov rcx, rbx
    mov edx, ebp
    mov r8d, r12d
    mov r9d, FC_KILL
    mov qword [rsp + 32], 0
    mov qword [rsp + 40], 0
    call file_send_ctrl
    mov rcx, rdi
    call ft_close
    xor eax, eax
    EPILOG FR
.sendit:
    mov rcx, rbx
    mov edx, ebp
    mov r8d, r12d
    ; r9d already = ctrl
    mov qword [rsp + 32], 0
    mov qword [rsp + 40], 0
    call file_send_ctrl
    xor eax, eax
    EPILOG FR
.reterr:
    mov rax, -1
    EPILOG FR
.ret:
    EPILOG FR

; int toxnet_file_poll_offer(rcx = char* name, edx = namebuf, r8 = int* friend,
;                            r9 = int* filenum, [5] = uint64* size) -> 1 / 0
toxnet_file_poll_offer:
%assign FR 0
    PROLOG FR
    mov rbx, rcx                    ; name buf
    mov r12d, edx                   ; name buf len
    mov r13, r8                     ; int* friend
    mov r14, r9                     ; int* filenum
    lea rsi, [rel offers]
    xor r15d, r15d
.scan:
    cmp dword [rsi + OFFER.used], 0
    jne .found
    add rsi, OFFER_size
    inc r15d
    cmp r15d, OFFER_SIZE
    jb .scan
    xor eax, eax                    ; no offer
    EPILOG FR
.found:
    test r13, r13
    jz .nof
    mov eax, [rsi + OFFER.friend]
    mov [r13], eax
.nof:
    test r14, r14
    jz .nofn
    mov eax, [rsi + OFFER.filenum]
    mov [r14], eax
.nofn:
    mov r9, [ARG5]                  ; uint64* size
    test r9, r9
    jz .nos
    mov rax, [rsi + OFFER.size]
    mov [r9], rax
.nos:
    mov ebp, [rsi + OFFER.namelen]
    cmp ebp, r12d
    jbe .cpy
    mov ebp, r12d
.cpy:
    test ebp, ebp
    jz .donecopy
    mov rcx, rbx
    lea rdx, [rsi + OFFER.name]
    mov r8d, ebp
    call mem_cpy
.donecopy:
    mov dword [rsi + OFFER.used], 0 ; consume the offer
    mov eax, 1
    EPILOG FR

; int toxnet_file_accept(ecx = friend, edx = filenum, r8 = char* path) -> 0 / -1
toxnet_file_accept:
%assign FR 32
    PROLOG FR
    mov r14d, edx                   ; filenum
    mov r15, r8                     ; path
    mov rax, -1
    cmp ecx, MAX_FRIENDS
    jae .ret
    test r14d, 0x10000              ; must be a receiving transfer
    jz .ret
    call friend_ptr
    mov rbx, rax
    mov edx, r14d
    and edx, 0xffff
    cmp edx, MAX_FILES
    jae .ret
    mov rax, rdx
    imul rax, rax, FT_size
    lea rdi, [rbx + FRIEND.frecv + rax]
    cmp byte [rdi + FT.status], FT_NOTACC
    jne .ret
    ; open destination for writing
    mov rcx, r15
    mov edx, GENERIC_WRITE
    xor r8, r8
    xor r9, r9
    mov qword [rsp + 32], CREATE_ALWAYS
    mov qword [rsp + 40], FILE_ATTR_NORMAL
    mov qword [rsp + 48], 0
    call WINAPI(CreateFileA)
    cmp rax, -1
    je .reterr
    mov [rdi + FT.handle], rax
    mov byte [rdi + FT.status], FT_XFER
    ; send ACCEPT (dir=1 receiving)
    movzx r8d, byte [rdi + FT.slot]
    mov rcx, rbx
    mov edx, 1
    mov r9d, FC_ACCEPT
    mov qword [rsp + 32], 0
    mov qword [rsp + 40], 0
    call file_send_ctrl
    xor eax, eax
    EPILOG FR
.reterr:
    mov rax, -1
    EPILOG FR
.ret:
    EPILOG FR
