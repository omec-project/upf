/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright 2022 Intel Corporation
 */

#include "gtpu_path_monitoring.h"

#include "utils/endian.h" /* for be16_t */
#include "utils/endian.h" /* for be32_t */
#include "utils/ether.h"  /* for ethernet header */
#include "utils/gtp.h"    /* for gtp header */
#include "utils/ip.h"     /* for ToIpv4Address() */
#include "utils/udp.h"    /* for udp header */

#include <unordered_map>

using bess::utils::be16_t;
using bess::utils::be32_t;
using bess::utils::Ethernet;
using bess::utils::Gtpv1;
using bess::utils::Gtpv1SeqPDUExt;
using bess::utils::Ipv4;
using bess::utils::ToIpv4Address;
using bess::utils::Udp;

const Commands GtpuPathMonitoring::cmds = {
    {"add", "GtpuPathMonitoringCommandAddDeleteArg",
     MODULE_CMD_FUNC(&GtpuPathMonitoring::CommandAdd), Command::THREAD_SAFE},
    {"delete", "GtpuPathMonitoringCommandAddDeleteArg",
     MODULE_CMD_FUNC(&GtpuPathMonitoring::CommandDelete), Command::THREAD_SAFE},
    {"clear", "GtpuPathMonitoringCommandClearArg",
     MODULE_CMD_FUNC(&GtpuPathMonitoring::CommandClear), Command::THREAD_SAFE},
    {"read", "GtpuPathMonitoringCommandReadArg",
     MODULE_CMD_FUNC(&GtpuPathMonitoring::CommandReadStats),
     Command::THREAD_SAFE},
};

void GtpuPathMonitoring::ProcessBatch(Context *ctx, bess::PacketBatch *batch) {
  int cnt = batch->cnt();

  for (int i = 0; i < cnt; i++) {
    // we should drop-or-emit each packet
    bess::Packet *pkt = batch->pkts()[i];

    // If gNB IP vector is empty, drop packet. It is like disabling this feature
    if (m_dstIp.empty()) {
      DLOG(INFO) << "gNBs IP vector is empty, dropping packet";
      DropPacket(ctx, pkt);
      continue;
    }

    Ethernet *eth = pkt->head_data<Ethernet *>();
    Ipv4 *iph = (Ipv4 *)((unsigned char *)eth + sizeof(Ethernet));
    Udp *udp = (Udp *)((unsigned char *)iph + (iph->header_length << 2));
    Gtpv1 *gtph = (Gtpv1 *)((unsigned char *)udp + sizeof(Udp));
    Gtpv1SeqPDUExt *speh =
        (Gtpv1SeqPDUExt *)((unsigned char *)gtph + sizeof(Gtpv1));

    uint8_t gtpuType = gtph->type;

    if (gtpuType == GTPU_ECHO_REQUEST) {
      speh->seqnum = static_cast<be16_t>(m_seqNumber);

      for (auto it : m_dstIp) {
        m_storedData[it.first][m_seqNumber] = tsc_to_ns(rdtsc());
        bess::Packet *newPkt = bess::Packet::copy(pkt);
        Ethernet *newEth = newPkt->head_data<Ethernet *>();
        Ipv4 *newIph = (Ipv4 *)((unsigned char *)newEth + sizeof(Ethernet));
        newIph->dst = static_cast<be32_t>(it.first);
        EmitPacket(ctx, newPkt);
      }

      m_seqNumber++;

    } else if (gtpuType == GTPU_ECHO_RESPONSE) {
      uint32_t srcIp = iph->src.value();
      uint16_t seqNumber = speh->seqnum.value();
      auto it = m_storedData.find(srcIp);
      if (it != m_storedData.end()) {
        auto itInn = it->second.find(seqNumber);
        if (itInn != it->second.end()) {
          Values values;
          uint64_t lat = (tsc_to_ns(rdtsc()) - itInn->second) / 2;
          auto itDst = m_latency.find(srcIp);
          if (itDst != m_latency.end()) {
            Values valuesIn = itDst->second;
            values.m_count = valuesIn.m_count + 1;
            values.m_min = (lat < valuesIn.m_min) ? lat : valuesIn.m_min;
            if (lat > valuesIn.m_mean) {
              values.m_mean =
                  valuesIn.m_mean + (lat - valuesIn.m_mean) / values.m_count;
            } else {
              values.m_mean =
                  valuesIn.m_mean - (valuesIn.m_mean - lat) / values.m_count;
            }
            values.m_max = (lat > valuesIn.m_max) ? lat : valuesIn.m_max;
          } else {
            values.m_count = 1;
            values.m_min = lat;
            values.m_mean = lat;
            values.m_max = lat;
          }

          m_latency[srcIp] = values;
          it->second.erase(itInn);

        } else {
          LOG(ERROR) << "Sequence number " << seqNumber << " not found in map";
        }
      } else {
        LOG(ERROR) << "gNB IP address not found in map";
      }

      LOG(INFO) << "type:" << +gtpuType
                << ", srcIp:" << ToIpv4Address(static_cast<be32_t>(srcIp))
                << ", seqNumber:" << seqNumber << ", latency:["
                << m_latency[srcIp].m_min << ", " << m_latency[srcIp].m_mean
                << ", " << m_latency[srcIp].m_max << "]";
      DropPacket(ctx, pkt);
    } else {
      LOG(ERROR) << "Unexpected GTP Type (" << +gtpuType << ")";
      DropPacket(ctx, pkt);
    }
  }
}

