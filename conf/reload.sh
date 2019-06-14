#!/bin/bash
SCRIPT_DIR="$(dirname "${BASH_SOURCE[0]}")"
bessctl daemon reset -- run file "$SCRIPT_DIR/spgwu.bess"
bessctl show pipeline
kill -HUP "$(ps ax | awk '$6 ~ "route_control.py" {print $1}')" 2> /dev/null
