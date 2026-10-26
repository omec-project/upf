/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright 2026 Forsway Scandinavia AB
 */
#ifndef BESS_MODULES_GTPU_DROP_LOG_H_
#define BESS_MODULES_GTPU_DROP_LOG_H_

#include <glog/logging.h>

#include <algorithm>
#include <cstdint>
#include <cstdio>
#include <string>

#include "../module.h"
#include "../packet.h"
#include "../worker.h"

/* Logs a sample of the packets the GTP-U modules drop as malformed, so their
 * source can be found: at most one line a second per worker, carrying the
 * number of drops not logged since that worker's previous line. A flood
 * therefore costs a time comparison and an increment per packet.
 *
 * The state is per worker because the modules using this run on several
 * workers at once; one shared timestamp would be a data race. */
class GtpuDropLog {
 public:
  void Sample(const Context *ctx, const std::string &module, const char *reason,
              bess::Packet *p) {
    State &s = state_[ctx->wid];
    if (ctx->current_ns < s.next_ns) {
      s.suppressed++;
      return;
    }
    s.next_ns = ctx->current_ns + kIntervalNs;

    const uint8_t *data = p->head_data<const uint8_t *>();
    int n = std::min(p->head_len(), kSampleBytes);
    std::string dump;
    dump.reserve(2 * n);
    char octet[3];
    for (int i = 0; i < n; i++) {
      snprintf(octet, sizeof(octet), "%02x", data[i]);
      dump.append(octet);
    }

    LOG(WARNING) << module
                 << ": dropped malformed packet on the GTP-U parsing path ("
                 << reason << "): head_len=" << p->head_len()
                 << " total_len=" << p->total_len()
                 << " nb_segs=" << p->nb_segs() << " first " << n << " bytes "
                 << dump << " (" << s.suppressed
                 << " not logged since this worker's previous sample)";
    s.suppressed = 0;
  }

 private:
  static constexpr uint64_t kIntervalNs = 1000000000ull;
  static constexpr int kSampleBytes = 64;

  /* One cache line each, as gate_hooks/track.h does for its per-worker
   * counters: a flood increments `suppressed` on every worker at once. */
  struct alignas(64) State {
    uint64_t next_ns = 0;
    uint64_t suppressed = 0;
  };
  State state_[Worker::kMaxWorkers];
};

#endif  // BESS_MODULES_GTPU_DROP_LOG_H_
