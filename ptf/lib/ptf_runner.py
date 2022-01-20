#!/usr/bin/env python3

# SPDX-License-Identifier: Apache-2.0
# Copyright 2021 Open Networking Foundation

import argparse
import logging
import os
import re
import subprocess
import sys

from trex_stf_lib.trex_client import CTRexClient

DUMMY_IFACE_NAME = "ptfdummy"
TREX_FILES_DIR = "/tmp/trex_files/"
DEFAULT_KILL_TIMEOUT = 10
LOG_FORMAT = "%(asctime)s %(levelname)s %(message)s"
logging.basicConfig(format=LOG_FORMAT, level=logging.INFO)
logging.getLogger().setLevel(logging.INFO)


def error(msg, *args, **kwargs):
    logging.error(msg, *args, **kwargs)


def warn(msg, *args, **kwargs):
    logging.warning(msg, *args, **kwargs)


def info(msg, *args, **kwargs):
    logging.info(msg, *args, **kwargs)


def check_ifaces(ifaces):
    """
    Checks that required interfaces exist.
    """
    ifconfig_out = subprocess.check_output(["ifconfig"]).decode("utf-8")
    iface_list = re.findall(r"^([a-zA-Z0-9]+)", ifconfig_out, re.S | re.M)
    present_ifaces = set(iface_list)
    ifaces = set(ifaces)
    return ifaces <= present_ifaces


def set_up_interfaces(ifaces):
    for iface in ifaces:
        try:
            subprocess.check_call(["ip", "link", "set", iface, "up"])
            subprocess.check_call(["ip", "link", "set", iface, "promisc", "on"])
            subprocess.check_call(["sysctl", f"net.ipv6.conf.{iface}.disable_ipv6=1"])
        except subprocess.CalledProcessError as e:
            info(f"Got an error when setting up {iface}: {e.output}")
            return False
    return True


def create_dummy_interface():
    try:
        subprocess.check_output(["ip", "link", "show", DUMMY_IFACE_NAME])
        return True  # device already exists, skip
    except:
        # interface does not exists
        pass
    try:
        subprocess.check_output(
            ["ip", "link", "add", DUMMY_IFACE_NAME, "type", "dummy"]
        )
    except subprocess.CalledProcessError as e:
        info(
            f'Got error when creating dummy interface "{DUMMY_IFACE_NAME}": {e.output}'
        )
        return False
    return True


def remove_dummy_interface():
    try:
        subprocess.check_output(["ip", "link", "show", DUMMY_IFACE_NAME])
        try:
            subprocess.check_output(["ip", "link", "delete", DUMMY_IFACE_NAME])
        except subprocess.CalledProcessError as e:
            info(
                f'Got error when deleting dummy interface "{DUMMY_IFACE_NAME}" {e.output}'
            )
            return False
        return True
    except:
        # interface does not exists
        return True


def set_up_trex_server(trex_daemon_client, trex_address, trex_config):
    """Start the TRex daemon client to run while PTF tests are running

    The TRex daemon client handles spawning TRex clients for each PTF
    test case. A TRex client is a temporary client that generates
    traffic. At the end of the PTF test, the TRex daemon client also
    handles closing this client.
    """

    try:
        info("Pushing TRex config %s to the server", trex_config)
        if not trex_daemon_client.push_files(trex_config):
            error("Unable to push %s to Trex server", trex_config)
            return False

        if not trex_daemon_client.is_idle():
            warn("The TRex daemon client is still running! Killing it for you...")
            trex_daemon_client.kill_all_trexes()
            trex_daemon_client.force_kill(confirm=False)

        trex_config_file_on_server = TREX_FILES_DIR + os.path.basename(trex_config)
        trex_daemon_client.start_stateless(cfg=trex_config_file_on_server)
    except ConnectionRefusedError:
        error(
            "Unable to connect to server %s.\n" + "Did you start the Trex daemon?",
            trex_address,
        )
        return False

    return True


