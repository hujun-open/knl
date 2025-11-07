package v1beta1

type Link struct {
	//+required
	Name *string `json:"name,omitempty"` //if empty, system auto gen a default one
	//+required
	Connectors []Connector `json:"nodes"`
	GWAddr     *string     `json:"gwAddr,omitempty"` //a prefix
	MTU        *uint16     `json:"mtu,omitempty"`
}

type Connector struct {
	//+required
	NodeName *string `json:"node"` //node name, in case of distruited system like vsim/mag-c, it is the name of IOM VM
	PortId   *string `json:"port,omitempty"`
	Addr     *string `json:"addr,omitempty"` //a prefix, used for cloud-init on vmLinux, and podlinux
	Mac      *string `json:"mac,omitempty"`  //used for cloud-init on vmLinux, and podlinux
}
