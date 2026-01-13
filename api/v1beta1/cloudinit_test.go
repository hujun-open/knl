/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1beta1

import (
	"fmt"
	"testing"
)

func TestCloiudinit(t *testing.T) {
	c := Connector{
		Mac:    ReturnPointerVal("11:22:33:44:55:66"),
		Routes: []string{"1.1.1.1/32 via 192.168.100.1", "2.2.2.2/32 via 192.168.100.1"},
		Addrs:  []string{"192.168.100.99/24", "2001:beef::1/64"},
	}
	cfg := getDefCloudinitNetworkCfg()
	cfg.AddConnector("nic1", &c)
	fmt.Println(string(cfg.Marshal()))

}
