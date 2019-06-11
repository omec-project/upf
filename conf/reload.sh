#!/bin/bash
bessctl daemon reset -- run file /conf/spgwu.bess
bessctl show pipeline
kill -HUP "$(ps ax | awk '$6 ~ "route_control.py" {print $1}')"
