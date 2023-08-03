import unittest
from unittest.mock import MagicMock, patch
from pyroute2 import IPRoute

import sys
from unittest.mock import MagicMock

sys.modules['pybess.bess'] = MagicMock()

from route_control import *


class BessControllerMock(object):
    """Mocks BessController class from route_control.py, the functions either pass or raise exceptions based in the parameters."""
    def __init__(self):
        pass
    def get_bess(self, raises: bool = False) -> None:
        if raises:
            raise Exception("Error in get_bess")
    
    def add_route_entry(self, raises: bool = False) -> None:
        if raises:
            raise Exception("Error in add_route_entry")
    
    def del_route_entry(self, raises: bool = False) -> None:
        if raises:
            raise Exception("Error in del_route_entry")
    
    def create_module(self, raises: bool = False) -> None:
        if raises:
            raise Exception("Error in create_module")
    
    def delete_module(self, raises: bool = False) -> None:
        if raises:
            raise Exception("Error in delete_module")
    
    def link_modules(self, raises: bool = False) -> None:
        if raises:
            raise Exception("Error in link_modules")

@patch("route_control.BessController", BessControllerMock)
class TestUtilityFunctions(unittest.TestCase):

    def test_validate_ipv4_with_valid_ip(self):
        self.assertTrue(validate_ipv4("192.168.1.1"))
        self.assertFalse(validate_ipv4("192.168.300.1"))

    def test_validate_ipv4_with_invalid_ip(self):
        self.assertFalse(validate_ipv4("::1"))
        self.assertFalse(validate_ipv4(""))

    def test_mac2hex_valid_mac(self):
        self.assertEqual(mac2hex("00:1a:2b:3c:4d:5e"), 0x001a2b3c4d5e)

    def test_mac2hex_invalid_mac(self):
        with self.assertRaises(ValueError):
            mac2hex("invalid_mac")

    def test_fetch_mac_valid_ip(self):
        ipr = IPRoute()
        ipr.get_neighbours = lambda dst, **kwargs: \
            [{"attrs": [("NDA_DST", dst), ("NDA_LLADDR", "00:1a:2b:3c:4d:5e")]}]
        self.assertEqual(fetch_mac("192.168.1.1"), "00:1a:2b:3c:4d:5e")
    
    def test_fetch_mac_mac_not_found(self):
        ipr = IPRoute()
        ipr.get_neighbours = lambda dst, **kwargs: []
        self.assertIsNone(fetch_mac("192.168.1.1"))


@patch('pybess.bess.BESS', MagicMock)
class TestRouteEntryParser(unittest.TestCase):
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
                ("RTA_OIF", 2)
            ],
            "header": {
                "length": 68,
                "type": 24,
                "target": "localhost",
                "stats": {"qsize": 0, "delta": 0, "delay": 0}
            },
            "event": "RTM_NEWROUTE"
        }
        result = self.parser.parse(example_route_entry)
        self.assertIsInstance(result, RouteEntry)
        self.assertEqual(result.dest_prefix, "0.0.0.0")
        self.assertEqual(result.next_hop_ip, "172.31.48.1")
        self.assertEqual(result.interface, self.ipdb.interfaces[2])
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
                ("RTA_DST", "192.168.1.0")
            ],
            "header": {
                "length": 68,
                "type": 24,
                "target": "localhost",
                "stats": {"qsize": 0, "delta": 0, "delay": 0}
            },
            "event": "RTM_NEWROUTE"
        }
        result = self.parser.parse(example_route_entry)
        self.assertIsInstance(result, RouteEntry)
        self.assertEqual(result.dest_prefix, "0.0.0.0")
        self.assertEqual(result.next_hop_ip, "172.31.48.1")
        self.assertEqual(result.interface, self.ipdb.interfaces[2])
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
                "stats": {"qsize": 0, "delta": 0, "delay": 0}
            },
            "event": "RTM_NEWROUTE"
        }
        result = self.parser.parse(example_route_entry)
        self.assertIsNone(result)




if __name__ == "__main__":
    unittest.main()
