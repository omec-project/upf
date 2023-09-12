#!/usr/bin/env python
# SPDX-License-Identifier: Apache-2.0
# Copyright 2023 Canonical Ltd.

import sys
import unittest
from unittest.mock import MagicMock, Mock, patch

from pyroute2 import IPDB  # type: ignore[import]

sys.modules["pybess.bess"] = MagicMock()

from conf.route_control import (  # noqa: E402
    NeighborEntry,
    RouteController,
    RouteEntry,
    fetch_mac,
    mac_to_hex,
    mac_to_int,
    validate_ipv4,
)


class BessControllerMock(object):
    """Mock of BessController to avoid using BESS from pybess.bess"""

    def __init__(self):
        pass

    def get_bess(self, *args, **kwargs) -> None:
        pass

    def add_route_to_module(self, *args, **kwargs) -> None:
        pass

    def del_route_entry(self, *args, **kwargs) -> None:
        pass

    def create_module(self, *args, **kwargs) -> None:
        pass

    def del_module_route_entry(self, *args, **kwargs) -> None:
        pass

    def link_modules(self, *args, **kwargs) -> None:
        pass


@patch("conf.route_control.BessController", BessControllerMock)
class TestUtilityFunctions(unittest.TestCase):
    """Tests utility functions in route_control.py."""

    def test_given_valid_ip_when_validate_ipv4_then_returns_true(self):
        self.assertTrue(validate_ipv4("192.168.1.1"))
        self.assertFalse(validate_ipv4("192.168.300.1"))

    def test_given_invalid_ip_when_validate_ipv4_then_returns_false(self):
        self.assertFalse(validate_ipv4("::1"))
        self.assertFalse(validate_ipv4(""))

    def test_given_valid_mac_when_mac_to_int_then_returns_int_representation(
        self
    ):
        self.assertEqual(mac_to_int("00:1a:2b:3c:4d:5e"), 112394521950)

    def test_given_invalid_mac_when_mac_to_int_then_raises_exception(self):
        with self.assertRaises(ValueError):
            mac_to_int("not a mac")

    def test_given_valid_mac_when_mac_to_hex_then_return_hex_string_representation(
        self
    ):
        self.assertEqual(mac_to_hex("00:1a:2b:3c:4d:5e"), "001A2B3C4D5E")

    def test_given_known_destination_when_fetch_mac_then_returns_mac(self):
        ipdb = IPDB()
        ipdb.nl.get_neighbours = lambda dst, **kwargs: [
            {"attrs": [("NDA_DST", dst), ("NDA_LLADDR", "00:1a:2b:3c:4d:5e")]}
        ]
        self.assertEqual(fetch_mac(ipdb, "192.168.1.1"), "00:1a:2b:3c:4d:5e")

    def test_given_unkonw_destination_when_fetch_mac_then_returns_none(self):
        ipdb = IPDB()
        ipdb.nl.get_neighbours = lambda dst, **kwargs: []
        self.assertIsNone(fetch_mac(ipdb, "192.168.1.1"))


