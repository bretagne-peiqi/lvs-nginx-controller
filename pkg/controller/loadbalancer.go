package controller

import (
	"fmt"
	"reflect"
	"strings"
	//"sync"
	"time"

	"github.com/lvs-controller/pkg/config"
	"github.com/lvs-controller/pkg/provider/ipvs"

	"github.com/golang/glog"
	"k8s.io/api/core/v1"
	v2 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

const (
	nginxServer  = "ingress-nginx"
	nginxPattern = "nginx-ingress-controller"

	deletion = "deleting"
	add      = "adding"
)

type LoadBalancerController struct {
	client kubernetes.Interface

	//This is used to monitor layer 4 tcp/udp config updates
	store      cache.Store
	controller cache.Controller
	queue      workqueue.RateLimitingInterface

	//Nginx ingress controller layer 7
	nginxController cache.Controller

	//For udp
	protoToPort    *ProtoToPort
	prePortToProto *ProtoToPort
	deltaL4Update  *ProtoToPort

	portToTcp      *ProtoToPort
	prePortToTcp   *ProtoToPort
	deltaTcpUpdate *ProtoToPort

	deltaAddr DeltaAddr

	lvsManager *ipvs.LvsManager

	//Flag to trigger process
	uflag bool
	tflag bool
}

func (lbc *LoadBalancerController) enqueueCm(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		fmt.Printf("Couldn't get key for layer 4 obj %v\n", err)
		return
	}
	lbc.queue.Add(key)
}

func (lbc *LoadBalancerController) updateCm(newobj, oldobj interface{}) {
	oldCm := oldobj.(*v1.ConfigMap)
	newCm := newobj.(*v1.ConfigMap)

	if reflect.DeepEqual(oldCm.Data, newCm.Data) {
		return
	}

	lbc.listNginx()
	lbc.enqueueCm(newobj)
}

func (lbc *LoadBalancerController) listNginx() {

	var options v2.ListOptions
	podlist, err := lbc.client.CoreV1().Pods(nginxServer).List(options)

	if err != nil {
		fmt.Printf("Failed to list pods, err %+v\n", err)
	}

	lbc.deltaAddr.lock.Lock()
	defer lbc.deltaAddr.lock.Unlock()

	for key, _ := range lbc.deltaAddr.addrs {
		delete(lbc.deltaAddr.addrs, key)
	}

	for _, v := range podlist.Items {
		if strings.Contains(v.GetName(), nginxPattern) && v.Status.PodIP != "" {
		key := add + "_" + v.GetName()
		lbc.deltaAddr.addrs[key] = v.Status.PodIP
		}
	}
	
	for key, _ := range lbc.deltaAddr.addrs {
		fmt.Printf("list pods, %+v, ip %+v\n",lbc.deltaAddr.addrs[key], key)
	}
}

func (lbc *LoadBalancerController) listCm() {
	var options v2.ListOptions
	cmlist, err := lbc.client.CoreV1().ConfigMaps(nginxServer).List(options)
	
	if err != nil {
		fmt.Printf("Failed to list cm, err %+v\n", err)
	}
	
        for key, _ := range lbc.deltaL4Update.core {
		delete(lbc.deltaL4Update.core, key)
		delete(lbc.deltaTcpUpdate.core, key)
	}

	for _, v := range cmlist.Items {
		if strings.Contains(v.GetName(), "udp-services") {
		for key, _ := range v.Data {
			lbc.deltaL4Update.core[key] = "add"
			}
		}
		if strings.Contains(v.GetName(), "tcp-services") {
		for key, _ := range v.Data {
			lbc.deltaTcpUpdate.core[key] = "add"
			}
		}
	}
        
	for key, v := range lbc.deltaL4Update.core {
		fmt.Printf("key cm list are %v and %v \n", key, v)
	}
}

func (lbc *LoadBalancerController) AddrAddFunc(obj interface{}) {

	var addr AddrStore
	pod := obj.(*v1.Pod)

	if pod.GetNamespace() == nginxServer && strings.Contains(pod.GetName(), nginxPattern) && pod.Status.PodIP != "" {
		addr.Addr = pod.Status.PodIP
		addr.Name = pod.GetName()
		addr.Action = add + "_" + addr.Name

		lbc.deltaAddr.lock.Lock()
		defer lbc.deltaAddr.lock.Unlock()

		lbc.deltaAddr.addrs[addr.Action] = addr.Addr
	}
}

func (lbc *LoadBalancerController) AddrUpdateFunc(newobj, oldobj interface{}) {
	//oldPod := oldobj.(*v1.Pod)
	newPod := newobj.(*v1.Pod)

	if newPod.GetNamespace() == nginxServer && newPod.Status.PodIP != "" {

		addr := make(map[string]string)
		if newPod.GetDeletionTimestamp() != nil {
			// delete pods...
			addr[deletion+"_"+newPod.GetName()] = newPod.Status.PodIP
		} else {
			addr[add+"_"+newPod.GetName()] = newPod.Status.PodIP
			// add pods ...
		}
		if ok := lbc.EnsureAddr(addr); !ok {
			fmt.Printf("updating nginxServer %v\n", newPod.GetName())
		}
		lbc.listCm()
	}
	return
}

