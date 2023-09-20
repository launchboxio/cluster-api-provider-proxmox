package machine

import (
	infrastructurev1alpha1 "github.com/launchboxio/cluster-api-provider-proxmox/api/v1alpha1"
	"github.com/luthermonson/go-proxmox"
	"math/rand"
	"time"
)

type Selector interface {
	Select(nodes proxmox.NodeStatuses) (*proxmox.NodeStatus, error)
}

// TODO: Eventually this will use a strategy for node selection
func (m *Machine) selectNode(selector Selector) (*proxmox.NodeStatus, error) {
	nodes, err := m.ProxmoxClient.Nodes()
	if err != nil {
		return nil, err
	}
	filteredNodes := filterNodes(nodes, m.ProxmoxMachine)
	if selector == nil {
		selector = &RandomNodeGroupSelector{}
	}

	return selector.Select(filteredNodes)
}

// TODO: This is just a stub function to further filter nodes
// to be filled out with a later update
// Tracked by: https://github.com/launchboxio/cluster-api-provider-proxmox/issues/7
// filterNodes removes any nodes from the list that don't have
// available CPU, memory, or disk
func filterNodes(nodes proxmox.NodeStatuses, machine *infrastructurev1alpha1.ProxmoxMachine) proxmox.NodeStatuses {
	return nodes
}

type RandomNodeGroupSelector struct{}

func (rng *RandomNodeGroupSelector) Select(nodes proxmox.NodeStatuses) (*proxmox.NodeStatus, error) {
	rand.Seed(time.Now().Unix())
	return nodes[rand.Intn(len(nodes))], nil
}