def run_test(
    bess_addr,
    ptfdir,
    trex_server_addr=None,
    extra_args=(),
):
    """
    Runs PTF tests included in provided directory.
    """

    # create a dummy interface for PTF
    if not create_dummy_interface() or not set_up_interfaces([DUMMY_IFACE_NAME]):
        return False

    pypath = "/upf-tests/lib"

    # build the ptf command to be run
    cmd = ["ptf"]
    cmd.extend(["--test-dir", ptfdir])
    cmd.extend(["--pypath", pypath])
    cmd.extend(["-i", f"296@{DUMMY_IFACE_NAME}"])
    test_params = "bess_upf_addr='{}'".format(bess_addr)
    if trex_server_addr is not None:
        test_params += ";trex_server_addr='{}'".format(trex_server_addr)
    cmd.append("--test-params={}".format(test_params))
    cmd.extend(extra_args)
    info("Executing PTF command: {}".format(" ".join(cmd)))

    try:
        # run ptf and send output to stdout
        p = subprocess.Popen(cmd)
        p.wait()
    except Exception:
        error("Error when running PTF tests")
        return False
    finally:
        # always clean up the dummy interface
        remove_dummy_interface()

    return p.returncode == 0


def check_ptf():
    try:
        with open(os.devnull, "w") as devnull:
            subprocess.check_call(["ptf", "--version"], stdout=devnull, stderr=devnull)
        return True
    except subprocess.CalledProcessError:
        return True
    except OSError:  # PTF not found
        return False


# noinspection PyTypeChecker
def main():

    """
    Read in all command line arguments.
    """

    parser = argparse.ArgumentParser(
        description="Start TRex daemon client and run PTF command"
    )
    parser.add_argument(
        "--ptf-dir", help="Directory containing PTF tests", type=str, required=True,
    )
    parser.add_argument(
        "--trex-address",
        help="Address of the remote TRex daemon server",
        type=str,
        required=False,
    )
    parser.add_argument(
        "--bess-address",
        help="Address of the remote BESS-UPF instance",
        type=str,
        required=False,
    )
    parser.add_argument(
        "--trex-config",
        help="Location of the TRex config file to be pushed to the remote server",
        type=str,
        required=False,
    )
    parser.add_argument(
        "--trex-hw-mode",
        help="Enables NIC HW acceleration and disables TRex software mode",
        action="store_true",
        required=False,
    )
    args, unknown_args = parser.parse_known_args()

    # ensure PTF command is available
    if not check_ptf():
        error("Cannot find PTF executable")
        sys.exit(1)

    """
    Run either linerate or unary test case, depending on arguments.
    """

    # if line rate test, need to perform set up of TRex traffic generator
    if args.trex_address is not None:
        if args.trex_hw_mode:
            trex_args = None
        else:
            trex_args = "--software --no-hw-flow-stat"
        
        trex_daemon_client = CTRexClient(args.trex_address, trex_args=trex_args)

        info("Starting TRex daemon client...")
        success = set_up_trex_server(
            trex_daemon_client, args.trex_address, args.trex_config,
        )
        if not success:
            error("Failed to set up TRex daemon client!")
            sys.exit(2)

        info("Running linerate test(s)...")
        success = run_test(
            bess_addr=args.bess_address,
            ptfdir=args.ptf_dir,
            trex_server_addr=args.trex_address,
            extra_args=unknown_args,
        )
        if not success:
            error("Failed to run linerate tests!")
            trex_daemon_client.stop_trex()
            sys.exit(3)

        trex_daemon_client.stop_trex()

    # if unary test, can skip TRex set up and just run PTF command
    else:
        info("Running unary test(s)...")
        success = run_test(
            p4info_path=args.p4info,
            device_id=args.device_id,
            grpc_addr=args.grpc_addr,
            cpu_port=args.cpu_port,
            ptfdir=args.ptf_dir,
            port_map_path=args.port_map,
            platform=args.platform,
            generate_tv=args.generate_tv,
            loopback=args.loopback,
            profile=args.profile,
            extra_args=unknown_args,
        )
        if not success:
            error("Failed running unary tests!")
            sys.exit(4)


if __name__ == "__main__":
    main()
