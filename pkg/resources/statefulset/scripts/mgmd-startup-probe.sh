#!/bin/bash

# Copyright (c) 2022, Oracle and/or its affiliates.
#
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

# Startup probe of the MySQL Cluster management nodes

set -e

# Extract nodeId from pod ordinal
sfsetPodOrdinalIdx=${HOSTNAME##*-}
nodeId=$((sfsetPodOrdinalIdx + 1))

# Get local mgmd status using `ndb_mgm -e "<nodeId> status"` command
nodeStatus=$(ndb_mgm -c "localhost" -e "${nodeId} status" --connect-retries=1)
# If nodeStatus has "Node ${nodeId}: connected", the management node can be considered ready
if ! [[ "${nodeStatus}" =~ .*Node\ "${nodeId}":\ connected.* ]]; then
  echo "Management node health check failed."
  echo "Node status output : "
  echo "${nodeStatus}"
  exit 1
fi

# This Management node is "ready" to accept connections.
# If this node is being restarted as a part of a the mgmd StatefulSet
# update, it should be considered fully "ready" only when all the other
# existing mgmd and data nodes have connected to it.
connectstrings=(${NDB_CONNECTSTRING//,/ })
if ((${#connectstrings[@]} == 1)); then
  # No need to handle update case as there is only one mgmd
  # (i.e replica=1) and update is denied for such cases.
  exit 0
fi

# Deduce the nodeId and connectstring of the "other" mgmd
otherMgmdNodeId=0
otherMgmdConnectstring=""
if ((nodeId == 1)); then
  otherMgmdNodeId=2
  otherMgmdConnectstring=${connectstrings[1]}
else
  otherMgmdNodeId=1
  otherMgmdConnectstring=${connectstrings[0]}
fi

# Check if the other mgmd is already running.
if [[ $(getent hosts "${otherMgmdConnectstring%:*}.${NDB_POD_NAMESPACE}" | awk '{print $1}') == "" ]]; then
  # DNS lookup returned empty string => the other mgmd pod doesn't exist yet
  # This is an ISR and this local mgmd is ready
  exit 0
fi

# Other mgmd is running. Local mgmd is ready when it reports the exact
# same status about the connected mgmd and data nodes as the other mgmd.
clusterStatusFromLocalMgmd=$(ndb_mgm -c localhost:1186 --connect-retries=1 -e show | sed -n '/id=3/,/id=2/p')
clusterStatusFromOtherMgmd=$(ndb_mgm -c "${otherMgmdConnectstring}" --connect-retries=1 -e show | sed -n '/id=3/,/id=2/p')
if [[ "${clusterStatusFromLocalMgmd}" != "${clusterStatusFromOtherMgmd}" ]]; then
  # Local Mgmd not ready
  exit 1
fi