package v1beta1

import (
	"context"
	"log"
	"math/rand"
	"time"

	"kubenetlab.net/knl/common"

	"github.com/kubevirt/macvtap-cni/pkg/deviceplugin"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/types"
)

// NOTE: macvtap configmap always lives in this NS, regardless where CR lives
var MacvtapConfigmapNameSpace = `knl`

const defaultCtxTimeout = 60 * time.Second

func UpdateMACVTAPDPCfg(clnt client.Client, deltas []deviceplugin.MacvtapConfig, add bool) error {
	// gconf := conf.GCONF
	ctx, cancelf := context.WithTimeout(context.Background(), defaultCtxTimeout)
	defer cancelf()
	var configmap corev1.ConfigMap
	var newMap map[string]deviceplugin.MacvtapConfig

	do := func() error {
		err := clnt.Get(ctx, types.NamespacedName{
			Namespace: MacvtapConfigmapNameSpace,
			Name:      deviceplugin.ConfigMapName,
		}, &configmap)
		if err != nil {
			return common.MakeErr(err)
		}
		if configmap.Data == nil {
			configmap.Data = make(map[string]string)
		}
		if cfg, ok := configmap.Data[deviceplugin.VTAPConfigKey]; !ok {
			if !add {
				return nil
			} else {
				newMap = make(map[string]deviceplugin.MacvtapConfig)
			}
		} else {
			newMap, err = deviceplugin.ReadConfigByStr(cfg)
			if err != nil || newMap == nil {
				newMap = make(map[string]deviceplugin.MacvtapConfig)
			}
		}

		for _, c := range deltas {
			if add {
				newMap[c.Name] = c
			} else {
				delete(newMap, c.Name)
			}
		}
		buf, err := deviceplugin.MarshalConfigMapToJson(newMap)
		if err != nil {
			return common.MakeErr(err)
		}
		configmap.Data[deviceplugin.VTAPConfigKey] = string(buf)
		return clnt.Update(ctx, &configmap)
	}
	var err error
	for i := 0; i < 3; i++ {
		err = do()
		if err == nil {
			return nil
		} else {
			log.Printf("error updating macvatp configmap, %v", err)
		}
		time.Sleep(time.Duration(rand.Intn(1000)) * time.Millisecond)
	}
	return err
}
