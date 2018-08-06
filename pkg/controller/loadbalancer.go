package controller

import (
	"fmt"
	"reflect"
	"strings"
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

type LoadBalancerController struct {
	client kubernetes.Interface

	//This is used to monitor layer 4 tcp/udp config updates
	store      cache.Store
	controller cache.Controller
	queue      workqueue.RateLimitingInterface

	//Nginx ingress controller layer 7
	ngnixStore      cache.Store
	nginxController cache.Controller
	nginxqueue      workqueue.RateLimitingInterface

	//For udp
	protoToPort    *ProtoToPort
	prePortToProto *ProtoToPort
	deltaL4Update  *ProtoToPort

	portToTcp		*ProtoToPort
	prePortToTcp	*ProtoToPort
	deltaTcpUpdate  *ProtoToPort

	nginxAddr      map[string]string
	preNginxAddr   map[string]string
	deltaAddr      map[string]string
	lvsManager     *ipvs.LvsManager

	//Flag to trigger process
	uflag bool
	tflag bool
	nflag bool
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
	lbc.enqueueCm(newobj)
}

func (lbc *LoadBalancerController) enqueueNginx(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		fmt.Printf("Couldn't get key for layer 3 obj %v\n", err)
		return
	}
	lbc.nginxqueue.Add(key)
}

func (lbc *LoadBalancerController) updateNginx(newobj, oldobj interface{}) {
	oldPod := oldobj.(*v1.Pod)
	newPod := newobj.(*v1.Pod)

	if strings.Contains(newPod.GetName(), "nginx-ingress-controller") && oldPod.Status.PodIP != newPod.Status.PodIP {
		//in our case, we need to separate add keys and del keys,
		lbc.enqueueNginx(newobj)
	}
	return
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

func (lbc *LoadBalancerController) hostWorker() {
	workFunc := func() bool {
		key, ok := lbc.nginxqueue.Get()
		if ok {
			return true
		}
		defer lbc.queue.Done(key)

		obj, exist, err := lbc.ngnixStore.GetByKey(key.(string))
		if !exist {
			fmt.Printf("nginx controller host has been lost, failed to get %v\n", key)
			return false
		}
		if err != nil {
			fmt.Printf("err to get nginx controller pod info, failed to get %v\n", err)
			return false
		}
		pod := obj.(*v1.Pod)

		//FIXME(peiqishi): need to change "nginx-ingress-controller" to a config flag Pnginx
		if strings.Contains(pod.GetName(), "nginx-ingress-controller")&&pod.Status.HostIP != "" {
			//IN our case, we need to separate add keys and del keys,
			lbc.nginxAddr[pod.Status.HostIP] = "add"
			fmt.Printf("nginx controller to add pod info, addr %v\n", pod.Status.PodIP)
		}
		lbc.nflag = true
		return false
	}

	for {
		if quit := workFunc(); quit {
			fmt.Printf("nginx controller hostWoker shutting down")
			return
		}
	}
}

func (lbc *LoadBalancerController) NPreProcess() {
	if reflect.DeepEqual(lbc.nginxAddr, lbc.preNginxAddr) {
		return
	}
	for key, _ := range lbc.deltaAddr {
		delete(lbc.deltaAddr, key)
	}
	for addr, _ := range lbc.nginxAddr {
		if _, ok := lbc.preNginxAddr[addr]; !ok {
			lbc.deltaAddr[addr] = "add"
			fmt.Printf("NPreProcess nginx controller add %v\n", addr)
		}
	}

	for addr, _ := range lbc.preNginxAddr {
		if _, ok := lbc.nginxAddr[addr]; !ok {
			lbc.deltaAddr[addr] = "del"
			fmt.Printf("NPreProcess nginx controller del %v\n", addr)
		}
	}

	for key, _ := range lbc.preNginxAddr {
		delete(lbc.preNginxAddr, key)
	}

	for key, _ := range lbc.nginxAddr {
		lbc.preNginxAddr[key] = lbc.nginxAddr[key]
		delete(lbc.nginxAddr, key)
	}
	return
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
		if lbc.nflag {
			time.Sleep(10 * time.Millisecond)
			lbc.NPreProcess()
			lbc.nflag = false
		}

		//optiminse needs to be done; store the previous states, compare with the
		//current one; select the different elements, and call ipvs func according
		//the result
		for addr, act := range lbc.deltaAddr {
			//fmt.Printf("Testing 2...add host and act %+v, %+v\n", addr, act)

			switch act {

			case "add":
				{
					for port, act := range lbc.deltaL4Update.core {
						value := "udp"
						switch act {
						case "add":
							if err := lbc.lvsManager.Sync(addr, port, value); err != nil {
								delete(lbc.deltaAddr, addr)
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
								delete(lbc.deltaAddr, addr)
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
				}
			case "del":
				{
					for port, _ := range lbc.deltaL4Update.core {
						if err := lbc.lvsManager.Del(addr, port, "udp"); err != nil {
							delete(lbc.deltaAddr, addr)
							fmt.Printf("Failed to clear dirty real server, err %+v\n", err)
						}
					}
					for port, _ := range lbc.deltaTcpUpdate.core {
						if err := lbc.lvsManager.Del(addr, port, "tcp"); err != nil {
							delete(lbc.deltaAddr, addr)
							fmt.Printf("Failed to clear dirty real server, err %+v\n", err)
						}
					}
					fmt.Printf("testing end 2... %v\n", addr)
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
		client:     cfg.Client,
		queue:      workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "addipvs_layer4"),
		nginxqueue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "addipvs_layer3"),
	}

	lbc.protoToPort = newProtoToPort()
	lbc.prePortToProto = newProtoToPort()
	lbc.deltaL4Update = newProtoToPort()

	lbc.portToTcp = newProtoToPort()
	lbc.prePortToTcp = newProtoToPort()
	lbc.deltaTcpUpdate = newProtoToPort()

	lbc.lvsManager = ipvs.NewLvsManager()
	lbc.lvsManager.Init()

	lbc.nginxAddr = make(map[string]string)
	lbc.preNginxAddr = make(map[string]string)
	lbc.deltaAddr = make(map[string]string)

	lbc.uflag = false
	lbc.tflag = false
	lbc.nflag = false

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

	lbc.ngnixStore, lbc.nginxController = cache.NewInformer(
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
			AddFunc:    lbc.enqueueNginx,
			UpdateFunc: lbc.updateNginx,
			DeleteFunc: lbc.enqueueNginx,
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
		go wait.Until(lbc.hostWorker, time.Second, stopCh)
		go wait.Until(lbc.ipvsWorker, time.Second, stopCh)
	}

	<-stopCh
	fmt.Printf("Shutting down ipvs config controller")
	lbc.queue.ShutDown()
	lbc.nginxqueue.ShutDown()
}
