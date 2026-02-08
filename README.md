# Toxnet
<p align="center">
  <img src="https://i.imgur.com/sMbpdVJ.gif" width="650" height="auto">
</p>

![Go](https://img.shields.io/badge/Go-1.20+-00ADD8?logo=go&logoColor=white)
![C](https://img.shields.io/badge/C-ISO%20C99-A8B9CC?logo=c&logoColor=black)
![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20Windows-blue)
![License](https://img.shields.io/badge/License-MIT-green)
![Status](https://img.shields.io/badge/Status-Research%20PoC-orange)
![Encryption](https://img.shields.io/badge/E2EE-Tox%20Protocol-purple)

> **Proof-of-concept encrypted peer-to-peer command and control over Tox**

---

## ⚠ Legal & Ethical Notice

This software is provided strictly for educational and research purposes.

Use only in accordance with your local laws and regulations.  
Misuse may lead to legal or ethical violations.  
The author does not support malicious use and accepts no responsibility for abuse of this software.

---

## Overview

**Toxnet** is a proof-of-concept end-to-end encrypted (E2EE), peer-to-peer (P2P) command-and-control (C2) framework built on the Tox protocol.

It demonstrates decentralized encrypted messaging as a resilient C2 transport without centralized infrastructure.

### Features

- End-to-end encrypted communications  
- Fully peer-to-peer (no central server)  
- Relay-style C2 message routing  
- Simple, commented codebase  
- Linux & Windows payload support  

---

## Tech Stack

| Component | Language | Library |
|----------|---------|---------|
| C2 Server | Go | go-toxcore-c |
| Client | C | c-toxcore |

---

## Prerequisites

### General

* Go **1.20+**
* GCC toolchain
* Git

```bash
sudo apt install build-essential gcc git
```

---

### Windows Payload Builds (Cross-Compile)

MinGW toolchains:

```bash
sudo apt install mingw-w64
```

Provides:

* `i686-w64-mingw32-gcc` (Win32)
* `x86_64-w64-mingw32-gcc` (Win64)

---

## High-Level Flow

```text
Operator (qTox)
     ↓
Encrypted Tox Network
     ↓
C2 Relay Node
     ↓
Client Payloads
