#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright 2023 Canonical Ltd.

import asyncio
import sys
import threading
import time
import unittest
from dataclasses import replace
from unittest.mock import MagicMock, Mock, patch

sys.modules["pybess.bess"] = MagicMock()

from conf.route_control import (
    RouteController,
    RouteEntry,
    fetch_mac,
    mac_to_hex,
    mac_to_int,
    route_key,
    validate_ipv4,
)

DEFAULT_MAC_ADDRESS = "00:1a:2b:3c:4d:5e"


class BessControllerMock:
    """Mock of BessController to avoid using BESS from pybess.bess"""

    def __init__(self):
        pass

    def _get_bess(self, *args, **kwargs) -> None:
        pass

    def add_route_to_module(self, *args, **kwargs) -> None:
        pass

    def delete_module_route_entry(self, *args, **kwargs) -> None:
        pass

    def create_module(self, *args, **kwargs) -> None:
        pass

    def delete_module(self, *args, **kwargs) -> None:
        pass

    def link_modules(self, *args, **kwargs) -> None:
        pass


@patch("conf.route_control.BessController", BessControllerMock)
class TestUtilityFunctions(unittest.TestCase):
    """Tests utility functions in route_control.py."""

    def test_given_valid_ip_when_validate_ipv4_then_returns_true(self):
        self.assertTrue(validate_ipv4("192.168.1.1"))

    def test_given_invalid_ip_when_validate_ipv4_then_returns_false(self):
        self.assertFalse(validate_ipv4("192.168.300.1"))

    def test_given_invalid_ip_when_validate_ipv6_then_returns_false(self):
        self.assertFalse(validate_ipv4("::1"))
        self.assertFalse(validate_ipv4(""))

    def test_given_valid_mac_when_mac_to_int_then_returns_int_representation(self):
        self.assertEqual(mac_to_int(DEFAULT_MAC_ADDRESS), 112394521950)

    def test_given_invalid_mac_when_mac_to_int_then_raises_exception(self):
        with self.assertRaises(ValueError):
            mac_to_int("not a mac")

    def test_given_valid_mac_when_mac_to_hex_then_return_hex_string_representation(
        self,
    ):
        self.assertEqual(mac_to_hex(DEFAULT_MAC_ADDRESS), "001A2B3C4D5E")

    def test_given_known_destination_when_fetch_mac_then_returns_mac(self):
        ndb = Mock()
        kwargs = {"ifindex": 1, "dst": "192.168.1.1", "lladdr": DEFAULT_MAC_ADDRESS}
        ndb.neighbours.dump.return_value = [kwargs]
        self.assertEqual(fetch_mac(ndb, "192.168.1.1"), DEFAULT_MAC_ADDRESS)

    def test_given_unknown_destination_when_fetch_mac_then_returns_none(self):
        ndb = Mock()
        kwargs = {"ifindex": 1, "dst": "192.168.1.1", "lladdr": None}
        ndb.neighbours.dump.return_value = [kwargs]
        self.assertIsNone(fetch_mac(ndb, "192.168.1.1"))


