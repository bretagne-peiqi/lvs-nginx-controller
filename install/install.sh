#!/usr/bin/bash

### This is for director ###
ifconfig eth0:1 10.135.135.19 broadcast 10.135.135.255 netmask 255.255.255.0 up
route add -host 10.135.135.19 dev eth0:1

ipvsadm -C
ipvsadm -A -t 10.135.135.19:8080 -s rr
ipvsadm -a -t 10.135.135.19:8080 -r 10.135.135.18:8080 -g
ipvsadm -a -t 10.135.135.19:8080 -r 10.135.135.20:8080 -g


#config
echo "net.ipv4.ip_forward = 1
net.ipv4.conf.all.send_redirects = 0
net.ipv4.conf.default.send_redirects = 0
net.ipv4.conf.default.rp_filter = 1
net.ipv4.conf.default.accept_source_route = 0
kernel.sysrq = 0
kernel.core_uses_pid = 1
net.ipv4.tcp_syncookies = 1
net.ipv4.conf.eth0.send_redirects = 0" >> /etc/sysctl.conf

sysctl -p

ipvsadm-save > /etc/sysconfig/ipvsadm

systemctl start ipvsadm


### This is for realserver ###

echo "net.ipv4.conf.all.arp_ignore = 1
net.ipv4.conf.all.arp_announce = 2
net.ipv4.ip_forward = 1
net.ipv4.conf.default.accept_source_route = 0
net.ipv4.conf.default.rp_filter = 1
net.ipv4.tcp_syncookies = 1
net.ipv4.conf.eth0.arp_ignore = 1
net.ipv4.conf.eth0.arp_announce = 2" > /etc/sysctl.conf

ifconfig lo:0 10.135.135.19 broadcast 10.135.135.255 netmask 255.255.255.255 up
route add -host 10.135.135.19 dev lo:0
modprobe ip_vs
