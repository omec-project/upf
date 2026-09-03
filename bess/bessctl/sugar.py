# Copyright (c) 2014-2016, The Regents of the University of California.
# Copyright (c) 2016-2017, Nefeli Networks, Inc.
# All rights reserved.
#
# SPDX-License-Identifier: BSD-3-Clause
#
# Redistribution and use in source and binary forms, with or without
# modification, are permitted provided that the following conditions are met:
#
# * Redistributions of source code must retain the above copyright notice, this
# list of conditions and the following disclaimer.
#
# * Redistributions in binary form must reproduce the above copyright notice,
# this list of conditions and the following disclaimer in the documentation
# and/or other materials provided with the distribution.
#
# * Neither the names of the copyright holders nor the names of their
# contributors may be used to endorse or promote products derived from this
# software without specific prior written permission.
#
# THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
# AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
# IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE
# ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE
# LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR
# CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF
# SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS
# INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN
# CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE)
# ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE
# POSSIBILITY OF SUCH DAMAGE.

import io
import re
import tokenize

"""
<BESS script language>
- Providing a Click-like module connection semantics
- All these syntactic sugars must be able to coexist with original Python
  syntax. e.g.,
    print('hello %s' % (%ENV!'Anynomous'))

--------------------------------------------------------------------------------
Syntax               Semantic                      Plain Python code
--------------------------------------------------------------------------------
$ENV                 The value of environment      __bess_env__('ENV')
                     variable ENV (string)
                     No empty string exists

$ENV!str             If ENV not exists, use x      __bess_env__('ENV', str)
                     (x can be a string literal,
                     variable of a string literal,
                     or parenthesis string literal
                     expression)

a::SomeModule()      Create a SomeModule and       __bess_module__('a',
                     assign into variable a          'SomeModule')

a::SomeModule(...)   Additional parameters for     __bess_module__('a',
                     creating module                 'SomeModule', ...)

a,b::SomeModule()    Create two SomeModule and     __bess_module__(('a','b'),
                     assign into a and b             'SomeModule')

a->b                 Connect a and b               a + b

a:x->b               Connect output gate x of a    a*i + b
                     and b                         a.connect(next_mod=b,
                     (x should be an integer)        ogate=x)

a->y:b               Connect a to input gate y     a + y*b
                     of b                          a.connect(next_mod=b,
                     (y should be an integer)        igate=y)

a:3->4:b             Connect output gate 3 of a    a*3 + 4*b
                     and input gate of 4           a.connect(next_mod=b,
                                                     ogate=3, igate=4)

--------------------------------------------------------------------------------

--------------------------------------------------------------------------------
Usage examples
--------------------------------------------------------------------------------

1. Chaining multiple module connection
Ringo:
    a -> b:1 -> c
Python:
    a + b*1 + c

2. Create and connect at once
Ringo:
    a::Foo() -> b::Bar()
Python:
    __bess_module__('a','Foo') + __bess_module__('b', 'Bar')

3. Create anonymous modules and connect them
Ringo:
    Foo() -> Bar()
Python:
    Foo() + Bar()

4. Composition example
Ringo:
    a::Foo():3 -> b
Python:
    __bess_module__('a', 'Foo')*3 + b

5.
a::Foo():1 -> 2:b::Foo()
- Connecting output gate 1 of a to input gate 2 of b
"""

# common python tokens
NAME = r"[a-zA-Z_]\w*"
COMMENT = r"#[^\r\n]*"
STRING_SHORT = r"\'.*?\'|\".*?\""
STRING_LONG = r'\'\'\'.*?\'\'\'|""".*?"""'
STRING_ALL = STRING_LONG + "|" + STRING_SHORT

# constants for duplicate literals
BESS_ENV_PREFIX = "__bess_env__('"


def replace_envvar(s):
    environment = (
        r"\$(" + NAME + ")" r"(!(" + STRING_SHORT + "|" + NAME + "))?" r"(!\()?"
    )

    # first group: # leading COMMENT -> skip
    # second group: single / double /triple quoted strings -> skip
    # third group: dollar sign with NAME -> $var!assignment
    # third group consists of
    #   fourth group: environment variable
    #   fifth group:
    #   e.g., $ENV!'100'
    #       third group  "$ENV!'100'"
    #       fourth group 'ENV'
    #       fifth group '!' or not

    pattern = "(" + COMMENT + ")|(" + STRING_ALL + ")|(" + environment + ")"
    regex = re.compile(pattern, re.MULTILINE | re.DOTALL)

    def _replacer(match):
        if match.group(3) is not None:
            # from our definition of pattern
            # if match.group(3) is not None, match.group(4) is not None
            # if match.group(5) is not None, then there is a parameter
            if match.group(5) is None and match.group(7) is None:
                return BESS_ENV_PREFIX + match.group(4) + "')"
            elif match.group(5) is not None:
                return BESS_ENV_PREFIX + match.group(4) + "', " + match.group(6) + ")"
            else:
                return BESS_ENV_PREFIX + match.group(4) + "', "

        else:
            return match.group()

    s = regex.sub(_replacer, s)
    return s


