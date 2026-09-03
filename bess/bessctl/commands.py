# Copyright (c) 2014-2017, The Regents of the University of California.
# Copyright (c) 2016-2017, Nefeli Networks, Inc.
# Copyright (c) 2017, Cloudigo.
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

import collections
import contextlib
import copy
import errno
import fcntl
import fnmatch
import inspect
import logging
import os
import os.path
import pprint
import re
import signal
import subprocess
import sys
import tempfile
import time
import traceback

import sugar

try:
    this_dir = os.path.dirname(os.path.realpath(__file__))
    sys.path.insert(1, os.path.join(this_dir, ".."))
    from pybess.module import *
    from pybess.port import *
except ImportError:
    print("Cannot import the API module (pybess)", file=sys.stderr)
    raise

logger = logging.getLogger(__name__)

# extention for configuration files.
CONF_EXT = "bess"

# constants for duplicate literals
DONE_MESSAGE = "Done.\n"
NONE_MESSAGE = "(none)\n"
FORMAT_16S_S = "%-16s %s\n"
COMMANDS_FORMAT = "\t\t commands: %s\n"
NO_COMMANDS_FORMAT = "\t\t (no commands)\n"


# errors in configuration file
class ConfError(Exception):
    pass


# useful for creating dummy file objects for use with `with`
@contextlib.contextmanager
def noop():
    yield None


@staticmethod
def _choose_arg(arg, kwargs):
    if kwargs:
        if arg:
            raise TypeError("You cannot specify both arg and keyword args")

        for key in kwargs:
            if isinstance(kwargs[key], (Module, Port)):
                kwargs[key] = kwargs[key].name

        return kwargs

    if isinstance(arg, (Module, Port)):
        return arg.name
    else:
        return arg


def __bess_env__(key, default=None):
    try:
        return os.environ[key]
    except KeyError:
        if default is None:
            raise ConfError(f"Environment variable {key} must be set.")

        print(
            f"Environment variable {key} is not set. Using default value {default}",
            file=sys.stderr,
        )
        return default


def __bess_module__(module_names, mclass_name, *args, **kwargs):
    def make_modules(names):
        result = []
        for module in names:
            if module in caller_globals:
                raise ConfError(f"Module name {module} already exists")
            if module in caller_locals:
                raise ConfError(f"Module name {module} shadowed by local variable")
        for module in names:
            obj = mclass_obj(*args, name=module, **kwargs)
            caller_globals[module] = obj
            result.append(obj)
        return result

    caller_frame = inspect.stack()[1][0]
    caller_globals = caller_frame.f_globals
    caller_locals = caller_frame.f_locals
    # If the locals *are* the globals, we are in the module of the
    # caller and should look only at the globals.  If not, we are
    # in a function defined in that module, and must check both.
    if caller_locals is caller_globals:
        caller_locals = {}

    if mclass_name not in caller_globals:
        raise ConfError(f"Module class {mclass_name} does not exist")
    mclass_obj = caller_globals[mclass_name]

    # a::SomeMod()
    if isinstance(module_names, str):
        return make_modules([module_names])[0]

    # a,b,c::SomeMod()
    if isinstance(module_names, tuple):
        return make_modules(module_names)

    assert False, f"Invalid argument {type(module_names)}"


def is_allowed_filename(basename):
    # do not allow whitespaces
    for c in basename:
        if c.isspace():
            return False

    return True


def _visible_candidate(name, partial_basename):
    """Check if the file should be shown based on dot-file rules and allowed list."""
    if name.startswith(".") and not partial_basename.startswith("."):
        return False
    # is_allowed_filename is assumed to be defined in the global/module scope
    return is_allowed_filename(name)


def _process_file_match(name, pattern, suffix, skip_suffix):
    """Check if file matches pattern and handle suffix stripping."""
    if not fnmatch.fnmatch(name, pattern):
        return None

    if suffix and not skip_suffix and name.endswith(suffix):
        return name[: -len(suffix)]
    return name


def complete_filename(partial_word, start_dir="", suffix="", skip_suffix=False):
    """Return filesystem completion candidates for partial_word under start_dir."""
    try:
        sub_dir, partial_basename = os.path.split(partial_word)
        target_dir = os.path.join(start_dir, os.path.expanduser(sub_dir)) or os.curdir

        basenames = os.listdir(target_dir) + [".", ".."]
        pattern = f"{partial_basename}*{suffix}"

        ret = []
        for name in basenames:
            if not _visible_candidate(name, partial_basename):
                continue

            full_path = os.path.join(target_dir, name)

            if os.path.isdir(full_path):
                # Handle directories
                ret.append(os.path.join(sub_dir, name + "/"))
            else:
                # Handle files
                processed_name = _process_file_match(name, pattern, suffix, skip_suffix)
                if processed_name is not None:
                    ret.append(os.path.join(sub_dir, processed_name))

        return ret

    except OSError:
        return []


