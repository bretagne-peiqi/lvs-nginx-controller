package controller

import (
	"fmt"
	"net"
	"reflect"
	"strings"
	"time"

	"github.com/lvs-controller/pkg/config"
	"github.com/lvs-controller/pkg/provider/ipvs"

	"github.com/golang/glog"
	"k8s.io/api/core/v1"
	//v2 "k8s.io/apimachinery/pkg/apis/meta/v1"
	//"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	//"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	//"k8s.io/client-go/util/workqueue"
)

const (
	nginxServer  = "ingress-nginx"
	nginxPattern = "nginx-ingress-controller"

	tcpService = "tcp-services"
	udpService = "udp-services"

	deletion = "deleting"
	add      = "adding"

	protoTcp = "tcp"
	protoUdp = "udp"
)

type LoadBalancerController struct {
	client kubernetes.Interface

	//This is used to monitor layer 4 tcp/udp config updates
	//store		   cache.Store
	controller cache.Controller
	//portQueue      workqueue.RateLimitingInterface

	//Nginx ingress controller layer 7
	//addrStore      cache.Store
	addrController cache.Controller
	//addrQueue      workqueue.RateLimitingInterface

	lbstore LBStore

	lvsManager *ipvs.LvsManager
}

func (lbc *LoadBalancerController) PortAddFunc(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		fmt.Printf("Couldn't get key for layer 4 obj %v\n", err)
		return
	}
	cm := obj.(*v1.ConfigMap)

	var port PortStore
	if cm.GetName() == tcpService {
		port.L4[tcpService] = make([]string)
		for key, _ := range cm.Data {
			port.L4[tcpService] = append(port.L4[tcpService], key)
		}
		//reset udp services;
		port.L4[udpService] = make([]string)
		port.Name = "add-tcp-services"

	} else if cm.GetName() == udpService {
		port.L4[udpService] = make([]string)
		for key, _ := range cm.Data {
			port.L4[udpService] = append(port.L4[udpService], key)
		}
		//reset tcp services;
		port.L4[tcpService] = make([]string)
		port.Name = "add-udp-services"

	} else {
		//no-op
	}

	lbc.lbstore.L4Set(port)
}

func (lbc *LoadBalancerController) PortUpdateFunc(newobj, oldobj interface{}) {
	oldCm := oldobj.(*v1.ConfigMap)
	newCm := newobj.(*v1.ConfigMap)

	if reflect.DeepEqual(oldCm.Data, newCm.Data) {
		return
	}

	lbc.PortAddFunc(newCm)
}

func (lbc *LoadBalancerController) PortDelFunc(obj interface{}) {
	cm := obj.(*v1.ConfigMap)
	var port PortStore
	if cm.GetName() == tcpService {
		port.L4[tcpService] = make([]string)
		for key, _ := range cm.Data {
			port.L4[tcpService] = append(port.L4[tcpService], key)
		}
		//reset udp-services cache,
		port.L4[udpService] = make([]string)
		port.Name = "del-tcp-services"

	} else if cm.GetName() == udpService {
		port.L4[udpService] = make([]string)
		for key, _ := range cm.Data {
			port.L4[udpService] = append(port.L4[udpService], key)
		}
		//reset tcp-services cache,
		port.L4[tcpService] = make([]string)
		port.Name = "del-udp-services"

	} else {
		//no-op
	}

	lbc.lbstore.L4Set(port)

}

func (lbc *LoadBalancerController) AddrAddFunc(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		fmt.Printf("Couldn't get key for layer 3 obj %v\n", err)
		return
	}

	var addr AddrStore
	pod := obj.(*v1.Pod)

	if pod.GetNamespace == nginxServer && strings.Contains(pod.GetName, nginxPattern) {
		addr.Addr = net.ParseIP(pod.Status.PodIP)
		addr.Name = pod.GetName()
		addr.Action = add

		lbc.lbstore.AddrSet(addr)
		//trigger
	}
	//lbc.addrQueue.Add(key)
}

func (lbc *LoadBalancerController) AddrUpdateFunc(newobj, oldobj interface{}) {
	oldPod := oldobj.(*v1.Pod)
	newPod := newobj.(*v1.Pod)

	if newPod.GetNamespace() == nginxServer && newPod.Status.PodIP != "" {

		var addr AddrStore
		if newPod.GetDeletionTimestamp() != nil {
			// delete pods...
			addr.Action = deletion
		} else {
			addr.Action = add
			// add pods ...
		}
		addr.Name = newPod.GetName()
		addr.Addr = net.ParseIP(newPod.Status.PodIP)
		if ok := lbc.EnsureAddr(addr); !ok {
			//trigger
			fmt.Printf("updating nginxServer %v\n", newPod.GetName())
		}
		//lbc.PortAddFunc(newobj)
	}
	return
}

