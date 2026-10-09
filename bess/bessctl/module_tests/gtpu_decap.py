# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Forsway Scandinavia AB

# GtpuParser and GtpuDecap on GTP-U packets whose headers do not fit in them,
# on GTP-U messages that are not IPv4 G-PDUs, and on the traffic they must
# keep handling exactly as before.

import os
import socket
import struct

from test_utils import *

N3 = "10.200.0.3"
GNB = "10.200.0.201"
UE = "10.250.0.1"
DN = "10.0.0.9"
TEID = 0x01020304

# GTPU_TEST_BASELINE=1 leaves out the zero-length extension header, for a run
# against code without this fix: there that case spins the worker for ever, so
# the run would hang instead of failing. gtp_test.cc covers the case once the
# bounded header length is in.
BASELINE = os.environ.get("GTPU_TEST_BASELINE") == "1"

ETH = scapy.Ether(src="02:00:00:00:00:01", dst="02:00:00:00:00:02")


def gtpu(flags, mtype, body, teid=TEID):
    """A GTP-U header and what follows it. The length field counts everything
    after the fixed header, as on the wire, even when that is cut short."""
    return bytes([flags, mtype]) + struct.pack("!HI", len(body), teid) + body


def n3(gtp_bytes):
    return (
        ETH
        / scapy.IP(src=GNB, dst=N3)
        / scapy.UDP(sport=2152, dport=2152)
        / scapy.Raw(gtp_bytes)
    )


OPTS_PSC = b"\x00\x00\x00\x85"  # seq, N-PDU, next type: PDU Session Container
PSC_END = b"\x01\x10\x09\x00"  # one unit, UL, QFI 9, next type: none

INNER_TCP = bytes(scapy.IP(src=UE, dst=DN) / scapy.TCP(sport=40000, dport=80) / (b"x" * 20))
INNER_UDP = bytes(scapy.IP(src=UE, dst=DN) / scapy.UDP(sport=40001, dport=53) / (b"y" * 12))
INNER_ICMP_MIN = bytes(scapy.IP(src=UE, dst=DN, proto=1))  # exactly 20 bytes
# The last fragment of a UDP datagram, with one byte of payload and no ports.
INNER_UDP_LAST_FRAGMENT = bytes(scapy.IP(src=UE, dst=DN, proto=17, frag=185) / b"z")

GOOD = {
    "psc_tcp": n3(gtpu(0x34, 0xFF, OPTS_PSC + PSC_END + INNER_TCP)),
    "plain_udp": n3(gtpu(0x30, 0xFF, INNER_UDP)),
    "min_icmp": n3(gtpu(0x30, 0xFF, INNER_ICMP_MIN)),
    "ports_exact": n3(gtpu(0x30, 0xFF, INNER_TCP[:24])),
    "last_fragment": n3(gtpu(0x30, 0xFF, INNER_UDP_LAST_FRAGMENT)),
    # TS 29.281 5.1: the receiver shall not evaluate the spare bit.
    "spare_bit_set": n3(gtpu(0x38, 0xFF, INNER_UDP)),
}


def eth_ipv4(ip_bytes):
    """An Ethernet frame carrying `ip_bytes` as IPv4, whatever they hold."""
    return bytes(ETH)[:12] + b"\x08\x00" + ip_bytes


def ipv4_header(ihl=5, proto=17, total=40):
    return struct.pack("!BBHHHBBH4s4s", 0x40 | ihl, 0, total, 1, 0, 64, proto, 0,
                       socket.inet_aton(GNB), socket.inet_aton(N3))


# Frames whose outer IPv4 or UDP header does not fit: the parser cannot tell
# whether they are GTP-U, and the decapsulator cannot strip them.
OUTER_MALFORMED = {
    "outer_ipv4_cut": eth_ipv4(ipv4_header()[:10]),
    "outer_ihl_below_5": eth_ipv4(ipv4_header(ihl=4) + b"\x08\x68\x08\x68" + b"\x00" * 12),
    "outer_ihl_past_end": eth_ipv4(ipv4_header(ihl=15) + b"\x00" * 8),
    "outer_udp_cut": eth_ipv4(ipv4_header() + b"\x08\x68\x08\x68"),
}
# A frame that is not GTP-U and does not hold the ports the parser reads.
TCP_WITHOUT_PORTS = eth_ipv4(ipv4_header(proto=6) + b"\x00\x50")

# An ARP reply. Read as IPv4 it has IHL 0, and its sender MAC, read as GTP-U
# flags, sets S and E; its non-zero target MAC then drives an unbounded
# extension walk past the frame. That is how the decapsulator was reached.
ARP_REPLY = bytes(
    scapy.Ether(src="0e:22:33:44:55:66", dst="06:11:22:33:44:55")
    / scapy.ARP(op=2, hwsrc="0e:22:33:44:55:66", psrc=N3, hwdst="06:11:22:33:44:55", pdst=GNB)
)

