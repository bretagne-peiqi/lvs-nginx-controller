package controller

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/lvs-controller/pkg/config"
	"github.com/lvs-controller/pkg/provider/ipvs"

	glog "github.com/zoumo/logdog"
	"k8s.io/api/core/v1"
	v2 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

const (
	nginxServer  = "ingress-nginx"
	nginxPattern = "nginx-ingress-controller"

	tcpService = "tcp-services"
	udpService = "udp-services"

	deletion = "deleting"
	addition = "adding"

	protoTcp = "tcp"
	protoUdp = "udp"
)

var locker = new(sync.Mutex)
var cond = sync.NewCond(locker)

type LoadBalancerController struct {
	client kubernetes.Interface

	//This is used to monitor layer 4 tcp && udp config updates
	controller cache.Controller

	//Nginx ingress controller layer 7
	addrController cache.Controller

	NwTcp map[uint16]string
	ExTcp map[uint16]string
	NwUdp map[uint16]string
	ExUdp map[uint16]string

	clbdata *LBStore
	plbdata *LBStore

	lbstore chan *LBStore

	lvsManager *ipvs.LvsManager
}

func (lbc *LoadBalancerController) Initial() error {
	cond.L.Lock()
	for len(lbc.lbstore) == 3 {
		glog.Info("Initial Func Unit is waiting for k8s events ...")
		cond.Wait()
	}

	if err := lbc.listCm(); err != nil {
		return fmt.Errorf("Failed to initial list l4 configmaps; err: %v\n", err)
	}

	for key, _ := range lbc.plbdata.Tcp {
		lbc.ExTcp[key] = protoTcp
	}

	for key, _ := range lbc.plbdata.Udp {
		lbc.ExUdp[key] = protoUdp
	}

	if err := lbc.listNginx(lbc.plbdata); err != nil {
		return fmt.Errorf("Failed to initial list l4 addrs; err: %v\n", err)
	}

	glog.Infof("Successfully Initialized controller cache, now l4 tcp ports %v, udp ports %v; l4 addrs %v\n", lbc.plbdata.Tcp, lbc.plbdata.Udp, lbc.plbdata.Addr)
	lbc.lbstore <- lbc.plbdata

	cond.L.Unlock()
	glog.Info("Initial Func Unit finished its task for k8s events ...")
	cond.Broadcast()
	return nil

}

func (lbc *LoadBalancerController) updatePorts(oldobj, newobj interface{}) {
	oldCm := oldobj.(*v1.ConfigMap)
	cm := newobj.(*v1.ConfigMap)

	if cm.GetDeletionTimestamp != nil {
		fmt.Errorf("configmap deleted pods %v\n", cm)
		//panic(fmt.Errorf("layer4 configmaps %v deleted!!! which should never happen\n", newCm.GetName()))
	}

	if reflect.DeepEqual(oldCm.Data, cm.Data) {
		return
	}

	if cm.GetName() == tcpService {
		cond.L.Lock()
		for len(lbc.lbstore) == 3 {
			glog.Info("Update Ports Func tcpsvc Unit is waiting for k8s events ...")
			cond.Wait()
		}
		for key, _ := range cm.Data {
			ok, port := ValidatePort(key)
			if !ok {
				glog.Errorf("Update Ports Func found invalid port %v\n", key)
			} else {
				lbc.NwTcp[port] = protoTcp
				glog.Debugf("Update Ports Func updating layer l4 port proto %v tcp\n", key)
			}
		}
		lbc.clbdata.L4Compare(lbc.ExTcp, lbc.NwTcp, lbc.clbdata.Tcp)
		if err := lbc.listNginx(lbc.clbdata); err != nil {
			glog.Errorf("Failed in list Nginx %v\n", err)
		}
		lbc.lbstore <- lbc.clbdata

		cond.L.Unlock()
		glog.Info("Update Ports Func Unit finished its task for k8s events ...")
		cond.Signal()
		time.Sleep(time.Millisecond * 60)
	} else if cm.GetName() == udpService {
		cond.L.Lock()
		for len(lbc.lbstore) == 3 {
			glog.Info("Update Ports Func updsvc Unit is waiting for k8s events ...")
			cond.Wait()
		}
		for key, _ := range cm.Data {
			ok, port := ValidatePort(key)
			if !ok {
				glog.Errorf("Update Ports Func found invalid port %v\n", key)
			} else {
				lbc.NwUdp[port] = protoUdp
				glog.Debugf("Update Ports Func updating layer l4 port proto %v udp\n", key)
			}
		}
		lbc.clbdata.L4Compare(lbc.ExUdp, lbc.NwUdp, lbc.clbdata.Udp)
		if err := lbc.listNginx(lbc.clbdata); err != nil {
			glog.Errorf("Failed in list Nginx %v\n", err)
		}
		lbc.lbstore <- lbc.clbdata

		cond.L.Unlock()
		glog.Info("Update Ports Func Unit finished its task for k8s events ...")
		cond.Signal()
		time.Sleep(time.Millisecond * 60)
	}
	glog.Debugf("After trigger from cm: lbc.ExTcp %v lbc.ExUdp %v\n", lbc.ExTcp, lbc.ExUdp)

	glog.Debugf("Before sending from cm: lbc.clbdata.Tcp %v, lbc.clbdata.Udp %v; lbc.clbdata.Addr %v\n", lbc.clbdata.Tcp, lbc.clbdata.Udp, lbc.clbdata.Addr)
	return

}

