###examples

docker run -d --restart=always --net=host --privileged --name=lvs-nginx-controller  -v /var/run/bird.ctl:/var/run/calico/bird.ctl docker-registry.saicstack.com/calico/bird_exporter:v1.0 -- -bird.socket=/var/run/bird.ctl

curl http://10.129.22.47:9324/metrics