def get_var_attrs(cli, var_token, partial_word):
    var_type = None
    var_desc = ""
    var_candidates = []

    try:
        if var_token == "ENABLE_DISABLE":
            var_type = "endis"
            var_candidates = ["enable", "disable"]

        elif var_token == "CORE":
            var_type = "int"

        elif var_token == "[SOCKET]":
            var_type = "socket"

        elif var_token == "WORKER_ID":
            var_type = "int"
            try:
                var_candidates = [
                    str(m.wid) for m in cli.bess.list_workers().workers_status
                ]
            except Exception:
                logger.debug("Failed to list workers for tab completion", exc_info=True)

        elif var_token == "WORKER_ID...":
            var_type = "wid+"
            var_desc = "one or more worker IDs"
            try:
                var_candidates = [
                    str(m.wid) for m in cli.bess.list_workers().workers_status
                ]
            except Exception:
                logger.debug("Failed to list workers for tab completion", exc_info=True)

        elif var_token == "DRIVER":
            var_type = "name"
            var_desc = "name of a port driver"
            try:
                var_candidates = cli.bess.list_drivers().driver_names
            except Exception:
                logger.debug("Failed to list drivers for tab completion", exc_info=True)

        elif var_token == "DRIVER...":
            var_type = "name+"
            var_desc = "one or more port driver names"
            try:
                var_candidates = cli.bess.list_drivers().driver_names
            except Exception:
                logger.debug("Failed to list drivers for tab completion", exc_info=True)

        elif var_token == "MCLASS":
            var_type = "name"
            var_desc = "name of a module class"
            try:
                var_candidates = cli.bess.list_mclasses().names
            except Exception:
                logger.debug(
                    "Failed to list mclasses for tab completion", exc_info=True
                )

        elif var_token == "MCLASS...":
            var_type = "name+"
            var_desc = "one or more module class names"
            try:
                var_candidates = cli.bess.list_mclasses().names
            except Exception:
                logger.debug(
                    "Failed to list mclasses for tab completion", exc_info=True
                )

        elif var_token == "[NEW_MODULE]":
            var_type = "name"
            var_desc = "specify a name of the new module instance"

        elif var_token == "MODULE":
            var_type = "name"
            var_desc = "name of an existing module instance"
            try:
                var_candidates = [m.name for m in cli.bess.list_modules().modules]
            except Exception:
                logger.debug("Failed to list modules for tab completion", exc_info=True)

        elif var_token == "[MODULE]":
            var_type = "name"
            var_desc = "name of an existing module instance (* means all)"
            var_candidates = ["*"]
            try:
                var_candidates += [m.name for m in cli.bess.list_modules().modules]
            except Exception:
                logger.debug("Failed to list modules for tab completion", exc_info=True)

        elif var_token == "MODULE...":
            var_type = "name+"
            var_desc = "one or more module names"
            try:
                var_candidates = [m.name for m in cli.bess.list_modules().modules]
            except Exception:
                logger.debug("Failed to list modules for tab completion", exc_info=True)

        elif var_token == "MODULE_CMD":
            var_type = "name"
            var_desc = 'module command to run (see "show mclass")'

        elif var_token == "ARG_TYPE":
            var_type = "name"
            var_desc = 'type of argument (see "show mclass")'

        elif var_token == "[NEW_PORT]":
            var_type = "name"
            var_desc = "specify a name of the new port"

        elif var_token == "[SCHEDULER]":
            var_type = "name"
            var_desc = "specify the type of scheduler (none for default)"
            var_candidates = ["", "experimental"]

        elif var_token == "PORT":
            var_type = "name"
            var_desc = "name of a port"
            try:
                var_candidates = [p.name for p in cli.bess.list_ports().ports]
            except Exception:
                logger.debug("Failed to list ports for tab completion", exc_info=True)

        elif var_token == "PORT...":
            var_type = "name+"
            var_desc = "one or more port names"
            try:
                var_candidates = [p.name for p in cli.bess.list_ports().ports]
            except Exception:
                logger.debug("Failed to list ports for tab completion", exc_info=True)

        elif var_token == "TC...":
            var_type = "name+"
            var_desc = "one or more traffic class names"
            try:
                var_candidates = [
                    getattr(c, "class").name for c in cli.bess.list_tcs().classes_status
                ]
            except Exception:
                logger.debug(
                    "Failed to list traffic classes for tab completion", exc_info=True
                )

        elif var_token == "CONF":
            var_type = "confname"
            var_desc = 'configuration name in "conf/" directory'
            var_candidates = complete_filename(
                partial_word, f"{cli.this_dir}/conf", "." + CONF_EXT
            )

        elif var_token == "CONF_FILE":
            var_type = "filename"
            var_desc = "configuration filename"
            var_candidates = complete_filename(partial_word)

        elif var_token == "PLUGIN_FILE":
            var_type = "filename"
            var_desc = "plugin filename (*.so)"
            var_candidates = complete_filename(
                partial_word, suffix=".so", skip_suffix=True
            )

        elif var_token in ("[DIRECTION]", "DIRECTION"):
            var_type = "dir"
            var_desc = 'gate direction discriminator (default "out")'
            var_candidates = ["in", "out"]

        elif var_token in ("[GATE]", "GATE"):
            var_type = "gate"
            var_desc = "gate index of a module"

        elif var_token == "[OGATE]":
            var_type = "gate"
            var_desc = "output gate of a module (default 0)"

        elif var_token == "[IGATE]":
            var_type = "gate"
            var_desc = "input gate of a module (default 0)"

        elif var_token == "GATEHOOKCLASS":
            var_type = "name"
            var_desc = "name of a gatehook class"
            try:
                var_candidates = cli.bess.list_gatehook_classes().names
            except Exception:
                logger.debug(
                    "Failed to list gatehook classes for tab completion", exc_info=True
                )

        elif var_token == "GATEHOOKCLASS...":
            var_type = "name+"
            var_desc = "one or more gatehook class names"
            try:
                var_candidates = cli.bess.list_gatehook_classes().names
            except Exception:
                logger.debug(
                    "Failed to list gatehook classes for tab completion", exc_info=True
                )

        elif var_token == "GATEHOOK":
            var_type = "name"
            var_desc = "name of an existing gatehook instance"

        elif var_token == "GATEHOOK_CMD":
            var_type = "name"
            var_desc = 'module command to run (see "show gatehookclass")'

        elif var_token == "[ENV_VARS...]":
            var_type = "map"
            var_desc = "Environmental variables for configuration"

        elif var_token == "[PORT_ARGS...]":
            var_type = "map"
            var_desc = "initial configuration for port"

        elif var_token == "[MODULE_ARGS...]":
            var_type = "pyobj"
            var_desc = "initial configuration for module"

        elif var_token == "[CMD_ARGS...]":
            var_type = "pyobj"
            var_desc = "arguments for module/gatehook command"

        elif var_token == "[TCPDUMP_OPTS...]":
            var_type = "opts"
            var_desc = 'tcpdump(1) command-line options (e.g., "-ne tcp port 22")'

        elif var_token == "[TSHARK_OPTS...]":
            var_type = "opts"
            var_desc = (
                "tshark(1) command-line options "
                '(default "-z proto,colinfo,frame.comment,frame.comment")'
            )

        elif var_token == "[GRAPHEASY_OPTS...]":
            var_type = "opts"
            var_desc = (
                "graph-easy(1p) command-line options "
                "(e.g. --as dot | dot -Tsvg -o graph.svg)"
            )

        elif var_token == "[BESSD_OPTS...]":
            var_type = "opts"
            var_desc = 'bess daemon command-line options (see "bessd -h")'

        elif var_token == "[GRPC_URL]":
            var_type = "filename"
            var_desc = "gRPC url"

        elif var_token == "[PAUSE_WORKERS]":
            var_type = "pause_workers"
            var_desc = 'determines whether to pause workers for the operation (default: "pause")'
            var_candidates = ["pause", "no_pause"]

        elif var_token == "[HOST]":
            var_type = "host"
            var_desc = 'HTTP server address to listen on (default: "localhost")'

        elif var_token == "[PORT_NUMBER]":
            var_type = "int"
            var_desc = "HTTP server address to listen on (default: 5000)"

    except OSError as e:
        if e.errno in [errno.ECONNRESET, errno.EPIPE]:
            cli.bess.disconnect()
        else:
            raise

    except (cli.bess.Error, cli.bess.APIError, cli.bess.RPCError):
        # ignore errors, this is just auto completion
        pass

    if var_type is None:
        return None
    else:
        return var_type, var_desc, var_candidates


# Return (head, tail)
#   head: consumed string portion
#   tail: the rest of input line
# You can assume that 'line == head + tail'
def split_var(cli, var_type, line):
    if var_type in [
        "host",
        "name",
        "gate",
        "confname",
        "filename",
        "endis",
        "int",
        "socket",
        "pause_workers",
        "dir",
    ]:
        pos = line.find(" ")
        if pos == -1:
            head = line
            tail = ""
        else:
            head = line[:pos]
            tail = line[pos:]

    elif var_type in ["wid+", "name+", "map", "pyobj", "opts"]:
        head = line
        tail = ""

    else:
        raise cli.InternalError('type "%s" is undefined', var_type)

    return head, tail


def _parse_map(**kwargs):
    return kwargs


# Return (mapped_value, tail)
#   mapped_value: Python value/object from the consumed token(s)
#   tail: the rest of input line
def bind_var(cli, var_type, line):
    head, remainder = split_var(cli, var_type, line)

    # default behavior
    val = head

    if var_type == "endis":
        if "enable".startswith(val):
            val = "enable"
        elif "disable".startswith(val):
            val = "disable"
        else:
            raise cli.BindError('"endis" must be either "enable" or "disable"')

    elif var_type == "dir":
        if "in".startswith(val):
            val = "in"
        elif "out".startswith(val):
            val = "out"
        else:
            raise cli.BindError('"dir" must be either "in" or "out"')

    elif var_type == "wid+":
        val = []
        for wid_str in head.split():
            if wid_str.isdigit():
                val.append(int(wid_str))
            else:
                raise cli.BindError('"wid" must be a positive number')
        val = sorted(set(val))

    elif var_type == "host":
        dns = re.match(r"^[a-zA-Z0-9][a-zA-Z0-9\-.]*$", val)
        ip = re.match(r"^[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}$", val)
        if dns is None and ip is None:
            raise cli.BindError('"host" must be a valid DNS name or IPv4 address')

    elif var_type == "name":
        if re.match(r"^[\S]*$", val) is None:
            raise cli.BindError('"name" must not contain whitespaces')

    elif var_type == "gate":
        if head.isdigit():
            val = int(head)
        else:
            raise cli.BindError('"gate" must be a positive number')

    elif var_type == "socket":
        if head.isdigit():
            val = int(head)
        else:
            raise cli.BindError('"socket" must be a positive number')

    elif var_type == "name+":
        val = sorted(set(head.split()))  # collect unique items

    elif var_type == "confname":
        if val.find("\0") >= 0:
            raise cli.BindError("Invalid configuration name")

    elif var_type == "filename":
        if val.find("\0") >= 0:
            raise cli.BindError("Invalid filename")

    elif var_type == "map":
        try:
            val = eval(f"_parse_map({head})")
        except Exception:  # noqa: BLE001 -- eval() of arbitrary user input can raise any exception type
            raise cli.BindError('"map" should be "key=val, key=val, ..."')

    elif var_type == "pyobj":
        try:
            if head.strip() == "":
                val = None
            else:
                val = eval(head)
        except Exception:  # noqa: BLE001 -- eval() of arbitrary user input can raise any exception type
            raise cli.BindError(
                '"pyobj" should be an object in python syntax'
                ' (e.g., 42, "foo", ["hello", "world"], {"bar": "baz"})'
            )

    elif var_type == "opts":
        val = val.split()

    elif var_type == "int":
        try:
            val = int(val)
        except (ValueError, TypeError):
            raise cli.BindError("Expected an integer")

    return val, remainder


cmdlist = []


def cmd(syntax, desc=""):
    def cmd_decorator(func):
        cmdlist.append((syntax, desc, func))

    return cmd_decorator


@cmd("help", "List available commands")
def help(cli):
    for syntax, desc, _ in cmdlist:
        cli.fout.write(f"  {syntax:<50}{desc}\n")
    cli.fout.flush()


@cmd("quit", "Quit CLI")
def help(cli):
    raise EOFError()


@cmd("history", "Show command history")
def history(cli):
    if cli.rl:
        len_history = cli.rl.get_current_history_length()
        begin_index = max(1, len_history - 100)  # max 100 items
        for i in range(begin_index, len_history):  # skip the last one
            cli.fout.write(f"{i:5d}  {cli.rl.get_history_item(i)}\n")
    else:
        cli.err('"readline" not available')


@cmd("debug ENABLE_DISABLE", "Enable/disable debug messages")
def debug(cli, flag):
    cli.bess.set_debug(flag == "enable")


@cmd("daemon connect [GRPC_URL]", "Connect to BESS daemon")
def daemon_connect(cli, grpc_url):
    kwargs = {}

    if grpc_url:
        kwargs["grpc_url"] = grpc_url

    cli.bess.connect(**kwargs)


