# SPDX-FileCopyrightText: Copyright 2020-present Open Networking Foundation.
# SPDX-License-Identifier: Apache-2.0
import argparse
import collections
import logging
import time

import numpy as np
from trex.stl.api import STLClient

# Multiplier for data rates
K = 1000
M = 1000 * K
G = 1000 * M

"""
Library of useful functions for parsing and reading TRex statistics.
"""

def to_readable(src: int, unit: str = "bps") -> str:
    """
    Convert number to human readable string.
    For example: 1,000,000 bps to 1Mbps. 1,000 bytes to 1KB

    :parameters:
        src : int
            the original data
        unit : str
            the unit ('bps', 'pps', or 'bytes')
    :returns:
        A human readable string
    """
    if src < 1000:
        return "{:.1f} {}".format(src, unit)
    elif src < 1000_000:
        return "{:.1f} K{}".format(src / 1000, unit)
    elif src < 1000_000_000:
        return "{:.1f} M{}".format(src / 1000_000, unit)
    else:
        return "{:.1f} G{}".format(src / 1000_000_000, unit)


def get_readable_port_stats(port_stats: dict) -> str:
    opackets = port_stats.get("opackets", 0)
    ipackets = port_stats.get("ipackets", 0)
    obytes = port_stats.get("obytes", 0)
    ibytes = port_stats.get("ibytes", 0)
    oerrors = port_stats.get("oerrors", 0)
    ierrors = port_stats.get("ierrors", 0)
    tx_bps = port_stats.get("tx_bps", 0)
    tx_pps = port_stats.get("tx_pps", 0)
    tx_bps_L1 = port_stats.get("tx_bps_L1", 0)
    tx_util = port_stats.get("tx_util", 0)
    rx_bps = port_stats.get("rx_bps", 0)
    rx_pps = port_stats.get("rx_pps", 0)
    rx_bps_L1 = port_stats.get("rx_bps_L1", 0)
    rx_util = port_stats.get("rx_util", 0)
    return """
    Output packets: {}
    Input packets: {}
    Output bytes: {} ({})
    Input bytes: {} ({})
    Output errors: {}
    Input errors: {}
    TX bps: {} ({})
    TX pps: {} ({})
    L1 TX bps: {} ({})
    TX util: {}
    RX bps: {} ({})
    RX pps: {} ({})
    L1 RX bps: {} ({})
    RX util: {}""".format(
        opackets,
        ipackets,
        obytes,
        to_readable(obytes, "Bytes"),
        ibytes,
        to_readable(ibytes, "Bytes"),
        oerrors,
        ierrors,
        tx_bps,
        to_readable(tx_bps),
        tx_pps,
        to_readable(tx_pps, "pps"),
        tx_bps_L1,
        to_readable(tx_bps_L1),
        tx_util,
        rx_bps,
        to_readable(rx_bps),
        rx_pps,
        to_readable(rx_pps, "pps"),
        rx_bps_L1,
        to_readable(rx_bps_L1),
        rx_util,
    )


def list_port_status(port_status: dict) -> None:
    """
    List all port status

    :parameters:
    port_status: dict
        Port status from Trex client API
    """
    for port in [0, 1, 2, 3]:
        readable_stats = get_readable_port_stats(port_status[port])
        print("States from port {}: \n{}".format(port, readable_stats))


LatencyStats = collections.namedtuple(
    "LatencyStats",
    [
        "pg_id",
        "jitter",
        "average",
        "total_max",
        "total_min",
        "last_max",
        "histogram",
        "dropped",
        "out_of_order",
        "duplicate",
        "seq_too_high",
        "seq_too_low",
        "percentile_50",
        "percentile_75",
        "percentile_90",
        "percentile_99",
        "percentile_99_9",
        "percentile_99_99",
        "percentile_99_999",
    ],
)

FlowStats = collections.namedtuple(
    "FlowStats", ["pg_id", "tx_packets", "rx_packets", "tx_bytes", "rx_bytes",],
)

FlowRateShares = collections.namedtuple(
    "FlowRateShares",
    ["rx_bps", "tx_bps", "rx_bps_total", "tx_bps_total", "rx_shares", "tx_shares"],
)

PortStats = collections.namedtuple(
    "PortStats",
    [
        "tx_packets",
        "rx_packets",
        "tx_bytes",
        "rx_bytes",
        "tx_errors",
        "rx_errors",
        "tx_bps",
        "tx_pps",
        "tx_bps_L1",
        "tx_util",
        "rx_bps",
        "rx_pps",
        "rx_bps_L1",
        "rx_util",
    ],
)


RateSamples = collections.namedtuple(
    "RateSamples",
    [
        "tx_bps",
        "tx_pps",
        "rx_bps",
        "rx_pps",
    ],
)


