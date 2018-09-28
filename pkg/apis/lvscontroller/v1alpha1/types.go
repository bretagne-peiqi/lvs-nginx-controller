package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// LVS is a specification for a LVS resource
type Configuration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	TCPEndpoints	 []TCPService	`json:"tCPEndpoints"`
	UDPEndpoints	 []UDPService	`json:"uDPEndpoints"`
}

type TCPService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Name		string		`json: "name"`
	Namespace   string     `json: "namespace"`

	Port		string       `json:"port"`
	Protocol    string		 `json:"protocol"`
	PersistConns     uint32  `json:"persistConns"`
	FWMark           uint32  `json:"fWMark"`
	Iptables	[]IptablesMangle	`json: "iptables"`
}

type  IptablesMangle struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Port	[]uint32	`json: "port"`

}

type UDPService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Name		string		`json: "name"`
	Namespace   string     `json: "namespace"`
	Port		uint32	 `json:"port"`
	Protocol	string		 `json:"protocol"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ConfigurationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Configuration `json:"items"`
}