@cmd("daemon disconnect", "Disconnect from BESS daemon")
def daemon_disconnect(cli):
    cli.bess.disconnect()


# return False iff canceled.
def warn(cli, msg, func, *args):
    if cli.interactive:
        if cli.rl:
            cli.rl.set_completer(cli.complete_dummy)

        try:
            try:
                prompt = raw_input  # Python 2
            except NameError:
                prompt = input  # Python 3

            resp = prompt(f'WARNING: {msg} Are you sure? (type "yes") ')

            if resp.strip() == "yes":
                func(cli, *args)
            else:
                cli.fout.write("Canceled.\n")
                return False

        except KeyboardInterrupt:
            cli.fout.write("Canceled.\n")
            return False
        finally:
            if cli.rl:
                cli.rl.set_completer(cli.complete)

                # don't leave response in the history
                hist_len = cli.rl.get_current_history_length()
                cli.rl.remove_history_item(hist_len - 1)

    else:
        func(cli, *args)

    return True


def _do_start(cli, opts):
    if opts is None:
        opts = []

    # need -E to pass GCOV_* env variables through
    cmd = "sudo -E {}/core/bessd -k {}".format(
        os.path.dirname(cli.this_dir),
        " ".join(opts),
    )

    cli.bess.disconnect()

    try:
        ret = os.system("sudo -n echo -n 2> /dev/null")
        if os.WEXITSTATUS(ret) != 0:
            cli.fout.write(
                "You need root privilege to launch BESS daemon, "
                'but "sudo" requires a password for this account.'
                "\n"
            )
        subprocess.check_call(cmd, shell=True)
    except subprocess.CalledProcessError:
        raise cli.CommandError("Cannot start BESS daemon")
    else:
        start = time.time()
        while time.time() - start < 3:
            try:
                cli.bess.connect()
                break
            except cli.bess.RPCError:
                # bessd is on, but its gRPC server may be not yet. Retry.
                time.sleep(0.1)
        else:
            raise cli.CommandError("Connection timed out")

    if cli.interactive:
        cli.fout.write(DONE_MESSAGE)


@cmd("daemon start [BESSD_OPTS...]", "Start BESS daemon in the local machine")
def daemon_start(cli, opts):
    daemon_exists = False

    try:
        with open("/var/run/bessd.pid", "r") as f:
            try:
                fcntl.flock(f.fileno(), fcntl.LOCK_SH | fcntl.LOCK_NB)
            except OSError as e:
                if e.errno in [errno.EAGAIN, errno.EACCES]:
                    daemon_exists = True
                else:
                    raise
    except OSError as e:
        if e.errno != errno.ENOENT:
            raise

    if daemon_exists:
        warn(cli, "Existing BESS daemon will be killed.", _do_start, opts)
    else:
        _do_start(cli, opts)


def is_pipeline_empty(cli):
    workers = cli.bess.list_workers().workers_status
    modules = cli.bess.list_modules().modules
    ports = cli.bess.list_ports().ports
    return len(workers) == 0 and len(modules) == 0 and len(ports) == 0


def _do_reset(cli):
    cli.bess.pause_all()
    cli.bess.reset_all()
    cli.bess.resume_all()
    if cli.interactive:
        cli.fout.write(DONE_MESSAGE)


@cmd("daemon reset", "Remove all ports and modules in the pipeline")
def daemon_reset(cli):
    if is_pipeline_empty(cli):
        _do_reset(cli)
    else:
        warn(cli, "The entire pipeline will be cleared.", _do_reset)


def _do_stop(cli):
    cli.bess.pause_all()
    cli.bess.kill()
    if cli.interactive:
        cli.fout.write(DONE_MESSAGE)


@cmd("daemon stop", "Stop BESS daemon")
def daemon_stop(cli):
    if is_pipeline_empty(cli):
        _do_stop(cli)
    else:
        warn(cli, "BESS daemon will be killed.", _do_stop)


def _clear_pipeline(cli):
    cli.bess.pause_all()
    cli.bess.reset_all()


def _collect_module_and_driver_names(cli):
    """Collect module class names and port driver names from BESS."""
    class_names = [str(i) for i in cli.bess.list_mclasses().names]
    driver_names = [str(i) for i in cli.bess.list_drivers().driver_names]
    return class_names, driver_names


def _find_duplicate_names(rsvd, class_names, driver_names):
    """Find duplicate names between reserved names, modules, and drivers."""
    counts = collections.Counter(rsvd.keys())
    counts.update(class_names)
    counts.update(driver_names)
    return [k for k in counts if counts[k] > 1]


def _generate_duplicate_error_message(dups, rsvd, class_names, driver_names):
    """Generate detailed error message for duplicate names."""
    errors = []
    for name in dups:
        if name in rsvd:
            why = f"reserved name {name} is used as "
        else:
            why = f"name {name} is used as "

        if name in class_names:
            if name in driver_names:
                why += "both a module class and a port driver"
            else:
                why += "a module class"
        else:
            why += "a port driver"
        errors.append(why)

    return "duplicate names found: {}".format("; ".join(errors))


def _create_module_creators(cli, class_names):
    """Create module class creators."""
    creators = {}
    for name in class_names:
        creators[name] = type(
            str(name), (Module,), {"bess": cli.bess, "choose_arg": _choose_arg}
        )
    return creators


def _create_port_creators(cli, driver_names):
    """Create port driver creators."""
    creators = {}
    for name in driver_names:
        creators[name] = type(
            str(name), (Port,), {"bess": cli.bess, "choose_arg": _choose_arg}
        )
    return creators


def _get_bess_module_and_port_creators(cli, rsvd):
    """
    Return module instance creators and port instance creators.

    A creator is, in effect, a class as if defined by:
        class Foo(Module):
            bess = bess
            choose_arg = _choose_arg
    (and similarly for a port creator but with Port as the base class).
    The choose_arg function is internal, meant for use in the __init__
    functions in the base classes; see class Module and class Port,
    defined elsewhere.

    The rsvd argument is a dictionary of reserved names (see below).
    """
    # TODO(torek) cache these for performance, rebuild when needed

    # Collect names from BESS
    class_names, driver_names = _collect_module_and_driver_names(cli)

    # Check for duplicates
    dups = _find_duplicate_names(rsvd, class_names, driver_names)
    if dups:
        error_msg = _generate_duplicate_error_message(
            dups, rsvd, class_names, driver_names
        )
        raise cli.InternalError(error_msg)

    # Create creators
    creators = {}
    creators.update(_create_module_creators(cli, class_names))
    creators.update(_create_port_creators(cli, driver_names))

    return creators


# NOTE: the name of this function is used below
def _do_run_file(cli, conf_file):
    try:
        xformed = sugar.xform_file(conf_file)
    except OSError:
        cli.err(f"Cannot open file {conf_file}")
        raise cli.HandledError()

    new_globals = {
        "__builtins__": __builtins__,
        "__file__": conf_file,
        "bess": cli.bess,
        "ConfError": ConfError,
        "__bess_env__": __bess_env__,
        "__bess_module__": __bess_module__,
        "__bess_creators__": None,  # will be replaced below
    }

    creators = _get_bess_module_and_port_creators(cli, new_globals)

    # Creator names are used globally in scripts, so export them
    # globally.  We keep them in __bess_creators__ for use in the
    # test code as well, which wants to create its own new set of
    # globals.
    new_globals["__bess_creators__"] = creators
    for name in creators:
        new_globals[name] = creators[name]

    try:
        code = compile(xformed, conf_file, "exec")
    except SyntaxError as e:
        # TODO: e.offset might be wrong if there's a correct syntactic
        #       sugar in an erroneous line

        # Mimic python's error reporting style
        cli.err(
            '\n  File "{}", line {}\n    {}\n    {}\nSyntaxError: {}'.format(
                conf_file, e.lineno, e.text, " " * (e.offset - 1) + "^", e.msg
            )
        )
        raise cli.HandledError()
    except Exception as e:  # noqa: BLE001 -- compiling an arbitrary user config script can raise any exception type
        cli.err(f"Failed to compile BESS config file ({conf_file}): {e}")
        raise cli.HandledError()

    if is_pipeline_empty(cli):
        cli.bess.pause_all()
    else:
        ret = warn(cli, "The current pipeline will be reset.", _clear_pipeline)
        if ret is False:
            return

    try:
        exec(code, new_globals)  # noqa: S102 -- required to run user *.bess config scripts
        if cli.interactive:
            cli.fout.write(DONE_MESSAGE)
    except Exception:
        cur_frame = inspect.currentframe()
        cur_func = inspect.getframeinfo(cur_frame).function
        t, v, tb = sys.exc_info()
        stack = traceback.extract_tb(tb)

        while len(stack) > 0 and stack.pop(0)[2] != cur_func:
            pass

        errmsg = "Unhandled exception in the configuration script"

        cli.err(f"{errmsg} (most recent call last)")
        cli.ferr.write("".join(traceback.format_list(stack)))

        if isinstance(v, (cli.bess.Error, cli.bess.RPCError)):
            raise
        else:
            cli.ferr.write("".join(traceback.format_exception_only(t, v)))
            raise cli.HandledError()
    finally:
        if cli.bess.is_connected():
            cli.bess.resume_all()


