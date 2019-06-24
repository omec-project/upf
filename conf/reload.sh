#!/usr/bin/env bash
SCRIPT_DIR="$(dirname "${BASH_SOURCE[0]}")"
bessctl daemon reset -- run file "$SCRIPT_DIR/spgwu.bess"
bessctl show pipeline
