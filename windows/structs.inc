; Data structures and limits.

MAX_RELAYS      equ 16
MAX_CONNECTED   equ 8       ; relays we keep connections open to
MAX_ROUTES      equ 8       ; peers routed per relay
MAX_PATHS       equ 4
MAX_FRIENDS     equ 4
MAX_TARGETS     equ 1 + MAX_FRIENDS
ONION_NODES     equ 8
MAX_PENDING     equ 64
WINDOW          equ 256     ; reliable-packet window (power of two)
MAX_DATA        equ 1373    ; max net_crypto payload
INBOX_SIZE      equ 32
RX_SIZE         equ 8192
TX_SIZE         equ 262144   ; relay transmit buffer (holds many queued packets)

MAX_FILES       equ 4       ; concurrent file transfers per direction, per friend
MAX_FILEDATA    equ MAX_DATA - 2 ; most file bytes in one data packet (1371)
FILE_RESERVE    equ 8       ; send-window slots kept free for messages/acks
FILE_MAXBURST   equ 48      ; most chunks a single pump pass queues (bounds TX use)
OFFER_SIZE      equ 8       ; incoming file-offer queue length (power of two)

; A DHT node / relay address (40 bytes). ip and port are in network byte order.
struc NODE
    .pk:    resb 32
    .ip:    resb 4
    .port:  resb 2
    .tport: resb 2          ; TCP port (only used in the built-in table)
endstruc

struc ROUTE
    .pk:     resb 32
    .id:     resb 1
    .online: resb 1
    .used:   resb 1
    .pad:    resb 5
endstruc

; relay states
RS_UNUSED   equ 0
RS_IDLE     equ 1
RS_CONNECT  equ 2
RS_HANDSHK  equ 3
RS_READY    equ 4

struc RELAY
    .state:  resd 1
    .rxlen:  resd 1
    .txlen:  resd 1
    .pad:    resd 1
    .sock:   resq 1
    .tstate: resq 1         ; time of last state change
    .node:   resb NODE_size
    .shared: resb 32
    .snonce: resb 24
    .rnonce: resb 24
    .tmpsk:  resb 32
    .routes: resb ROUTE_size * MAX_ROUTES
    .rx:     resb RX_SIZE
    .tx:     resb TX_SIZE
endstruc

struc PATH
    .valid:   resb 1
    .fails:   resb 1
    .relay:   resb 1
    .pad:     resb 1
    .gen:     resd 1
    .created: resq 1
    .n2:      resb NODE_size
    .n3:      resb NODE_size
    .pk2:     resb 32
    .pk3:     resb 32
    .k2:      resb 32
    .k3:      resb 32
endstruc

struc ONODE
    .node:     resb NODE_size
    .pingid:   resb 32
    .datapk:   resb 32
    .lastsent: resq 1
    .lastrecv: resq 1
    .pathgen:  resd 1
    .used:     resb 1
    .stored:   resb 1
    .path:     resb 1
    .unans:    resb 1
    .fast:     resb 1
    .pad:      resb 7
endstruc

struc TARGET
    .pk:     resb 32
    .sendpk: resb 32
    .sendsk: resb 32
    .isself: resb 1
    .pad:    resb 7
    .nodes:  resb ONODE_size * ONION_NODES
endstruc

struc PEND
    .used:   resb 1
    .target: resb 1
    .slot:   resb 1
    .path:   resb 1
    .pathgen: resd 1
    .id:     resq 1
    .sent:   resq 1
    .nodepk: resb 32
    .pad:    resb 8
endstruc

struc SPKT                  ; sent reliable packet
    .len:  resd 1
    .pad:  resd 1
    .sent: resq 1
    .data: resb 1376
endstruc

struc RPKT                  ; received reliable packet waiting for its turn
    .present: resd 1
    .len:     resd 1
    .data:    resb 1376
endstruc

; file-transfer status and pause bits (c-toxcore compatible on the wire)
FT_NONE     equ 0
FT_NOTACC   equ 1           ; offered, not yet accepted
FT_XFER     equ 2           ; transferring
FT_DONE     equ 3           ; all bytes handed off; awaiting cleanup
FPAUSE_US    equ 1
FPAUSE_OTHER equ 2

; One file transfer. For a transfer we send, .slot equals its table index; for
; one we receive, .slot is the sender-assigned wire slot (so the peer and we
; name it the same way). .handle is the open Win32 file being read or written.
struc FT
    .status:      resb 1
    .paused:      resb 1
    .slot:        resb 1
    .finalsent:   resb 1    ; sender: the final zero-length packet was sent
    .pad:         resd 1
    .size:        resq 1    ; total bytes (or ~0 for an unknown-length stream)
    .transferred: resq 1    ; bytes read+sent (sender) or written (receiver)
    .handle:      resq 1
    .id:          resb 32
endstruc

; connection states
CS_NONE     equ 0
CS_COOKIE   equ 1
CS_HSSENT   equ 2
CS_NOTCONF  equ 3
CS_ESTAB    equ 4

struc FRIEND
    .used:       resb 1
    .sendreq:    resb 1
    .hasdht:     resb 1
    .online:     resb 1
    .everonline: resb 1
    .cstatus:    resb 1
    .pad00:      resb 1
    .pad0:       resb 1
    .reqlen:     resd 1
    .namelen:    resd 1
    .templen:    resd 1
    .temptries:  resd 1
    .sendstart:  resd 1
    .sendend:    resd 1
    .recvstart:  resd 1
    .recvend:    resd 1
    .pad1:       resd 1
    .lastnoreplay: resq 1
    .lastreqsent:  resq 1   ; friend request (onion)
    .lastdhtpk:    resq 1
    .tempsent:     resq 1
    .lastalive:    resq 1
    .lastalivesent: resq 1
    .lastrequest:  resq 1   ; net_crypto request packet
    .lastshare:    resq 1
    .lastrouted:   resq 1
    .realpk:     resb 32
    .nospam:     resb 4
    .pad2:       resb 4
    .dhtpk:      resb 32
    .peerdht:    resb 32
    .dhtshared:  resb 32
    .cookienum:  resb 8
    .sesspk:     resb 32
    .sesssk:     resb 32
    .sentnonce:  resb 24
    .recvnonce:  resb 24
    .shared:     resb 32
    .req:        resb 256
    .name:       resb 128
    .temp:       resb 400
    .sbuf:       resb SPKT_size * WINDOW
    .rbuf:       resb RPKT_size * WINDOW
    .numsending: resd 1     ; active FT_XFER/FT_DONE entries in fsend (pump gate)
    .fpad:       resd 1
    .fsend:      resb FT_size * MAX_FILES
    .frecv:      resb FT_size * MAX_FILES
endstruc

; An incoming file offer, queued for the application to poll and accept.
struc OFFER
    .used:    resd 1
    .friend:  resd 1
    .filenum: resd 1
    .namelen: resd 1
    .size:    resq 1
    .name:    resb 256
endstruc

struc INMSG
    .friend: resd 1
    .len:    resd 1
    .text:   resb 1376
endstruc
