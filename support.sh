#!/usr/bin/env bash

# Required tools
DOCKER="${DOCKER:-docker}"
NSENTER="${NSENTER:-nsenter}"
BRIDGE="${BRIDGE:-bridge}"
IPTABLES="${IPTABLES:-iptables}"
IPVSADM="${IPVSADM:-ipvsadm}"
IP="${IP:-ip}"

NSDIR=/var/run/docker/netns

function die {
    echo $*
    exit 1
}

type -P ${DOCKER} > /dev/null || die "This tool requires the docker binary"
type -P ${NSENTER} > /dev/null || die "This tool requires nsenter"
type -P ${BRIDGE} > /dev/null || die "This tool requires bridge"
type -P ${IPTABLES} > /dev/null || die "This tool requires iptables"
type -P ${IPVSADM} > /dev/null || die "This tool requires ipvsadm"
type -P ${IP} > /dev/null || die "This tool requires ip"

echo "Host iptables configuration"
echo filter
${IPTABLES} -w1 -n -v -L -t filter | grep -v '^$'
echo nat
${IPTABLES} -w1 -n -v -L -t nat | grep -v '^$'
echo mangle
${IPTABLES} -w1 -n -v -L -t mangle | grep -v '^$'
echo ""

echo "Host addresses and routes"
${IP} -o -4 address show
${IP} -4 route show
echo ""

echo "Overlay network configuration"
for networkID in $(${DOCKER} network ls --filter driver=overlay -q) "ingress_sbox"; do
    echo "Network ${networkID}"
    if [ "${networkID}" != "ingress_sbox" ]; then
        nspath=(${NSDIR}/*-${networkID:0:10})
        ${DOCKER} network inspect --verbose ${networkID}
    else
        nspath=(${NSDIR}/${networkID})
    fi
    ${NSENTER} --net=${nspath[0]} ${IP} -o -4 address show
    ${NSENTER} --net=${nspath[0]} ${IP} -4 route show
    ${NSENTER} --net=${nspath[0]} ${IP} -4 neigh show
    ${NSENTER} --net=${nspath[0]} ${BRIDGE} fdb show
    echo filter
    ${NSENTER} --net=${nspath[0]} ${IPTABLES} -w1 -n -v -L -t filter | grep -v '^$'
    echo nat
    ${NSENTER} --net=${nspath[0]} ${IPTABLES} -w1 -n -v -L -t nat | grep -v '^$'
    echo mangle
    ${NSENTER} --net=${nspath[0]} ${IPTABLES} -w1 -n -v -L -t mangle | grep -v '^$'
    ${NSENTER} --net=${nspath[0]} ${IPVSADM} -l -n
    echo ""
done

echo "Container network configuration"
for containerID in $(${DOCKER} container ls -q); do
    echo "Container ${containerID}"
    nspath=$(docker container inspect --format {{.NetworkSettings.SandboxKey}} ${containerID})
    ${DOCKER} container inspect ${containerID} --format '{{json .Id |printf "%s\n"}} {{json .Created | printf "%s\n"}} {{json .State |printf "%s\n" }} {{json .Name | printf "%s\n"}} {{json .RestartCount | printf "%s\n" }} {{json .Config.Hostname | printf "%s\n"}} {{json .Config.Labels | printf "%s\n"}} {{json .NetworkSettings}}'
    ${NSENTER} --net=${nspath[0]} ${IP} -o -4 address show
    ${NSENTER} --net=${nspath[0]} ${IP} -4 route show
    ${NSENTER} --net=${nspath[0]} ${IP} -4 neigh show
    echo nat
    ${NSENTER} --net=${nspath[0]} ${IPTABLES} -w1 -n -v -L -t nat | grep -v '^$'
    echo mangle
    ${NSENTER} --net=${nspath[0]} ${IPTABLES} -w1 -n -v -L -t mangle | grep -v '^$'
    ${NSENTER} --net=${nspath[0]} ${IPVSADM} -l -n
    echo ""
done
