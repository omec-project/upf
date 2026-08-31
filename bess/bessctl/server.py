# SPDX-FileCopyrightText: 2016-2017, Nefeli Networks, Inc.
# SPDX-FileCopyrightText: 2017, The Regents of the University of California.
# SPDX-License-Identifier: BSD-3-Clause

import json

from flask import Flask, jsonify
from google.protobuf.json_format import MessageToJson

app = Flask(__name__)
app.config['SEND_FILE_MAX_AGE_DEFAULT'] = 0


@app.route('/')
def get_html():
    return app.send_static_file('graph.html')


@app.route('/pipeline')
def get_pipeline():
    if not app.bess.is_connected():
        app.bess.disconnect()
        app.bess.connect(grpc_url=app.bess.peer)
    modules = {}
    for m_pb in app.bess.list_modules().modules:
        # NOTE: MessageToJson will convert 64-bit integers to strings!
        info_pb = app.bess.get_module_info(m_pb.name)
        modules[m_pb.name] = json.loads(
            MessageToJson(info_pb, always_print_fields_with_no_presence=True))
    return jsonify(modules)
