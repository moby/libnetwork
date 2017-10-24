#!/bin/bash
set -euo pipefail
cp /var/libnetwork/bin/cnictl /opt/cni/bin/dnet-cni
echo ${DNET_CNI_CONF} > /etc/cni/net.d/00-dnet-cni.conf

/var/libnetwork/bin/cniserver  &> /home/libnetwork/cniserver.log
