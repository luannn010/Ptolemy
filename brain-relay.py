#!/usr/bin/env python3
"""TCP relay so the Windows voice catcher can reach the WSL llama-server.

The llama-server runs inside WSL bound to 127.0.0.1:1089 (loopback only), and
Windows svchost squats Windows port 1089, so WSL2's localhost-forwarding cannot
expose 1089 to Windows. This relay listens on a *free* loopback port in WSL
(default 18089) and forwards to the llama-server; WSL2 then forwards Windows
localhost:18089 -> this relay -> llama-server:1089 (the same pattern that already
works for ollama on 11434).

Usage:
    python3 brain-relay.py [listen_port] [target_port]
Defaults: listen 18089 -> target 1089 (both on 127.0.0.1).
"""
import socket
import sys
import threading

LISTEN_HOST = "127.0.0.1"
TARGET_HOST = "127.0.0.1"


def pipe(src: socket.socket, dst: socket.socket) -> None:
    try:
        while True:
            data = src.recv(65536)
            if not data:
                break
            dst.sendall(data)
    except OSError:
        pass
    finally:
        for s in (src, dst):
            try:
                s.shutdown(socket.SHUT_RDWR)
            except OSError:
                pass


def handle(client: socket.socket, target_port: int) -> None:
    try:
        upstream = socket.create_connection((TARGET_HOST, target_port))
    except OSError:
        client.close()
        return
    threading.Thread(target=pipe, args=(client, upstream), daemon=True).start()
    threading.Thread(target=pipe, args=(upstream, client), daemon=True).start()


def main() -> None:
    listen_port = int(sys.argv[1]) if len(sys.argv) > 1 else 18089
    target_port = int(sys.argv[2]) if len(sys.argv) > 2 else 1089

    srv = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    srv.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    srv.bind((LISTEN_HOST, listen_port))
    srv.listen(128)
    print(
        f"brain-relay: {LISTEN_HOST}:{listen_port} -> {TARGET_HOST}:{target_port}",
        flush=True,
    )
    while True:
        client, _ = srv.accept()
        handle(client, target_port)


if __name__ == "__main__":
    main()
