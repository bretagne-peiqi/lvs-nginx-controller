# IPVS based kubernetes controller

This project is aimed to provide external traffic loadbalancing to kubernetes based container application.
Especially for heavy traffic loads mode.

## Running
to print gc logs, run:

GODEBUG='gctrace=1' ./lvs-controller --debug --kubeconfig kubeconfig -vip 10.135.22.77 --schedname rr 2>&1>gc.log &