class TestRouteController(unittest.TestCase):
    def setUp(self):
        self.mock_bess_controller = Mock(BessControllerMock)
        self.ndb = Mock()
        self.ipr = Mock()
        interfaces = ["access", "core"]
        self.route_controller = RouteController(
            self.mock_bess_controller,
            self.ndb,
            interfaces=interfaces,
            ipr=self.ipr,
        )

    @patch("conf.route_control.fetch_mac")
    @patch("conf.route_control.get_merge_module_name")
    @patch("conf.route_control.get_route_module_name")
    @patch("conf.route_control.get_update_module_name")
    def add_route_entry(
        self,
        route_entry,
        mock_get_update_module_name,
        mock_get_route_module_name,
        mock_get_merge_module_name,
        mock_fetch_mac,
    ) -> None:
        """Adds a new route entry using the route controller."""
        kwargs = {"ifindex": 1, "dst": "192.168.1.1", "lladdr": DEFAULT_MAC_ADDRESS}
        self.ndb.neighbours.dump.return_value = [kwargs]
        mock_get_update_module_name.return_value = "merge_module"
        mock_get_route_module_name.return_value = "route_module"
        mock_get_merge_module_name.return_value = "update_module"
        mock_fetch_mac.return_value = DEFAULT_MAC_ADDRESS
        self.route_controller.add_new_route_entry(route_entry)
        return route_entry

    def test_given_valid_route_message_when_parse_message_then_parses_message(self):
        self.ndb.interfaces = {2: {"ifname": "core"}}
        example_route_entry = {
            "family": 2,
            "dst_len": 24,
            "flags": 0,
            "attrs": [
                ("RTA_TABLE", 254),
                ("RTA_PRIORITY", 100),
                ("RTA_PREFSRC", "172.31.55.52"),
                ("RTA_GATEWAY", "172.31.48.1"),
                ("RTA_OIF", 2),
                ("RTA_DST", "192.168.1.0"),
            ],
            "header": {
                "length": 68,
                "type": 24,
                "target": "localhost",
                "stats": {"qsize": 0, "delta": 0, "delay": 0},
            },
            "event": "RTM_NEWROUTE",
        }
        result = self.route_controller._parse_route_entry_msg(example_route_entry)
        self.assertIsInstance(result, RouteEntry)
        if result is not None:
            self.assertEqual(result.dest_prefix, "192.168.1.0")
            self.assertEqual(result.next_hop_ip, "172.31.48.1")
            self.assertEqual(result.interface, self.ndb.interfaces[2].get("ifname"))
            self.assertEqual(result.prefix_len, 24)
        else:
            self.fail(
                "Result is None, expected an object with dest_prefix, next_hop_ip, interface, and prefix_len"
            )

    def test_given_valid_route_message_and_dst_len_is_zero_when_parse_message_then_parses_message_as_default_route(
        self,
    ):
        self.ndb.interfaces = {2: {"ifname": "core"}}
        example_route_entry = {
            "family": 2,
            "dst_len": 0,
            "flags": 0,
            "attrs": [
                ("RTA_TABLE", 254),
                ("RTA_PRIORITY", 100),
                ("RTA_PREFSRC", "172.31.55.52"),
                ("RTA_GATEWAY", "172.31.48.1"),
                ("RTA_OIF", 2),
            ],
            "header": {
                "length": 68,
                "type": 24,
                "target": "localhost",
                "stats": {"qsize": 0, "delta": 0, "delay": 0},
            },
            "event": "RTM_NEWROUTE",
        }
        result = self.route_controller._parse_route_entry_msg(example_route_entry)
        self.assertIsInstance(result, RouteEntry)
        if result is not None:
            self.assertEqual(result.dest_prefix, "0.0.0.0")
            self.assertEqual(result.next_hop_ip, "172.31.48.1")
            self.assertEqual(result.interface, self.ndb.interfaces[2].get("ifname"))
            self.assertEqual(result.prefix_len, 0)
        else:
            self.fail(
                "Result is None, expected an object with dest_prefix, next_hop_ip, interface, and prefix_len"
            )

    def test_given_invalid_route_message_when_parse_message_then_returns_none(self):
        self.ndb.interfaces = {2: {"ifname": "not the needed interface"}}
        example_route_entry = {
            "family": 2,
            "flags": 0,
            "attrs": [
                ("RTA_TABLE", 254),
                ("RTA_PRIORITY", 100),
                ("RTA_PREFSRC", "172.31.55.52"),
                ("RTA_GATEWAY", "172.31.48.1"),
                ("RTA_OIF", 2),
            ],
            "header": {
                "length": 68,
                "type": 24,
                "target": "localhost",
                "stats": {"qsize": 0, "delta": 0, "delay": 0},
            },
            "event": "RTM_NEWROUTE",
        }
        result = self.route_controller._parse_route_entry_msg(example_route_entry)
        self.assertIsNone(result)

    @patch("conf.route_control.send_ping")
    def test_given_new_route_when_add_new_route_entry_and_mac_not_known_then_destination_is_pinged(
        self,
        mock_send_ping,
    ):
        kwargs = {"ifindex": 1, "dst": "192.168.1.1", "lladdr": None}
        self.ndb.neighbours.dump.return_value = [kwargs]
        route_entry = RouteEntry(
            next_hop_ip="1.2.3.4",
            interface="random_interface",
            dest_prefix="1.1.1.1",
            prefix_len=24,
        )
        self.route_controller.add_new_route_entry(route_entry)
        mock_send_ping.assert_called_once()

    @patch("conf.route_control.send_ping")
    def test_given_valid_new_route_when_add_new_route_entry_and_mac_known_then_route_is_added_in_bess(
        self, _
    ):
        kwargs = {"ifindex": 1, "dst": "192.168.1.1", "lladdr": DEFAULT_MAC_ADDRESS}
        self.ndb.neighbours.dump.return_value = [kwargs]
        mock_routes = [{"event": "RTM_NEWROUTE"}, {"event": "OTHER_ACTION"}]
        self.ipr.get_routes.return_value = mock_routes
        route_entry = RouteEntry(
            next_hop_ip="1.2.3.4",
            interface="random_interface",
            dest_prefix="1.1.1.1",
            prefix_len=24,
        )
        self.route_controller.add_new_route_entry(route_entry)
        self.mock_bess_controller.add_route_to_module()
        self.mock_bess_controller.add_route_to_module.assert_called_once()

    @patch("conf.route_control.send_ping")
    def test_given_valid_new_route_when_add_new_route_entry_and_mac_known_and_neighbor_not_known_then_update_module_is_created_and_modules_are_linked(
        self, _
    ):
        kwargs = {"ifindex": 1, "dst": "1.2.3.4", "lladdr": DEFAULT_MAC_ADDRESS}
        self.ndb.neighbours.dump.return_value = [kwargs]
        mock_routes = [{"event": "RTM_NEWROUTE"}, {"event": "OTHER_ACTION"}]
        self.ipr.get_routes.return_value = mock_routes
        route_entry = RouteEntry(
            next_hop_ip="1.2.3.4",
            interface="random_interface",
            dest_prefix="1.1.1.1",
            prefix_len=24,
        )
        self.route_controller.add_new_route_entry(route_entry)
        self.mock_bess_controller.create_module.assert_called()
        self.mock_bess_controller.link_modules.assert_called()

    @patch.object(RouteController, "add_new_route_entry")
    def test_given_new_route_when_bootstrap_routes_then_add_new_entry_is_called(
        self,
        mock_add_new_route_entry,
    ):
        mock_routes = [
            {
                "event": "RTM_NEWROUTE",
                "attrs": {
                    "RTA_OIF": 2,
                    "RTA_GATEWAY": "1.2.3.4",
                    "RTA_DST": "1.1.1.1",
                },
                "dst_len": 24,
            },
            {"event": "OTHER_ACTION"},
        ]
        self.ipr.get_routes.return_value = mock_routes
        self.ndb.interfaces = {2: {"ifname": "core"}}
        valid_route_entry = RouteEntry(
            next_hop_ip="1.2.3.4",
            interface="core",
            dest_prefix="1.1.1.1",
            prefix_len=24,
        )
        self.ipr.get_routes.return_value = mock_routes
        self.route_controller.bootstrap_routes()
        self.ipr.get_routes.assert_called_once()
        mock_add_new_route_entry.assert_called_with(valid_route_entry)

    @patch.object(RouteController, "add_new_route_entry")
    def test_given_no_new_route_when_bootstrap_routes_then_add_new_entry_is_not_called(
        self,
        mock_add_new_route_entry,
    ):
        mock_routes = [
            {
                "event": "Not a new route",
                "attrs": {
                    "RTA_OIF": 2,
                    "RTA_GATEWAY": "1.2.3.4",
                    "RTA_DST": "1.1.1.1",
                },
                "dst_len": 24,
            },
            {"event": "OTHER_ACTION"},
        ]
        self.ipr.get_routes.return_value = mock_routes
        self.route_controller._parse_route_entry_msg = Mock()
        self.route_controller.bootstrap_routes()
        self.ipr.get_routes.assert_called_once()
        mock_add_new_route_entry.assert_not_called()

    @patch.object(RouteController, "add_new_route_entry")
    def test_given_new_route_and_invalid_message_when_bootstrap_routes_then_add_new_entry_is_not_called(
        self,
        mock_add_new_route_entry,
    ):
        mock_routes = [
            {
                "event": "RTM_NEWROUTE",
                "attrs": {},
            },
            {"event": "OTHER_ACTION"},
        ]
        self.ipr.get_routes.return_value = mock_routes
        self.route_controller.bootstrap_routes()

        self.ipr.get_routes.assert_called_once()
        mock_add_new_route_entry.assert_not_called()

    @patch.object(RouteController, "add_new_route_entry")
    def test_given_netlink_message_when_rtm_newroute_event_then_add_new_route_entry_is_called(
        self, mock_add_new_route_entry
    ):
        self.ndb.interfaces = {2: {"ifname": "core"}}
        example_route_entry = {
            "family": 2,
            "dst_len": 24,
            "flags": 0,
            "attrs": [
                ("RTA_TABLE", 254),
                ("RTA_PRIORITY", 100),
                ("RTA_PREFSRC", "172.31.55.52"),
                ("RTA_GATEWAY", "172.31.48.1"),
                ("RTA_OIF", 2),
                ("RTA_DST", "192.168.1.0"),
            ],
            "header": {
                "length": 68,
                "type": 24,
                "target": "localhost",
                "stats": {"qsize": 0, "delta": 0, "delay": 0},
            },
            "event": "RTM_NEWROUTE",
        }
        self.route_controller._handle_route_event(example_route_entry)
        mock_add_new_route_entry.assert_called()

    def test_given_existing_neighbor_and_route_count_not_zero_when_delete_route_entry_then_route_entry_deleted_in_bess(
        self,
    ):
        route_entry = RouteEntry(
            next_hop_ip="1.2.3.4",
            interface="random_interface",
            dest_prefix="1.1.1.1",
            prefix_len=24,
        )
        self.add_route_entry(route_entry)
        self.route_controller.delete_route_entry(route_entry)
        self.mock_bess_controller.delete_module_route_entry.assert_called_once()

    def test_given_existing_neighbor_and_route_count_greater_than_one_when_delete_route_entry_then_module_not_deleted(
        self,
    ):
        route_entry_1 = RouteEntry(
            next_hop_ip="1.2.3.4",
            interface="random_interface",
            dest_prefix="1.1.1.1",
            prefix_len=24,
        )
        route_entry_2 = RouteEntry(
            next_hop_ip="1.2.3.4",
            interface="random_interface_2",
            dest_prefix="1.1.1.2",
            prefix_len=24,
        )
        self.add_route_entry(route_entry_1)
        self.add_route_entry(route_entry_2)
        self.route_controller.delete_route_entry(route_entry_1)
        self.mock_bess_controller.delete_module.assert_not_called()

    def test_given_existing_neighbor_and_route_count_is_one_when_delete_route_entry_then_module_deleted(
        self,
    ):
        route_entry = RouteEntry(
            next_hop_ip="1.2.3.4",
            interface="random_interface",
            dest_prefix="1.1.1.1",
            prefix_len=24,
        )
        self.add_route_entry(route_entry)
        self.route_controller.delete_route_entry(route_entry)
        self.mock_bess_controller.delete_module.assert_called_once()

    @patch.object(RouteController, "delete_route_entry")
    def test_given_netlink_message_when_rtm_delroute_event_then_delete_route_entry_is_called(
        self, mock_delete_route_entry
    ):
        self.ndb.interfaces = {2: {"ifname": "core"}}
        example_route_entry = {
            "family": 2,
            "dst_len": 24,
            "flags": 0,
            "attrs": [
                ("RTA_TABLE", 254),
                ("RTA_PRIORITY", 100),
                ("RTA_PREFSRC", "172.31.55.52"),
                ("RTA_GATEWAY", "172.31.48.1"),
                ("RTA_OIF", 2),
                ("RTA_DST", "192.168.1.0"),
            ],
            "header": {
                "length": 68,
                "type": 24,
                "target": "localhost",
                "stats": {"qsize": 0, "delta": 0, "delay": 0},
            },
            "event": "RTM_DELROUTE",
        }
        self.route_controller._handle_route_event(example_route_entry)
        mock_delete_route_entry.assert_called()

    @patch("conf.route_control.send_ping")
    def test_given_new_neighbor_in_unresolved_when_add_unresolved_new_neighbor_then_route_added_in_bess(
        self,
        _,
    ):
        kwargs = {"ifindex": 1, "dst": "192.168.1.1", "lladdr": DEFAULT_MAC_ADDRESS}
        self.ndb.neighbours.dump.return_value = [kwargs]
        mock_netlink_msg = {
            "attrs": {
                "NDA_DST": "1.2.3.4",
                "NDA_LLADDR": DEFAULT_MAC_ADDRESS,
            }
        }
        mock_routes = [{"event": "RTM_NEWROUTE"}, {"event": "OTHER_ACTION"}]
        self.ipr.get_routes.return_value = mock_routes
        route_entry = RouteEntry(
            next_hop_ip="1.2.3.4",
            interface="random_interface",
            dest_prefix="1.1.1.1",
            prefix_len=24,
        )
        self.route_controller.add_new_route_entry(route_entry)
        self.route_controller.add_unresolved_new_neighbor(mock_netlink_msg)
        self.mock_bess_controller.add_route_to_module.assert_called_once()

    @patch.object(RouteController, "add_unresolved_new_neighbor")
    def test_given_netlink_message_when_rtm_newneigh_event_then_add_unresolved_new_neighbor_is_called(
        self, mock_add_unresolved_new_neighbor
    ):
        self.route_controller._handle_neighbor_event({"event": "RTM_NEWNEIGH"})
        mock_add_unresolved_new_neighbor.assert_called()