# Packets whose outer or GTP-U headers do not fit: neither module can strip or
# classify them.
UNSTRIPPABLE = {
    "gtp_fixed_cut": n3(bytes([0x30, 0xFF, 0x00, 0x00, 0x01])),
    "gtp_options_cut": n3(gtpu(0x34, 0xFF, b"\x00\x00")),
    "chain_past_end": n3(gtpu(0x34, 0xFF, OPTS_PSC + b"\x40\x00\x00\x00")),
    "next_type_at_end": n3(gtpu(0x34, 0xFF, OPTS_PSC)),
}
if not BASELINE:
    UNSTRIPPABLE["zero_length_ext"] = n3(
        gtpu(0x34, 0xFF, OPTS_PSC + b"\x00\x00\x00\x85" + INNER_TCP)
    )

# G-PDUs whose outer and GTP-U headers fit but whose inner packet does not.
# Decapsulation does not read the inner packet; classifying it does.
INNER_MALFORMED = {
    "inner_short": n3(gtpu(0x30, 0xFF, INNER_TCP[:10])),
    "inner_ihl_past": n3(gtpu(0x30, 0xFF, b"\x4f" + INNER_TCP[1:24])),
    "inner_ports_past": n3(gtpu(0x30, 0xFF, INNER_TCP[:22])),
    "empty_gpdu": n3(gtpu(0x30, 0xFF, b"")),
}

UNSTRIPPABLE.update(OUTER_MALFORMED)

# Not GTPv1-U: version 2, and PT 0 (GTP', TS 32.295), each with an IPv4 body.
NOT_GTPV1U = {
    "version_2": n3(gtpu(0x50, 0xFF, INNER_TCP)),
    "pt_0_gtp_prime": n3(gtpu(0x20, 0xFF, INNER_TCP)),
}

MALFORMED = {**UNSTRIPPABLE, **INNER_MALFORMED, **NOT_GTPV1U, "tcp_without_ports": TCP_WITHOUT_PORTS}

# Legitimate on N3, but not an IPv4 G-PDU: never classified as user traffic.
NOT_IPV4_GPDU = {
    # Error Indication: S set, TEID 0; TEID Data I and GTP-U Peer Address IEs.
    "error_indication": n3(
        gtpu(0x32, 26, b"\x00\x00\x00\x00" + b"\x10" + struct.pack("!I", TEID)
             + b"\x85\x00\x04" + bytes([192, 168, 252, 201]), teid=0)
    ),
    "end_marker": n3(gtpu(0x30, 254, b"")),
    # Not a G-PDU, though its body would parse as the UE's IPv4 packet: only
    # the message type keeps it from being classified as user traffic.
    "end_marker_with_ipv4_body": n3(gtpu(0x30, 254, INNER_TCP)),
    "ipv6_gpdu": n3(
        gtpu(0x30, 0xFF, bytes(scapy.IPv6(src="fd00::1", dst="fd00::2") / scapy.UDP()))
    ),
}

# Not GTP-U at all: handled exactly as before.
OTHER = {
    "arp": ETH / scapy.ARP(psrc=GNB, pdst=N3),
    "udp_not_gtpu": ETH / scapy.IP(src=GNB, dst=N3) / scapy.UDP(sport=5000, dport=53) / (b"z" * 20),
    # A later fragment on N6: no ports to read, classified on its addresses.
    "udp_later_fragment": ETH / scapy.IP(src=DN, dst=UE, proto=17, frag=100) / b"zz",
}

# The attributes GtpuParser writes, in the order the encap below prepends them.
ATTRS = [
    ("src_ip", 4),
    ("dst_ip", 4),
    ("src_port", 2),
    ("dst_port", 2),
    ("teid", 4),
    ("tunnel_ipv4_dst", 4),
    ("ip_proto", 1),
]


def ip4(addr):
    return bytes(int(o) for o in addr.split("."))


