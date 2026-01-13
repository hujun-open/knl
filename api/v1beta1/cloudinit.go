package v1beta1

import (
	"net/netip"

	"github.com/goccy/go-yaml"
	k8slan "github.com/hujun-open/k8slan/api/v1beta1"
)

// +kubebuilder:object:generate=false
// +kubebuilder:object:root=false
type CloudInitNetworkConfig struct {
	Network *CloudInitNetworkConfigBody
}

// +kubebuilder:object:generate=false
// +kubebuilder:object:root=false
type CloudInitNetworkConfigBody struct {
	Version   int
	Ethernets map[string]*NicConfig
}

// +kubebuilder:object:generate=false
// +kubebuilder:object:root=false
type NicConfig struct {
	Dhcp4     bool
	Match     NicMatcher     `yaml:",omitempty"`
	Addresses []netip.Prefix `yaml:",omitempty"`
	Routes    []k8slan.Route `yaml:",omitempty"`
}

// +kubebuilder:object:generate=false
// +kubebuilder:object:root=false
type NicMatcher struct {
	Macaddress string `yaml:",omitempty"`
}

func (netcfg *CloudInitNetworkConfig) Marshal() []byte {

	buf, err := yaml.Marshal(*netcfg)
	if err != nil {
		panic(err)
	}
	return buf
}
func (netcfg *CloudInitNetworkConfig) AddConnector(nicName string, c *Connector) {
	netcfg.Network.Ethernets[nicName] = &NicConfig{
		Dhcp4: false,
		Match: NicMatcher{
			Macaddress: *c.Mac,
		},
		Addresses: []netip.Prefix{},
		Routes:    []k8slan.Route{},
	}
	for _, addStr := range c.Addrs {
		netcfg.Network.Ethernets[nicName].Addresses = append(netcfg.Network.Ethernets[nicName].Addresses, netip.MustParsePrefix(addStr))
	}
	for _, routeStr := range c.Routes {
		r := mustParseRoute(routeStr)
		netcfg.Network.Ethernets[nicName].Routes = append(netcfg.Network.Ethernets[nicName].Routes, *r)
	}
}

func getDefCloudinitNetworkCfg() CloudInitNetworkConfig {
	return CloudInitNetworkConfig{
		Network: &CloudInitNetworkConfigBody{
			Version: 2,
			Ethernets: map[string]*NicConfig{
				"firstnic": {
					Dhcp4: true,
					Match: NicMatcher{
						Macaddress: VMBaseMAC.String(),
					},
				},
			},
		},
	}

}
