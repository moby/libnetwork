#!/usr/local/bin/bash

MEMEBERS=${1:-3}
BASE_NAME="node-"
SERVER_PORT=8000
ENV_VARIABLES="GOPATH=/work"
APP="/work/src/github.com/docker/libnetwork/test/networkDb/ndbTester.go"

echo "Spawning $MEMEBERS nodes"

# map <name> --> IP
declare -A node2IP

# map <name> --> Web port to interact with
declare -A node2ClientPort

firstNode=""

function launchContainers() {
  for (( i = 0; i < $MEMEBERS; i++ )) do
    echo "Spawn ${BASE_NAME}${i}"
    node_name=${BASE_NAME}${i}
    docker run --rm -d --name ${node_name} -e${ENV_VARIABLES} -p${SERVER_PORT} -v ~/work:/work golang:alpine go run ${APP} ${node_name} ${SERVER_PORT} &> /dev/null
    status=$?
    if [ $status -ne 0 ]; then
      echo "Node $node_name container launch failed"
    fi
    node2IP[$node_name]=$(docker inspect $node_name --format='{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}')
    node2ClientPort[$node_name]=$(docker inspect $node_name --format='{{ (index (index .NetworkSettings.Ports "8000/tcp") 0).HostPort }}')
    echo "Node $node_name ip:${node2IP[$node_name]} port:${node2ClientPort[$node_name]}"
  done
}

function waitInit() {
  echo "Wait for node readiness"
  for node in "${!node2IP[@]}"
  do
    while true; do
      output=$(curl localhost:${node2ClientPort[$node]}/ready 2> /dev/null)
      ret=$?
      if [[ $ret -eq 0 && $output == "OK" ]]; then
        break
      fi
      echo -n "."
      sleep 1
    done
    echo ""
    echo "Node $node is ready"
  done
}

# Init phase
function nodeInit() {
  local i=0

  for node in "${!node2IP[@]}"
  do
    echo "$node: curl ${node2IP[$node]}:${node2ClientPort[$node]}/init?NodeName=$node"
    curl localhost:${node2ClientPort[$node]}/init?NodeName=$node
    if [ $i -eq 0 ]; then
      firstNode=$node
    fi
    ((i++))
  done
}

# Join phase
function nodeJoin() {
  local i=0
  for node in "${!node2IP[@]}"
  do
    if [ $i -gt 0 ]; then
      echo "$node: curl ${node2IP[$node]}:${node2ClientPort[$node]}/join?members=${node2IP[$firstNode]}"
      curl localhost:${node2ClientPort[$node]}/join?members=${node2IP[$firstNode]}
    fi
    ((i++))
  done
}

function printNodes() {
  # Print nodes
  for node in "${!node2IP[@]}"
  do
    echo "name  : $node"
    echo "ip: ${node2IP[$node]}"
    echo "port: ${node2ClientPort[$node]}"
  done
}

function cleanup() {
  # Cleanup
  docker stop ${!node2IP[@]}
}

# Main program starts here
launchContainers
waitInit
nodeInit
nodeJoin
printNodes
read -p "ready"
cleanup