func (lbc *LoadBalancerController) listNginx(LB *LBStore) error {

	var options v2.ListOptions
	podlist, err := lbc.client.CoreV1().Pods(nginxServer).List(options)

	if err != nil {
		return fmt.Errorf("Failed to list pods, err %+v\n", err)
	}

	for key, _ := range LB.Addr {
		delete(LB.Addr, key)
	}

	for _, pod := range podlist.Items {
		if strings.Contains(pod.GetName(), nginxPattern) && pod.Status.PodIP != "" {
			key := fmt.Sprintf("adding_%s", pod.GetName())
			ok, IPv4 := ValidateIPv4(pod.Status.PodIP)
			if !ok {
				return fmt.Errorf("Failed to parse ipv4 %v %v\n", key)
			} else {
				LB.Addr[key] = IPv4
			}
		}
	}
	return nil
}

func (lbc *LoadBalancerController) listCm() error {
	var options v2.ListOptions
	cmlist, err := lbc.client.CoreV1().ConfigMaps(nginxServer).List(options)

	if err != nil {
		return fmt.Errorf("Failed to list cm, err %+v\n", err)
	}

	for port, _ := range lbc.plbdata.Tcp {
		delete(lbc.plbdata.Tcp, port)
	}

	for port, _ := range lbc.plbdata.Udp {
		delete(lbc.plbdata.Udp, port)
	}

	for _, cm := range cmlist.Items {
		if strings.Contains(cm.GetName(), udpService) {
			for key, _ := range cm.Data {
				ok, port := ValidatePort(key)
				if !ok {
					return fmt.Errorf("Failed to validate udp port in listCm %v\n", key)
				} else {
					lbc.plbdata.Udp[port] = addition
				}
			}
		}
		if strings.Contains(cm.GetName(), tcpService) {
			for key, _ := range cm.Data {
				ok, port := ValidatePort(key)
				if !ok {
					return fmt.Errorf("Failed to validate tcp port in listCm %v\n", key)
				} else {
					lbc.plbdata.Tcp[port] = addition
				}
			}
		}
	}
	return nil
}

//FIXME this function has been deprecated, remove it
func (lbc *LoadBalancerController) addrAddFunc(obj interface{}) {

	pod := obj.(*v1.Pod)
	if pod.GetNamespace() == nginxServer && strings.Contains(pod.GetName(), nginxPattern) && pod.Status.PodIP != "" {

		key := fmt.Sprintf("adding_%v\n", pod.GetName())
		ok, addr := ValidateIPv4(pod.Status.PodIP)
		if !ok {
			glog.Errorf("Failed parse ipv4 %v\n", addr)
		} else {
			lbc.plbdata.Addr[key] = addr
		}
	}
}