func (lbc *LoadBalancerController) EnsureAddr(addr map[string]string) bool {

	lbc.deltaAddr.lock.Lock()
	defer lbc.deltaAddr.lock.Unlock()

	for key, _ := range lbc.deltaAddr.addrs {
		delete(lbc.deltaAddr.addrs, key)
	}

	for key, ip := range addr {
		if _, ok := lbc.deltaAddr.addrs[key]; ok {
			if reflect.DeepEqual(lbc.deltaAddr.addrs[key], ip) {
				return true
			}
		} else {
			lbc.deltaAddr.addrs[key] = ip
		}
	}
	return true
}

func (lbc *LoadBalancerController) ipvsWorker() {
	workFunc := func() bool {
		key, ok := lbc.queue.Get()
		if ok {
			return true
		}
		defer lbc.queue.Done(key)

		obj, exist, err := lbc.store.GetByKey(key.(string))
		if !exist {
			fmt.Printf("ipvs config has been deleted %v\n", key)
			return false
		}
		if err != nil {
			fmt.Printf("err to get ipvs config %v\n", err)
			return false
		}
		cm := obj.(*v1.ConfigMap)
		if cm.GetName() == "tcp-services" {
			for key, _ := range cm.Data {
				lbc.portToTcp.Write(key, "tcp")
				fmt.Printf("ipvsWorker layer l4 port proto %v tcp\n", key)
			}
			lbc.tflag = true
		} else if cm.GetName() == "udp-services" {
			for key, _ := range cm.Data {
				lbc.protoToPort.Write(key, "udp")
				fmt.Printf("ipvsWorker layer l4 port proto %v udp\n", key)
			}
			lbc.uflag = true
		}
		return false
	}

	for {
		if quit := workFunc(); quit {
			fmt.Printf("nginx controller hostWoker shutting down")
			return
		}
	}
}

func (lbc *LoadBalancerController) PreProcess() {

	if reflect.DeepEqual(lbc.prePortToProto, lbc.protoToPort) {
		return
	}

	for key, _ := range lbc.deltaL4Update.core {
		delete(lbc.deltaL4Update.core, key)
	}

	//FIXME(peiqishi) optimize the algorithm
	for port, proto := range lbc.protoToPort.core {
		if _, ok := lbc.prePortToProto.core[port]; !ok {
			//store the new key
			lbc.deltaL4Update.core[port] = "add"
		} else if lbc.prePortToProto.core[port] != proto {
			// very rare, but note it anyway;
			// change protocol
			lbc.deltaL4Update.core[port] = "update"
		}
	}

	for port, _ := range lbc.prePortToProto.core {
		if _, pok := lbc.protoToPort.core[port]; !pok {
			//need to delete the dead
			lbc.deltaL4Update.core[port] = "del"
		}
	}

	for key, v := range lbc.prePortToProto.core {
		fmt.Printf("PreProcess l4 pre ... %v %v\n", key, v)
		delete(lbc.prePortToProto.core, key)
	}

	for key, _ := range lbc.protoToPort.core {
		lbc.prePortToProto.core[key] = lbc.protoToPort.core[key]
		delete(lbc.protoToPort.core, key)
	}

	return
}

