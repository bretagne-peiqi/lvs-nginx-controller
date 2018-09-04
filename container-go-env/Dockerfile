###https://hub.docker.com/_/golang/   docker pull golang
FROM docker-registry.saicstack.com/chenqiang/nginx-hello:v2.0

RUN echo "nameserver 114.114.114.114" >> /etc/resolv.conf

RUN rm -rf /etc/yum.repos.d/*
ADD  ./Centos-7.repo /etc/yum.repos.d/

RUN yum clean all && yum makecache && yum update -y


RUN yum search go
RUN yum install golang -y