def _run_file(cli, conf_file, env_map):
    if env_map:
        try:
            original_env = os.environ.copy()

            for k, v in env_map.items():
                os.environ[k] = str(v)

            _do_run_file(cli, conf_file)
        finally:
            os.environ.clear()
            for k, v in original_env.items():
                os.environ[k] = v
    else:
        _do_run_file(cli, conf_file)


@cmd("run CONF [ENV_VARS...]", 'Run a *.bess configuration in "conf/"')
def run_conf(cli, conf, env_map):
    target_dir = f"{cli.this_dir}/conf"
    basename = os.path.expanduser(f"{conf}.{CONF_EXT}")
    conf_file = os.path.join(target_dir, basename)
    _run_file(cli, conf_file, env_map)


@cmd("run file CONF_FILE [ENV_VARS...]", "Run a configuration file")
def run_file(cli, conf_file, env_map):
    _run_file(cli, os.path.expanduser(conf_file), env_map)


@cmd("add worker WORKER_ID CORE [SCHEDULER]", "Create a worker")
def add_worker(cli, wid, core, scheduler):
    cli.bess.add_worker(wid, core, scheduler or "")


@cmd("add port DRIVER [NEW_PORT] [PORT_ARGS...]", "Add a new port")
def add_port(cli, driver, port, args):
    ret = cli.bess.create_port(driver, port, args)

    if port is None:
        cli.fout.write(f'  The new port "{ret.name}" has been created\n')


@cmd(
    "add module MCLASS [NEW_MODULE] [PAUSE_WORKERS] [MODULE_ARGS...]",
    "Add a new module",
)
def add_module(cli, mclass, module, pause_workers, args):
    if pause_workers != "no_pause":
        cli.bess.pause_all()
    try:
        ret = cli.bess.create_module(mclass, module, args)
    finally:
        if pause_workers != "no_pause":
            cli.bess.resume_all()

    if module is None:
        cli.fout.write(f'  The new module "{ret.name}" has been created\n')


@cmd(
    "add connection MODULE MODULE [OGATE] [IGATE] [PAUSE_WORKERS]",
    "Add a connection between two modules",
)
def add_connection(cli, m1, m2, ogate, igate, pause_workers="pause"):
    if ogate is None:
        ogate = 0

    if igate is None:
        igate = 0

    if pause_workers != "no_pause":
        cli.bess.pause_all()
    try:
        cli.bess.connect_modules(m1, m2, ogate, igate)
    finally:
        if pause_workers != "no_pause":
            cli.bess.resume_all()


@cmd(
    "command module MODULE MODULE_CMD ARG_TYPE [CMD_ARGS...]",
    "Send a command to a module",
)
def command_module(cli, module, cmd, arg_type, args):
    if args is None:
        args = {}

    cli.bess.pause_all()
    try:
        ret = cli.bess.run_module_command(module, cmd, arg_type, args)
        cli.fout.write(f"response: {ret!r}\n")
    finally:
        cli.bess.resume_all()


@cmd(
    "command gatehook GATEHOOK MODULE DIRECTION GATE GATEHOOK_CMD ARG_TYPE [CMD_ARGS...]",
    "Send a command to a gatehook",
)
def command_gatehook(cli, name, module, direction, gate, cmd, arg_type, args):
    if args is None:
        args = {}

    cli.bess.pause_all()
    try:
        ret = cli.bess.run_gatehook_command(
            name, module, direction, gate, cmd, arg_type, args
        )
        cli.fout.write(f"response: {ret!r}\n")
    finally:
        cli.bess.resume_all()


# Please do not rely on this API. This API may be replaced with `command port PORT`
# in the same way with gatehook and module
@cmd("configure port PORT [PORT_ARGS...]", "[Experimental] Update a port configuration")
def conigure_port(cli, name, args):
    cli.bess.pause_all()
    try:
        cli.bess.set_port_config(name, args or {})
    finally:
        cli.bess.resume_all()


@cmd("delete worker WORKER_ID...", "Delete a worker")
def delete_worker(cli, wids):
    wids = sorted(set(wids))
    for wid in wids:
        cli.bess.destroy_worker(wid)


@cmd("delete port PORT", "Delete a port")
def delete_port(cli, port):
    cli.bess.destroy_port(port)


@cmd("delete module MODULE", "Delete a module")
def delete_module(cli, module):
    cli.bess.pause_all()
    try:
        cli.bess.destroy_module(module)
    finally:
        cli.bess.resume_all()


@cmd(
    "delete connection MODULE ogate [OGATE]", "Delete a connection between two modules"
)
def delete_connection(cli, module, ogate):
    if ogate is None:
        ogate = 0

    cli.bess.pause_all()
    try:
        cli.bess.disconnect_modules(module, ogate)
    finally:
        cli.bess.resume_all()


def _show_worker_header(cli):
    cli.fout.write(
        "  {:10}{:10}{:10}{:10}{:16}\n".format(
            "Worker ID", "Status", "CPU core", "# of TCs", "Deadend pkts"
        )
    )


def _show_worker(cli, w):
    cli.fout.write(
        "  {:10d}{:10}{:10d}{:10d}{:16d}\n".format(
            w.wid,
            "RUNNING" if w.running else "PAUSED",
            w.core,
            w.num_tcs,
            w.silent_drops,
        )
    )


@cmd("show worker", "Show the status of all worker threads")
def show_worker_all(cli):
    workers = cli.bess.list_workers().workers_status

    if len(workers) == 0:
        raise cli.CommandError("There is no active worker thread to show.")

    _show_worker_header(cli)
    for worker in workers:
        _show_worker(cli, worker)


@cmd("show worker WORKER_ID...", "Show the status of specified worker threads")
def show_worker_list(cli, worker_ids):
    workers = cli.bess.list_workers().workers_status

    for wid in worker_ids:
        for worker in workers:
            if worker.wid == wid:
                break
        else:
            raise cli.CommandError(f"Worker ID {wid} does not exist")

    _show_worker_header(cli)
    for worker in workers:
        if worker.wid in worker_ids:
            _show_worker(cli, worker)


def _limit_to_str(limit):
    if "count" in limit:
        return f"{limit['count']} times/s"
    elif "cycle" in limit:
        return f"{limit['cycle'] / 1e6:.3f} MHz"
    elif "packet" in limit:
        if limit["packet"] < 1e3:
            return f"{limit['packet']:.0f} pps"
        elif limit["packet"] < 1e6:
            return f"{limit['packet'] / 1e3:.3f} kpps"
        else:
            return f"{limit['packet'] / 1e6:.3f} Mpps"
    elif "bit" in limit:
        if limit["bit"] < 1e3:
            return f"{limit['bit']:.0f} bps"
        elif limit["bit"] < 1e6:
            return f"{limit['bit'] / 1e3:.3f} kbps"
        else:
            return f"{limit['bit'] / 1e6:.3f} Mbps"
    else:
        return "unlimited"


def _burst_to_str(burst):
    # no output if max_burst is not set
    if len(burst) == 0 or next(iter(burst.values())) == 0:
        return ""

    if "count" in burst:
        return f"burst: {burst['count']} times"
    elif "cycle" in burst:
        return f"burst: {burst['cycle']} cycles"
    elif "packet" in burst:
        if burst["packet"] < 1e3:
            return f"burst: {burst['packet']:.0f} pkts"
        elif burst["packet"] < 1e6:
            return f"burst: {burst['packet'] / 1e3:.3f} kpkts"
        else:
            return f"burst: {burst['packet'] / 1e6:.3f} Mpkts"
    elif "bit" in burst:
        if burst["bit"] < 1e3:
            return f"burst: {burst['bit']:.0f} bits"
        elif burst["bit"] < 1e6:
            return f"burst: {burst['bit'] / 1e3:.3f} kbits"
        else:
            return f"burst: {burst['bit'] / 1e6:.3f} Mbits"
    else:
        return ""


def _show_tcs_node(cli, node, indent, prefix, lastsibling):
    line = prefix
    if indent > 0:
        line += "+-- "
    line += str(node["name"])
    line = line.ljust(30)

    if "show_list" in node:
        attrs = node["show_list"]
        for v in attrs:
            line += f" {v:<19}"

    cli.fout.write(line.rstrip(" ") + "\n")

    recursions = []
    if indent > 0:
        childprefix = prefix + ("    " if lastsibling else "|   ")
    else:
        childprefix = prefix + ("  " if lastsibling else "| ")
    children = node["children"]
    for i in range(len(children)):
        recursions.append(
            (
                children[i],  # node
                indent + 1,  # indent
                childprefix,  # prefix
                i >= len(children) - 1,  # lastsibling
            )
        )
    return recursions


def _show_tcs_tree(cli, root):
    stack = []
    stack.append((root, 0, "", True))

    while stack:
        args = stack.pop()
        ret = _show_tcs_node(cli, *args)
        stack.extend(reversed(ret))


