/*
 * SPDX-License-Identifier: Apache-2.0
 * Copyright 2026 Forsway Scandinavia AB
 */
#include "gtp.h"

#include <gtest/gtest.h>

#include <vector>

using bess::utils::Gtpv1;

namespace {

// Flag octets (TS 29.281 §5.1): version 1, PT 1, and E, S or PN as named.
constexpr uint8_t kNoOptions = 0x30;
constexpr uint8_t kNpdu = 0x31;
constexpr uint8_t kSeq = 0x32;
constexpr uint8_t kExt = 0x34;
constexpr uint8_t kAllOptions = 0x37;

// A GTP-U header as it arrives, followed by `trailing` bytes of payload. The
// header length is only ever computed over the bytes a test says are present.
std::vector<uint8_t> Header(uint8_t flags, std::vector<uint8_t> after_fixed,
                            size_t trailing = 0) {
  std::vector<uint8_t> h = {flags, 0xff, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01};
  h.insert(h.end(), after_fixed.begin(), after_fixed.end());
  h.insert(h.end(), trailing, 0xaa);
  return h;
}

size_t Length(const std::vector<uint8_t> &bytes, size_t avail) {
  return reinterpret_cast<const Gtpv1 *>(bytes.data())->header_length(avail);
}

size_t Length(const std::vector<uint8_t> &bytes) {
  return Length(bytes, bytes.size());
}

// Sequence number, N-PDU number, next extension header type.
std::vector<uint8_t> Options(uint8_t next_type) {
  return {0x00, 0x00, 0x00, next_type};
}

// A PDU Session Container (TS 38.415) of one 4-octet unit, as UERANSIM sends.
std::vector<uint8_t> PduSessionContainer(uint8_t next_type) {
  return {0x01, 0x10, 0x09, next_type};
}

std::vector<uint8_t> Concat(std::vector<std::vector<uint8_t>> parts) {
  std::vector<uint8_t> out;
  for (const auto &p : parts) {
    out.insert(out.end(), p.begin(), p.end());
  }
  return out;
}

TEST(GtpHeaderLengthTest, FixedHeaderOnly) {
  EXPECT_EQ(Length(Header(kNoOptions, {}, 20)), 8u);
}

TEST(GtpHeaderLengthTest, OptionalFieldsWithoutExtensions) {
  EXPECT_EQ(Length(Header(kSeq, Options(0x00), 20)), 12u);
}

TEST(GtpHeaderLengthTest, NpduNumberAloneBringsTheOptionalFields) {
  EXPECT_EQ(Length(Header(kNpdu, Options(0x00), 20)), 12u);
}

TEST(GtpHeaderLengthTest, AllThreeFlagsWithOneExtension) {
  auto h = Header(kAllOptions,
                  Concat({Options(0x85), PduSessionContainer(0x00)}), 20);
  EXPECT_EQ(Length(h), 16u);
}

TEST(GtpHeaderLengthTest, OnePduSessionContainer) {
  auto h = Header(kExt, Concat({Options(0x85), PduSessionContainer(0x00)}), 20);
  EXPECT_EQ(Length(h), 16u);
}

TEST(GtpHeaderLengthTest, TwoChainedExtensions) {
  // A two-unit extension, then a PDU Session Container ending the chain.
  std::vector<uint8_t> two_units = {0x02, 0x00, 0x00, 0x00,
                                    0x00, 0x00, 0x00, 0x85};
  auto h = Header(
      kExt, Concat({Options(0x40), two_units, PduSessionContainer(0)}), 20);
  EXPECT_EQ(Length(h), 8u + 4u + 8u + 4u);
}

TEST(GtpHeaderLengthTest, ExactFitIsAccepted) {
  auto h = Header(kExt, Concat({Options(0x85), PduSessionContainer(0x00)}));
  ASSERT_EQ(h.size(), 16u);
  EXPECT_EQ(Length(h), 16u);
}

TEST(GtpHeaderLengthTest, FixedHeaderCutShort) {
  auto h = Header(kNoOptions, {});
  for (size_t avail = 0; avail < 8; avail++) {
    EXPECT_EQ(Length(h, avail), 0u) << "avail " << avail;
  }
}

TEST(GtpHeaderLengthTest, OptionalFieldsCutShort) {
  auto h = Header(kSeq, Options(0x00));
  for (size_t avail = 8; avail < 12; avail++) {
    EXPECT_EQ(Length(h, avail), 0u) << "avail " << avail;
  }
}

TEST(GtpHeaderLengthTest, ExtensionCutShort) {
  auto h = Header(kExt, Concat({Options(0x85), PduSessionContainer(0x00)}));
  // Every cut from just after the options up to one byte short of the end.
  for (size_t avail = 12; avail < 16; avail++) {
    EXPECT_EQ(Length(h, avail), 0u) << "avail " << avail;
  }
}

TEST(GtpHeaderLengthTest, ChainPastTheBound) {
  // The extension claims 64 units (256 octets); only 4 are present.
  std::vector<uint8_t> overlong = {0x40, 0x00, 0x00, 0x00};
  auto h = Header(kExt, Concat({Options(0x85), overlong}), 8);
  EXPECT_EQ(Length(h), 0u);
}

TEST(GtpHeaderLengthTest, NextTypeWithNothingAfterIt) {
  // The options say an extension follows, and the packet ends there.
  auto h = Header(kExt, Options(0x85));
  EXPECT_EQ(Length(h), 0u);
}

// TS 29.281 §5.2.1: an extension header is n*4 octets with n positive. A zero
// length after a non-zero next type never advances, so an unbounded walk over
// it does not terminate.
TEST(GtpHeaderLengthTest, ZeroLengthExtensionIsRejected) {
  std::vector<uint8_t> zero_length = {0x00, 0x00, 0x00, 0x85};
  auto h = Header(kExt, Concat({Options(0x85), zero_length}), 20);
  EXPECT_EQ(Length(h), 0u);
}

TEST(GtpHeaderLengthTest, ExtensionFlagClearIgnoresNextType) {
  // With S set and E clear the options are present, but their next-type octet
  // is not followed: there is no chain to walk.
  auto h = Header(kSeq, Options(0x85), 20);
  EXPECT_EQ(Length(h), 12u);
}

}  // namespace