def start_and_monitor_port_stats(
        client: STLClient, num_samples: int, tx_port: int,
        rx_port: int, min_tx_bps: int = 0,
        ramp_up_timeout: int = 3, interval: int = 1) -> RateSamples:
    """
    Starts traffic generation and collects port TX/RX rate samples,
    while verifying that TRex is able to reach the given minimum sending
    rate.

    :param client: TRex client
    :param num_samples: number of samples to collect before stopping traffic
    :param tx_port: sending port
    :param rx_port: receiving port
    :param min_tx_bps: minimum sending rate to deem the collected samples valid
    :param ramp_up_timeout: how many seconds to wait before TRex can reach min_tx_bps
    :param interval: sampling interval in seconds
    :return:
    """

    samples = RateSamples(tx_bps=[], tx_pps=[], rx_bps=[], rx_pps=[])
    ports = [tx_port, rx_port]
    duration = (num_samples * interval + ramp_up_timeout) * 2
    start_time = time.time()

    prev = {
        p: {
            "opackets": 0,
            "ipackets": 0,
            "obytes": 0,
            "ibytes": 0,
            "time": start_time,
        }
        for p in ports
    }

    client.start(ports=[tx_port], duration=duration)

    time.sleep(interval)
    while client.is_traffic_active():
        stats = client.get_stats(ports=ports)

        sample_time = time.time()
        elapsed = sample_time - start_time

        # Compute samples per-port
        tx_bps, tx_pps, rx_bps, rx_pps = 0, 0, 0, 0
        for port in ports:
            opackets = stats[port]["opackets"]
            ipackets = stats[port]["ipackets"]
            obytes = stats[port]["obytes"]
            ibytes = stats[port]["ibytes"]
            time_diff = sample_time - prev[port]["time"]

            if port == tx_port:
                tx_bps = 8 * (obytes - prev[port]["obytes"]) / time_diff
                tx_pps = opackets - prev[port]["opackets"] / time_diff
                samples.tx_bps.append(tx_bps)
                samples.tx_pps.append(tx_pps)
            else:
                rx_bps = 8 * (ibytes - prev[port]["ibytes"]) / time_diff
                rx_pps = ipackets - prev[port]["ipackets"] / time_diff
                samples.rx_bps.append(rx_bps)
                samples.rx_pps.append(rx_pps)

            prev[port]["opackets"] = opackets
            prev[port]["ipackets"] = ipackets
            prev[port]["obytes"] = obytes
            prev[port]["ibytes"] = ibytes
            prev[port]["time"] = sample_time

        bad_sample = tx_bps < min_tx_bps
        # Loss is approximated due to non-atomic reads of TX and RX ports
        loss_rate = (tx_pps - rx_pps) / tx_pps
        print(f" {elapsed:.2f}s: "
              f"TX {to_readable(tx_bps)} ({to_readable(tx_pps, 'pps')}) "
              f"--> RX {to_readable(rx_bps)} "
              f"(~{abs(loss_rate):.1%} pkt loss)"
              f"{' *' if bad_sample else ''}")

        if bad_sample:
            if elapsed > ramp_up_timeout:
                client.stop(ports=[tx_port])
                raise Exception(
                    f"TX port ({tx_port}) did not reach or sustain "
                    f"min sending rate ({to_readable(min_tx_bps)})")
            else:
                # Discard last sample
                samples.tx_bps.pop()
                samples.tx_pps.pop()
                samples.rx_bps.pop()
                samples.rx_pps.pop()

        if len(samples.tx_bps) == num_samples:
            # We have enough samples.
            client.stop(ports=[tx_port])
            client.wait_on_traffic(ports=[tx_port], timeout=2)
            break

        time.sleep(interval - elapsed % interval)

    return samples


def get_port_stats(port: int, stats) -> PortStats:
    port_stats = stats.get(port)
    return PortStats(
        tx_packets=port_stats.get("opackets", 0),
        rx_packets=port_stats.get("ipackets", 0),
        tx_bytes=port_stats.get("obytes", 0),
        rx_bytes=port_stats.get("ibytes", 0),
        tx_errors=port_stats.get("oerrors", 0),
        rx_errors=port_stats.get("ierrors", 0),
        tx_bps=port_stats.get("tx_bps", 0),
        tx_pps=port_stats.get("tx_pps", 0),
        tx_bps_L1=port_stats.get("tx_bps_L1", 0),
        tx_util=port_stats.get("tx_util", 0),
        rx_bps=port_stats.get("rx_bps", 0),
        rx_pps=port_stats.get("rx_pps", 0),
        rx_bps_L1=port_stats.get("rx_bps_L1", 0),
        rx_util=port_stats.get("rx_util", 0),
    )