def _create_tc_node(tc):
    """Create a node for a traffic class."""
    c_ = getattr(tc, "class")
    node = {"children": [], "name": c_.name, "policy": c_.policy, "show_list": []}
    return node, c_


def _build_parent_child_relationships(tcs, nodes, root):
    """Build parent-child relationships between traffic classes."""
    for tc in tcs:
        c_ = getattr(tc, "class")

        if tc.parent and tc.parent in nodes:
            nodes[tc.parent]["children"].append(nodes[c_.name])
        else:
            root["children"].append(nodes[c_.name])


def _populate_show_list(tc, nodes):
    """Populate the show list for a traffic class based on its policy."""
    c_ = getattr(tc, "class")

    # Always add the policy
    nodes[c_.name]["show_list"].append(c_.policy)

    # Handle parent-specific attributes
    if tc.parent and tc.parent in nodes:
        parent_policy = nodes[tc.parent]["policy"]
        if parent_policy == "weighted_fair" and c_.HasField("share"):
            nodes[c_.name]["show_list"].append(f"share: {c_.share}")
        elif parent_policy == "priority" and c_.HasField("priority"):
            nodes[c_.name]["show_list"].append(f"priority: {c_.priority}")

    # Handle rate limiting policy
    if c_.policy == "rate_limit":
        nodes[c_.name]["show_list"].append(_limit_to_str(c_.limit))
        nodes[c_.name]["show_list"].append(_burst_to_str(c_.max_burst))


def _build_tcs_tree(tcs):
    """Build a tree structure from traffic classes."""
    nodes = {}
    root = {"children": []}

    # Create all nodes
    for tc in tcs:
        node, c_ = _create_tc_node(tc)
        nodes[c_.name] = node

    # Build parent-child relationships
    _build_parent_child_relationships(tcs, nodes, root)

    # Populate show lists
    for tc in tcs:
        _populate_show_list(tc, nodes)

    return root


@cmd("check constraints", "Check constraints")
def check_constraints(cli):
    try:
        cli.bess.check_constraints()
    except (
        cli.bess.Error,
        cli.bess.RPCError,
        cli.bess.APIError,
        cli.bess.ConstraintError,
    ) as e:
        cli.fout.write(f"Constraint check failed {e!r}\n")


def _show_tc_list(cli, tcs):
    wids = sorted({getattr(tc, "class").wid for tc in tcs})

    for wid in wids:
        matched = [tc for tc in tcs if getattr(tc, "class").wid == wid]

        root = _build_tcs_tree(matched)
        if wid == -1:
            root["name"] = "<unattached>"
        else:
            root["name"] = f"<worker {wid}>"

        _show_tcs_tree(cli, root)


@cmd("show tc", "Show the list of traffic classes")
def show_tc_all(cli):
    classes = cli.bess.list_tcs().classes_status

    if len(classes) == 0:
        raise cli.CommandError("There is no traffic class to show.")
    else:
        _show_tc_list(cli, classes)


@cmd("show tc worker WORKER_ID...", "Show the list of traffic classes")
def show_tc_workers(cli, wids):
    wids = sorted(set(wids))
    for wid in wids:
        _show_tc_list(cli, cli.bess.list_tcs(wid).classes_status)


@cmd("show status", "Show the overall status")
def show_status(cli):
    workers = sorted(cli.bess.list_workers().workers_status, key=lambda x: x.wid)
    drivers = sorted(cli.bess.list_drivers().driver_names)
    plugins = sorted(cli.bess.list_plugins().paths)
    mclasses = sorted(cli.bess.list_mclasses().names)
    modules = sorted(cli.bess.list_modules().modules, key=lambda x: x.name)
    ports = sorted(cli.bess.list_ports().ports, key=lambda x: x.name)

    cli.fout.write("  Active worker threads: ")
    if workers:
        worker_list = [f"worker{worker.wid} (cpu {worker.core})" for worker in workers]
        cli.fout.write("{}\n".format(", ".join(worker_list)))
    else:
        cli.fout.write(NONE_MESSAGE)

    cli.fout.write("  Available drivers: ")
    if drivers:
        cli.fout.write("{}\n".format(", ".join(drivers)))
    else:
        cli.fout.write(NONE_MESSAGE)

    cli.fout.write("  Available plugins: ")
    if plugins:
        cli.fout.write("{}\n".format(", ".join(plugins)))
    else:
        cli.fout.write(NONE_MESSAGE)

    cli.fout.write("  Available module classes: ")
    if mclasses:
        cli.fout.write("{}\n".format(", ".join(mclasses)))
    else:
        cli.fout.write(NONE_MESSAGE)

    cli.fout.write("  Active ports: ")
    if ports:
        port_list = [f"{p.name}/{p.driver}" for p in ports]
        cli.fout.write("{}\n".format(", ".join(port_list)))
    else:
        cli.fout.write(NONE_MESSAGE)

    cli.fout.write("  Active modules: ")
    if modules:
        module_list = []
        for m in modules:
            module_list.append(f"{m.name}::{m.mclass}({m.desc})")

        cli.fout.write("{}\n".format(", ".join(module_list)))
    else:
        cli.fout.write(NONE_MESSAGE)


# last_stats: a map of (node name, gateid) -> (timestamp, counter value)
def _draw_pipeline(cli, field, units, last_stats=None, graph_args=None):
    if graph_args is None:
        graph_args = []

    modules = sorted(cli.bess.list_modules().modules, key=lambda x: x.name)
    names = []
    node_labels = {}

    for m in modules:
        name = m.name
        mclass = m.mclass
        names.append(name)
        node_labels[name] = f"{name}\\n{mclass}"
        node_labels[name] += f"\\n{m.desc}"

    # graph_args may chain further commands with a literal "|" token
    # (e.g., "--as dot | dot -Tsvg -o graph.svg"). Build each stage as its
    # own argv list and connect them without invoking a shell, to avoid
    # command injection via shell metacharacters in user-supplied input.
    commands = [["graph-easy"]]
    for arg in graph_args:
        if arg == "|":
            if not commands[-1]:
                raise cli.CommandError(
                    'Invalid pipeline syntax: empty command before "|"'
                )
            commands.append([])
        else:
            commands[-1].append(arg)
    if not commands[-1]:
        raise cli.CommandError('Invalid pipeline syntax: empty command after "|"')

    try:
        procs = []
        next_stdin = subprocess.PIPE
        last_idx = len(commands) - 1

        try:
            for i, command in enumerate(commands):
                proc = subprocess.Popen(
                    command,
                    shell=False,
                    stdin=next_stdin,
                    stdout=subprocess.PIPE,
                    # Only the last stage's stderr is drained (via communicate()
                    # below); piping stderr for earlier stages without reading it
                    # could fill the pipe buffer and hang the whole pipeline.
                    stderr=(subprocess.PIPE if i == last_idx else None),
                    universal_newlines=True,
                )
                if procs:
                    # Let the upstream process get SIGPIPE if this one exits early.
                    procs[-1].stdout.close()
                procs.append(proc)
                next_stdin = proc.stdout
        except OSError:
            # A later stage failed to start; reap the ones already spawned.
            for proc in procs:
                try:
                    proc.kill()
                except ProcessLookupError:
                    pass
                proc.wait()
            raise

        f = procs[0]
        last_proc = procs[-1]

        for m in modules:
            print(f"[{node_labels[m.name]}]", file=f.stdin)

        for name in names:
            gates = cli.bess.get_module_info(name).ogates

            for gate in gates:
                if gate.timestamp == 0.0:  # stats disabled?
                    label = "?"
                else:
                    if last_stats is None:  # show pipeline
                        val = getattr(gate, field)
                    else:  # monitor pipeline
                        last_time, last_val = last_stats[(name, gate.ogate)]
                        new_time, new_val = gate.timestamp, getattr(gate, field)
                        last_stats[(name, gate.ogate)] = (new_time, new_val)

                        val = (new_val - last_val) / (new_time - last_time)

                    if field == "bytes":
                        label = f"{val * 8 / 1e6:.1f}"
                    else:
                        label = f"{val:.0f}"

                edge_attr = f"{{label::{gate.ogate}  {label} {units} {gate.igate}:;}}"

                print(
                    f"[{node_labels[name]}] ->{edge_attr} [{node_labels[gate.name]}]",
                    file=f.stdin,
                )
        f.stdin.close()
        output, _ = last_proc.communicate()
        for proc in procs:
            proc.wait()
        return output

    except FileNotFoundError as e:
        if e.filename == "graph-easy":
            raise cli.CommandError(
                '"graph-easy" program is not available? '
                'Check if the package "libgraph-easy-perl" '
                "is installed."
            ) from e
        raise cli.CommandError(f'"{e.filename}" program is not available.') from e
    except BrokenPipeError:
        # A downstream stage exited early; reap the still-running processes
        # so we don't leave them running (or as zombies).
        for proc in procs:
            try:
                proc.kill()
            except ProcessLookupError:
                pass
        for proc in procs:
            proc.wait()
        raise cli.CommandError(
            "One of the piped commands in the pipeline exited early (broken pipe)."
        )


