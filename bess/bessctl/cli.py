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

import sys
import os
from operator import itemgetter


class ColorizedOutput(object):  # for pretty printing

    def __init__(self, orig_out, color):
        self.orig_out = orig_out
        self.color = color

    def __getattr__(self, attr):
        def_color = '\033[0;0m'  # resets all terminal attributes

        if attr == 'write':
            return lambda x: self.orig_out.write(self.color + x + def_color)
        else:
            return getattr(self.orig_out, attr)


class CLI(object):

    class CommandError(Exception):  # general command errors
        pass

    class HandledError(Exception):
        pass

    class InvalidCommandError(Exception):
        pass

    # variable binding errors
    class BindError(Exception):
        pass

    # some internal logic errors that might be your (or my) fault
    class InternalError(Exception):
        pass

    def __init__(self, cmdlist, fin=sys.stdin, fout=sys.stdout, ferr=None,
                interactive=None, history_file=None):
        self.cmdlist = cmdlist
        self.fin = fin
        self.fout = fout
        self.last_cmd = ''
        self.rl = None

        self.history_file = self._setup_history_file(history_file)
        self.ferr = self._setup_error_output(ferr)
        self.interactive = self._setup_interactive_mode(interactive, fin, fout)

        if self.interactive:
            self.go_interactive()

    def _setup_history_file(self, history_file):
        """Setup history file path with fallback to default."""
        if history_file is not None:
            return history_file

        try:
            return os.path.expanduser('~/.bess_history')
        except Exception:
            return None

    def _setup_error_output(self, ferr):
        """Setup error output with colorization support."""
        if ferr is not None:
            return ferr

        if os.environ.get('TERM') != 'dumb' and sys.stderr.isatty():
            return ColorizedOutput(sys.stderr, '\033[31m')  # dark red

        return sys.stderr

    def _setup_interactive_mode(self, interactive, fin, fout):
        """Determine if CLI should run in interactive mode."""
        if interactive is not None:
            return interactive

        return fin.isatty() and fout.isatty()

    def err(self, msg):
        self.ferr.write('*** Error: %s\n' % msg)
        if not self.interactive:
            self.stop_loop = True

    # If not a variable, simply return None
    # Otherwise, return (var_type, desc, candidates):
    #    var_type can be: 'int', 'str', 'list'(list of strings), 'map'
    #    candidates is a list of string values.
    def get_var_attrs(self, var_token, partial_word):
        return None

    # Return (head, tail)
    #   head: consumed string portion
    #   tail: the rest of input line
    # You can assume that 'line == head + tail'
    def split_var(self, var_type, line):
        if var_type == 'keyword':
            pos = line.find(' ')
            if pos == -1:
                return line, ''
            else:
                return line[:pos], line[pos:]

        raise self.InternalError('type "%s" is undefined' % var_type)

    # Return (mapped_value, tail)
    #   mapped_value: Python value/object from the consumed token(s)
    #   tail: the rest of input line
    def bind_var(self, var_type, line):
        if var_type == 'keyword':
            return None, self.split_var(var_type, line)[1]

        raise self.InternalError('type "%s" is undefined' % var_type)

    # Compare a command with a user-typed line.
    # It returns (match_type, candidates, syntax_token, score).
    # match_type can be:
    #  - 'full': all tokens in syntax was consumed
    #  - 'partial': prefix matched
    #  - 'nonmatch': not a match
    # candidates is a list of suggested strings to be added as the last token.
    # syntax_token is where the user input is currently on, if any.
    # score is the number of matched keywords
    # exact_score is the number of "exactly" matched keywords
    def match(self, syntax, line):
        """
        Match command syntax against user input line.

        Returns:
            tuple: (match_type, candidates, syntax_token, score, exact_score)
            match_type: 'full', 'partial', or 'nonmatch'
        """
        # Initialize matching state
        candidates = []
        remainder = line
        score = 0
        exact_score = 0
        new_token = (line != '' and line[-1] == ' ')
        syntax_tokens = syntax.split()

        # Process each syntax token
        for i, syntax_token in enumerate(syntax_tokens):
            # Get current word and variable attributes
            line_word = self._get_line_word(remainder)
            var_type, var_desc, var_candidates = self._get_var_attrs(syntax_token, line_word)

            # Handle empty remainder (end of input)
            if remainder.strip() == '':
                return self._handle_empty_remainder(
                    i, syntax_token, syntax_tokens, var_type, var_candidates,
                    new_token, candidates, score, exact_score
                )

            # Process current token
            token, remainder = self.split_var(var_type, remainder)
            remainder = remainder.lstrip()

            # Update matching state based on variable type
            if var_type == 'keyword':
                result = self._process_keyword_match(
                    syntax_token, token, new_token, score, exact_score
                )
                if result['match_type'] == 'nonmatch':
                    return 'nonmatch', [], '', score, exact_score
                candidates = result['candidates']
                score = result['score']
                exact_score = result['exact_score']
            else:
                candidates = self._process_variable_match(
                    var_candidates, token, new_token
                )

        # Final validation after processing all tokens
        return self._finalize_match(
            remainder, syntax_token, new_token, candidates, score, exact_score
        )

    def _get_line_word(self, remainder):
        """Extract the first word from remainder or return empty string."""
        if remainder.split():
            return remainder.split()[0]
        return ''

    def _get_var_attrs(self, syntax_token, line_word):
        """Get variable attributes for a syntax token."""
        attrs = self.get_var_attrs(syntax_token, line_word)
        if attrs:
            return attrs
        return 'keyword', '', []

    def _handle_empty_remainder(self, i, syntax_token, syntax_tokens, var_type,
                            var_candidates, new_token, candidates, score, exact_score):
        """Handle case when user input is exhausted."""
        if new_token:
            # Clear candidates unless previous token allows continuation
            if i == 0 or '...' not in syntax_tokens[i - 1]:
                candidates = []

            candidates.extend(var_candidates)

            if var_type == 'keyword':
                candidates.append(syntax_token)

            # Check if current token is skippable (optional)
            if syntax_token[0] == '[':
                return 'full', candidates, syntax_token, score, exact_score

            return 'partial', candidates, syntax_token, score, exact_score

        return 'partial', candidates, syntax_tokens[max(0, i - 1)], score, exact_score

    def _process_keyword_match(self, syntax_token, token, new_token, score, exact_score):
        """Process matching for keyword tokens."""
        if syntax_token == token:
            # Exact match
            candidates = [] if new_token else [syntax_token]
            score += 1
            if syntax_token.strip() == token:
                exact_score += 1
        else:
            # Partial match
            if not syntax_token.startswith(token):
                return {'match_type': 'nonmatch', 'candidates': [], 'score': score, 'exact_score': exact_score}
            candidates = [syntax_token]
            score += 1
            if syntax_token.strip() == token:
                exact_score += 1

        return {'match_type': 'continue', 'candidates': candidates, 'score': score, 'exact_score': exact_score}

    def _process_variable_match(self, var_candidates, token, new_token):
        """Process matching for variable tokens."""
        if new_token:
            return var_candidates

        # Filter candidates that match the partial token
        candidates = []
        token_parts = token.split()
        if token_parts:
            last_part = token_parts[-1]
            for var in var_candidates:
                if var.startswith(last_part):
                    candidates.append(var)

        return candidates

    def _finalize_match(self, remainder, syntax_token, new_token, candidates, score, exact_score):
        """Final validation and result determination."""
        if remainder.strip() == '':
            # Check for ellipsis (variable length argument)
            if '...' in syntax_token:
                return 'full', candidates, syntax_token, score, exact_score

            # Handle new token case
            if new_token:
                return 'full', ['\n'], '', score, exact_score

            return 'full', candidates, syntax_token, score, exact_score

        return 'nonmatch', [], '', score, exact_score

    # filter is one of 'full', 'partial', 'nonmatch'
    def list_matched(self, line, filter):
        matched_list = []

        for cmd in self.cmdlist:
            syntax = cmd[0]
            match_type, _, _, score, exact_score = self.match(syntax, line)

            if match_type == filter:
                matched_list.append((cmd, score, exact_score))

        if len(matched_list) == 0:
            return [], []

        max_score = max([x[1] for x in matched_list])

        ret = [m[0] for m in matched_list if m[1] == max_score]
        ret_low = [m[0] for m in matched_list if m[1] != max_score]

        # Find exact matches without ambiguity
        if filter == 'full' and len(ret) > 1:
            full_matches = [m for m in matched_list if m[1] == max_score]
            # sorted by exact score
            full_matches.sort(key=itemgetter(2), reverse=True)
            if full_matches[0][2] != full_matches[1][2]:
                return [full_matches[0][0]], []

        return ret, ret_low

    def _do_complete(self, line, partial_word):
        """
        Handle command completion for the CLI.

        Returns:
            list: Completion candidates or empty list if help should be displayed
        """
        # Find matching commands and collect candidates
        possible_cmds, candidates = self._find_matching_commands(line, partial_word)

        # Try to find common prefix for auto-completion
        completion_candidates = self._try_common_prefix_completion(candidates, partial_word)
        if completion_candidates:
            return completion_candidates

        # Build and display help information
        help_buffer = self._build_help_buffer(possible_cmds, partial_word)
        self._write_help_output(help_buffer, line)

        return []

    def _find_matching_commands(self, line, partial_word):
        """Find commands that match the current input and collect completion candidates."""
        possible_cmds = []
        candidates = []

        for cmd in self.cmdlist:
            syntax = cmd[0]
            match_type, sub_candidates, syntax_token, _, _ = self.match(syntax, line)

            if match_type in ['full', 'partial']:
                possible_cmds.append((cmd, match_type, syntax_token))

                # Collect candidates that match the partial word
                for candidate in sub_candidates:
                    if candidate.startswith(partial_word):
                        formatted_candidate = self._format_candidate(candidate)
                        candidates.append(formatted_candidate)

        return possible_cmds, sorted(list(set(candidates)))

    def _format_candidate(self, candidate):
        """Format a completion candidate by adding space if needed."""
        if not candidate.endswith('/') and candidate != '\n':
            return candidate + ' '
        return candidate

    def _try_common_prefix_completion(self, candidates, partial_word):
        """Try to complete using common prefix if available."""
        if not candidates:
            return None

        common_prefix = self._find_common_prefix(candidates)

        if (common_prefix and len(partial_word) < len(common_prefix) and
                partial_word == common_prefix[:len(partial_word)]):
            filtered_candidates = [c for c in candidates if c.strip() != '']
            return filtered_candidates if filtered_candidates else None

        return None

    def _find_common_prefix(self, candidates):
        """Find the longest common prefix among all candidates."""
        if not candidates:
            return ''

        s_min = candidates[0]
        s_max = candidates[-1]

        for i, c in enumerate(s_min):
            if i >= len(s_max) or c != s_max[i]:
                return s_min[:i]

        return s_min

    def _build_help_buffer(self, possible_cmds, partial_word):
        """Build help buffer showing available commands and their descriptions."""
        buf = []
        num_full_matches = sum(1 for _, match_type, _ in possible_cmds if match_type == 'full')

        for cmd, match_type, syntax_token in possible_cmds:
            syntax, desc, _ = cmd

            # Add command description
            if match_type == 'full' and num_full_matches == 1:
                buf.append('  %-50s %s\n' % (syntax + ' <enter>', desc))
            else:
                buf.append('  %-50s %s\n' % (syntax, desc))

            # Add variable information if available
            if syntax_token:
                var_info = self._get_variable_info(syntax_token, partial_word)
                buf.extend(var_info)

        return buf

    def _get_variable_info(self, syntax_token, partial_word):
        """Get formatted information about variables for a syntax token."""
        attrs = self.get_var_attrs(syntax_token, partial_word)
        if not attrs:
            return []

        var_type, var_desc, var_candidates = attrs
        buf = ['    %s (%s): %s\n' % (syntax_token, var_type, var_desc)]

        # Add eligible variable candidates
        for var in var_candidates:
            if var.startswith(partial_word):
                buf.append('      %s\n' % var)

        return buf

    def _write_help_output(self, help_buffer, line):
        """Write help output to the terminal."""
        if help_buffer:
            self.fout.write('\n')
            self.fout.write(''.join(help_buffer))
            self.fout.write('%s%s' % (self.get_prompt(), line))
            self.fout.flush()

    def complete(self, partial_word, state):
        if state == 0:
            line = self.rl.get_line_buffer()

            # We currently support auto completion only at the EOL
            if len(line) != self.rl.get_endidx():
                return None

            # All exceptions happening here is ignored by the caller,
            # so we add our exception handler for debugging
            try:
                self.candidates = self._do_complete(line, partial_word)
            except BaseException as e:
                import traceback
                traceback.print_exc()
                sys.exit(1)

        try:
            return self.candidates[state]
        except IndexError:
            return None

    def complete_dummy(self, partial_word, state):
        return None

    def get_prompt(self):
        return '> '

    def find_cmd(self, line):
        # return commands being matched with every token
        matched, matched_low = self.list_matched(line, 'full')
        line_stripped = line.strip()

        if len(matched) == 1:
            return matched[0]

        elif len(matched) >= 2:
            self.err('Ambiguous command "%s". Candidates:' % line_stripped)
            for cmd, desc, _ in matched + matched_low:
                self.ferr.write('  %-50s%s\n' % (cmd, desc))

        elif len(matched) == 0:
            matched, matched_low = self.list_matched(line, 'partial')
            if len(matched) > 0:
                self.err('Incomplete command "%s". Candidates:' %
                         line_stripped)
                for cmd, desc, _ in matched + matched_low:
                    self.ferr.write('  %-50s%s\n' % (cmd, desc))
            else:
                self.err('Unknown command "%s".' % line_stripped)

        raise self.InvalidCommandError()

    def get_default_args(self):
        return []

    def bind_args(self, cmd, line):
        syntax, desc, func = cmd
        remainder = line
        args = []

        for i, syntax_token in enumerate(syntax.split()):
            if remainder.strip() == '':
                if syntax_token[0] == '[':
                    args.append(None)
                    continue

                raise self.InternalError('Partial match on "%s"? line: "%s"' %
                                         (syntax, line))

            attrs = self.get_var_attrs(syntax_token, remainder.split()[0])
            if attrs:
                var_type = attrs[0]
            else:
                var_type = 'keyword'

            val, remainder = self.bind_var(var_type, remainder)

            if var_type != 'keyword':
                args.append(val)

            remainder = remainder.lstrip()

        args = self.get_default_args() + args
        return func, args

    def call_func(self, func, args):
        func(*args)

    def print_banner(self):
        # The method is intentionally left empty
        # because not all subclasses require a banner.
        # Subclasses can override this method if needed.
        pass

    def process_one_line(self):
        """
        Process a single line of input from the CLI.

        Handles both interactive and non-interactive modes, with proper
        exception handling and command execution.
        """
        # Read input line
        line = self._read_input_line()
        if line is None:
            return

        line = line.strip()

        if line:
            self.last_cmd = line
            self._execute_command(line)

    def _read_input_line(self):
        """Read input line from appropriate source (interactive or file)."""
        if self.interactive:
            return self._read_interactive_input()
        else:
            return self._read_file_input()

    def _read_interactive_input(self):
        """Read input from interactive prompt with Python 2/3 compatibility."""
        try:
            try:
                prompt = raw_input  # Python 2
            except NameError:
                prompt = input      # Python 3
            return prompt(self.get_prompt())
        except KeyboardInterrupt:
            self.fout.write('\n')
            return None

    def _read_file_input(self):
        """Read input from file with EOF handling."""
        line = self.fin.readline()
        if len(line) == 0:
            raise EOFError()
        return line

    def _execute_command(self, line):
        """Execute a command line with proper exception handling."""
        try:
            # We use a nested try to ensure the 'stop_loop' logic runs
            # for EVERY type of exception before we decide to handle/ignore it.
            try:
                cmd = self.find_cmd(line + ' ')
                func, args = self.bind_args(cmd, line)
                self.call_func(func, args)
            except:
                if not self.interactive:
                    self.stop_loop = True
                raise # Re-raise to be caught by the outer block

        except self.HandledError:
            pass
        except self.InvalidCommandError:
            pass
        except self.BindError as e:
            self.err(e)
        except self.CommandError as e:
            self.err(e)

    def save_history(self):
        if self.interactive and self.rl and self.history_file:
            try:
                self.rl.write_history_file(self.history_file)
            except OSError:
                self.err('Cannot write to history file "%s"' %
                         self.history_file)
            except Exception as e:
                self.err('Unexpected error saving history file "%s": %s' %
                        (self.history_file, e))

    def disable_echoctl(self):
        try:
            # termios module might not be available. Ignore ImportError if so.
            import termios

            self.old_flags = termios.tcgetattr(sys.stdin)
            new_flags = self.old_flags
            new_flags[3] &= ~termios.ECHOCTL
            termios.tcsetattr(sys.stdin, termios.TCSADRAIN, new_flags)
        except ImportError:
            pass
        except Exception:
            pass

    def restore_echoctl(self):
        try:
            import termios

            if not hasattr(self, 'old_flags'):
                 return

            cur_flags = termios.tcgetattr(sys.stdin)
            new_flags = cur_flags
            if self.old_flags[3] & termios.ECHOCTL:
                new_flags[3] |= termios.ECHOCTL
            else:
                new_flags[3] &= ~termios.ECHOCTL
            termios.tcsetattr(sys.stdin, termios.TCSADRAIN, new_flags)
        except ImportError:
            pass
        except Exception:
            pass

    def go_interactive(self):
        try:
            import readline
            self.rl = readline
        except ImportError:
            self.err('"readline" not available. No auto completion.\n')
            return

        if 'libedit' in self.rl.__doc__:
            self.rl.parse_and_bind('bind -e')
            self.rl.parse_and_bind("bind '\t' rl_complete")
        else:
            self.rl.parse_and_bind('tab: complete')

        self.rl.set_completer(self.complete)

        # Remove `~!@#$%^&*()-=+[{]}\|;:'",<>?/ from readline delimiters
        # leaving only space, tab, LF
        self.rl.set_completer_delims(' \x09\x0a')

        try:
            if self.history_file and os.path.exists(self.history_file):
                self.rl.read_history_file(self.history_file)
        except OSError:
            self.err('Cannot read from history file "%s"' %
                     self.history_file)
        except Exception as e:
            self.err('Unexpected error reading history file "%s": %s' %
                    (self.history_file, e))

        self.print_banner()
        self.fout.flush()

    def loop(self):
        self.disable_echoctl()

        try:
            self.stop_loop = False

            # the main command loop
            while not self.stop_loop:
                self.process_one_line()
        except EOFError:
            if self.interactive:
                self.fout.write('\n')
        finally:
            self.save_history()
            self.restore_echoctl()