class TestOnLinkNetworks(unittest.TestCase):
    """Tests routes for networks attached to a watched interface."""

    def setUp(self):
        self.mock_bess_controller = Mock(BessControllerMock)
        self.ndb = Mock()
        self.ipr = Mock()
        self.route_controller = RouteController(
            self.mock_bess_controller,
            self.ndb,
            interfaces=["access", "core"],
            ipr=self.ipr,
        )
        self.ndb.interfaces = {2: {"ifname": "access"}}
        self.ndb.neighbours.dump.return_value = []
        # _interface_name resolves an index through the C library rather than
        # NDB, so the index-to-name map is stubbed rather than the NDB mock.
        names = {2: "access", 3: "core"}

        def ifname(index):
            try:
                return names[int(index)]
            except KeyError:
                raise OSError("no such interface") from None

        patcher = patch("conf.route_control.if_indextoname", side_effect=ifname)
        patcher.start()
        self.addCleanup(patcher.stop)

    @staticmethod
    def connected_route_msg(dst="10.122.0.0", dst_len=24, oif=2, **extra):
        msg = {
            "family": 2,
            "dst_len": dst_len,
            "attrs": [("RTA_OIF", oif), ("RTA_DST", dst)],
            "event": "RTM_NEWROUTE",
        }
        msg.update(extra)
        return msg

    def test_given_route_without_gateway_when_parse_message_then_entry_is_on_link(self):
        result = self.route_controller._parse_route_entry_msg(
            self.connected_route_msg()
        )
        self.assertIsInstance(result, RouteEntry)
        assert result is not None
        self.assertTrue(result.on_link)
        self.assertIsNone(result.next_hop_ip)
        self.assertEqual(result.dest_prefix, "10.122.0.0")
        self.assertEqual(result.prefix_len, 24)
        self.assertEqual(result.interface, "access")

    def test_given_local_table_route_when_parse_message_then_returns_none(self):
        for extra in (
            {"table": 255},
            {"type": 2},
            {"type": 3},
            {"scope": 254},
            {"scope": "host"},
        ):
            with self.subTest(extra=extra):
                self.assertIsNone(
                    self.route_controller._parse_route_entry_msg(
                        self.connected_route_msg(**extra)
                    )
                )

    def test_given_gatewayless_default_route_when_parse_message_then_returns_none(self):
        self.assertIsNone(
            self.route_controller._parse_route_entry_msg(
                self.connected_route_msg(dst_len=0)
            )
        )

    @patch("conf.route_control.fetch_mac")
    def test_given_attached_network_when_added_then_a_route_is_added_per_known_host(
        self, mock_fetch_mac
    ):
        mock_fetch_mac.return_value = DEFAULT_MAC_ADDRESS
        self.ndb.neighbours.dump.return_value = [
            {"ifindex": 2, "dst": "10.122.0.21", "lladdr": DEFAULT_MAC_ADDRESS},
            {"ifindex": 2, "dst": "10.122.0.22", "lladdr": DEFAULT_MAC_ADDRESS},
            # Outside the attached network.
            {"ifindex": 2, "dst": "10.199.0.9", "lladdr": DEFAULT_MAC_ADDRESS},
            # A different interface.
            {"ifindex": 7, "dst": "10.122.0.23", "lladdr": DEFAULT_MAC_ADDRESS},
            # Not resolved yet, so there is no MAC to stamp.
            {"ifindex": 2, "dst": "10.122.0.24", "lladdr": None},
        ]
        entry = self.route_controller._parse_route_entry_msg(self.connected_route_msg())
        assert entry is not None
        self.route_controller.add_new_route_entry(entry)

        added = [
            call.args[0]
            for call in self.mock_bess_controller.add_route_to_module.call_args_list
        ]
        self.assertEqual(
            sorted(e.dest_prefix for e in added), ["10.122.0.21", "10.122.0.22"]
        )
        for e in added:
            self.assertEqual(e.prefix_len, 32)
            self.assertEqual(e.next_hop_ip, e.dest_prefix)
            self.assertEqual(e.interface, "access")

    @patch("conf.route_control.fetch_mac")
    def test_given_attached_network_when_host_appears_then_its_route_is_added(
        self, mock_fetch_mac
    ):
        mock_fetch_mac.return_value = DEFAULT_MAC_ADDRESS
        entry = self.route_controller._parse_route_entry_msg(self.connected_route_msg())
        assert entry is not None
        self.route_controller.add_new_route_entry(entry)
        self.mock_bess_controller.add_route_to_module.reset_mock()

        self.route_controller.add_unresolved_new_neighbor(
            {
                "ifindex": 2,
                "attrs": [
                    ("NDA_DST", "10.122.0.21"),
                    ("NDA_LLADDR", DEFAULT_MAC_ADDRESS),
                ],
            }
        )

        self.mock_bess_controller.add_route_to_module.assert_called_once()
        added = self.mock_bess_controller.add_route_to_module.call_args.args[0]
        self.assertEqual(added.dest_prefix, "10.122.0.21")
        self.assertEqual(added.prefix_len, 32)

    @patch("conf.route_control.fetch_mac")
    def test_given_no_attached_network_when_host_appears_then_no_route_is_added(
        self, mock_fetch_mac
    ):
        mock_fetch_mac.return_value = DEFAULT_MAC_ADDRESS
        self.route_controller.add_unresolved_new_neighbor(
            {
                "ifindex": 2,
                "attrs": [
                    ("NDA_DST", "10.122.0.21"),
                    ("NDA_LLADDR", DEFAULT_MAC_ADDRESS),
                ],
            }
        )
        self.mock_bess_controller.add_route_to_module.assert_not_called()

    @patch("conf.route_control.fetch_mac")
    def test_given_host_already_routed_when_it_appears_again_then_it_is_not_re_added(
        self, mock_fetch_mac
    ):
        mock_fetch_mac.return_value = DEFAULT_MAC_ADDRESS
        entry = self.route_controller._parse_route_entry_msg(self.connected_route_msg())
        assert entry is not None
        self.route_controller.add_new_route_entry(entry)
        neighbor = {
            "ifindex": 2,
            "attrs": [
                ("NDA_DST", "10.122.0.21"),
                ("NDA_LLADDR", DEFAULT_MAC_ADDRESS),
            ],
        }
        self.route_controller.add_unresolved_new_neighbor(neighbor)
        self.mock_bess_controller.add_route_to_module.reset_mock()

        self.route_controller.add_unresolved_new_neighbor(neighbor)

        self.mock_bess_controller.add_route_to_module.assert_not_called()

    @patch("conf.route_control.MAX_ON_LINK_NEIGHBORS_PER_NETWORK", 2)
    @patch("conf.route_control.fetch_mac")
    def test_given_attached_network_at_its_limit_when_host_appears_then_it_is_refused(
        self, mock_fetch_mac
    ):
        mock_fetch_mac.return_value = DEFAULT_MAC_ADDRESS
        self.ndb.neighbours.dump.return_value = [
            {"ifindex": 2, "dst": f"10.122.0.{n}", "lladdr": DEFAULT_MAC_ADDRESS}
            for n in (21, 22, 23)
        ]
        entry = self.route_controller._parse_route_entry_msg(self.connected_route_msg())
        assert entry is not None
        with self.assertLogs("conf.route_control", level="ERROR") as logs:
            self.route_controller.add_new_route_entry(entry)
        self.assertEqual(self.mock_bess_controller.add_route_to_module.call_count, 2)
        self.assertTrue(any("10.122.0.23" in line for line in logs.output))

    @patch("conf.route_control.fetch_mac")
    def test_given_attached_network_when_it_is_removed_then_its_host_routes_go_too(
        self, mock_fetch_mac
    ):
        mock_fetch_mac.return_value = DEFAULT_MAC_ADDRESS
        self.ndb.neighbours.dump.return_value = [
            {"ifindex": 2, "dst": "10.122.0.21", "lladdr": DEFAULT_MAC_ADDRESS},
        ]
        entry = self.route_controller._parse_route_entry_msg(self.connected_route_msg())
        assert entry is not None
        self.route_controller.add_new_route_entry(entry)

        self.route_controller.delete_route_entry(entry)

        self.mock_bess_controller.delete_module_route_entry.assert_called_once()
        removed = self.mock_bess_controller.delete_module_route_entry.call_args.args[0]
        self.assertEqual(removed.dest_prefix, "10.122.0.21")
        self.assertEqual(removed.prefix_len, 32)

    @patch("conf.route_control.fetch_mac")
    def test_given_host_route_when_the_host_falls_quiet_then_the_route_stays(
        self, mock_fetch_mac
    ):
        mock_fetch_mac.return_value = DEFAULT_MAC_ADDRESS
        self.ndb.neighbours.dump.return_value = [
            {"ifindex": 2, "dst": "10.122.0.21", "lladdr": DEFAULT_MAC_ADDRESS},
        ]
        entry = self.route_controller._parse_route_entry_msg(self.connected_route_msg())
        assert entry is not None
        self.route_controller.add_new_route_entry(entry)

        # The kernel ages the entry out and says so. The event is handled the
        # way the event thread handles it, so withdrawing the route on it would
        # fail here: dropping the route is what would discard the next traffic
        # to this host, and it is the tidy-up this test exists to refuse.
        self.ndb.neighbours.dump.return_value = []
        self.route_controller._handle_neighbor_event(
            {
                "event": "RTM_DELNEIGH",
                "ifindex": 2,
                "attrs": [
                    ("NDA_DST", "10.122.0.21"),
                    ("NDA_LLADDR", DEFAULT_MAC_ADDRESS),
                ],
            }
        )

        self.mock_bess_controller.delete_module_route_entry.assert_not_called()
        network = next(iter(self.route_controller._on_link_networks["access"]))
        self.assertIn(
            "10.122.0.21", self.route_controller._on_link_networks["access"][network]
        )

    @patch("conf.route_control.fetch_mac")
    def test_given_gateway_route_when_added_then_it_is_unchanged_by_on_link_handling(
        self, mock_fetch_mac
    ):
        mock_fetch_mac.return_value = DEFAULT_MAC_ADDRESS
        msg = self.connected_route_msg(dst="10.126.0.0")
        msg["attrs"].append(("RTA_GATEWAY", "10.122.0.1"))
        entry = self.route_controller._parse_route_entry_msg(msg)
        assert entry is not None
        self.assertFalse(entry.on_link)
        self.assertEqual(entry.next_hop_ip, "10.122.0.1")

        self.route_controller.add_new_route_entry(entry)

        self.mock_bess_controller.add_route_to_module.assert_called_once()
        added = self.mock_bess_controller.add_route_to_module.call_args.args[0]
        self.assertEqual(added.dest_prefix, "10.126.0.0")
        self.assertEqual(added.prefix_len, 24)

    @patch("conf.route_control.fetch_mac")
    def test_given_attached_network_when_reconfigured_then_host_routes_are_programmed_again(
        self, mock_fetch_mac
    ):
        mock_fetch_mac.return_value = DEFAULT_MAC_ADDRESS
        self.ndb.neighbours.dump.return_value = [
            {"ifindex": 2, "dst": "10.122.0.21", "lladdr": DEFAULT_MAC_ADDRESS},
        ]
        route_msg = self.connected_route_msg()
        entry = self.route_controller._parse_route_entry_msg(route_msg)
        assert entry is not None
        self.route_controller.add_new_route_entry(entry)
        self.assertEqual(self.mock_bess_controller.add_route_to_module.call_count, 1)

        self.ipr.get_routes.return_value = [route_msg]
        self.mock_bess_controller.add_route_to_module.reset_mock()
        with patch("conf.route_control.signal.pause"):
            self.route_controller.reconfigure(1)

        self.mock_bess_controller.add_route_to_module.assert_called_once()
        added = self.mock_bess_controller.add_route_to_module.call_args.args[0]
        self.assertEqual(added.dest_prefix, "10.122.0.21")
        self.assertEqual(added.prefix_len, 32)

    @patch("conf.route_control.fetch_mac")
    def test_given_gateway_on_an_attached_network_when_both_are_added_then_one_gate_is_shared(
        self, mock_fetch_mac
    ):
        mock_fetch_mac.return_value = DEFAULT_MAC_ADDRESS
        self.ndb.neighbours.dump.return_value = [
            {"ifindex": 2, "dst": "10.122.0.1", "lladdr": DEFAULT_MAC_ADDRESS},
        ]
        attached = self.route_controller._parse_route_entry_msg(
            self.connected_route_msg()
        )
        assert attached is not None
        self.route_controller.add_new_route_entry(attached)

        via_gateway_msg = self.connected_route_msg(dst="10.126.0.0")
        via_gateway_msg["attrs"].append(("RTA_GATEWAY", "10.122.0.1"))
        via_gateway = self.route_controller._parse_route_entry_msg(via_gateway_msg)
        assert via_gateway is not None
        self.route_controller.add_new_route_entry(via_gateway)

        gates = {
            call.kwargs["gate_idx"]
            for call in self.mock_bess_controller.add_route_to_module.call_args_list
        }
        self.assertEqual(len(gates), 1, "the shared next hop should share one gate")
        self.mock_bess_controller.create_module.assert_called_once()
        self.assertEqual(
            self.route_controller._neighbor_cache["10.122.0.1"].route_count, 2
        )

    @patch("conf.route_control.fetch_mac")
    def test_given_two_attached_networks_on_one_interface_then_both_are_served(
        self, mock_fetch_mac
    ):
        mock_fetch_mac.return_value = DEFAULT_MAC_ADDRESS
        self.ndb.neighbours.dump.return_value = [
            {"ifindex": 2, "dst": "10.122.0.21", "lladdr": DEFAULT_MAC_ADDRESS},
            {"ifindex": 2, "dst": "10.123.0.30", "lladdr": DEFAULT_MAC_ADDRESS},
        ]
        for prefix in ("10.122.0.0", "10.123.0.0"):
            entry = self.route_controller._parse_route_entry_msg(
                self.connected_route_msg(dst=prefix)
            )
            assert entry is not None
            self.route_controller.add_new_route_entry(entry)

        added = sorted(
            call.args[0].dest_prefix
            for call in self.mock_bess_controller.add_route_to_module.call_args_list
        )
        self.assertEqual(added, ["10.122.0.21", "10.123.0.30"])
        self.assertEqual(len(self.route_controller._on_link_networks["access"]), 2)

    @patch("conf.route_control.send_ping")
    @patch("conf.route_control.fetch_mac")
    def test_given_neighbor_event_when_host_is_added_then_nothing_is_probed(
        self, mock_fetch_mac, mock_send_ping
    ):
        mock_fetch_mac.return_value = None
        self.ndb.neighbours.dump.return_value = [
            {"ifindex": 2, "dst": "10.122.0.21", "lladdr": DEFAULT_MAC_ADDRESS},
        ]
        entry = self.route_controller._parse_route_entry_msg(self.connected_route_msg())
        assert entry is not None
        self.route_controller.add_new_route_entry(entry)
        self.route_controller.add_unresolved_new_neighbor(
            {
                "ifindex": 2,
                "attrs": [
                    ("NDA_DST", "10.122.0.22"),
                    ("NDA_LLADDR", DEFAULT_MAC_ADDRESS),
                ],
            }
        )

        mock_send_ping.assert_not_called()
        mock_fetch_mac.assert_not_called()
        added = sorted(
            call.args[0].dest_prefix
            for call in self.mock_bess_controller.add_route_to_module.call_args_list
        )
        self.assertEqual(added, ["10.122.0.21", "10.122.0.22"])

    @patch("conf.route_control.send_ping")
    @patch("conf.route_control.fetch_mac")
    def test_given_unresolved_gateway_on_an_attached_network_when_it_resolves_then_it_is_also_a_host(
        self, mock_fetch_mac, mock_send_ping
    ):
        mock_fetch_mac.return_value = None
        attached = self.route_controller._parse_route_entry_msg(
            self.connected_route_msg()
        )
        assert attached is not None
        self.route_controller.add_new_route_entry(attached)

        via_gateway_msg = self.connected_route_msg(dst="10.126.0.0")
        via_gateway_msg["attrs"].append(("RTA_GATEWAY", "10.122.0.1"))
        via_gateway = self.route_controller._parse_route_entry_msg(via_gateway_msg)
        assert via_gateway is not None
        self.route_controller.add_new_route_entry(via_gateway)
        self.mock_bess_controller.add_route_to_module.assert_not_called()

        self.route_controller.add_unresolved_new_neighbor(
            {
                "ifindex": 2,
                "attrs": [
                    ("NDA_DST", "10.122.0.1"),
                    ("NDA_LLADDR", DEFAULT_MAC_ADDRESS),
                ],
            }
        )

        added = sorted(
            call.args[0].dest_prefix
            for call in self.mock_bess_controller.add_route_to_module.call_args_list
        )
        self.assertEqual(added, ["10.122.0.1", "10.126.0.0"])
        gates = {
            call.kwargs["gate_idx"]
            for call in self.mock_bess_controller.add_route_to_module.call_args_list
        }
        self.assertEqual(len(gates), 1, "the shared next hop should share one gate")
        self.assertEqual(
            self.route_controller._neighbor_cache["10.122.0.1"].route_count, 2
        )

    @patch("conf.route_control.fetch_mac")
    def test_given_host_route_that_could_not_be_installed_then_it_is_retried(
        self, mock_fetch_mac
    ):
        mock_fetch_mac.return_value = DEFAULT_MAC_ADDRESS
        entry = self.route_controller._parse_route_entry_msg(self.connected_route_msg())
        assert entry is not None
        self.route_controller.add_new_route_entry(entry)
        network = next(iter(self.route_controller._on_link_networks["access"]))
        neighbor = {
            "ifindex": 2,
            "attrs": [
                ("NDA_DST", "10.122.0.21"),
                ("NDA_LLADDR", DEFAULT_MAC_ADDRESS),
            ],
        }

        self.mock_bess_controller.add_route_to_module.side_effect = Exception(
            "BESS is unreachable"
        )
        self.route_controller.add_unresolved_new_neighbor(neighbor)
        self.assertNotIn(
            "10.122.0.21", self.route_controller._on_link_networks["access"][network]
        )

        self.mock_bess_controller.add_route_to_module.side_effect = None
        self.route_controller.add_unresolved_new_neighbor(neighbor)
        self.assertIn(
            "10.122.0.21", self.route_controller._on_link_networks["access"][network]
        )
        added = self.mock_bess_controller.add_route_to_module.call_args_list[-1].args[0]
        self.assertEqual(added.dest_prefix, "10.122.0.21")
        self.assertEqual(added.prefix_len, 32)


