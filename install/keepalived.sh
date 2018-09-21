#!/bin/bash

cd `dirname $0`

docker rm -f keepalived | true > /dev/null 2>&1

# delete vip dev to avoid old vip existed
ip a | grep $1 > /dev/null 2>&1
[[ $? == 0 ]] && sudo ip a del $1 dev eth0 > /dev/null 2>&1

docker run --name=keepalived \
    --restart=always \
    --net=host \
    --privileged=true \
    --volume=/etc/keepalived/:/ka-data/scripts/ \
    -d docker-registry.telecom.com/kubernetes/keepalived:1.2.7 \
    --master \
    --enable-check \
    --auth-pass pass \
    --vrid 57 eth0 101 $1/24/eth0> /dev/null 2>&1
