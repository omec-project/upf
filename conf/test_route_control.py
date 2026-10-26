#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright 2023 Canonical Ltd.

import sys
import unittest
from unittest.mock import MagicMock, Mock, patch

sys.modules["pybess.bess"] = MagicMock()

from conf.route_control import (
    RouteController,
    RouteEntry,
    fetch_mac,
    mac_to_hex,
    mac_to_int,
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
    async def test_given_netlink_message_when_rtm_newroute_event_then_add_new_route_entry_is_called(
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
        await self.route_controller._netlink_route_handler(
            self.ndb, example_route_entry
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
    async def test_given_netlink_message_when_rtm_delroute_event_then_delete_route_entry_is_called(
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
        await self.route_controller._netlink_route_handler(
            self.ndb, example_route_entry
        )
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
    async def test_given_netlink_message_when_rtm_newneigh_event_then_add_unresolved_new_neighbor_is_called(
        self, mock_add_unresolved_new_neighbor
    ):
        await self.route_controller._netlink_neighbor_handler(
            self.ndb, {"event": "RTM_NEWNEIGH"}
        )
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

        self.ndb.neighbours.dump.return_value = []
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