class ThreadRecordingNdb:
    """Stands in for NDB and records the thread that queries it.

    NDB answers a query only once its event loop thread is free, so a query
    made on that thread, from inside an event handler, never returns.
    """

    def __init__(self, ifname: str, neighbors: list[dict]):
        self.query_threads: list[threading.Thread] = []
        self.task_manager = Mock()
        self.interfaces = self._Recording(self, lambda _: {"ifname": ifname})
        self.neighbours = Mock()
        self.neighbours.dump.side_effect = lambda: self._record(neighbors)

    def _record(self, value):
        self.query_threads.append(threading.current_thread())
        return value

    class _Recording:
        def __init__(self, ndb, lookup):
            self._ndb = ndb
            self._lookup = lookup

        def __getitem__(self, key):
            return self._ndb._record(self._lookup(key))


class TestNetlinkEventThread(unittest.TestCase):
    """Tests that netlink events are handled off NDB's event loop thread."""

    TIMEOUT_S = 5

    def setUp(self):
        self.mock_bess_controller = Mock(BessControllerMock)
        self.ndb = ThreadRecordingNdb(
            ifname="core",
            neighbors=[{"dst": "172.31.48.1", "lladdr": DEFAULT_MAC_ADDRESS}],
        )
        self.route_controller = RouteController(
            self.mock_bess_controller,
            self.ndb,
            interfaces=["access", "core"],
            ipr=Mock(),
        )
        self.route_controller.register_handlers()
        self.route_controller.start_handling_netlink_events()

    @staticmethod
    def new_route_message(dst: str) -> dict:
        return {
            "family": 2,
            "dst_len": 24,
            "flags": 0,
            "attrs": [
                ("RTA_TABLE", 254),
                ("RTA_GATEWAY", "172.31.48.1"),
                ("RTA_OIF", 2),
                ("RTA_DST", dst),
            ],
            "event": "RTM_NEWROUTE",
        }

    def deliver_from_ndb_thread(self, handler, netlink_message) -> threading.Thread:
        """Runs a handler the way NDB does: awaited on its own loop thread."""
        ndb_thread = threading.Thread(
            target=lambda: asyncio.run(handler(self.ndb, netlink_message)),
            name="ndb-event-loop",
        )
        ndb_thread.start()
        ndb_thread.join(self.TIMEOUT_S)
        self.assertFalse(ndb_thread.is_alive(), "the NDB handler did not return")
        return ndb_thread

    def wait_for_routes(self, count: int) -> list[str]:
        """Waits until count routes reached BESS and returns their prefixes."""
        deadline = time.monotonic() + self.TIMEOUT_S
        while time.monotonic() < deadline:
            calls = self.mock_bess_controller.add_route_to_module.call_args_list
            if len(calls) >= count:
                return [call.args[0].dest_prefix for call in calls]
            time.sleep(0.01)
        self.fail(f"fewer than {count} routes reached BESS")

    def test_given_new_route_event_when_ndb_delivers_it_then_ndb_is_queried_off_its_event_thread(
        self,
    ):
        ndb_thread = self.deliver_from_ndb_thread(
            self.route_controller._netlink_route_handler,
            self.new_route_message("192.168.1.0"),
        )

        self.assertEqual(self.wait_for_routes(1), ["192.168.1.0"])
        self.assertTrue(self.ndb.query_threads, "NDB was never queried")
        self.assertNotIn(ndb_thread, self.ndb.query_threads)

    def test_given_route_events_after_startup_when_ndb_delivers_them_then_every_route_is_added_in_order(
        self,
    ):
        prefixes = ["192.168.1.0", "192.168.2.0", "192.168.3.0"]
        for prefix in prefixes:
            self.deliver_from_ndb_thread(
                self.route_controller._netlink_route_handler,
                self.new_route_message(prefix),
            )

        self.assertEqual(self.wait_for_routes(len(prefixes)), prefixes)

    @patch.object(RouteController, "_handle_route_event")
    def test_given_route_event_when_ndb_handler_runs_then_event_is_only_queued(
        self, mock_handle_route_event
    ):
        # Patches put on the queue the worker already waits on, so the worker
        # keeps waiting on a real queue however the threads are scheduled.
        with patch.object(self.route_controller._netlink_events, "put") as mock_put:
            asyncio.run(
                self.route_controller._netlink_route_handler(
                    self.ndb, self.new_route_message("192.168.1.0")
                )
            )

        mock_put.assert_called_once()
        mock_handle_route_event.assert_not_called()
        self.assertEqual(self.ndb.query_threads, [])

    @patch.object(RouteController, "add_unresolved_new_neighbor")
    def test_given_neighbor_event_when_ndb_handler_runs_then_it_is_handled_on_the_event_thread(
        self, mock_add_unresolved_new_neighbor
    ):
        handled = threading.Event()
        handled_on: list[threading.Thread] = []

        def record(_):
            handled_on.append(threading.current_thread())
            handled.set()

        mock_add_unresolved_new_neighbor.side_effect = record

        self.deliver_from_ndb_thread(
            self.route_controller._netlink_neighbor_handler, {"event": "RTM_NEWNEIGH"}
        )

        self.assertTrue(handled.wait(self.TIMEOUT_S), "the neighbor was not handled")
        self.assertEqual(handled_on, [self.route_controller._netlink_event_thread])

    def test_given_event_that_fails_when_handled_then_later_events_are_still_handled(
        self,
    ):
        def fail(_):
            raise RuntimeError("handling failed")

        self.route_controller._netlink_events.put((fail, {}))
        self.deliver_from_ndb_thread(
            self.route_controller._netlink_route_handler,
            self.new_route_message("192.168.1.0"),
        )

        self.assertEqual(self.wait_for_routes(1), ["192.168.1.0"])

    def test_given_event_thread_started_when_started_again_then_one_event_thread(
        self,
    ):
        event_thread = self.route_controller._netlink_event_thread

        self.route_controller.start_handling_netlink_events()

        self.assertIs(self.route_controller._netlink_event_thread, event_thread)
        self.assertTrue(event_thread.is_alive())


