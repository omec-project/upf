#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright(c) 2019 Intel Corporation

INIT_TIME=7
DEINIT_TIME=2
# It is actually 30 secs, but we're pausing
# here for 20 more secs for revamp time
TEST_DURATION=50
PKTSIZE=("512" "128" "128" "128")
PPS=("2000000" "4000000" "5000000" "6000000")
SUBS=("50" "500" "5000" "50000")
num_entries=${#PPS[@]}
num_subs=${#SUBS[@]}
# Assuming that we are using 2 tmux screens,
# 1st for `./il_nperf.sh -g`, 2nd for `./il_nperf.sh -r`
# Ensure that each tmux screen is logged in as root,
# and both screens are in il_trafficgen/pktgen/ dir.
TMUX1=1:0.0
TMUX2=2:0.0
ILTRAFFICGEN_PATH=~/il_trafficgen/pktgen

function SET_VAL() {
	sed_cmd="s/$1=/$1=$2\#/g"
	sed -i -e $sed_cmd ${ILTRAFFICGEN_PATH}/autotest/user_input.cfg
}

for ((i = 0; i < num_entries; i++)); do
	for ((j = 0; j < num_subs; j++)); do
		SET_VAL "pps" "${PPS[$i]}"
		SET_VAL "pkt_size" "${PKTSIZE[$i]}"
		SET_VAL "flows" "${SUBS[$j]}"

		source ${ILTRAFFICGEN_PATH}/autotest/user_input.cfg

		sudo rm -rf ${ILTRAFFICGEN_PATH}/autotest/log/*

		# Spawn processes
		tmux send-keys -t $TMUX1 './il_nperf.sh -g' Enter &
		sleep $INIT_TIME
		tmux send-keys -t $TMUX2 './il_nperf.sh -r' Enter &
		sleep $INIT_TIME

		# Start pktgen
		tmux send-keys -t $TMUX2 'start 0' &
		tmux send-keys -t $TMUX1 'start 0' &
		tmux send-keys -t $TMUX2 ' ' Enter &
		tmux send-keys -t $TMUX1 ' ' Enter &
		tmux send-keys -t $TMUX2 ' ' Enter &
		tmux send-keys -t $TMUX1 ' ' Enter &
		sleep $TEST_DURATION

		# Quit processes
		tmux send-keys -t $TMUX1 'quit' &
		tmux send-keys -t $TMUX1 ' ' Enter &
		tmux send-keys -t $TMUX2 'quit' &
		tmux send-keys -t $TMUX2 ' ' Enter &
		sleep $DEINIT_TIME

		tmux send-keys -t $TMUX1 ' ' Enter &
		tmux send-keys -t $TMUX2 ' ' Enter &

		sudo mv ${ILTRAFFICGEN_PATH}/autotest/log/* ${ILTRAFFICGEN_PATH}/autotest/trial_${SUBS[$j]}_${PPS[$i]}_${PKTSIZE[$i]}.log
		cp ${ILTRAFFICGEN_PATH}/autotest/user_input.cfg.bk ${ILTRAFFICGEN_PATH}/autotest/user_input.cfg
	done
done