func (lbc *LoadBalancerController) addrUpdateFunc(newobj, oldobj interface{}) {
	//oldPod := oldobj.(*v1.Pod)
	newPod := newobj.(*v1.Pod)

	if newPod.GetNamespace() == nginxServer && newPod.Status.PodIP != "" {
		glog.Info("Addressing update func received k8s events ...")
		cond.L.Lock()
		for len(lbc.lbstore) == 3 {
			glog.Info("Addressing update func is waiting for k8s events ...")
			cond.Wait()
		}

		for key, _ := range lbc.plbdata.Addr {
			delete(lbc.plbdata.Addr, key)
		}

		if newPod.GetDeletionTimestamp() != nil {
			key := fmt.Sprintf("deleting_%v\n", newPod.GetName())
			ok, addr := ValidateIPv4(newPod.Status.PodIP)
			if !ok {
				glog.Errorf("Failed parse ipv4 %v\n", addr)
			} else {
				lbc.plbdata.Addr[key] = addr
			}
		} else {
			key := fmt.Sprintf("adding_%v\n", newPod.GetName())
			ok, addr := ValidateIPv4(newPod.Status.PodIP)
			if !ok {
				glog.Errorf("Failed parse ipv4 %v\n", addr)
			} else {
				lbc.plbdata.Addr[key] = addr
			}
		}
		err := lbc.listCm()
		if err != nil {
			glog.Errorf("Failed in getting layer4 list %v\n", err)
		}

		glog.Debugf("Before sending from nginx: lbc.plbdata.Tcp %v, lbc.plbdata.Udp %v; lbc.plbdata.Addr %v\n", lbc.plbdata.Tcp, lbc.plbdata.Udp, lbc.plbdata.Addr)
		lbc.lbstore <- lbc.plbdata

		cond.L.Unlock()
		glog.Info("Addressing update func finished its task in k8s events ...")

		cond.Signal()
		time.Sleep(time.Millisecond * 60)
	}
	return
}

func (lbc *LoadBalancerController) processItem() {

	processFunc := func() bool {

		cond.L.Lock()

		for len(lbc.lbstore) == 0 {
			glog.Info("Processing Item Unit is waiting for k8s events ...")
			cond.Wait()
		}

		lbdata := <-lbc.lbstore

		for key, addr := range lbdata.Addr {
			glog.Debugf("lbdata address %v and its action %v\n", string(addr), key)
			keys := strings.Split(key, "_")
			switch keys[0] {
			case addition:
				{
					for port, act := range lbdata.Tcp {
						switch act {
						case addition:
							if err := lbc.lvsManager.Add(addr, port, protoTcp); err != nil {
								glog.Errorf("Failed to sync ipvs tcp config in dr mode, err %+v\n", err)
							}
						case deletion:
							if err := lbc.lvsManager.Del(addr, port, protoTcp); err != nil {
								glog.Errorf("Failed to del ipvs tcp config in dr mode, err %+v\n", err)
							}
						default:
							glog.Errorf("Failed, found unrecognizable action")
						}
					}
					for port, act := range lbdata.Udp {
						switch act {
						case addition:
							if err := lbc.lvsManager.Add(addr, port, protoUdp); err != nil {
								glog.Errorf("Failed to sync ipvs udp config in dr mode, err %+v\n", err)
							}
						case deletion:
							if err := lbc.lvsManager.Del(addr, port, protoUdp); err != nil {
								glog.Errorf("Failed to del ipvs udp config in dr mode, err %+v\n", err)
							}
						default:
							glog.Errorf("Failed, found unrecognizable action")
						}
					}
				}
			case deletion:
				{
					glog.Debugf("deletion case: lbdata address %v and its action %v\n", string(addr), key)
					if err := lbc.lvsManager.DelRServer(addr); err != nil {
						glog.Errorf("Failed to clear dirty real server, err %+v\n", err)
					}
				}
			}
		}

		cond.L.Unlock()
		cond.Broadcast()
		time.Sleep(time.Millisecond * 60)

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
	}

	lbc.lbstore = make(chan *LBStore, 3)
	lbc.clbdata = NewLBStore()
	lbc.plbdata = NewLBStore()

	lbc.ExTcp = make(map[uint16]string)
	lbc.ExUdp = make(map[uint16]string)
	lbc.NwTcp = make(map[uint16]string)
	lbc.NwUdp = make(map[uint16]string)

	lbc.lvsManager = ipvs.NewLvsManager()
	lbc.lvsManager.Init(cfg)

	podListWatcher := cache.NewListWatchFromClient(lbc.client.CoreV1().RESTClient(), "pods", nginxServer, fields.Everything())
	_, lbc.controller = cache.NewInformer(
		podListWatcher,
		&v1.Pod{},
		0,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    lbc.addrAddFunc,
			UpdateFunc: lbc.addrUpdateFunc,
		},
	)

	cmListWatcher := cache.NewListWatchFromClient(lbc.client.CoreV1().RESTClient(), "configmaps", nginxServer, fields.Everything())
	_, lbc.addrController = cache.NewInformer(
		cmListWatcher,
		&v1.ConfigMap{},
		0,
		cache.ResourceEventHandlerFuncs{
			UpdateFunc: lbc.updatePorts,
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

	glog.Debugf("workers numbers total are %v\n", workers)
	for i := 0; i < workers; i++ {
		go wait.Until(lbc.processItem, time.Second, stopCh)
	}

	<-stopCh
	close(lbc.lbstore)
	glog.Error("Shutting down ipvs config controller")
}
