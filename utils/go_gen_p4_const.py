# Copyright 2022-present Open Networking Foundation
#
# SPDX-License-Identifier: Apache-2.0

import argparse
import google.protobuf.text_format as tf
import re
from p4.config.v1 import p4info_pb2

copyright = '''/*
 * Copyright 2022-present Open Networking Foundation
 *
 * SPDX-License-Identifier: Apache-2.0
 */
'''

PKG_FMT = 'package %s'

CONST_OPEN = '''//noinspection GoSnakeCaseUsage
    const ('''
CONST_CLOSE = ')'


UINT32 = 'uint32'
EMPTY_STR = ''

HF_VAR_PREFIX = 'Hdr_'
TBL_VAR_PREFIX = 'Table_'
CTR_VAR_PREFIX = 'Counter_'
DIRCTR_VAR_PREFIX = 'DirectCounter_'
ACT_VAR_PREFIX = "Action_"
ACTPARAM_VAR_PREFIX = "ActionParam_"
ACTPROF_VAR_PREFIX = "ActionProfile_"
PACKETMETA_VAR_PREFIX = "PacketMeta_"
MTR_VAR_PREFIX = "Meter_"

class ConstantClassGenerator(object):
    header_fields = dict()
    tables = set()
    counters = set()
    direct_counters = set()
    actions = set()
    action_params = dict()
    action_profiles = set()
    packet_metadata = set()
    meters = set()

    # https://stackoverflow.com/questions/1175208/elegant-python-function-to-convert-camelcase-to-snake-case
    def convert_camel_to_all_caps(self, name):
        s1 = re.sub('(.)([A-Z][a-z]+)', r'\1_\2', name)
        s1 = re.sub('([a-z0-9])([A-Z])', r'\1_\2', s1).title()
        return s1.replace('_', '').replace('.', '')

    def __init__(self, pkg_name):
        self.package_name = PKG_FMT % (pkg_name, )

    def parse(self, p4info):
        for tbl in p4info.tables:
            for mf in tbl.match_fields:
                try:
                    self.header_fields[tbl.preamble.name]
                except KeyError:
                    self.header_fields[tbl.preamble.name] = set()
                finally:
                    self.header_fields[tbl.preamble.name].add((mf.name, mf.id))

            self.tables.add((tbl.preamble.name, tbl.preamble.id))

        for ctr in p4info.counters:
            self.counters.add((ctr.preamble.name, ctr.preamble.id))

        for dir_ctr in p4info.direct_counters:
            self.direct_counters.add((dir_ctr.preamble.name, dir_ctr.preamble.id))

        for act in p4info.actions:
            self.actions.add((act.preamble.name, act.preamble.id))

            for param in act.params:
                try:
                    self.action_params[act.preamble.name]
                except KeyError as e:
                    self.action_params[act.preamble.name] = set()
                finally:
                    self.action_params[act.preamble.name].add((param.name, param.id))

        for act_prof in p4info.action_profiles:
            self.action_profiles.add((act_prof.preamble.name, act_prof.preamble.id))

        for cpm in p4info.controller_packet_metadata:
            for mta in cpm.metadata:
                self.packet_metadata.add((mta.name, mta.id))
        for mtr in p4info.meters:
            self.meters.add((mtr.preamble.name, mtr.preamble.id))

    def const_line(self, obj_type, name, id, var_type=UINT32, table_prefix=None):
        var_name = self.convert_camel_to_all_caps(name)
        if table_prefix:
            var_name = self.convert_camel_to_all_caps(table_prefix) + "_" + var_name
        var_name = obj_type+var_name
        base_line = "\t%s %s = %i"
        line = base_line % (var_name, var_type, id)
        return line

    def generate_go(self):
        lines = list()
        lines.append(copyright)
        lines.append(self.package_name)
        lines.append(CONST_OPEN)

        if len(self.header_fields) != 0:
            lines.append('    // Header field IDs')
        for (tbl, hfs) in self.header_fields.items():
            for hf in hfs:
                lines.append(self.const_line(HF_VAR_PREFIX, hf[0], hf[1], table_prefix=tbl))

        if len(self.tables) != 0:
            lines.append('    // Table IDs')
        for tbl in self.tables:
            lines.append(self.const_line(TBL_VAR_PREFIX, tbl[0], tbl[1]))

        if len(self.counters) != 0:
            lines.append('    // Indirect Counter IDs')
        for ctr in self.counters:
            lines.append(self.const_line(CTR_VAR_PREFIX, ctr[0], ctr[1]))

        if len(self.direct_counters) != 0:
            lines.append('    // Direct Counter IDs')
        for dctr in self.direct_counters:
            lines.append(self.const_line(DIRCTR_VAR_PREFIX, dctr[0], dctr[1]))

        if len(self.actions) != 0:
            lines.append('    // Action IDs')
        for act in self.actions:
            lines.append(self.const_line(ACT_VAR_PREFIX, act[0], act[1]))

        if len(self.action_params) != 0:
            lines.append('    // Action Param IDs')
        for (tbl, act_prms) in self.action_params.items():
            for act_prm in act_prms:
                lines.append(self.const_line(ACTPARAM_VAR_PREFIX, act_prm[0], act_prm[1], table_prefix=tbl))

        if len(self.action_profiles) != 0:
            lines.append('    // Action Profile IDs')
        for act_prof in self.action_profiles:
            lines.append(self.const_line(ACTPROF_VAR_PREFIX, act_prof[0], act_prof[1]))

        if len(self.packet_metadata) != 0:
            lines.append('    // Packet Metadata IDs')
        for pmeta in self.packet_metadata:
            if not pmeta[0].startswith("_"):
                lines.append(self.const_line(PACKETMETA_VAR_PREFIX, pmeta[0], pmeta[1]))

        if len(self.meters) != 0:
            lines.append('    // Meter IDs')
        for mtr in self.meters:
            lines.append(self.const_line(MTR_VAR_PREFIX, mtr[0], mtr[1]))
        lines.append(CONST_CLOSE)
        # end of class

        return '\n'.join(lines)

def main():
    parser = argparse.ArgumentParser(prog='go-gen-p4-const',
                                     description='P4Info to Go constant generator.')
    parser.add_argument('-o', '--output', help='path to output file', default='-')
    parser.add_argument('-p', '--p4info', help='path to p4info file (text format)')
    args = parser.parse_args()

    p4info_file = args.p4info
    output_file = args.output

    pieces = args.output.split('/')
    pkg_name = pieces[-2] if len(pieces) > 1 else 'undefined'

    p4info = p4info_pb2.P4Info()
    with open(p4info_file, 'r') as input_file:
        s = input_file.read()
        tf.Merge(s, p4info)

    gen = ConstantClassGenerator(pkg_name)
    gen.parse(p4info)
    go_code = gen.generate_go()

    if output_file == '-':
        # std output
        print(go_code)
    else:
        with open(output_file, 'w') as output_file:
            output_file.write(go_code)


if __name__ == '__main__':
    main()
