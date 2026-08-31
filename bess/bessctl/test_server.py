# SPDX-FileCopyrightText: 2026 Intel Corporation
# SPDX-License-Identifier: BSD-3-Clause

"""
Tests for MessageToJson compatibility with server.py.

Validates that MessageToJson works with the 'always_print_fields_with_no_presence'
parameter used in server.py, and with the project's Protobuf messages.
"""

from __future__ import print_function
from __future__ import absolute_import
from __future__ import unicode_literals

import json
import os
import sys
import unittest

# Add pybess to path - use same pattern as test_utils.py
this_dir = os.path.dirname(os.path.realpath(__file__))
sys.path.insert(1, os.path.join(this_dir, '..'))

# Import for side effects: pybess.bess configures protobuf import paths.
try:
    from pybess import bess as _bess
    from google.protobuf.json_format import MessageToJson
    from builtin_pb import bess_msg_pb2 as bess_msg
except ImportError as e:
    print('Cannot import required modules: {}'.format(e), file=sys.stderr)
    raise


class TestMessageToJsonCompatibility(unittest.TestCase):
    """Validate MessageToJson compatibility with server.py's usage pattern."""

    @classmethod
    def setUpClass(cls):
        """Determine which parameters are supported by the installed protobuf version."""
        import google.protobuf
        import inspect

        cls.protobuf_version = google.protobuf.__version__

        # Check which parameter name is supported
        sig = inspect.signature(MessageToJson)
        cls.supports_always_print = 'always_print_fields_with_no_presence' in sig.parameters

    @classmethod
    def _always_print_support_message(cls):
        return (
            f"Protobuf {cls.protobuf_version} does not support "
            f"'always_print_fields_with_no_presence' required by server.py. "
            f"Upgrade protobuf to the version in env/requirements.txt.")

    def _require_always_print_support(self):
        if not self.supports_always_print:
            self.skipTest(self._always_print_support_message())

    def _message_to_json_with_defaults(self, msg):
        """Call MessageToJson with always_print_fields_with_no_presence, matching server.py."""
        self._require_always_print_support()
        return MessageToJson(msg, always_print_fields_with_no_presence=True)

    def test_message_to_json_import(self):
        """Test that MessageToJson can be imported successfully."""
        from google.protobuf.json_format import MessageToJson
        self.assertIsNotNone(MessageToJson)

    def test_protobuf_supports_always_print(self):
        """Test that installed Protobuf supports the always_print_fields_with_no_presence parameter."""
        self.assertTrue(
            self.supports_always_print,
            self._always_print_support_message())

    def test_message_to_json_basic_conversion(self):
        """Test basic MessageToJson conversion with a simple message."""
        # Create a simple EmptyResponse message
        msg = bess_msg.EmptyResponse()

        # Convert to JSON
        json_str = MessageToJson(msg)

        # Verify it's a valid JSON string
        self.assertIsInstance(json_str, str)
        parsed = json.loads(json_str)
        self.assertIsInstance(parsed, dict)

    def test_message_to_json_with_default_fields_parameter(self):
        """
        Test MessageToJson with parameter for printing default/unpopulated fields.

        This parameter is critical for server.py's functionality.
        Verify an unset scalar field is emitted with its default value.
        """
        # Leave a scalar field unset so the test exercises default-field output.
        msg = bess_msg.VersionResponse()

        # Convert with the appropriate parameter for the installed version
        json_str = self._message_to_json_with_defaults(msg)

        # Verify it's valid JSON
        self.assertIsInstance(json_str, str)
        parsed = json.loads(json_str)
        self.assertIsInstance(parsed, dict)

        # Verify the unset scalar field is still present with its default value.
        self.assertIn('version', parsed)
        self.assertEqual(parsed['version'], '')

    def test_message_to_json_with_nested_message(self):
        """Test MessageToJson with nested messages (Error field)."""
        # Create a response with an error
        msg = bess_msg.EmptyResponse()
        msg.error.code = 1
        msg.error.errmsg = "Test error message"

        # Convert to JSON with appropriate parameter
        json_str = self._message_to_json_with_defaults(msg)

        # Verify it's valid JSON
        parsed = json.loads(json_str)
        self.assertIsInstance(parsed, dict)

        # Verify error fields are present
        self.assertIn('error', parsed)
        self.assertEqual(parsed['error']['code'], 1)
        self.assertEqual(parsed['error']['errmsg'], 'Test error message')

    def test_message_to_json_with_repeated_fields(self):
        """Test MessageToJson with repeated fields."""
        # Create a ListPluginsResponse with multiple paths
        msg = bess_msg.ListPluginsResponse()
        msg.paths.extend(['path1.so', 'path2.so', 'path3.so'])

        # Convert to JSON
        json_str = self._message_to_json_with_defaults(msg)

        # Verify it's valid JSON
        parsed = json.loads(json_str)
        self.assertIsInstance(parsed, dict)

        # Verify repeated field is present and correct
        self.assertIn('paths', parsed)
        self.assertEqual(len(parsed['paths']), 3)
        self.assertEqual(parsed['paths'][0], 'path1.so')

    def test_message_to_json_with_int64_fields(self):
        """
        Test MessageToJson with 64-bit integer fields.

        According to the comment in server.py, MessageToJson converts
        64-bit integers to strings. This test validates that behavior.
        """
        # Create a WorkerStatus message with 64-bit integers
        msg = bess_msg.ListWorkersResponse.WorkerStatus()
        msg.wid = 123456789  # Use a reasonable 64-bit integer
        msg.core = 8
        msg.running = True
        msg.num_tcs = 5
        msg.silent_drops = 987654321  # Use a reasonable 64-bit integer

        # Convert to JSON
        json_str = self._message_to_json_with_defaults(msg)

        # Verify it's valid JSON
        parsed = json.loads(json_str)
        self.assertIsInstance(parsed, dict)

        # Verify int64 fields are rendered as strings per protobuf JSON mapping.
        self.assertIn('wid', parsed)
        self.assertIn('silentDrops', parsed)
        self.assertIsInstance(parsed['wid'], str)
        self.assertEqual(parsed['wid'], '123456789')
        self.assertIsInstance(parsed['silentDrops'], str)
        self.assertEqual(parsed['silentDrops'], '987654321')
        self.assertEqual(parsed['running'], True)

    def test_message_to_json_empty_message(self):
        """Test MessageToJson with an empty message."""
        msg = bess_msg.EmptyRequest()

        # Convert to JSON
        json_str = self._message_to_json_with_defaults(msg)

        # Verify it's valid JSON (should be empty object)
        parsed = json.loads(json_str)
        self.assertIsInstance(parsed, dict)

    def test_message_to_json_server_usage_pattern(self):
        """Test the exact usage pattern from server.py's get_pipeline function."""
        msg = bess_msg.EmptyResponse()
        msg.error.code = 0
        msg.error.errmsg = ""

        # Mirrors the actual call in server.py
        json_dict = json.loads(self._message_to_json_with_defaults(msg))

        self.assertIsInstance(json_dict, dict)
        self.assertIn('error', json_dict)


if __name__ == '__main__':
    unittest.main()