def get_latency_stats(pg_id: int, stats) -> LatencyStats:
    lat_stats = stats["latency"].get(pg_id)
    lat = lat_stats["latency"]
    # Estimate latency percentiles from the histogram.
    l = list(lat["histogram"].keys())
    l.sort()
    all_latencies = []
    for sample in l:
        range_start = sample
        if range_start == 0:
            range_end = 10
        else:
            range_end = range_start + pow(10, (len(str(range_start)) - 1))
        val = lat["histogram"][sample]
        # Assume whole the bucket experienced the range_end latency.
        all_latencies += [range_end] * val
    q = [50, 75, 90, 99, 99.9, 99.99, 99.999]
    percentiles = np.percentile(all_latencies, q)

    ret = LatencyStats(
        pg_id=pg_id,
        jitter=lat["jitter"],
        average=lat["average"],
        total_max=lat["total_max"],
        total_min=lat["total_min"],
        last_max=lat["last_max"],
        histogram=lat["histogram"],
        dropped=lat_stats["err_cntrs"]["dropped"],
        out_of_order=lat_stats["err_cntrs"]["out_of_order"],
        duplicate=lat_stats["err_cntrs"]["dup"],
        seq_too_high=lat_stats["err_cntrs"]["seq_too_high"],
        seq_too_low=lat_stats["err_cntrs"]["seq_too_low"],
        percentile_50=percentiles[0],
        percentile_75=percentiles[1],
        percentile_90=percentiles[2],
        percentile_99=percentiles[3],
        percentile_99_9=percentiles[4],
        percentile_99_99=percentiles[5],
        percentile_99_999=percentiles[6],
    )
    return ret


def get_readable_latency_stats(stats: LatencyStats) -> str:
    histogram = ""
    # need to listify in order to be able to sort them.
    l = list(stats.histogram.keys())
    l.sort()
    for sample in l:
        range_start = sample
        if range_start == 0:
            range_end = 10
        else:
            range_end = range_start + pow(10, (len(str(range_start)) - 1))
        val = stats.histogram[sample]
        histogram = (
            histogram
            + "\n        Packets with latency between {0:>5} us and {1:>5} us: {2:>10}".format(
                range_start, range_end, val
            )
        )

    return f"""
    Latency info for pg_id {stats.pg_id}
    Dropped packets: {stats.dropped}
    Out-of-order packets: {stats.out_of_order}
    Sequence too high packets: {stats.seq_too_high}
    Sequence too low packets: {stats.seq_too_low}
    Maximum latency: {stats.total_max} us
    Minimum latency: {stats.total_min} us
    Maximum latency in last sampling period: {stats.last_max} us
    Average latency: {stats.average} us
    50th percentile latency: {stats.percentile_50} us
    75th percentile latency: {stats.percentile_75} us
    90th percentile latency: {stats.percentile_90} us
    99th percentile latency: {stats.percentile_99} us
    99.9th percentile latency: {stats.percentile_99_9} us
    99.99th percentile latency: {stats.percentile_99_99} us
    99.999th percentile latency: {stats.percentile_99_999} us
    Jitter: {stats.jitter} us
    Latency distribution histogram: {histogram}
    """


def get_flow_stats(pg_id: int, stats) -> FlowStats:
    flow_stats = stats["flow_stats"].get(pg_id)
    ret = FlowStats(
        pg_id=pg_id,
        tx_packets=flow_stats["tx_pkts"]["total"],
        rx_packets=flow_stats["rx_pkts"]["total"],
        tx_bytes=flow_stats["tx_bytes"]["total"],
        rx_bytes=flow_stats["rx_bytes"]["total"],
    )
    return ret


def get_readable_flow_stats(stats: FlowStats) -> str:
    return f"""Flow info for pg_id {stats.pg_id}
    TX packets: {stats.tx_packets}
    RX packets: {stats.rx_packets}
    TX bytes: {stats.tx_bytes}
    RX bytes: {stats.rx_bytes}"""


def get_flow_rate_shares(seconds: int, *stats_list: FlowStats) -> FlowRateShares:
    rx_bps = {}
    tx_bps = {}
    for stats in stats_list:
        rx_bps[stats.pg_id] = stats.rx_bytes * 8 / seconds
        tx_bps[stats.pg_id] = stats.tx_bytes * 8 / seconds
    rx_bps_total = sum(rx_bps.values())
    tx_bps_total = sum(tx_bps.values())
    rx_shares = {k: v / rx_bps_total for k, v in rx_bps.items()}
    tx_shares = {k: v / tx_bps_total for k, v in tx_bps.items()}
    return FlowRateShares(
        rx_bps=rx_bps,
        tx_bps=tx_bps,
        rx_bps_total=rx_bps_total,
        tx_bps_total=tx_bps_total,
        rx_shares=rx_shares,
        tx_shares=tx_shares,
    )


def get_readable_flow_rate_shares(stats: FlowRateShares) -> str:
    rx_str = "\n".join(
        [
            f"        pg_id {pg_id}: {to_readable(val)} ({stats.rx_shares[pg_id]:.1%})"
            for pg_id, val in stats.rx_bps.items()
        ]
    )
    tx_str = "\n".join(
        [
            f"        pg_id {pg_id}: {to_readable(val)} ({stats.tx_shares[pg_id]:.1%})"
            for pg_id, val in stats.tx_bps.items()
        ]
    )
    return f"""Flow rate shares:
    TX total: {to_readable(stats.tx_bps_total)}\n{tx_str}
    RX total: {to_readable(stats.rx_bps_total)}\n{rx_str}"""
