import sys
import unittest
from unittest.mock import MagicMock, Mock, patch

from pyroute2 import IPDB, IPRoute  # type: ignore[import]

sys.modules["pybess.bess"] = MagicMock()

from route_control import (  # noqa: E402
    RouteController,
    RouteEntry,
    RouteEntryParser,
    fetch_mac,
    mac_to_hex_int,
    validate_ipv4,
)


class BessControllerMock(object):
    """Mock of BessController to avoid using BESS from pybess.bess"""

    def __init__(self):
        pass

    def get_bess(self, *args, **kwargs) -> None:
        pass

    def add_route_entry(self, *args, **kwargs) -> None:
        pass

    def del_route_entry(self, *args, **kwargs) -> None:
        pass

    def create_module(self, *args, **kwargs) -> None:
        pass

    def delete_module(self, *args, **kwargs) -> None:
        pass

    def link_modules(self, *args, **kwargs) -> None:
        pass


@patch("route_control.BessController", BessControllerMock)
class TestUtilityFunctions(unittest.TestCase):
    """Tests utility functions in route_control.py."""

    def test_validate_ipv4_with_valid_ip(self):
        self.assertTrue(validate_ipv4("192.168.1.1"))
        self.assertFalse(validate_ipv4("192.168.300.1"))

    def test_validate_ipv4_with_invalid_ip(self):
        self.assertFalse(validate_ipv4("::1"))
        self.assertFalse(validate_ipv4(""))

    def test_mac_to_hex_valid_mac(self):
        self.assertEqual(mac_to_hex_int("00:1a:2b:3c:4d:5e"), 0x001A2B3C4D5E)

    def test_fetch_mac_address_found(self):
        ipr = IPRoute()
        ipr.get_neighbours = lambda dst, **kwargs: [
            {"attrs": [("NDA_DST", dst), ("NDA_LLADDR", "00:1a:2b:3c:4d:5e")]}
        ]  # noqa: E501
        self.assertEqual(fetch_mac(ipr, "192.168.1.1"), "00:1a:2b:3c:4d:5e")

    def test_fetch_mac_address_not_found(self):
        ipr = IPRoute()
        ipr.get_neighbours = lambda dst, **kwargs: []
        self.assertIsNone(fetch_mac(ipr, "192.168.1.1"))


@patch("route_control.BessController", BessControllerMock)
class TestRouteEntryParser(unittest.TestCase):
    """Tests the functions of the RouteEntryParser class."""

    def setUp(self):
        self.ipdb = IPDB()
        self.parser = RouteEntryParser(self.ipdb)

    def tearDown(self):
        self.ipdb.release()

    def test_parse_valid_entry_and_dst_len_is_zero(self):
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
        result = self.parser.parse(example_route_entry)
        self.assertIsInstance(result, RouteEntry)
        self.assertEqual(result.dest_prefix, "0.0.0.0")
        self.assertEqual(result.next_hop_ip, "172.31.48.1")
        self.assertEqual(result.interface, self.ipdb.interfaces[2].ifname)
        self.assertEqual(result.prefix_len, 0)

    def test_parse_valid_entry_and_dst_len_is_not_zero(self):
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
        result = self.parser.parse(example_route_entry)
        self.assertIsInstance(result, RouteEntry)
        self.assertEqual(result.dest_prefix, "192.168.1.0")
        self.assertEqual(result.next_hop_ip, "172.31.48.1")
        self.assertEqual(result.interface, self.ipdb.interfaces[2].ifname)
        self.assertEqual(result.prefix_len, 24)

    def test_parse_entry_with_missing_fields(self):
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
        result = self.parser.parse(example_route_entry)
        self.assertIsNone(result)


class TestRouteController(unittest.TestCase):
    def setUp(self):
        with patch(
            "route_control.BessController", new_callable=BessControllerMock
        ) as mock_bess_controller:
            self.ipdb = IPDB()
            self.route_parser = RouteEntryParser(ipdb=self.ipdb)
            self.ipr = IPRoute()
            self.route_controller = RouteController(
                mock_bess_controller, self.route_parser, self.ipdb, self.ipr
            )

    @patch.object(IPRoute, "get_routes")
    @patch.object(RouteEntryParser, "parse")
    @patch.object(RouteController, "_handle_new_route_entry")
    def test_bootstrap_routes(
        self, mock_handle_new_route_entry, mock_parse, mock_get_routes
    ):
        mock_routes = [
            {"event": "RTM_NEWROUTE"},
            {"event": "OTHER_ACTION"}
        ]
        mock_get_routes.return_value = mock_routes

        mock_route_entry = Mock()
        mock_parse.return_value = mock_route_entry

        self.route_controller.bootstrap_routes()

        mock_get_routes.assert_called_once()
        mock_parse.assert_called_with(mock_routes[0])
        mock_handle_new_route_entry.assert_called_with(mock_route_entry)

    @patch.object(IPRoute, "get_routes")
    @patch.object(RouteEntryParser, "parse")
    def test_bootstrap_routes_with_no_expected_event(
        self, mock_parse, mock_get_routes
    ):
        mock_routes = [
            {"event": "NOT_RTM_NEWROUTE"},
            {"event": "OTHER_ACTION"}
        ]
        mock_get_routes.return_value = mock_routes
        self.route_controller.bootstrap_routes()

        mock_get_routes.assert_called_once()
        mock_parse.assert_not_called()

    @patch.object(IPRoute, "get_routes")
    @patch.object(RouteEntryParser, "parse")
    @patch.object(RouteController, "_handle_new_route_entry")
    def test_bootstrap_routes_with_no_valid_entries(
        self, mock_handle_new_route_entry, mock_parse, mock_get_routes
    ):
        mock_routes = [
            {"event": "RTM_NEWROUTE"},
            {"event": "OTHER_ACTION"}
        ]
        mock_get_routes.return_value = mock_routes
        mock_parse.return_value = None

        self.route_controller.bootstrap_routes()

        mock_get_routes.assert_called_once()
        mock_parse.assert_called_with(mock_routes[0])
        mock_handle_new_route_entry.assert_not_called()

    def test_start_pinging_missing_entries_when_thread_is_alive(
        self
    ):
        self.route_controller.ping_missing.start()

        with patch.object(
            self.route_controller.ping_missing, "start",
            side_effect=self.route_controller.ping_missing.start,
        ) as mock_start:
            self.route_controller.start_pinging_missing_entries()

            mock_start.assert_not_called()

    def test_start_pinging_missing_entries_when_thread_is_not_alive(
        self
    ):
        with patch.object(
            self.route_controller.ping_missing, "start",
            side_effect=self.route_controller.ping_missing.start,
        ) as mock_start:
            self.route_controller.start_pinging_missing_entries()
            mock_start.assert_called_once()