func (lbc *LoadBalancerController) EnsureAddr(addr AddrStore) bool {

	lbc.lbstore.lock.Lock()
	defer lbc.lbstore.lock.Unlock()

	if _, ok := lbc.lbstore.addrStore[addr.Name]; ok {
		if reflect.DeepEqual(lbc.lbstore.addrStore[addr.Name], addr) {
			return true
		}
	} else {
		lbc.lbstore.addrStore[addr.Name] = addr
		return false
	}
}

func (lbc *LoadBalancerController) ProcessItem(<-chan lbc.lbstore) {

	processFunc := func() bool {

		//optiminse needs to be made; store the previous states, compare with the
		//current one; select the different elements, and call ipvs func according
		//the result
		addrs, ok := lbc.lbstore.AddrGet()
		if !ok {
			fmt.Printf("Failed, error in getting addrs info from cache")
			return true
		}

		tports, tname, ok := lbc.lbstore.L4Get(tcpService)
		if !ok {
			fmt.Printf("Failed, error in getting tports info from cache")
		}
		uports, uname, ok := lbc.lbstore.L4Get(udpService)
		if !ok {
			fmt.Printf("Failed, error in getting tports info from cache")
		}

		for addr, act := range addrs {
			//fmt.Printf("Testing 2...add host and act %v, %+v\n", addr, act)

			switch act {

			case add:
				{
					for _, port := range tports {
						switch tname {
						case "add-tcp-services":
							if err := lbc.lvsManager.Sync(addr, port, protoTcp); err != nil {
								fmt.Printf("Failed to sync ipvs dr mode, err %+v\n", err)
							}
						case "del-tcp-services":
							if err := lbc.lvsManager.Del(addr, port, protTcp); err != nil {
								fmt.Printf("Failed to del ipvs dr mode, err %+v\n", err)
							}
						default:
							fmt.Printf("Failed, found unrecognizable action")
						}
					}
					for _, port := range uports {
						switch uname {
						case "add-udp-services":
							if err := lbc.lvsManager.Sync(addr, port, protoUdp); err != nil {
								fmt.Printf("Failed to sync ipvs dr mode, err %+v\n", err)
							}
						case "del-udp-services":
							if err := lbc.lvsManager.Del(addr, port, protTcp); err != nil {
								fmt.Printf("Failed to del ipvs dr mode, err %+v\n", err)
							}
						default:
							fmt.Printf("Failed, found unrecognizable action")
						}
					}
				}
			case deletion:
				{
					if err := lbc.lvsManager.DelRServer(addr); err != nil {
						fmt.Printf("Failed to clear dirty real server, err %+v\n", err)
					}
				}
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
		//queue:      workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "ipvs_layer4"),
		//Addrqueue:  workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "ipvs_layer3"),
	}

	lbc.lbstore = NewLBStore()
	lbc.lvsManager = ipvs.NewLvsManager()
	lbc.lvsManager.Init()

	podListWatcher := cache.NewListWatchFromClient(clientset.CoreV1().RESTClient(), "pods", v1.NamespaceAll, fields.Everything())
	_, lbc.controller = cache.NewInformer(
		podListWatcher,
		&v1.Pod{},
		0,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    lbc.AddrAddFunc,
			UpdateFunc: lbc.AddrUpdateFunc,
		},
	)

	cmListWatcher := cache.NewListWatchFromClient(clientset.CoreV1().RESTClient(), "configmaps", v1.NamespaceAll, fields.Everything())
	_, lbc.addrController = cache.NewInformer(
		cmListWatcher,
		&v1.ConfigMap{},
		0,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    lbc.PortAddFunc,
			UpdateFunc: lbc.PortUpdateFunc,
			DeleteFunc: lbc.PortDelFunc,
		},
	)

	return lbc
}

func (lbc *LoadBalancerController) Run(workers int, stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()

	glog.Infof("Startting ipvs loadbalancer controller")

	go lbc.controller.Run(stopCh)
	go lbc.addrController.Run(stopCh)
	if !cache.WaitForCacheSync(stopCh, lbc.controller.HasSynced, lbc.addrController.HasSynced) {
		return
	}

	for i := 0; i < workers; i++ {
		go wait.Until(lbc.ProcessItem, time.Second, stopCh)
	}

	<-stopCh
	fmt.Printf("Shutting down ipvs config controller")
	//lbc.queue.ShutDown()
	//lbc.addrQueue.ShutDown()
}