@cmd("show pipeline [GRAPHEASY_OPTS...]", "Show the current datapath pipeline")
def show_pipeline(cli, opts):
    cli.fout.write(_draw_pipeline(cli, "pkts", "", graph_args=opts))


@cmd(
    "show pipeline batch [GRAPHEASY_OPTS...]",
    "Show the current datapath pipeline with batch counters",
)
def show_pipeline_batch(cli, opts):
    cli.fout.write(_draw_pipeline(cli, "cnt", "", graph_args=opts))


@cmd(
    "show pipeline bit [GRAPHEASY_OPTS...]",
    "Show the current datapath pipeline with Megabit counters",
)
def show_pipeline_bit(cli, opts):
    cli.fout.write(_draw_pipeline(cli, "bytes", "Mb", graph_args=opts))


def _show_port(cli, port):
    link_status = cli.bess.get_link_status(port.name)

    if link_status.speed == 0:
        speed = "UNKNOWN"
    else:
        speed = f"{link_status.speed:,}Mbps"

    if link_status.link_up:
        link = "UP"
    else:
        link = "DOWN"

    if link_status.full_duplex:
        duplex = "FULL"
    else:
        duplex = "HALF"

    if link_status.autoneg:
        autoneg = "ON"
    else:
        autoneg = "OFF"

    port_config = cli.bess.get_port_config(port.name)

    cli.fout.write(
        f"  {port.name:<12} Driver {port.driver:<10} HWaddr {port.mac_addr:<18} MTU {port_config.conf.mtu:<6}\n"
    )
    cli.fout.write(
        "  {:<12} Speed {:<11} Link {:<5} Duplex {:<5} Autoneg {:<5}\n".format(
            "", speed, link, duplex, autoneg
        )
    )
    stats = cli.bess.get_port_stats(port.name)

    cli.fout.write("       Inc/RX  ")
    cli.fout.write(f"packets: {stats.inc.packets:<20,}")
    cli.fout.write(f"bytes: {stats.inc.bytes:<20,}\n")
    cli.fout.write("{:<14} dropped: {:<20,}\n".format("", stats.inc.dropped))

    cli.fout.write("       Out/TX  ")
    cli.fout.write(f"packets: {stats.out.packets:<20,}")
    cli.fout.write(f"bytes: {stats.out.bytes:<20,}\n")
    cli.fout.write("{:<14} dropped: {:<20,}\n".format("", stats.out.dropped))


@cmd("show port", "Show the status of all ports")
def show_port_all(cli):
    ports = cli.bess.list_ports().ports

    if len(ports) == 0:
        raise cli.CommandError("There is no active port to show.")
    else:
        for i, port in enumerate(ports):
            if i > 0:
                cli.fout.write("\n")  # add a separator line between ports
            _show_port(cli, port)


@cmd("show port PORT...", "Show the status of spcified ports")
def show_port_list(cli, port_names):
    ports = cli.bess.list_ports().ports

    port_names = list(set(port_names))
    for port_name in port_names:
        for port in ports:
            if port_name == port.name:
                _show_port(cli, port)
                break
        else:
            raise cli.CommandError(f'Port "{port_name}" does not exist')


def _show_module(cli, module_name):
    info = cli.bess.get_module_info(module_name)

    cli.fout.write(f"  {info.name}::{info.mclass}({info.desc})\n")

    if len(info.metadata) > 0:
        cli.fout.write("    Per-packet metadata fields:\n")
        for field in info.metadata:
            cli.fout.write(
                "{:16} {:<6}{:2} bytes ".format(
                    field.name + ":", field.mode, field.size
                )
            )

            if field.offset >= 0:
                cli.fout.write(f"at offset {field.offset}\n")
            elif field.offset == -1:
                cli.fout.write("(no downstream reader)\n")
            elif field.offset == -2:
                cli.fout.write("(no upstream writer)\n")
            else:
                cli.fout.write("\n")

    if len(info.igates) > 0:
        cli.fout.write("    Input gates:\n")
        for gate in info.igates:
            track_str = "batches N/A packets N/A"
            try:
                track_str = f"batches {gate.cnt:<11d} packets {gate.pkts:<12d}"
            except Exception:
                logger.debug("Failed to format igate stats", exc_info=True)
            cli.fout.write(
                "      {:3d}: {} {}\t{}\n".format(
                    gate.igate,
                    track_str,
                    ", ".join(f"{g.name}:{g.ogate} ->" for g in gate.ogates),
                    ", ".join(f"{h.class_name}::{h.hook_name}" for h in gate.gatehooks),
                )
            )

    if len(info.ogates) > 0:
        cli.fout.write("    Output gates:\n")
        for gate in info.ogates:
            track_str = "batches N/A packets N/A"
            try:
                track_str = f"batches {gate.cnt:<11d} packets {gate.pkts:<12d}"
            except Exception:
                logger.debug("Failed to format ogate stats", exc_info=True)
            cli.fout.write(
                "      {:3d}: {} -> {}:{}\t{}\n".format(
                    gate.ogate,
                    track_str,
                    gate.igate,
                    gate.name,
                    ", ".join(f"{h.class_name}::{h.hook_name}" for h in gate.gatehooks),
                )
            )
    cli.fout.write(f"    Deadends: {info.deadends:<12d}\n")

    if hasattr(info, "dump"):
        dump_str = pprint.pformat(info.dump, width=74)
        dump_str = "\n      ".join(dump_str.split("\n"))
        cli.fout.write("    Dump:\n")
        cli.fout.write(f"      {dump_str}\n")


@cmd("show module", "Show the status of all modules")
def show_module_all(cli):
    modules = cli.bess.list_modules().modules

    if not modules:
        raise cli.CommandError("There is no active module to show.")

    for module in modules:
        _show_module(cli, module.name)


@cmd("show module MODULE...", "Show the status of specified modules")
def show_module_list(cli, module_names):
    for module_name in module_names:
        _show_module(cli, module_name)


def _show_mclass(cli, cls_name, detail):
    info = cli.bess.get_mclass_info(cls_name)
    cli.fout.write(FORMAT_16S_S % (info.name, info.help))

    if detail:
        if len(info.cmds) > 0:
            cli.fout.write(
                COMMANDS_FORMAT
                % (
                    ", ".join(
                        map(
                            lambda cmd, msg: f"{cmd}({msg})",
                            info.cmds,
                            info.cmd_args,
                        )
                    )
                )
            )
        else:
            cli.fout.write(NO_COMMANDS_FORMAT)


@cmd("show mclass", "Show all module classes")
def show_mclass_all(cli):
    mclasses = cli.bess.list_mclasses().names
    for cls_name in mclasses:
        _show_mclass(cli, cls_name, False)


@cmd("show mclass MCLASS...", "Show the details of specified module classes")
def show_mclass_list(cli, cls_names):
    for cls_name in cls_names:
        _show_mclass(cli, cls_name, True)


@cmd("show gatehook", "Show the status of all gatehook")
def show_gatehook_all(cli):
    gatehooks = cli.bess.list_gatehooks().hooks

    if not gatehooks:
        raise cli.CommandError("There is no active gatehook to show.")
    for gatehook in gatehooks:
        if gatehook.HasField("igate"):
            hook_name = f"{gatehook.class_name}::{gatehook.hook_name}"
            cli.fout.write(f"{hook_name:<16} {gatehook.igate}:{gatehook.module_name}\n")
        else:
            hook_name = f"{gatehook.class_name}::{gatehook.hook_name}"
            cli.fout.write(f"{hook_name:<16} {gatehook.module_name}:{gatehook.ogate}\n")


def _show_gatehook_class(cli, cls_name, detail):
    info = cli.bess.get_gatehook_class_info(cls_name)
    cli.fout.write(FORMAT_16S_S % (info.name, info.help))

    if detail:
        if len(info.cmds) > 0:
            cli.fout.write(
                COMMANDS_FORMAT
                % (
                    ", ".join(
                        map(
                            lambda cmd, msg: f"{cmd}({msg})",
                            info.cmds,
                            info.cmd_args,
                        )
                    )
                )
            )
        else:
            cli.fout.write(NO_COMMANDS_FORMAT)


@cmd("show gatehookclass", "Show all gatehook classes")
def show_gatahook_class_all(cli):
    gatehook_classes = cli.bess.list_gatehook_classes().names
    for cls_name in gatehook_classes:
        _show_gatehook_class(cli, cls_name, False)


@cmd(
    "show gatehookclass GATEHOOKCLASS...", "Show the details of specified gate classes"
)
def show_gatehook_class_list(cli, cls_names):
    for cls_name in cls_names:
        _show_gatehook_class(cli, cls_name, True)


@cmd("import plugin PLUGIN_FILE", "Import the specified plugin (*.so)")
def import_plugin(cli, plugin):
    cli.bess.pause_all()
    try:
        cli.bess.import_plugin(plugin)
    finally:
        cli.bess.resume_all()


