#!/bin/bash
set -euo pipefail
/var/libnetwork/bin/dnet -d -p cni -c /var/libnetwork/config/config.toml &> /home/libnetwork/dnet.log 
