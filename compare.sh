#!/bin/bash

libnetworkPath="/home/flaviocrisciani/macos/src/github.com/docker/libnetwork"
libnetworkvendor="/tmp/libnetworkVendor"
dockerPath="/home/flaviocrisciani/work/src/github.com/docker/docker"
dockervendor="/tmp/dockerVendor"

cat ${libnetworkPath}/vendor.conf | sort > $libnetworkvendor
cat ${dockerPath}/vendor.conf | sort > $dockervendor

declare -A vendorMap

while IFS= read -r var
do
  if [[ "$var" == "" || $var == \#* ]]; then
    continue
  fi
  key=$(echo $var | awk '{print $1}')
  value=$(echo $var | awk '{print $2}')
  vendorMap["$key"]=$value
done < "$dockervendor"

echo "${vendorMap[@]}"

while IFS= read -r var
do
  if [[ "$var" == "" || $var == \#* ]]; then
    continue
  fi
  key=$(echo $var | awk '{print $1}')
  value=$(echo $var | awk '{print $2}')
  if [[ $value != ${vendorMap["$key"]} ]]; then
    RED='\033[0;31m'  # Red Color
    NC='\033[0m'      # No Color
    printf "$key --> ${RED}$value ${NC}== ${RED}${vendorMap["$key"]}${NC}\n"
  fi
done < "$libnetworkvendor"
