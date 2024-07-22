#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright 2023 Canonical Ltd.

import sys
import unittest
from unittest.mock import MagicMock, Mock, patch

sys.modules["pybess.bess"] = MagicMock()

from conf.route_control import (RouteController, RouteEntry,
                                fetch_mac, mac_to_hex, mac_to_int,
                                validate_ipv4)


class BessControllerMock(object):
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
        self.assertEqual(mac_to_int("00:1a:2b:3c:4d:5e"), 112394521950)

    def test_given_invalid_mac_when_mac_to_int_then_raises_exception(self):
        with self.assertRaises(ValueError):
            mac_to_int("not a mac")

    def test_given_valid_mac_when_mac_to_hex_then_return_hex_string_representation(
        self,
    ):
        self.assertEqual(mac_to_hex("00:1a:2b:3c:4d:5e"), "001A2B3C4D5E")

    def test_given_known_destination_when_fetch_mac_then_returns_mac(self):
        ndb = Mock()
        kwargs = {
            "ifindex": 1,
            "dst": "192.168.1.1",
            "lladdr": "00:1a:2b:3c:4d:5e"
        }
        ndb.neighbours.dump.return_value = [kwargs]
        self.assertEqual(fetch_mac(ndb, "192.168.1.1"), "00:1a:2b:3c:4d:5e")

    def test_given_unknown_destination_when_fetch_mac_then_returns_none(self):
        ndb = Mock()
        kwargs = {
            "ifindex": 1,
            "dst": "192.168.1.1",
            "lladdr": None
        }
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
        kwargs = {
            "ifindex": 1,
            "dst": "192.168.1.1",
            "lladdr": "00:1a:2b:3c:4d:5e"
        }
        self.ndb.neighbours.dump.return_value = [kwargs]
        mock_get_update_module_name.return_value = "merge_module"
        mock_get_route_module_name.return_value = "route_module"
        mock_get_merge_module_name.return_value = "update_module"
        mock_fetch_mac.return_value = "00:1a:2b:3c:4d:5e"
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
        self.assertEqual(result.dest_prefix, "192.168.1.0")
        self.assertEqual(result.next_hop_ip, "172.31.48.1")
        self.assertEqual(result.interface, self.ndb.interfaces[2].get("ifname"))
        self.assertEqual(result.prefix_len, 24)

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
        self.assertEqual(result.dest_prefix, "0.0.0.0")
        self.assertEqual(result.next_hop_ip, "172.31.48.1")
        self.assertEqual(result.interface, self.ndb.interfaces[2].get("ifname"))
        self.assertEqual(result.prefix_len, 0)

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
        kwargs = {
            "ifindex": 1,
            "dst": "192.168.1.1",
            "lladdr": None
        }
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
        kwargs = {
            "ifindex": 1,
            "dst": "192.168.1.1",
            "lladdr": "00:1a:2b:3c:4d:5e"
        }
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
    def test_given_valid_new_route_when_add_new_route_entry_and_mac_known_and_neighbor_not_known_then_update_module_is_created_and_modules_are_linked(  # noqa: E501
        self, _
    ):
        kwargs = {
            "ifindex": 1,
            "dst": "1.2.3.4",
            "lladdr": "00:1a:2b:3c:4d:5e"
        }
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
        self.route_controller._netlink_route_handler(
            self.ndb,
            example_route_entry
        )
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
        self.route_controller._netlink_route_handler(
            self.ndb,
            example_route_entry
        )
        mock_delete_route_entry.assert_called()

    @patch("conf.route_control.send_ping")
    def test_given_new_neighbor_in_unresolved_when_add_unresolved_new_neighbor_then_route_added_in_bess(
        self,
        _,
    ):
        kwargs = {
            "ifindex": 1,
            "dst": "192.168.1.1",
            "lladdr": "00:1a:2b:3c:4d:5e"
        }
        self.ndb.neighbours.dump.return_value = [kwargs]
        mock_netlink_msg = {
            "attrs": {
                "NDA_DST": "1.2.3.4",
                "NDA_LLADDR": "00:1a:2b:3c:4d:5e",
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
        self.route_controller._netlink_neighbor_handler(
            self.ndb,
            {"event": "RTM_NEWNEIGH"}
        )
        mock_add_unresolved_new_neighbor.assert_called()
