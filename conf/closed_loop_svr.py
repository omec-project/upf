#!/usr/bin/env python3
# SPDX-FileCopyrightText: 2025 Intel Corporation
# SPDX-License-Identifier: Apache-2.0

import logging
import os
import socket
import sys
import threading

import scapy.all as scapy
from flask import Flask, jsonify

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s - %(levelname)s - %(message)s",
    handlers=[logging.StreamHandler(sys.stdout), logging.StreamHandler(sys.stderr)],
)
logger = logging.getLogger(__name__)

app = Flask(__name__)

rogueIPs = []


@app.route("/", methods=["GET"])
def get_rogueIps():
    logger.info("received request for / endpoint")
    response = jsonify({"ipaddresses": rogueIPs})
    rogueIPs.clear()
    return response


def unix_socket_client(socket_path):
    client = socket.socket(socket.AF_UNIX, socket.SOCK_SEQPACKET)
    try:
        client.connect(socket_path)
        logger.info(f"connected to Unix socket server at {socket_path}")

        while True:
            data = client.recv(2048)
            if data:
                pkt = scapy.Ether(data)
                if scapy.IP in pkt:
                    dst_ip = pkt[scapy.IP].dst
                    if dst_ip not in rogueIPs:
                        rogueIPs.append(dst_ip)
                        logger.info(f"added new rogue IP: {dst_ip}")
            else:
                break
    except Exception:
        logger.exception("error connecting to Unix socket server")
    finally:
        client.close()


if __name__ == "__main__":
    closed_loop_path = os.getenv("CLOSED_LOOP_SOCKET_PATH", "/tmp/closedloop")
    client_thread = threading.Thread(
        target=unix_socket_client, args=(closed_loop_path,), daemon=True
    )
    client_thread.start()

    port = int(os.getenv("CLOSED_LOOP_PORT", "9301"))
    logger.info(f"starting server on port {port}")
    app.run(host="0.0.0.0", port=port)