CommandResponse GtpuPathMonitoring::CommandReadStats(
    const bess::pb::GtpuPathMonitoringCommandReadArg &arg) {
  bess::pb::GtpuPathMonitoringCommandReadResponse resp;
  for (auto &element : m_latency) {
    bess::pb::GtpuPathMonitoringCommandReadResponse::Statistic stat;
    stat.set_gnb_ip(element.first);
    stat.set_count(element.second.m_count);
    stat.set_latency_min(element.second.m_min);
    stat.set_latency_mean(element.second.m_mean);
    stat.set_latency_max(element.second.m_max);
    *resp.add_statistics() = stat;

    if (arg.clear()) {
      element.second.m_count = 0;
      element.second.m_min = std::numeric_limits<uint64_t>::max();
      element.second.m_mean = 0;
      element.second.m_max = 0;
    }
  }

  return CommandSuccess(resp);
}

CommandResponse GtpuPathMonitoring::Init(const bess::pb::EmptyArg &) {
  GtpuPathMonitoring::Clear();

  return CommandSuccess();
}

CommandResponse GtpuPathMonitoring::CommandAdd(
    const bess::pb::GtpuPathMonitoringCommandAddDeleteArg &arg) {
  uint32_t dst = arg.gnb_ip();
  auto it = m_dstIp.find(dst);
  if (it == m_dstIp.end()) {
    m_dstIp.emplace(dst, 1);
  } else {
    it->second++;
  }

  return CommandSuccess();
}

CommandResponse GtpuPathMonitoring::CommandDelete(
    const bess::pb::GtpuPathMonitoringCommandAddDeleteArg &arg) {
  uint32_t dst = arg.gnb_ip();
  auto it = m_dstIp.find(dst);
  if (it == m_dstIp.end()) {
    LOG(ERROR) << "Address " << ToIpv4Address(static_cast<be32_t>(dst))
               << " is not known";
  } else {
    if (it->second == 1) {
      m_dstIp.erase(dst);
    } else {
      it->second--;
    }
  }

  return CommandSuccess();
}

CommandResponse GtpuPathMonitoring::CommandClear(
    const bess::pb::GtpuPathMonitoringCommandClearArg &) {
  GtpuPathMonitoring::Clear();

  return CommandSuccess();
}

void GtpuPathMonitoring::Clear() {
  m_storedData.clear();
  m_latency.clear();
  m_dstIp.clear();
  m_seqNumber = 0;
}

ADD_MODULE(GtpuPathMonitoring, "gtpu_path_monitoring",
           "Gtpu path monitoring module")
