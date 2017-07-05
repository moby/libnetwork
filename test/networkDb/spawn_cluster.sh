#!/usr/local/bin/bash

MEMEBERS=${1:-3}
SERVICE_NAME="testdb"
BASE_IMAGE="fcrisciani/networkdb-test"
SERVER_PORT=8000
ENV_VARIABLES="GOPATH=/work"
APP="/work/src/github.com/docker/libnetwork/test/networkDb/ndbTester.go"

echo "Spawning $MEMEBERS nodes"

# map <name> --> IP
declare -A node2IP

# map <name> --> Web port to interact with
declare -A node2ClientPort

# array with tasks names
declare -a tasks
firstNode=""

function launchSwarmService() {
  docker service create --name $SERVICE_NAME --replicas $MEMEBERS --env TASK_ID="{{.Task.ID}}" -p mode=host,target=$SERVER_PORT $BASE_IMAGE
  # now wait that the replicas are up
  local ready=0
  while [[ ready -ne $MEMEBERS ]]; do
    echo -n "."
    sleep 1
    ready=$(docker service ls -f name=testdb | awk '{ print $4 '} | tail -n 1 | cut -d '/' -f 1)
  done
  echo ""
  echo "All replicas are up"
  tasks=($(docker service ps $SERVICE_NAME | awk '{print $1}' | tail -n +2))
  echo "tasks vector:${tasks[@]}"
  for task in "${tasks[@]}"
  do
      node2ClientPort[$task]=$(docker inspect $task -f '{{ (index (.Status.PortStatus.Ports) 0).PublishedPort }}')
  done
}

# function launchContainers() {
#   for (( i = 0; i < $MEMEBERS; i++ )) do
#     echo "Spawn ${BASE_NAME}${i}"
#     node_name=${BASE_NAME}${i}
#     docker run --rm -d --name ${node_name} -e${ENV_VARIABLES} -p${SERVER_PORT} -v ~/work:/work golang:alpine go run ${APP} ${node_name} ${SERVER_PORT} &> /dev/null
#     status=$?
#     if [ $status -ne 0 ]; then
#       echo "Node $node_name container launch failed"
#     fi
#     node2IP[$node_name]=$(docker inspect $node_name --format='{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}')
#     node2ClientPort[$node_name]=$(docker inspect $node_name --format='{{ (index (index .NetworkSettings.Ports "8000/tcp") 0).HostPort }}')
#     echo "Node $node_name ip:${node2IP[$node_name]} port:${node2ClientPort[$node_name]}"
#   done
# }

function waitInit() {
  echo "Wait for node readiness"
  for task in "${tasks[@]}"
  do
    while true; do
      output=$(curl localhost:${node2ClientPort[$task]}/ready 2> /dev/null)
      ret=$?
      if [[ $ret -eq 0 && $output == "OK" ]]; then
        break
      fi
      echo -n "."
      sleep 1
    done
    echo ""
    node2IP[$task]=$(curl localhost:${node2ClientPort[$task]}/myip 2> /dev/null)
    echo "Task $task is ready ${node2IP[$task]}"
  done
}

# Init phase
# function nodeInit() {
#   local i=0
#
#   for node in "${!node2IP[@]}"
#   do
#     echo "$node: curl ${node2IP[$node]}:${node2ClientPort[$node]}/init?NodeName=$node"
#     curl localhost:${node2ClientPort[$node]}/init?NodeName=$node
#     if [ $i -eq 0 ]; then
#       firstNode=$node
#     fi
#     ((i++))
#   done
# }

# Join phase
function nodeJoin() {
  local i=0
  for node in "${!node2IP[@]}"
  do
    if [ $i -eq 0 ]; then
      firstNode=$node
    else
      echo "$node: curl ${node2IP[$node]}:${node2ClientPort[$node]}/join?members=${node2IP[$firstNode]}"
      curl localhost:${node2ClientPort[$node]}/join?members=${node2IP[$firstNode]}
    fi
    ((i++))
  done
}

function nodeJoinTestNetwork() {
  local i=0
  for node in "${!node2IP[@]}"
  do
    echo "$node: curl ${node2IP[$node]}:${node2ClientPort[$node]}/joinnetwork?nid=test}"
    curl localhost:${node2ClientPort[$node]}/joinnetwork?nid=test
  done
}

function printNodes() {
  # Print nodes
  local ports=""
  for node in "${!node2IP[@]}"
  do
    echo "name  : $node"
    echo "ip: ${node2IP[$node]}"
    echo "port: ${node2ClientPort[$node]}"
    ports+=${node2ClientPort[$node]}','
  done
  echo "ports: $ports"
}

function cleanup() {
  # Cleanup
  docker service rm $SERVICE_NAME
}

# Main program starts here
launchSwarmService
# launchContainers
waitInit
# nodeInit
nodeJoin
printNodes
nodeJoinTestNetwork
read -p "ready"
cleanup
