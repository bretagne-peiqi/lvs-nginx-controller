### Desgin doc

# Data structure design:

Suppose nginx pod always set in hostNetwork mode.

In the project, we keep a data structure for storing nginx pod status: running; nginx pod Host IP; action: delete/add/update; nginx Name;

type AddrStore struct {
	lock 	sync.RWMutex
	name    string 			 // nginx ingress controller Name
	status  string
	addr	map[string]Ports
	action  string 
}

var ports []int16

type PortStore struct {
	lock sync.RWMutex
	l4   map[string]ports
	name string // tcp-services && udp-services
}

type LBStore struct {
	lock 		  sync.RWMutex
	portStoreList []PortStore
	addrStore	  []AddrStore 
}

var LBQueue <- chan LBStore, len

# Logic design

* listwatch

* Finite State Machine:

# Usecases:

* add nginx-controller: 
	events: store new nginx-controller struct; keep old l4-cm struct;  <-chan trigger to sync add
* crash nginx-controller (update):
	events: update nginx-controller status to noRunning; keep old l4-cm struct; <- chan trigger to sync del
* del nginx-controller:
	same as crash

* add l4-tcp:
	events: store new service name; clear l4-udp cache; keep nginx-controller list; <- chan trigger to syn add l4  
* del l4-udp:
	events: store del service name and its ports; clear l4-tcp cache; keep nginx-controller list; <- chan trigger to sync del 
* update l4-tcp:
	events: store new l4-tcp all ports; clear l4-udp all ports; <- chan trigger to sync update 