@cmd("unload plugin PLUGIN_FILE", "Unload the specified plugin (*.so)")
def unload_plugin(cli, plugin):
    # FIXME check whether the plugin is being used
    # currently this command can crash the BESS daemon
    cli.bess.pause_all()
    try:
        cli.bess.unload_plugin(plugin)
    finally:
        cli.bess.resume_all()


@cmd("show plugin", "Show all imported plugins")
def show_plugin_all(cli):
    plugins = cli.bess.list_plugins().paths
    for plugin_name in plugins:
        cli.fout.write(f"{plugin_name:<16}\n")


def _show_driver(cli, drv_name, detail):
    info = cli.bess.get_driver_info(drv_name)
    cli.fout.write(FORMAT_16S_S % (info.name, info.help))

    if detail:
        if info.commands:
            cli.fout.write(COMMANDS_FORMAT % (", ".join(info.commands)))
        else:
            cli.fout.write(NO_COMMANDS_FORMAT)


@cmd("show driver", "Show all port drivers")
def show_driver_all(cli):
    drivers = cli.bess.list_drivers().driver_names

    for drv_name in drivers:
        _show_driver(cli, drv_name, False)


@cmd("show driver DRIVER...", "Show the details of specified drivers")
def show_driver_list(cli, drv_names):
    for drv_name in drv_names:
        _show_driver(cli, drv_name, True)


@cmd("show version", "Show the version of BESS daemon")
def show_version(cli):
    version = cli.bess.get_version()
    cli.fout.write(f"{version.version}\n")


def _monitor_pipeline(cli, field, units, graph_args=None):
    if graph_args is None:
        graph_args = []

    modules = sorted(cli.bess.list_modules().modules, key=lambda x: x.name)

    last_stats = {}
    for module in modules:
        gates = cli.bess.get_module_info(module.name).ogates

        for gate in gates:
            last_stats[(module.name, gate.ogate)] = (
                gate.timestamp,
                getattr(gate, field),
            )

    try:
        while True:
            time.sleep(1)
            cli.fout.write(
                _draw_pipeline(cli, field, units, last_stats, graph_args=graph_args)
            )
            cli.fout.write("\n")
    except KeyboardInterrupt:
        pass


@cmd(
    "monitor pipeline [GRAPHEASY_OPTS...]",
    "Monitor packet counters in the datapath pipeline",
)
def monitor_pipeline(cli, opts):
    _monitor_pipeline(cli, "pkts", "", graph_args=opts)


@cmd(
    "monitor pipeline batch [GRAPHEASY_OPTS...]",
    "Monitor batch counters in the datapath pipeline",
)
def monitor_pipeline_batch(cli, opts):
    _monitor_pipeline(cli, "cnt", "", graph_args=opts)


@cmd(
    "monitor pipeline bit [GRAPHEASY_OPTS...]",
    "Monitor Megabit counters in the datapath pipeline",
)
def monitor_pipeline_bit(cli, opts):
    _monitor_pipeline(cli, "bytes", "Mbps", graph_args=opts)


PortRate = collections.namedtuple(
    "PortRate",
    [
        "inc_packets",
        "inc_dropped",
        "inc_bytes",
        "out_packets",
        "out_dropped",
        "out_bytes",
    ],
)


def _calculate_port_delta(old, new):
    """Calculate rate-based statistics for port."""
    sec_diff = new.timestamp - old.timestamp
    return PortRate(
        inc_packets=(new.inc.packets - old.inc.packets) / sec_diff,
        inc_dropped=(new.inc.dropped - old.inc.dropped) / sec_diff,
        inc_bytes=(new.inc.bytes - old.inc.bytes) / sec_diff,
        out_packets=(new.out.packets - old.out.packets) / sec_diff,
        out_dropped=(new.out.dropped - old.out.dropped) / sec_diff,
        out_bytes=(new.out.bytes - old.out.bytes) / sec_diff,
    )


def _aggregate_port_stats(stats_array):
    """Aggregate statistics from multiple ports."""
    total = copy.deepcopy(stats_array[0])
    for stat in stats_array[1:]:
        total.inc.packets += stat.inc.packets
        total.inc.dropped += stat.inc.dropped
        total.inc.bytes += stat.inc.bytes
        total.out.packets += stat.out.packets
        total.out.dropped += stat.out.dropped
        total.out.bytes += stat.out.bytes
    return total


def _format_and_write_port_data(cli, name, delta, csv_f=None):
    """Format and write a single line of port data."""
    # If inc/out_bytes == 0 and inc_packets != 0, driver doesn't account packet bytes.
    inc_mbps = (
        ((delta.inc_bytes + delta.inc_packets * 24) * 8 / 1e6)
        if delta.inc_bytes
        else 0.0
    )
    out_mbps = (
        ((delta.out_bytes + delta.out_packets * 24) * 8 / 1e6)
        if delta.out_bytes
        else 0.0
    )

    data = (
        inc_mbps,
        delta.inc_packets / 1e6,
        int(delta.inc_dropped),
        out_mbps,
        delta.out_packets / 1e6,
        int(delta.out_dropped),
    )

    cli.fout.write(
        "{:<20}{:>14.1f}{:>10.3f}{:>10d}        {:>14.1f}{:>10.3f}{:>10d}\n".format(
            name, *data
        )
    )
    if csv_f is not None:
        csv_line = "{},{},{}\n".format(
            time.strftime("%X"), name, ",".join(f"{x:.3f}" for x in data)
        )
        csv_f.write(csv_line)


def _monitor_ports_loop(cli, ports, drivers, csv_f=None):
    """Main monitoring loop for ports."""
    last = {port: cli.bess.get_port_stats(port) for port in ports}

    while True:
        time.sleep(1)
        now = {port: cli.bess.get_port_stats(port) for port in ports}

        # 1. Write Header
        timestamp = now[ports[-1]].timestamp
        cli.fout.write("\n")
        cli.fout.write(
            "{:<20}{:>14}{:>10}{:>10}        {:>14}{:>10}{:>10}\n".format(
                time.strftime("%X") + str(timestamp % 1)[1:8],
                "INC     Mbps",
                "Mpps",
                "dropped",
                "OUT     Mbps",
                "Mpps",
                "Dropped",
            )
        )
        cli.fout.write("{}\n".format("-" * 96))

        # 2. Write Deltas
        for port in ports:
            delta = _calculate_port_delta(last[port], now[port])
            _format_and_write_port_data(cli, f"{port}{drivers[port]}", delta, csv_f)

        cli.fout.write("{}\n".format("-" * 96))

        # 3. Write Totals (if applicable)
        if len(ports) > 1:
            total_last = _aggregate_port_stats(list(last.values()))
            total_now = _aggregate_port_stats(list(now.values()))
            total_delta = _calculate_port_delta(total_last, total_now)
            _format_and_write_port_data(cli, "Total", total_delta, csv_f)

        # 4. Update stats for next loop
        last = now


def _monitor_ports(cli, *ports):
    """Monitor port statistics."""
    all_ports = sorted(cli.bess.list_ports().ports, key=lambda x: x.name)
    drivers = {port.name: port.driver for port in all_ports}

    if not ports:
        ports = [port.name for port in all_ports]
        if not ports:
            raise cli.CommandError("No port to monitor")

    cli.fout.write("Monitoring ports: {}\n".format(", ".join(ports)))

    try:
        csv_path = os.getenv("CSV", None)
        with open(csv_path, "w") if csv_path is not None else noop() as csv_f:
            if csv_f is not None:
                csv_f.write(
                    "Timestamp,Port,Mbps In,Mpps In,Dropped In,"
                    "Mbps Out,Mpps Out,Dropped Out\n"
                )
            _monitor_ports_loop(cli, ports, drivers, csv_f)
    except KeyboardInterrupt:
        pass


@cmd("monitor port", "Monitor the current traffic of all ports")
def monitor_port_all(cli):
    _monitor_ports(cli)


@cmd("monitor port PORT...", "Monitor the current traffic of specified ports")
def monitor_port_all(cli, ports):
    _monitor_ports(cli, *ports)


TcCounterRate = collections.namedtuple(
    "TcCounterRate", ["count", "cycles", "bits", "packets"]
)


def _calculate_tc_delta(old, new):
    """Calculate rate-based statistics for traffic class."""
    sec_diff = new.timestamp - old.timestamp
    if sec_diff <= 0:
        return TcCounterRate(count=0.0, cycles=0.0, bits=0.0, packets=0.0)
    return TcCounterRate(
        count=(new.count - old.count) / sec_diff,
        cycles=(new.cycles - old.cycles) / sec_diff,
        bits=(new.bits - old.bits) / sec_diff,
        packets=(new.packets - old.packets) / sec_diff,
    )


def _format_tc_data(delta):
    """Calculate ratios and format traffic class data for display."""
    ppb = delta.packets / delta.count if delta.count >= 1 else 0.0
    cpp = delta.cycles / delta.packets if delta.packets >= 1 else 0.0
    return (
        delta.cycles / 1e6,
        int(delta.count),
        delta.packets / 1e6,
        delta.bits / 1e6,
        ppb,
        cpp,
    )