def route_message(event: str, dst: str, gateway: str = "172.31.48.1") -> dict:
    """A netlink route message for a /24 via gateway on interface 2."""
    return {
        "family": 2,
        "dst_len": 24,
        "flags": 0,
        "attrs": [
            ("RTA_TABLE", 254),
            ("RTA_GATEWAY", gateway),
            ("RTA_OIF", 2),
            ("RTA_DST", dst),
        ],
        "event": event,
    }


@patch("conf.route_control.send_ping", Mock())
@patch("conf.route_control.fetch_mac", Mock(return_value=DEFAULT_MAC_ADDRESS))
class TestStartupOverlap(unittest.TestCase):
    """Tests route changes made while route_control reads the routes at start.

    The handlers are registered before the routes are read, so a change made
    in between is both in what is read and in the queued events.
    """

    TIMEOUT_S = 5
    ROUTE = "192.168.1.0"
    GATEWAY = "172.31.48.1"

    def setUp(self):
        self.mock_bess_controller = Mock(BessControllerMock)
        self.ndb = Mock()
        self.ndb.interfaces = {2: {"ifname": "core"}}
        self.ipr = Mock()
        self.ipr.get_routes.return_value = []
        self.route_controller = RouteController(
            self.mock_bess_controller,
            self.ndb,
            interfaces=["access", "core"],
            ipr=self.ipr,
        )

    def deliver(self, *netlink_messages: dict) -> None:
        """Delivers route events through the NDB handler, as NDB does."""
        for netlink_message in netlink_messages:
            asyncio.run(
                self.route_controller._netlink_route_handler(self.ndb, netlink_message)
            )

    def start_with(self, routes_read: list[dict], events: list[dict]) -> None:
        """Starts the controller with events that arrive while routes_read
        are being read, and waits until every event has been handled."""

        def read_routes(family):
            self.deliver(*events)
            return routes_read

        self.ipr.get_routes.side_effect = read_routes
        self.route_controller.start()
        self.wait_for_queued_events()

    def wait_for_queued_events(self) -> None:
        handled = threading.Event()
        self.route_controller._netlink_events.put((lambda _: handled.set(), None))
        self.assertTrue(
            handled.wait(self.TIMEOUT_S), "the queued events were not handled"
        )

    def neighbor(self):
        return self.route_controller._neighbor_cache.get(self.GATEWAY)

    def test_given_handlers_when_start_then_registered_before_routes_are_read(self):
        order = Mock()
        controller = self.route_controller
        with (
            patch.object(controller, "register_handlers", order.register_handlers),
            patch.object(controller, "bootstrap_routes", order.bootstrap_routes),
            patch.object(
                controller,
                "start_handling_netlink_events",
                order.start_handling_netlink_events,
            ),
            patch.object(
                controller,
                "start_pinging_missing_entries",
                order.start_pinging_missing_entries,
            ),
        ):
            controller.start()

        self.assertEqual(
            [name for name, _, _ in order.mock_calls],
            [
                "register_handlers",
                "bootstrap_routes",
                "start_handling_netlink_events",
                "start_pinging_missing_entries",
            ],
        )

    def test_given_route_added_while_routes_are_read_when_start_then_added_once(self):
        self.start_with(
            routes_read=[route_message("RTM_NEWROUTE", self.ROUTE)],
            events=[route_message("RTM_NEWROUTE", self.ROUTE)],
        )

        self.mock_bess_controller.add_route_to_module.assert_called_once()
        self.assertEqual(self.neighbor().route_count, 1)

    def test_given_route_deleted_after_it_was_read_when_start_then_route_deleted(
        self,
    ):
        self.start_with(
            routes_read=[route_message("RTM_NEWROUTE", self.ROUTE)],
            events=[route_message("RTM_DELROUTE", self.ROUTE)],
        )

        self.mock_bess_controller.delete_module_route_entry.assert_called_once()
        self.mock_bess_controller.delete_module.assert_called_once()
        self.assertIsNone(self.neighbor())

    def test_given_route_added_and_deleted_before_it_was_read_when_start_then_not_left_in_bess(
        self,
    ):
        self.start_with(
            routes_read=[],
            events=[
                route_message("RTM_NEWROUTE", self.ROUTE),
                route_message("RTM_DELROUTE", self.ROUTE),
            ],
        )

        self.mock_bess_controller.delete_module_route_entry.assert_called_once()
        self.assertIsNone(self.neighbor())

    def test_given_route_added_twice_when_deleted_once_then_next_hop_released(self):
        route_entry = RouteEntry(
            next_hop_ip=self.GATEWAY,
            interface="core",
            dest_prefix=self.ROUTE,
            prefix_len=24,
        )

        self.route_controller.add_new_route_entry(route_entry)
        self.route_controller.add_new_route_entry(route_entry)
        self.route_controller.delete_route_entry(route_entry)

        self.mock_bess_controller.add_route_to_module.assert_called_once()
        self.mock_bess_controller.delete_module.assert_called_once()
        self.assertIsNone(self.neighbor())

    def test_given_route_not_in_bess_when_deleted_then_shared_next_hop_kept(self):
        in_bess = RouteEntry(
            next_hop_ip=self.GATEWAY,
            interface="core",
            dest_prefix=self.ROUTE,
            prefix_len=24,
        )
        never_added = replace(in_bess, dest_prefix="192.168.2.0")

        self.route_controller.add_new_route_entry(in_bess)
        self.route_controller.delete_route_entry(never_added)

        self.mock_bess_controller.delete_module_route_entry.assert_not_called()
        self.mock_bess_controller.delete_module.assert_not_called()
        self.assertEqual(self.neighbor().route_count, 1)

    @patch("conf.route_control.signal.pause", Mock())
    def test_given_route_in_bess_when_reconfigure_then_route_added_again(self):
        self.ipr.get_routes.return_value = [route_message("RTM_NEWROUTE", self.ROUTE)]
        self.route_controller.bootstrap_routes()

        self.route_controller.reconfigure(1)

        self.assertEqual(self.mock_bess_controller.add_route_to_module.call_count, 2)
        self.assertEqual(self.neighbor().route_count, 1)

    def test_given_route_waiting_for_arp_when_installed_then_neighbor_event_does_not_install_it_again(
        self,
    ):
        """A next hop resolving must not install a route that is already in BESS.

        add_unresolved_new_neighbor installs the waiting route itself, so the
        ledger has to be consulted there too: installing it twice counted it
        twice against its next hop, and one delete then left the next hop's
        Update module and gate behind.
        """
        route_entry = RouteEntry(
            next_hop_ip=self.GATEWAY,
            interface="core",
            dest_prefix=self.ROUTE,
            prefix_len=24,
        )
        # the state _probe_addr leaves behind while the next hop is unresolved
        self.route_controller._unresolved_arp_queries_cache[self.GATEWAY] = route_entry

        # the next hop resolves and the route is installed through the route path
        self.route_controller.add_new_route_entry(route_entry)
        self.mock_bess_controller.add_route_to_module.assert_called_once()

        # the neighbor event for that next hop must find the route already there
        self.route_controller.add_unresolved_new_neighbor(
            {
                "attrs": {
                    "NDA_DST": self.GATEWAY,
                    "NDA_LLADDR": DEFAULT_MAC_ADDRESS,
                },
            }
        )

        self.mock_bess_controller.add_route_to_module.assert_called_once()
        self.assertEqual(self.neighbor().route_count, 1)

        # and one delete is enough to release the next hop
        self.route_controller.delete_route_entry(route_entry)
        self.mock_bess_controller.delete_module.assert_called_once()
        self.assertIsNone(self.neighbor())

    def test_given_bess_refuses_a_delete_when_deleted_again_then_bess_is_asked_again(
        self,
    ):
        """A route BESS refused to drop stays in the ledger, so a later delete
        retries it instead of reporting a route that was never there."""
        route_entry = RouteEntry(
            next_hop_ip=self.GATEWAY,
            interface="core",
            dest_prefix=self.ROUTE,
            prefix_len=24,
        )
        self.route_controller.add_new_route_entry(route_entry)

        self.mock_bess_controller.delete_module_route_entry.side_effect = Exception(
            "BESS refused the delete"
        )
        self.route_controller.delete_route_entry(route_entry)

        self.assertEqual(
            self.mock_bess_controller.delete_module_route_entry.call_count, 1
        )
        self.assertIn(route_key(route_entry), self.route_controller._programmed_routes)
        self.assertEqual(self.neighbor().route_count, 1)

        self.mock_bess_controller.delete_module_route_entry.side_effect = None
        self.route_controller.delete_route_entry(route_entry)

        self.assertEqual(
            self.mock_bess_controller.delete_module_route_entry.call_count, 2
        )
        self.assertNotIn(
            route_key(route_entry), self.route_controller._programmed_routes
        )
        self.assertIsNone(self.neighbor())

    def test_given_route_deleted_while_waiting_then_resolving_its_next_hop_does_not_install_it(
        self,
    ):
        """A delete must cancel a route that is still waiting for its next hop.

        start() replays the events from the route read, so a delete can follow
        an add of a route whose next hop is unresolved. Without the delete
        taking it off the queue, the next hop resolving installs a route the
        kernel no longer has.
        """
        route_entry = RouteEntry(
            next_hop_ip=self.GATEWAY,
            interface="core",
            dest_prefix=self.ROUTE,
            prefix_len=24,
        )
        # the state _probe_addr leaves behind while the next hop is unresolved
        self.route_controller._unresolved_arp_queries_cache[self.GATEWAY] = route_entry

        self.route_controller.delete_route_entry(route_entry)
        self.route_controller.add_unresolved_new_neighbor(
            {
                "attrs": {
                    "NDA_DST": self.GATEWAY,
                    "NDA_LLADDR": DEFAULT_MAC_ADDRESS,
                },
            }
        )

        self.mock_bess_controller.add_route_to_module.assert_not_called()
        self.assertEqual(self.route_controller._unresolved_arp_queries_cache, {})

    def test_given_ipv6_default_route_deleted_then_the_ipv4_default_is_left_alone(self):
        """An IPv6 default route shares the IPv4 default's key.

        It carries no RTA_DST, so it parses to 0.0.0.0/0. Looking that key up
        and substituting the recorded next hop would delete the IPv4 default.
        """
        ipv4_default = RouteEntry(
            next_hop_ip=self.GATEWAY,
            interface="core",
            dest_prefix="0.0.0.0",
            prefix_len=0,
        )
        self.route_controller.add_new_route_entry(ipv4_default)
        self.mock_bess_controller.add_route_to_module.assert_called_once()

        self.route_controller.delete_route_entry(
            replace(ipv4_default, next_hop_ip="fe80::1")
        )

        self.mock_bess_controller.delete_module_route_entry.assert_not_called()
        self.assertIn(route_key(ipv4_default), self.route_controller._programmed_routes)
        self.assertEqual(self.neighbor().route_count, 1)

    def test_given_route_moved_to_another_next_hop_then_the_old_one_is_released(self):
        """A route changing next hop must give the old next hop's reference back.

        Reachable without `ip route replace`: the dump returns a prefix via B
        while the queue holds NEW via A, DEL via A, NEW via B, and replaying
        those used to leave B counted twice for one installed route.
        """
        other_gateway = "172.31.48.2"
        via_first = RouteEntry(
            next_hop_ip=self.GATEWAY,
            interface="core",
            dest_prefix=self.ROUTE,
            prefix_len=24,
        )
        via_second = replace(via_first, next_hop_ip=other_gateway)

        self.route_controller.add_new_route_entry(via_first)
        self.assertEqual(self.neighbor().route_count, 1)

        self.route_controller.add_new_route_entry(via_second)

        self.assertIsNone(self.neighbor())
        self.assertEqual(
            self.route_controller._neighbor_cache[other_gateway].route_count, 1
        )
        self.assertEqual(
            self.route_controller._programmed_routes[route_key(via_first)],
            other_gateway,
        )

    def test_given_waiting_route_moved_off_programmed_next_hop_when_it_resolves_then_installed_without_error(
        self,
    ):
        """Resolving the next hop of a moved route must not raise.

        The route is in BESS via the first gateway and waits for the second.
        Installing it releases the first, which must not cancel the waiting
        entry being resolved, or removing that entry afterwards raises.
        """
        other_gateway = "172.31.48.2"
        via_first = RouteEntry(
            next_hop_ip=self.GATEWAY,
            interface="core",
            dest_prefix=self.ROUTE,
            prefix_len=24,
        )
        via_second = replace(via_first, next_hop_ip=other_gateway)
        self.route_controller.add_new_route_entry(via_first)
        # the state _probe_addr leaves behind while the next hop is unresolved
        self.route_controller._unresolved_arp_queries_cache[other_gateway] = via_second

        self.route_controller.add_unresolved_new_neighbor(
            {
                "attrs": {
                    "NDA_DST": other_gateway,
                    "NDA_LLADDR": DEFAULT_MAC_ADDRESS,
                },
            }
        )

        self.assertEqual(self.route_controller._unresolved_arp_queries_cache, {})
        self.assertIsNone(self.neighbor())
        self.assertEqual(
            self.route_controller._programmed_routes[route_key(via_first)],
            other_gateway,
        )

    def test_given_bess_refuses_the_release_when_route_moves_then_ledger_and_next_hop_are_kept(
        self,
    ):
        """A move whose release BESS refuses must not install the new next hop.

        Installing over the old route would overwrite the ledger entry and
        leave the first next hop counted for a route it no longer carries.
        """
        other_gateway = "172.31.48.2"
        via_first = RouteEntry(
            next_hop_ip=self.GATEWAY,
            interface="core",
            dest_prefix=self.ROUTE,
            prefix_len=24,
        )
        via_second = replace(via_first, next_hop_ip=other_gateway)
        self.route_controller.add_new_route_entry(via_first)
        self.mock_bess_controller.add_route_to_module.reset_mock()

        self.mock_bess_controller.delete_module_route_entry.side_effect = Exception(
            "BESS refused the delete"
        )
        self.assertFalse(
            self.route_controller._add_neighbor(via_second, DEFAULT_MAC_ADDRESS)
        )

        self.mock_bess_controller.add_route_to_module.assert_not_called()
        self.assertEqual(
            self.route_controller._programmed_routes[route_key(via_first)],
            self.GATEWAY,
        )
        self.assertEqual(self.neighbor().route_count, 1)
        self.assertNotIn(other_gateway, self.route_controller._neighbor_cache)