def is_gate_expr(exp, is_ogate):
    # check if the leading/trailing whitespace characters contains '\n'
    if is_ogate:
        prefix, postfix = "1*", "+1"
    else:
        prefix, postfix = "1+", "*1"

    exp_stripped = exp.strip()
    while len(exp_stripped) > 0 and exp_stripped[-1] == "\\":
        exp_stripped = exp_stripped[:-1].strip()

    try:
        compile(f"({exp_stripped})", "", mode="eval")
        compile(f"{prefix}{exp}{postfix}", "", mode="eval")
    except SyntaxError:
        return False
    else:
        return True


def replace_rarrows(s):
    """Replace right arrows (->) with appropriate Python operators."""
    arrows = find_arrow_positions(s)
    segments = split_string_by_arrows(s, arrows)
    transform_gate_expressions(segments)
    return "+".join(segments)


def find_arrow_positions(s):
    """Find all arrow positions in the string using tokenization."""
    last_token = None
    arrows = []
    try:
        for t in tokenize.generate_tokens(io.StringIO(s).readline):
            token = t[1]
            row, col = t[2]
            # Handle Python 2.x (separate tokens) and Python 3 (single token)
            if last_token == "-" and token == ">":
                arrows.append((row - 1, col - 1))
            elif token == "->":
                arrows.append((row - 1, col))
            last_token = token
    except (tokenize.TokenError, IndentationError):
        pass
    return arrows


def split_string_by_arrows(s, arrows):
    """Split string into segments at '->' positions."""
    segments = []
    curr_seg = []
    arrow_idx = 0
    lines = io.StringIO(s).readlines()
    line_idx = 0
    col_offset = 0

    while line_idx < len(lines):
        line = lines[line_idx]
        row, col = arrows[arrow_idx] if arrow_idx < len(arrows) else (None, None)

        if row is None or line_idx < row:
            curr_seg.append(line[col_offset:])
            line_idx += 1
            col_offset = 0
        elif line_idx == row:
            curr_seg.append(line[col_offset:col])
            segments.append("".join(curr_seg))
            curr_seg = []
            col_offset = col + 2  # Skip the '->' characters
            arrow_idx += 1
        else:
            # This should be unreachable given the logic above
            raise RuntimeError("Splitting logic encountered an invalid state")

    segments.append("".join(curr_seg))
    return segments


def transform_gate_expressions(segments):
    """Transform output and input gate expressions in segments."""
    for i in range(len(segments) - 1):
        process_output_gate(segments, i)
        process_input_gate(segments, i)


def process_output_gate(segments, i):
    seg = segments[i]
    colon_pos = seg.rfind(":")
    while colon_pos != -1:
        ogate = seg[colon_pos + 1 :]
        if not ogate.strip():  # Break immediately if empty
            break
        if is_gate_expr(ogate, True):
            segments[i] = seg[:colon_pos] + "*" + parenthesize(ogate)
            break
        colon_pos = seg.rfind(":", 0, colon_pos)


def process_input_gate(segments, i):
    seg = segments[i + 1]
    colon_pos = seg.find(":")
    while colon_pos != -1:
        igate = seg[:colon_pos]
        if not igate.strip():  # Break immediately if empty
            break
        if is_gate_expr(igate, False):
            segments[i + 1] = parenthesize(igate) + "*" + seg[colon_pos + 1 :]
            break
        colon_pos = seg.find(":", colon_pos + 1)


def parenthesize(exp):
    """Add parentheses around expression if it contains operators."""
    try:
        for t in tokenize.generate_tokens(io.StringIO(exp).readline):
            if t[0] == tokenize.OP:
                l = len(exp) - len(exp.lstrip())
                r = len(exp) - len(exp.rstrip())
                return f"{exp[:l]}({exp.strip()}){exp[len(exp) - r :]}"
    except (tokenize.TokenError, IndentationError):
        pass
    return exp


def create_module_string(s):

    # single module -> return a module name string
    if s.find(",") < 0:
        return "'" + s + "'"

    # multiple module -> return a tuple of module name string
    mstr = "("
    for module in s.split(","):
        mstr += "'" + module.strip() + "', "
    mstr += ")"
    return mstr


def replace_module_assignment(s):
    target = "(" + NAME + r"(,[ \t]*" + NAME + ")*" + ")::(" + NAME + r")[ \t]*\("

    # first group: # leading COMMENT -> skip
    # second group: single / double / triple quoted strings -> skip
    # third group: replace target  'NAME::NAME'
    pattern = "(" + COMMENT + ")|(" + STRING_ALL + ")|(" + target + ")"
    regex = re.compile(pattern, re.MULTILINE | re.DOTALL)

    def _replacer(match):
        if match.group(3) is not None:
            # if match.group(3) is not None,
            # match.group(4), match.group(6) is not None
            # match.group(4) -> module NAMEs
            # match.group(6) -> module class NAME
            modules = create_module_string(match.group(4))
            f_str = "__bess_module__(" + modules + ", '" + match.group(6) + "', "
            return f_str

        else:
            return match.group()

    return regex.sub(_replacer, s)


def xform_str(s):
    s = replace_envvar(s)
    s = replace_module_assignment(s)
    s = replace_rarrows(s)
    return s


def xform_file(filename):
    with open(filename) as f:
        return xform_str(f.read())
