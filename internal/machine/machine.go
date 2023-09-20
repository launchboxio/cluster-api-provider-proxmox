package machine

import (
	"github.com/go-logr/logr"
	infrastructurev1alpha1 "github.com/launchboxio/cluster-api-provider-proxmox/api/v1alpha1"
	"github.com/luthermonson/go-proxmox"
	"k8s.io/client-go/tools/record"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Represents a requested Machine scope
type Machine struct {
	ProxmoxClient  *proxmox.Client
	ProxmoxMachine *infrastructurev1alpha1.ProxmoxMachine
	Machine        *clusterv1.Machine
	Cluster        *clusterv1.Cluster
	ProxmoxCluster *infrastructurev1alpha1.ProxmoxCluster
	Logger         logr.Logger
	client.Client
	Recorder record.EventRecorder
}

type BootstrapSecretFile struct {
	Path        string `yaml:"path"`
	Owner       string `yaml:"owner"`
	Permissions string `yaml:"permissions"`
	Content     string `yaml:"content"`
	Encoding    string `yaml:"encoding,omitempty"`
}

type BootstrapUser struct {
	Name                string   `yaml:"name"`
	Lock_Passwd         bool     `yaml:"lock_passwd"`
	Sudo                string   `yaml:"sudo"`
	Groups              []string `yaml:"groups"`
	Ssh_Authorized_Keys []string `yaml:"ssh_authorized_keys"`
}

type BootstrapSecret struct {
	RunCmd []string        `yaml:"runcmd"`
	Users  []BootstrapUser `yaml:"users,omitempty"`
	// TODO: Not sure why we need the underscore in the property name
	Write_Files []BootstrapSecretFile `yaml:"write_files"`
}
