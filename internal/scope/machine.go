package scope

import (
	"github.com/launchboxio/cluster-api-provider-proxmox/api/v1alpha1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

type MachineScope struct {
	Machine      *clusterv1.Machine
	InfraMachine *v1alpha1.ProxmoxMachine
}