class GtpuDecapTest(BessModuleTestCase):
    def deadends(self, module):
        return self.bess.get_module_info(module.name).deadends

    def test_parser_forwards_well_formed_gpdus(self):
        parser = GtpuParser()
        out = self.run_module(parser, 0, list(GOOD.values()), [0, 1])
        self.assertEqual(len(out[0]), 0, "a well-formed G-PDU was dropped")
        self.assertEqual(len(out[1]), len(GOOD))
        self.assertBessAlive()

    def test_parser_attributes_are_unchanged(self):
        parser = GtpuParser()
        encap = GenericEncap(fields=[{"size": s, "attribute": a} for a, s in ATTRS])
        self.bess.connect_modules(parser.name, encap.name, 1, 0)

        out = self.run_pipeline(parser, encap, 0, [GOOD["psc_tcp"]], [0], proto=bytes)
        self.assertEqual(len(out[0]), 1)
        prefix = out[0][0][:21]
        self.assertEqual(
            prefix,
            ip4(UE) + ip4(DN) + struct.pack("!HH", 40000, 80)
            + struct.pack("!I", TEID) + ip4(N3) + b"\x06",
        )

    def test_parser_writes_its_own_attributes_for_every_forwarded_frame(self):
        # A frame the parser forwards must not be classified on values left in the buffer by an
        # earlier packet: ARP gets values that match no rule, and an IPv4 protocol without special
        # handling gets its own addresses. Each probe follows a G-PDU of a session.
        all_ones = b"\xff" * 4
        probes = {
            "arp_request": (
                bytes(ETH / scapy.ARP(op=1, psrc=GNB, pdst=N3)),
                all_ones * 2 + b"\xff\xff" * 2 + all_ones * 2 + b"\xff",
            ),
            "arp_reply": (
                ARP_REPLY,
                all_ones * 2 + b"\xff\xff" * 2 + all_ones * 2 + b"\xff",
            ),
            "ipv4_gre": (
                bytes(ETH / scapy.IP(src=DN, dst=UE, proto=47) / (b"g" * 24)),
                ip4(DN) + ip4(UE) + b"\xff\xff" * 2 + all_ones * 2 + b"\x2f",
            ),
        }
        for name, (probe, want) in probes.items():
            with self.subTest(case=name):
                parser = GtpuParser()
                encap = GenericEncap(fields=[{"size": s, "attribute": a} for a, s in ATTRS])
                self.bess.connect_modules(parser.name, encap.name, 1, 0)
                out = self.run_pipeline(parser, encap, 0, [GOOD["psc_tcp"], probe], [0], proto=bytes)
                self.assertEqual(len(out[0]), 2, f"{name} did not leave the parser by gate 1")
                self.assertEqual(out[0][1][:21], want, f"{name} carried attributes it did not write")

    def test_parser_drops_malformed(self):
        for name, pkt in MALFORMED.items():
            with self.subTest(case=name):
                parser = GtpuParser()
                out = self.run_module(parser, 0, [pkt], [0, 1])
                self.assertEqual(len(out[1]), 0, f"{name} was classified")
                self.assertEqual(len(out[0]), 1, f"{name} did not leave by gate 0")
                self.assertBessAlive()

    def test_parser_drops_messages_that_are_not_ipv4_gpdus(self):
        for name, pkt in NOT_IPV4_GPDU.items():
            with self.subTest(case=name):
                parser = GtpuParser()
                out = self.run_module(parser, 0, [pkt], [0, 1])
                self.assertEqual(len(out[1]), 0, f"{name} was classified")
                self.assertEqual(len(out[0]), 1)

    def test_parser_keeps_other_frames_on_their_path(self):
        for name, pkt in OTHER.items():
            with self.subTest(case=name):
                parser = GtpuParser()
                out = self.run_module(parser, 0, [pkt], [0, 1])
                self.assertEqual(len(out[1]), 1, f"{name} left the path it takes today")

    def test_decap_strips_well_formed(self):
        decap = GtpuDecap()
        inners = {
            "psc_tcp": INNER_TCP,
            "plain_udp": INNER_UDP,
            "min_icmp": INNER_ICMP_MIN,
            "last_fragment": INNER_UDP_LAST_FRAGMENT,
            "spare_bit_set": INNER_UDP,
        }
        for name, inner in inners.items():
            with self.subTest(case=name):
                out = self.run_module(decap, 0, [GOOD[name]], [0], proto=bytes)
                self.assertEqual(len(out[0]), 1)
                self.assertEqual(out[0][0], bytes(GOOD[name])[:14] + inner)

    def test_decap_drops_malformed_and_survives(self):
        decap = GtpuDecap()
        before = self.deadends(decap)
        out = self.run_module(decap, 0, list(UNSTRIPPABLE.values()), [0])
        self.assertBessAlive()
        self.assertEqual(len(out[0]), 0, "a packet that does not hold its headers was decapsulated")
        self.assertEqual(self.deadends(decap) - before, len(UNSTRIPPABLE))

    def test_decap_drops_an_arp_reply(self):
        decap = GtpuDecap()
        before = self.deadends(decap)
        out = self.run_module(decap, 0, [ARP_REPLY], [0])
        self.assertBessAlive()
        self.assertEqual(len(out[0]), 0, "an ARP reply was decapsulated")
        self.assertEqual(self.deadends(decap) - before, 1)

    def test_decap_counts_a_flood(self):
        decap = GtpuDecap()
        before = self.deadends(decap)
        flood = [MALFORMED["chain_past_end"]] * 10000
        self.run_module(decap, 0, flood, [0], time_out=20)
        self.assertBessAlive()
        self.assertEqual(self.deadends(decap) - before, len(flood))


suite = unittest.TestLoader().loadTestsFromTestCase(GtpuDecapTest)
results = unittest.TextTestRunner(verbosity=2).run(suite)

if results.failures or results.errors:
    sys.exit(1)
