// SPDX-FileCopyrightText: 2016-2017, Nefeli Networks, Inc.
// SPDX-FileCopyrightText: 2017, The Regents of the University of California.
// SPDX-License-Identifier: BSD-3-Clause

// colorscheme:
// #01295f cool black
// #437f97 queen blue
// #849324 olive drab
// #ffb30f dark tangerine
// #fd151b vivid red

// (node name, gate type, gate ID) -> [(timestamp, pkts, bits, cnt), ...]
const stats = {};

function gates_to_str(gates, gate_type) {
    let ret = '';

    for (let i = 0; i < gates.length; i++) {
        const gate_num = gates[i][gate_type]
        if (gate_type == 'igate') {
            color = '#437f97'
        } else {
            color = '#01295f'
        }
        ret += `<td port="${gate_type}${gate_num}" border="0" cellpadding="0" bgcolor="${color}"><font color="#ffffff" point-size="6">${gate_num}</font></td>
`;
    }

    return `
      <tr>
        <td border="0" cellspacing="0" cellpadding="0">
          <table border="0" cellborder="0" cellspacing="0" cellpadding="0">
            <tr>
              ${ret}
            </tr>
          </table>
        </td>
      </tr>`;
}

function add_datapoints(stats, module_name, gates, gate_type) {
    for (let i = 0; i < gates.length; i++) {
        const gate = gates[i];
        const key = [module_name, gate_type, gate[gate_type]];
        const value = {timestamp: gate.timestamp,
                 bits: Number(gate.bytes * 8),
                 pkts: Number(gate.pkts),
                 cnt: Number(gate.cnt),
                 batchsize: gate.cnt ? gate.pkts / gate.cnt : 0};
        if (!(key in stats)) {
            stats[key] = [value];
        } else {
            stats[key].push(value);
        }
    }
}

function get_edge_label(stats, options) {
    if (!stats || stats.length === 0) {
        return format_html_label('?');
    }

    const value = stats[stats.length - 1];

    if (value.timestamp <= 0) {
        return format_html_label('?');
    }

    const label = calculate_label_by_mode(stats, value, options);

    // Mode 'none' returns an empty string, which should not be wrapped in HTML
    if (label === '') {
        return '';
    }

    const formatted_label = format_label_text(label, options);
    return format_html_label(formatted_label);
}

function calculate_label_by_mode(stats, value, options) {
    switch (options.mode) {
        case 'total':
            return value[options.field];
        case 'rate':
            return calculate_rate_label(stats, value, options);
        case 'none':
            return '';
        default:
            throw new Error('Unknown mode ' + options.mode);
    }
}

function calculate_rate_label(stats, value, options) {
    if (stats.length < 2) {
        return '?';
    }

    const last = stats[stats.length - 2];
    const time_diff = value.timestamp - last.timestamp;

    if (options.field === 'batchsize') {
        const packets = value.pkts - last.pkts;
        const batches = value.cnt - last.cnt;
        return batches ? packets / batches : 'N/A';
    }

    const value_diff = value[options.field] - last[options.field];
    return Math.round(value_diff / time_diff);
}

function format_label_text(label, options) {
    if (typeof label !== 'number' || !options.humanreadable) {
        return label;
    }

    if (options.mode === 'rate') {
        return format_rate_number(label, options.field);
    }

    return label.toLocaleString('en-US', {maximumFractionDigits: 2}) + ' ';
}

function format_rate_number(label, field) {
    let unit = ' ';
    let scaled = label;

    if (label > 1000000000) {
        scaled /= 1000000000;
        unit += 'G';
    } else if (label > 1000000) {
        scaled /= 1000000;
        unit += 'M';
    } else if (label > 1000) {
        scaled /= 1000;
        unit += 'k';
    }

    if (field === 'pkts') unit += 'pps';
    else if (field === 'bits') unit += 'bps';

    return scaled.toLocaleString('en-US', {maximumFractionDigits: 2}) + unit;
}

function format_html_label(label) {
    return `<<table border="0" cellpadding="0"><tr><td bgcolor="white">${label}</td></tr></table>>`;
}

function graph_to_dot(modules) {
    const options = get_graph_options();
    // Pass options down so sub-functions can use them
    const nodes = generate_nodes(modules, options);
    const edges = generate_edges(modules, options);

    return `digraph G {
  graph [ rankdir=TB ];
  node [ fontsize=12 ];
  edge [ fontsize=9, color="#ffb30f", arrowsize=0.5, labeldistance=1.2 ];
${nodes}
${edges}
}
`;
}

function get_graph_options() {
    return {
        field: document.querySelector('input[name="metric"]:checked').value,
        mode: document.querySelector('input[name="mode"]:checked').value,
        humanreadable: document.querySelector('input[name="humanreadable"]').checked
    };
}

function generate_nodes(modules, options) {
    let nodes = '';
    for (const module_name in modules) {
        const module_data = modules[module_name];

        // Pass options if add_datapoints needs them
        add_datapoints(stats, module_name, module_data.ogates, 'ogate');

        set_gate_visibility(module_data);

        // Pass options if gates_to_str needs them
        const node_content = create_module_node(module_data, module_name, options);
        nodes += node_content;
    }
    return nodes;
}

function set_gate_visibility(module) {
    const check = (gates, type) => gates.length > 1 || (gates.length === 1 && gates[0][type] != 0);
    module.show_igates = check(module.igates, 'igate');
    module.show_ogates = check(module.ogates, 'ogate');
}

function create_module_node(module, module_name, options) {
    const desc = module.desc ? `<font point-size="9">${module.desc}</font>` : '';
    // Original gates_to_str might need options
    const igates = module.show_igates ? gates_to_str(module.igates, 'igate', options) : '';
    const ogates = module.show_ogates ? gates_to_str(module.ogates, 'ogate', options) : '';

    return `
  "${module_name}" [shape=plaintext label=
    <<table port="mod" border="1" cellborder="0" cellspacing="0" cellpadding="1">
      ${igates}<tr>
        <td width="60">${module_name}</td>
      </tr>
      <tr>
        <td><font color="#888888" point-size="9"><i>${module.mclass}</i></font></td>
      </tr>
      <tr>
        <td>${desc}</td>
      </tr>
      ${ogates}</table>>];
`;
}

function generate_edges(modules, options) {
    let edges = '';
    for (const module_name in modules) {
        const module = modules[module_name];
        edges += create_module_edges(module, module_name, modules, options);
    }
    return edges;
}

function create_module_edges(module, module_name, modules, options) {
    let edges = '';
    for (const gate of module.ogates) {
        const dst_module = modules[gate.name];
        edges += create_single_edge(module, module_name, gate, dst_module, options);
    }
    return edges;
}

function create_single_edge(module, module_name, gate, dst_module, options) {
    const out_port = module.show_ogates ? `ogate${gate.ogate}:s` : 'mod';
    const in_port = dst_module.show_igates ? `igate${gate.igate}:n` : 'mod';

    // Pass options to get_edge_label
    let label = get_edge_label(stats[[module_name, 'ogate', gate.ogate]], options);
    if (label !== '') {
        label = ` [label=${label}]`;
    }

    return `  "${module_name}":${out_port} -> "${gate.name}":${in_port}${label}\n`;
}