func (lbc *LoadBalancerController) PreTcpProcess() {

	for key, _ := range lbc.deltaTcpUpdate.core {
		delete(lbc.deltaTcpUpdate.core, key)
	}

	//FIXME(peiqishi) optimize the algorithm
	for port, _ := range lbc.portToTcp.core {
		if _, ok := lbc.prePortToTcp.core[port]; !ok {
			//store the new key
			lbc.deltaTcpUpdate.core[port] = "add"
		}
		//else if lbc.prePortToTcp.core[port] != proto {
		// very rare, but note it anyway;
		// change protocol
		//lbc.deltaTcpUpdate.core[port] = "update"
		//}
	}
	for port, _ := range lbc.prePortToTcp.core {
		if _, pok := lbc.portToTcp.core[port]; !pok {
			//need to delete the dead
			lbc.deltaTcpUpdate.core[port] = "del"
		}
	}

	for key, v := range lbc.prePortToTcp.core {
		fmt.Printf("PreProcess l4 pre ... %v %v\n", key, v)
		delete(lbc.prePortToTcp.core, key)
	}

	for key, _ := range lbc.portToTcp.core {
		lbc.prePortToTcp.core[key] = lbc.portToTcp.core[key]
		delete(lbc.portToTcp.core, key)
	}

	return
}
func (lbc *LoadBalancerController) ProcessItem() {

	processFunc := func() bool {

		if lbc.uflag {
			time.Sleep(10 * time.Millisecond)
			lbc.PreProcess()
			lbc.uflag = false
		}
		if lbc.tflag {
			time.Sleep(10 * time.Millisecond)
			lbc.PreTcpProcess()
			lbc.tflag = false
		}

		//optiminse needs to be done; store the previous states, compare with the
		//current one; select the different elements, and call ipvs func according
		//the result
		lbc.deltaAddr.lock.RLock()
		defer lbc.deltaAddr.lock.RUnlock()
		for key, addr := range lbc.deltaAddr.addrs {
			arrary := strings.Split(key, "_")
			//fmt.Printf("Testing 2...add host and act %+v, %+v\n", key, addr)

			switch arrary[0] {
			case "adding":
				{
					for port, act := range lbc.deltaL4Update.core {
						value := "udp"
						switch act {
						case "add":
							if err := lbc.lvsManager.Sync(addr, port, value); err != nil {
								fmt.Printf("Failed to sync ipvs dr mode, err %+v\n", err)
							}
						case "del":
							if err := lbc.lvsManager.Del(addr, port, value); err != nil {
								fmt.Printf("Failed to del ipvs dr mode, err %+v\n", err)
							}
						case "update":
							if err := lbc.lvsManager.Update(addr, port, value); err != nil {
								fmt.Printf("Failed to update ipvs dr mode, err %+v\n", err)
							}
						default:
							fmt.Printf("Failed, found unrecognizable action")
						}
					}
					for port, act := range lbc.deltaTcpUpdate.core {
						value := "tcp"
						switch act {
						case "add":
							if err := lbc.lvsManager.Sync(addr, port, value); err != nil {
								fmt.Printf("Failed to sync ipvs dr mode, err %+v\n", err)
							}
						case "del":
							if err := lbc.lvsManager.Del(addr, port, value); err != nil {
								fmt.Printf("Failed to del ipvs dr mode, err %+v\n", err)
							}
						case "update":
							if err := lbc.lvsManager.Update(addr, port, value); err != nil {
								fmt.Printf("Failed to update ipvs dr mode, err %+v\n", err)
							}
						default:
							fmt.Printf("Failed, found unrecognizable action")
						}
					}
					//delete(lbc.deltaAddr, addr)
				}
			case "deleting":
				{
					if err := lbc.lvsManager.DelRServer(addr); err != nil {
						fmt.Printf("Failed to clear dirty real server, err %+v\n", err)
					}
				}
				//delete(lbc.deltaAddr, addr)
			}
		}

		return false
	}

	for {
		if quit := processFunc(); quit {
			fmt.Printf("nginx controller processItem shutting down")
			return
		}
	}
}

func NewLoadBalancerController(cfg config.Config) *LoadBalancerController {
	lbc := &LoadBalancerController{
		client: cfg.Client,
		queue:  workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "addipvs_layer4"),
	}

	lbc.protoToPort = newProtoToPort()
	lbc.prePortToProto = newProtoToPort()
	lbc.deltaL4Update = newProtoToPort()

	lbc.portToTcp = newProtoToPort()
	lbc.prePortToTcp = newProtoToPort()
	lbc.deltaTcpUpdate = newProtoToPort()

	lbc.lvsManager = ipvs.NewLvsManager()
	lbc.lvsManager.Init()

	lbc.deltaAddr.addrs = make(map[string]string)

	lbc.uflag = false
	lbc.tflag = false

	lbc.store, lbc.controller = cache.NewInformer(
		&cache.ListWatch{
			ListFunc: func(options v2.ListOptions) (runtime.Object, error) {
				return lbc.client.CoreV1().ConfigMaps(v1.NamespaceAll).List(options)
			},
			WatchFunc: func(options v2.ListOptions) (watch.Interface, error) {
				return lbc.client.CoreV1().ConfigMaps(v1.NamespaceAll).Watch(options)
			},
		},
		&v1.ConfigMap{},
		0,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    lbc.enqueueCm,
			UpdateFunc: lbc.updateCm,
			DeleteFunc: lbc.enqueueCm,
		},
	)

	_, lbc.nginxController = cache.NewInformer(
		&cache.ListWatch{
			ListFunc: func(options v2.ListOptions) (runtime.Object, error) {
				return lbc.client.CoreV1().Pods(v1.NamespaceAll).List(options)
			},
			WatchFunc: func(options v2.ListOptions) (watch.Interface, error) {
				return lbc.client.CoreV1().Pods(v1.NamespaceAll).Watch(options)
			},
		},
		&v1.Pod{},
		0,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    lbc.AddrAddFunc,
			UpdateFunc: lbc.AddrUpdateFunc,
		},
	)
	return lbc
}

func (lbc *LoadBalancerController) Run(workers int, stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()

	glog.Infof("Startting ipvs loadbalancer controller")

	go lbc.controller.Run(stopCh)
	go lbc.nginxController.Run(stopCh)
	if !cache.WaitForCacheSync(stopCh, lbc.controller.HasSynced, lbc.nginxController.HasSynced) {
		return
	}

	for i := 0; i < workers; i++ {
		go wait.Until(lbc.ProcessItem, time.Second, stopCh)
		go wait.Until(lbc.ipvsWorker, time.Second, stopCh)
	}

	<-stopCh
	fmt.Printf("Shutting down ipvs config controller")
	lbc.queue.ShutDown()
}
