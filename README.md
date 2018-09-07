# IPVS based kubernetes controller

This project is aimed to provide external traffic loadbalancing to kubernetes based container application.
Especially for heavy traffic loads mode.

It currently only works in DR mode, we nend setup several ingress-nginx-controller in HostNetwork mode as backend of a pair of LVS.
ingress-nginx-controller and its tcp/udp configmaps should be created in ingress-nginx namespace, 
and we can scale out the number using HPA in heavy traffic mode.

external traffics will direct from lvs to ingress-nginx-controller then to endpoint pods.

### Adavantages of the architecture

This architecture first offers a Front ipv4 for L7 ingress traffic, as ingress-nginx will expose node ip in public which is dangerous, 
besides, It implements the scalability of ingress-nginx-controllers, as LVS is the only entrypoint of the cluster.

Second, we can simply config tcp/udp configmaps of ingress-nginx-controller, then the lvs-nginx-controller will update and reload lvs 
configs. Thus offering L4 TCP/UDP loadbalancing functionnalities.

In a very heavy traffic situation, we can also deploy several lvs pairs in Front for different application services.

This architecture avoids the vulnerability of traditinnal NodePort mode, as it exposes ports in every nodes, which may be
possible in cloud environ, but definitly would be crazy in bare-metal environment.

## Running
In order to make it works, we need to config nodes running ingress-nginx-controller according scripts: install/install.sh

to print gc logs, run:
GODEBUG='gctrace=1' ./lvs-controller --debug --kubeconfig kubeconfig -vip 10.135.22.77 --schedname rr 2>&1>gc.log &
https://sheepbao.github.io/post/golang_debug_gctrace/