class TestRouteController(unittest.TestCase):
    def setUp(self):
        self.mock_bess_controller = Mock(BessControllerMock)
        self.ipdb = Mock()
        self.ipr = Mock()
        interfaces = ['access', 'core']
        self.route_controller = RouteController(
            self.mock_bess_controller,
            self.ipdb,
            interfaces=interfaces,
            ipr=self.ipr,
        )

    def test_given_valid_route_message_when_parse_message_then_parses_message(
        self
    ):
        self.ipdb.interfaces = {2: Mock(ifname='core')}
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
        result = self.route_controller._parse_route_entry_msg(
            example_route_entry
        )
        self.assertIsInstance(result, RouteEntry)
        self.assertEqual(result.dest_prefix, "192.168.1.0")
        self.assertEqual(result.next_hop_ip, "172.31.48.1")
        self.assertEqual(result.interface, self.ipdb.interfaces[2].ifname)
        self.assertEqual(result.prefix_len, 24)

    def test_given_valid_route_message_and_dst_len_is_zero_when_parse_message_then_parses_message_as_default_route(
        self,
    ):
        self.ipdb.interfaces = {2: Mock(ifname='core')}
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
        result = self.route_controller._parse_route_entry_msg(
            example_route_entry
        )
        self.assertIsInstance(result, RouteEntry)
        self.assertEqual(result.dest_prefix, "0.0.0.0")
        self.assertEqual(result.next_hop_ip, "172.31.48.1")
        self.assertEqual(
            result.interface, self.ipdb.interfaces[2].ifname
        )
        self.assertEqual(result.prefix_len, 0)

    def test_given_invalid_route_message_when_parse_message_then_returns_none(
        self
    ):
        self.ipdb.interfaces = {2: Mock(ifname='not the needed interface')}
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
        result = self.route_controller._parse_route_entry_msg(
            example_route_entry
        )
        self.assertIsNone(result)

    @patch.object(RouteController, "_handle_new_route_entry")
    @patch.object(RouteController, "_parse_route_entry_msg")
    def test_given_new_route_when_bootstrap_routes_then_handle_new_entry_is_called(
        self,
        mock_parse_route_entry,
        mock_handle_new_route_entry,
    ):
        mock_routes = [
            {"event": "RTM_NEWROUTE"},
            {"event": "OTHER_ACTION"}
        ]
        self.ipr.get_routes.return_value = mock_routes
        mock_valid_route_entry = Mock()
        mock_parse_route_entry.return_value = mock_valid_route_entry

        self.route_controller.bootstrap_routes()

        self.ipr.get_routes.assert_called_once()
        mock_parse_route_entry.assert_called_with(mock_routes[0])
        mock_handle_new_route_entry.assert_called_with(mock_valid_route_entry)

    @patch.object(RouteController, "_handle_new_route_entry")
    @patch.object(RouteController, "_parse_route_entry_msg")
    def test_given_no_new_route_when_bootstrap_routes_then_handle_new_entry_is_not_called(
        self,
        mock_parse_route_entry,
        mock_handle_new_route_entry,
    ):
        mock_routes = [
            {"event": "NOT_RTM_NEWROUTE"},
            {"event": "OTHER_ACTION"}
        ]
        self.ipr.get_routes.return_value = mock_routes
        self.route_controller._parse_route_entry_msg = Mock()
        self.route_controller.bootstrap_routes()

        self.ipr.get_routes.assert_called_once()
        mock_parse_route_entry.assert_not_called()
        mock_handle_new_route_entry.assert_not_called()

    @patch.object(RouteController, "_handle_new_route_entry")
    @patch.object(RouteController, "_parse_route_entry_msg")
    def test_given_new_route_and_invalid_message_when_bootstrap_routes_then_handle_new_entry_is_not_called(
        self,
        mock_parse_route_entry,
        mock_handle_new_route_entry,
    ):
        mock_routes = [
            {"event": "RTM_NEWROUTE"},
            {"event": "OTHER_ACTION"}
        ]
        self.ipr.get_routes.return_value = mock_routes
        mock_parse_route_entry.return_value = None
        self.route_controller.bootstrap_routes()

        self.ipr.get_routes.assert_called_once()
        mock_parse_route_entry.assert_called_with(mock_routes[0])
        mock_handle_new_route_entry.assert_not_called()

    @patch.object(RouteController, "_parse_route_entry_msg")
    @patch.object(RouteController, "_probe_addr")
    @patch("conf.route_control.fetch_mac")
    def test_given_valid_route_entry_and_mac_not_know_when_bootstrap_routes_then_calls_probe_address(
        self,
        mock_fetch_mac,
        mock_probe_addr,
        mock_parse_route_entry,
    ):
        mock_routes = [
            {"event": "RTM_NEWROUTE"},
            {"event": "OTHER_ACTION"}
        ]
        self.ipr.get_routes.return_value = mock_routes
        mock_valid_route_entry = RouteEntry(
            next_hop_ip="172.31.48.1",
            interface="core",
            dest_prefix="192.168.1.0",
            prefix_len=24,
        )
        mock_parse_route_entry.return_value = mock_valid_route_entry

        mock_fetch_mac.return_value = None

        self.route_controller.bootstrap_routes()

        self.ipr.get_routes.assert_called_once()
        mock_parse_route_entry.assert_called_with(mock_routes[0])
        mock_probe_addr.assert_called_with(mock_valid_route_entry)

    @patch.object(RouteController, "_parse_route_entry_msg")
    @patch("conf.route_control.fetch_mac")
    @patch.object(RouteController, "_get_gate_idx")
    def test_given_valid_route_entry_and_mac_is_known_and_neighbor_known_when_bootstrap_routes_then_route_is_added_in_bess(
        self,
        mock_get_gate_index,
        mock_fetch_mac,
        mock_parse_route_entry,
    ):
        mock_routes = [
            {"event": "RTM_NEWROUTE"},
            {"event": "OTHER_ACTION"}
        ]
        self.ipr.get_routes.return_value = mock_routes
        mock_valid_route_entry = RouteEntry(
            next_hop_ip="172.31.48.1",
            interface="core",
            dest_prefix="192.168.1.0",
            prefix_len=24,
        )
        mock_parse_route_entry.return_value = mock_valid_route_entry

        mock_fetch_mac.return_value = "mac"
        mock_get_gate_index.return_value = 0
        self.route_controller._neighbor_cache[mock_valid_route_entry.next_hop_ip] = NeighborEntry(mac_address="mac")

        self.route_controller.bootstrap_routes()

        self.ipr.get_routes.assert_called_once()
        mock_parse_route_entry.assert_called_with(mock_routes[0])
        self.mock_bess_controller.add_route_to_module.assert_called()

    @patch.object(RouteController, "_parse_route_entry_msg")
    @patch("conf.route_control.fetch_mac")
    @patch.object(RouteController, "_get_gate_idx")
    def test_given_valid_route_entry_and_mac_is_known_when_bootstrap_routes_then_route_count_is_increased(
        self,
        mock_get_gate_index,
        mock_fetch_mac,
        mock_parse_route_entry,
    ):
        mock_routes = [
            {"event": "RTM_NEWROUTE"},
            {"event": "OTHER_ACTION"}
        ]
        self.ipr.get_routes.return_value = mock_routes
        mock_valid_route_entry = RouteEntry(
            next_hop_ip="172.31.48.1",
            interface="core",
            dest_prefix="192.168.1.0",
            prefix_len=24,
        )
        mock_parse_route_entry.return_value = mock_valid_route_entry

        mock_fetch_mac.return_value = "mac"
        mock_get_gate_index.return_value = 0
        self.route_controller._neighbor_cache[mock_valid_route_entry.next_hop_ip] = NeighborEntry(mac_address="mac")
        self.assertEqual(
            self.route_controller._neighbor_cache[mock_valid_route_entry.next_hop_ip].route_count, 0
        )

        self.route_controller.bootstrap_routes()

        self.assertEqual(
            self.route_controller._neighbor_cache[mock_valid_route_entry.next_hop_ip].route_count, 1
        )

    @patch.object(RouteController, "_parse_route_entry_msg")
    @patch("conf.route_control.fetch_mac")
    @patch.object(RouteController, "_get_gate_idx")
    def test_given_valid_route_entry_and_mac_is_known_and_neighbor_not_known_when_bootstrap_routes_then_update_module_is_created_and_modules_are_linked(
        self,
        mock_get_gate_index,
        mock_fetch_mac,
        mock_parse_route_entry,
    ):
        mock_routes = [
            {"event": "RTM_NEWROUTE"},
            {"event": "OTHER_ACTION"}
        ]
        self.ipr.get_routes.return_value = mock_routes
        mock_valid_route_entry = RouteEntry(
            next_hop_ip="172.31.48.1",
            interface="core",
            dest_prefix="192.168.1.0",
            prefix_len=24,
        )
        mock_parse_route_entry.return_value = mock_valid_route_entry

        mock_fetch_mac.return_value = "01:23:45:67:89:AB"
        mock_get_gate_index.return_value = 0

        self.route_controller.bootstrap_routes()

        self.assertEqual(
            self.route_controller._neighbor_cache[mock_valid_route_entry.next_hop_ip].route_count, 1
        )
        self.mock_bess_controller.create_module.assert_called()
        self.mock_bess_controller.link_modules.assert_called()

    @patch.object(RouteController, "_handle_new_route_entry")
    def test_given_netlink_message_when_rtm_newroute_event_then_handle_new_route_entry_is_called(
        self, mock_handle_new_route_entry
    ):
        self.ipdb.interfaces = {2: Mock(ifname='core')}
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
        self.route_controller._netlink_event_listener(
            self.ipdb, example_route_entry, "RTM_NEWROUTE"
        )
        mock_handle_new_route_entry.assert_called()

    @patch.object(RouteController, "_delete_route")
    def test_given_netlink_message_when_rtm_delroute_event_then_del_route_is_called(
        self, mock_delete_route
    ):
        self.ipdb.interfaces = {2: Mock(ifname='core')}
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
        self.route_controller._netlink_event_listener(
            self.ipdb, example_route_entry, "RTM_DELROUTE"
        )
        mock_delete_route.assert_called()

    @patch.object(RouteController, "_parse_new_neighbor")
    def test_given_netlink_message_when_rtm_newneigh_event_then_parse_new_neighbor_is_called(
        self, mock_parse_new_neighbor
    ):
        self.route_controller._netlink_event_listener(
            self.ipdb, "new neighbour message", "RTM_NEWNEIGH"
        )
        mock_parse_new_neighbor.assert_called()
