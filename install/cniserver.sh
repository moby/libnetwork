#!/bin/bash
set -euo pipefail
cp /var/libnetwork/bin/cnictl /opt/cni/bin/libnetwork-cni
cp /var/libnetwork/config/net.conf /etc/cni/net.d/00-libnetwork-cni.conf

/var/libnetwork/bin/cniserver  &> /home/libnetwork/cniserver.log