def _monitor_tc_loop(cli, tcs, wids, max_len, fields, csv_f=None):
    """Main monitoring loop for traffic classes."""
    last_stats = {tc: cli.bess.get_tc_stats(tc) for tc in tcs}

    while True:
        time.sleep(1)
        current_stats = {tc: cli.bess.get_tc_stats(tc) for tc in tcs}

        # 1. Write Header
        timestamp = current_stats[tcs[-1]].timestamp
        cli.fout.write("\n")
        fmt_head = f"{{:<{max_len}}}{{:>12}}{{:>12}}{{:>12}}{{:>12}}{{:>12}}{{:>12}}\n"
        cli.fout.write(
            fmt_head.format(time.strftime("%X") + str(timestamp % 1)[1:8], *fields)
        )
        cli.fout.write("{}\n".format("-" * (72 + max_len)))

        # 2. Write Data for each TC
        for tc in tcs:
            delta = _calculate_tc_delta(last_stats[tc], current_stats[tc])
            data = _format_tc_data(delta)
            tc_display_name = f"W{wids[tc]} {tc}"

            fmt_data = f"{{:<{max_len}}}{{:>12.3f}}{{:>12d}}{{:>12.3f}}{{:>12.3f}}{{:>12.3f}}{{:>12.3f}}\n"
            cli.fout.write(fmt_data.format(tc_display_name, *data))

            if csv_f is not None:
                csv_line = "{},{},{}\n".format(
                    time.strftime("%X"),
                    tc_display_name,
                    ",".join(f"{x:.3f}" for x in data),
                )
                csv_f.write(csv_line)

        # 3. Write Footer and update stats
        cli.fout.write("{}\n".format("-" * (72 + max_len)))
        last_stats = current_stats


def _monitor_tcs(cli, *tcs):
    """Monitor traffic class statistics."""
    GUTTER_WIDTH = 5
    FIELDS = ("CPU MHz", "scheduled", "Mpps", "Mbps", "pkts/sched", "cycles/p")

    # Get TC information
    all_tcs = cli.bess.list_tcs().classes_status
    wids = {}
    max_len = 0

    for tc in all_tcs:
        class_ = getattr(tc, "class")
        max_len = max(len(class_.name), max_len)
        wids[class_.name] = class_.wid
    max_len += GUTTER_WIDTH

    # Determine which TCs to monitor
    if not tcs:
        tcs = [getattr(tc, "class").name for tc in all_tcs]
        if not tcs:
            raise cli.CommandError("No traffic class to monitor")

    cli.fout.write("Monitoring traffic classes: {}\n".format(", ".join(tcs)))

    try:
        csv_path = os.getenv("CSV", None)
        with open(csv_path, "w") if csv_path is not None else noop() as csv_f:
            if csv_f is not None:
                csv_f.write(
                    "{}\n".format(",".join(("Timestamp", "traffic class") + FIELDS))
                )
            _monitor_tc_loop(cli, tcs, wids, max_len, FIELDS, csv_f)
    except KeyboardInterrupt:
        pass


@cmd("monitor tc", "Monitor the statistics of all traffic classes")
def monitor_tc_all(cli):
    _monitor_tcs(cli)


@cmd("monitor tc TC...", "Monitor the statistics of specified traffic classes")
def monitor_tc_all(cli, tcs):
    _monitor_tcs(cli, *tcs)


def _capture_gate(cli, module_name, direction, gate, opts, program, hook_fn):
    if gate is None:
        gate = 0

    if opts is None:
        opts = []

    if direction is None:
        direction = "out"

    fifo = tempfile.mktemp()
    os.mkfifo(fifo, 0o600)  # random people should not see packets...

    fd = os.open(fifo, os.O_RDWR)

    capture_cmd = [program]
    capture_cmd.extend(["-r", fifo])
    capture_cmd.extend(opts)
    capture_cmd = " ".join(capture_cmd)

    cli.fout.write(f"  Running: {capture_cmd}\n")
    proc = subprocess.Popen(capture_cmd, shell=True, start_new_session=True)

    unhook = True
    try:
        ret = hook_fn(True, "", module_name, direction, gate, fifo)
        proc.wait()
    except KeyboardInterrupt:
        # kill all descendants in the process group
        os.killpg(proc.pid, signal.SIGTERM)
    except cli.bess.Error:
        unhook = False
        # kill all descendants in the process group
        os.killpg(proc.pid, signal.SIGTERM)
        raise
    finally:
        proc.wait()
        try:
            if unhook:
                hook_fn(False, ret.name, module_name, direction, gate)
        finally:
            try:
                os.close(fd)
                os.unlink(fifo)
                os.system("stty sane")  # more/less may screw the terminal
            except Exception:
                logger.debug("Failed to clean up capture fifo", exc_info=True)


# tcpdump can write pcap files, so we don't need to support it separately


@cmd("tcpdump MODULE [DIRECTION] [GATE] [TCPDUMP_OPTS...]", "Capture packets on a gate")
def tcpdump_gate(cli, module_name, direction, gate, opts):
    _capture_gate(
        cli, module_name, direction, gate, opts, "tcpdump", cli.bess.tcpdump_gate
    )


@cmd(
    "tshark MODULE [DIRECTION] [GATE] [TSHARK_OPTS...]",
    "Capture packets on a gate with metadata",
)
def tshark_gate(cli, module_name, direction, gate, opts):
    if opts is None:
        opts = ["-z", "proto,colinfo,frame.comment,frame.comment"]
    _capture_gate(
        cli, module_name, direction, gate, opts, "tshark", cli.bess.pcapng_gate
    )


def _track_gate(cli, bits, flag, module_name, direction, gate):
    if direction is None:
        direction = "out"
    if module_name in [None, "*"]:
        module_name = ""
    if gate is None:
        gate = -1

    cli.bess.pause_all()
    try:
        if flag == "enable":
            cli.bess.track_gate(True, "", module_name, bits, direction, gate)
        else:
            cli.bess.track_gate(False, "", module_name, bits, direction, gate)
    finally:
        cli.bess.resume_all()


@cmd(
    "track ENABLE_DISABLE [MODULE] [DIRECTION] [GATE]",
    "Count the packets and batches on specified or all gates",
)
def track_gate(cli, flag, module_name, direction, gate):
    _track_gate(cli, False, flag, module_name, direction, gate)


@cmd(
    "track bit ENABLE_DISABLE [MODULE] [DIRECTION] [GATE]",
    "Count the packets, batches, and bits on specified or all gates",
)
def track_gate_bits(cli, flag, module_name, direction, gate):
    _track_gate(cli, True, flag, module_name, direction, gate)


# really should support "all gates" but that requires that we
# iterate over all gates


@cmd(
    "track reset MODULE DIRECTION GATE",
    "Reset counts of packets, batches, and bits on specified gate",
)
def track_reset(cli, module_name, direction, gate):
    cli.bess.run_gate_command(
        "Track", module_name, direction, gate, "reset", "EmptyArg", {}
    )


@cmd("interactive", "Switch to interactive mode")
def interactive(cli):
    if cli.interactive:
        return

    old_fin = cli.fin
    old_fout = cli.fout
    cli.fin = sys.stdin
    cli.fout = sys.stdout
    cli.interactive = True

    cli.go_interactive()
    cli.loop()

    cli.fin = old_fin
    cli.fout = old_fout
    cli.interactive = False


@cmd("show system packets [SOCKET]", "Dump the mempool of one or more sockets")
def show_system_packets(cli, socket):
    if socket is None:
        socket = -1
    resp = cli.bess.dump_mempool(socket)
    for dump in resp.dumps:
        cli.fout.write(f"Socket {dump.socket}\n")
        cli.fout.write(f"\tinitialized: {dump.initialized}\n")
        if not dump.initialized:
            continue
        cli.fout.write(f"\tmp_size: {dump.mp_size}\n")
        cli.fout.write(f"\tmp_cache_size: {dump.mp_cache_size}\n")
        cli.fout.write(f"\tmp_element_size: {dump.mp_element_size}\n")
        cli.fout.write(f"\tmp_populated_size: {dump.mp_populated_size}\n")
        cli.fout.write(f"\tmp_available_count: {dump.mp_available_count}\n")
        cli.fout.write(f"\tmp_in_use_count: {dump.mp_in_use_count}\n")
        cli.fout.write(f"\tring_count: {dump.ring_count}\n")
        cli.fout.write(f"\tring_free_count: {dump.ring_free_count}\n")
        cli.fout.write(f"\tring_bytes: {dump.ring_bytes}\n")


@cmd("http [HOST] [PORT_NUMBER]", "Run an HTTP server")
def http(cli, host, port):
    host = host or "localhost"
    port = port or 5000

    try:
        import server
    except ImportError:
        raise cli.CommandError('Failed to load server ("pip install flask"?)')

    server.app.env = "development"
    server.app.bess = cli.bess
    server.app.run(host=host, port=int(port))
