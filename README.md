# Toxnet

Toxnet is a proof-of-concept end-to-end encrypted (E2EE), peer-to-peer (P2P) command-and-control (C2) framework built on the [Tox](https://tox.chat) protocol. It demonstrates decentralised encrypted messaging as a resilient C2 transport with no centralised infrastructure, paired with a from-scratch reimplementation of the parts of the protocol it actually needs.

<img width="1280" height="640" alt="TOXNET" src="https://github.com/user-attachments/assets/507552c3-0020-4c25-aa40-f6bd13106771" />

![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go&logoColor=white)
![x86-64 ASM](https://img.shields.io/badge/x86--64-ASM-654FF0?logo=assemblyscript&logoColor=white)
![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20Windows-blue)
![License](https://img.shields.io/badge/License-MIT-green)
![Status](https://img.shields.io/badge/Status-Research%20PoC-orange)
![Encryption](https://img.shields.io/badge/E2EE-Tox%20Protocol-purple)

---

It's two projects in one:

1. 🧩 **The Tox implementation.** [DHT](https://en.wikipedia.org/wiki/Distributed_hash_table), [onion routing](https://en.wikipedia.org/wiki/Onion_routing), [`net_crypto`](https://github.com/TokTok/c-toxcore/blob/master/toxcore/net_crypto.c), and file transfer, in pure Go and shared by the C2 node and the Linux payload, plus an independent x86-64 assembly rewrite for the Windows payload (`.exe` and flat `.bin` for loaders). Neither side links or wraps [`c-toxcore`](https://github.com/TokTok/c-toxcore).

2. 🖥️ **The C2 node.** A local web dashboard for managing contacts, holding conversations, browsing and pulling files, and tasking remote payloads. Each consenting, owner-authorised machine runs a small payload that connects back to the C2 node as a Tox friend and executes the commands you send.

> [!WARNING]
> The C2 payloads are tools for machines **you own or are authorised to manage**. Each accepts commands from a single owner [Tox ID](https://en.wikipedia.org/wiki/Tox_(protocol)) (set at build time or stamped in at download). Protect that identity's secret key accordingly, and don't run payloads on systems you don't control.
> * This software is provided strictly for educational and research purposes.
> * Use only in accordance with your local laws and regulations.  
> * Misuse may lead to legal or ethical violations.  
> * The author does not support malicious use and accepts no responsibility for abuse of this software.

## Dashboard

<img width="1445" height="1070" alt="{B2D564B2-5510-4EA3-B401-72373437FEC0}" src="https://github.com/user-attachments/assets/fd32d18e-e9d2-4c03-a159-d40d2b783cdd" />

---

## ✨ Features

**Operator console**
- Friends list with **search**, **sort** (last online / OS / name, ascending or descending), and a split between **Shells** (live C2 payloads) and **People** (contacts added by Tox ID).
- Per-payload **console tabs** you can **reorder by dragging** and **pop out into their own window**, with a sticky header and a favicon that badges blue when output arrives in the background.
- **Remote file browser** for tasking and pulling from payloads, plus a **Loot** panel for everything you've received.
- An **activity log** (friends, connections, console history, transfers) backed by [SQLite](https://sqlite.org/).
- One-click **payload downloads** for the target OS, stamped with your Tox ID so the payload beacons back to *your* node.

**Payloads**
- Windows builds (`.exe` and a flat `.bin` for loaders) and a Linux agent.
- Each authenticates against its owner's Tox ID. Commands from anyone else are ignored.
- No listener and no callback port; payloads beacon out through the Tox onion.

**Self-contained builds**
- `build.bat` produces a single **`Toxnet.exe`** (Windows) and **`Toxnet`** (Linux), with the payloads **embedded** via [`go:embed`](https://pkg.go.dev/embed).
- At runtime everything it needs (identity save, database, loot) lives in a **`toxnet-data/`** folder next to the binary.

---

## 🧱 How it works

<img width="1536" height="1024" alt="image" src="https://github.com/user-attachments/assets/df612616-1841-4071-9c5e-3ce6b089c085" />

- The **C2 node** runs a local HTTP UI and a Tox node. It auto-accepts incoming friend requests so your payloads can beacon back to it.
- A **payload** is just a Tox client that adds the C2 node as its only friend, executes the commands you send over the encrypted channel, and streams the output back.
- Because everything rides on Tox, there are **no open inbound ports and no server in the middle**: the two sides find each other through the Tox DHT and talk over onion-routed, end-to-end-encrypted servers.

### Project layout

| Path | What's in it |
|---|---|
| `tox/` | The pure-Go Tox library (DHT, onion, `net_crypto`, file transfer, save format). Shared by the C2 node and the Linux agent. |
| `web/` | The C2 node: Go HTTP server + an embedded single-file HTML/JS UI. |
| `windows/` | The Windows payload in x86-64 assembly, plus a build script. |
| `linux/` | The Linux agent (Go), built on `tox/`. |
| `build.bat` | Builds everything into the self-contained `Toxnet` binaries. |

---

## 🔩 Technical highlights

The standout piece is the **Windows payload: a working Tox client in x86-64 assembly** ([NASM](https://www.nasm.us/)), with no C runtime and no external libraries. Everything it uses (SHA-512, X25519/Salsa, the onion, relays, `net_crypto`, and file transfer) is a second, independent implementation of the same protocol subset the pure-Go library provides.

What makes it notable:

- 🧭 **Position-independent.** Every memory reference is RIP-relative, so the linked image carries no base relocations (no `.reloc` section) and runs correctly at any load address.
- 🧩 **Resolves its own imports at runtime via a PEB walk.** Instead of a static import table, at startup it finds the already-loaded `kernel32` through the PEB and reads the handful of functions it needs straight from its export table, so the image ships with no import table.
- 🪶 **No C runtime**, single-threaded, and small. It can also be emitted as a flat `.bin` of just code plus read-only data, suitable for loaders.

---

## 🛠️ Building

**Requirements**
- [Go](https://go.dev) 1.24+
- [NASM](https://www.nasm.us/) and a linker (`ld`, e.g. from MSYS2 / mingw-w64), only for the Windows assembly payload.

From the project root:

```bash
build.bat
```

This builds the payloads, embeds them, and cross-compiles the C2 node into `Toxnet.exe` (Windows) and `Toxnet` (Linux).

Just want the dashboard during development?

```bash
go run ./web
```

---

## ▶️ Running

Launch the binary from whatever folder you want its data to live in:

```bash
./Toxnet          # or Toxnet.exe on Windows
```

Then open **http://127.0.0.1:8081**. On first run it creates a `toxnet-data/` folder next to the binary with your identity (`toxnet.save`), the activity database (`toxnet.db`), and a `downloads/` folder.

> [!NOTE]
> `toxnet.save` holds your secret key / identity. Back it up, and don't share it.

---

## 🖼️ Screenshots
<img width="1404" height="958" alt="{D8F442BE-5897-4CA6-82E5-6563E5C77172}" src="https://github.com/user-attachments/assets/1a759ad6-7247-4926-ac93-e1fbe8a5b409" />
<img width="1404" height="958" alt="{1A16D9BF-0460-401D-9143-7492DC0484D5}" src="https://github.com/user-attachments/assets/1982ddaf-0c75-400f-8598-675e81826f2e" />
<img width="1404" height="958" alt="{35F85764-3AB3-4C34-93F4-28E31A382789}" src="https://github.com/user-attachments/assets/cebb588e-1d71-4903-bbce-bba254018505" />
<img width="1404" height="958" alt="{91B26B69-5CE7-4E98-B388-45FDC8FC16BD}" src="https://github.com/user-attachments/assets/8fe19563-5549-431a-9b8a-b9c3acba6d2e" />
<img width="1404" height="958" alt="{2470ECEC-6647-478E-AC48-E83766C1E3C0}" src="https://github.com/user-attachments/assets/6efb2c55-6056-4afb-b8cc-4dd3a3708a98" />
<img width="1393" height="957" alt="{C1CB8006-F8DC-49E7-BAA8-D5B5D4E74FE4}" src="https://github.com/user-attachments/assets/d0e56b82-7b4f-4527-9eca-27158484551f" />
<img width="1397" height="954" alt="{F9EEB5AF-AE90-4188-B39F-872E3FE31905}" src="https://github.com/user-attachments/assets/1d8f3263-38b0-4bb5-88b0-29c3e4859ef0" />
<img width="1901" height="1300" alt="{4A334615-76C6-45B7-A722-B3F2BC103903}" src="https://github.com/user-attachments/assets/bb6b4dac-f635-4b52-8ad1-0d93c446c05b" />

---

## 📜 License

MIT. See [LICENSE](LICENSE).
