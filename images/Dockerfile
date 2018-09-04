from docker-registry.saicstack.com/base/centos-saic:7.4
  
RUN echo "Asia/Shanghai" > /etc/timezone
ADD lvs-controller /lvs-controller
   
RUN chown root:users /lvs-controller && chmod 755 /lvs-controller
   
entrypoint ["/lvs-controller"]
