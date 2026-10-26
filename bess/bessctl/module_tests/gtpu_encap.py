# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Intel Corporation

# GtpuEncap: the outer IPv4 header it prepends, and its ID in particular.

import socket
import struct

from test_utils import *

N3 = "10.200.0.3"
GNB = "10.200.0.201"
UE = "10.250.0.1"
DN = "10.0.0.9"
TEID = 0x01020304
QFI = 9

# BESS default batch size
BATCH_SIZE = 64

ETH = scapy.Ether(src="02:00:00:00:00:01", dst="02:00:00:00:00:02")
ETH_LEN = len(bytes(ETH))
IP_ID = slice(ETH_LEN + 4, ETH_LEN + 6)

# A downlink packet as GtpuEncap gets it: the UE's IPv4 packet in an Ethernet
# frame, with the tunnel it goes into carried as metadata.
INNER = bytes(scapy.IP(src=DN, dst=UE) / scapy.UDP(sport=53, dport=40001) / (b"y" * 12))
DOWNLINK = bytes(ETH) + INNER


def attr(name, size, wire):
    """The attribute GtpuEncap writes `wire` from. It reads the addresses, the
    TEID and the port as native-order words and writes them big-endian, so the
    attribute holds the bytes it puts on the wire reversed."""
    return {"name": name, "size": size, "value_int": int.from_bytes(wire, "little")}


# What the pipeline writes for GtpuEncap: PDU type 0 (downlink), and the
# tunnel the packet goes into.
TUNNEL = [
    {"name": "action", "size": 1, "value_int": 0},
    {"name": "qfi", "size": 1, "value_int": QFI},
    attr("tunnel_out_src_ip4addr", 4, socket.inet_aton(N3)),
    attr("tunnel_out_dst_ip4addr", 4, socket.inet_aton(GNB)),
    attr("tunnel_out_teid", 4, struct.pack("!I", TEID)),
    attr("tunnel_out_udp_port", 2, struct.pack("!H", 2152)),
]


class GtpuEncapTest(BessModuleTestCase):
    def encap(self, pkts, add_psc=True, batched=False):
        """The frames GtpuEncap forwards for `pkts`, as bytes. With `batched`, a
        Buffer in front withholds them until it has a full batch, so the module
        gets them all in one ProcessBatch call."""
        tunnel = SetMetadata(attrs=TUNNEL)
        encap = GtpuEncap(add_psc=add_psc)
        self.bess.connect_modules(tunnel.name, encap.name, 0, 0)

        first = tunnel
        if batched:
            first = Buffer()
            self.bess.connect_modules(first.name, tunnel.name, 0, 0)

        out = self.run_pipeline(first, encap, 0, pkts, [1], proto=bytes)
        self.assertEqual(len(out[1]), len(pkts), "a packet was not encapsulated")
        return out[1]

    def ids(self, frames):
        return [struct.unpack("!H", frame[IP_ID])[0] for frame in frames]

    def test_each_packet_gets_its_own_outer_ip_id(self):
        # DF is clear, so the ID must be unique per datagram (RFC 6864 2.1):
        # fragments of two datagrams sharing an ID reassemble into one packet.
        for add_psc in (True, False):
            with self.subTest(add_psc=add_psc):
                ids = self.ids(self.encap([DOWNLINK] * 8, add_psc=add_psc))
                self.assertEqual(len(set(ids)), len(ids), f"outer IPv4 IDs repeat: {ids}")

    def test_a_full_batch_gets_an_id_for_every_packet(self):
        # A packet at a time says nothing about a batch: an ID taken once per
        # ProcessBatch call would pass that and repeat here.
        ids = self.ids(self.encap([DOWNLINK] * BATCH_SIZE, batched=True))
        self.assertEqual(len(set(ids)), len(ids), f"outer IPv4 IDs repeat: {ids}")

    def test_nothing_else_differs_between_identical_packets(self):
        out = self.encap([DOWNLINK] * 4)
        without_id = {pkt[: IP_ID.start] + pkt[IP_ID.stop :] for pkt in out}
        self.assertEqual(len(without_id), 1, "the frames differ beyond the outer IPv4 ID")

    def test_the_outer_headers_are_otherwise_unchanged(self):
        # add_psc also picks the size of the header prepended, so each arm
        # pins the GTP-U flags and the bytes between the header and the
        # packet carried: an 8-byte PDU Session Container, or nothing.
        for add_psc, flags, ext_len in ((True, 0x34, 8), (False, 0x30, 0)):
            with self.subTest(add_psc=add_psc):
                frame = self.encap([DOWNLINK], add_psc=add_psc)[0]
                self.assertEqual(frame[:ETH_LEN], DOWNLINK[:ETH_LEN])

                outer = scapy.IP(frame[ETH_LEN:])
                self.assertEqual((outer.src, outer.dst), (N3, GNB))
                self.assertEqual(outer.ttl, 64)
                self.assertEqual(outer.flags, 0, "DF is set: the ID may repeat")
                self.assertEqual(outer.len, len(frame) - ETH_LEN)
                udp = outer[scapy.UDP]
                self.assertEqual((udp.sport, udp.dport), (2152, 2152))

                gtp = frame[ETH_LEN + 20 + 8 :]
                ghdr, mtype, length, teid = struct.unpack("!BBHI", gtp[:8])
                self.assertEqual(ghdr, flags, "not a GTPv1-U header")
                self.assertEqual(mtype, 0xFF, "not a G-PDU")
                self.assertEqual(teid, TEID)
                self.assertEqual(length, ext_len + len(INNER))
                self.assertEqual(gtp[-len(INNER) :], INNER, "the inner packet changed")
                if add_psc:
                    # seq, N-PDU, next type PSC; then one unit, DL, QFI, no next
                    self.assertEqual(gtp[8:16], b"\x00\x00\x00\x85\x01\x00\x09\x00")


suite = unittest.TestLoader().loadTestsFromTestCase(GtpuEncapTest)
results = unittest.TextTestRunner(verbosity=2).run(suite)

if results.failures or results.errors:
    sys.exit(1)
