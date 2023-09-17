/*
Copyright 2023.

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

package controllers

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"github.com/c2fo/vfs/v6/vfssimple"
	"github.com/luthermonson/go-proxmox"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"path/filepath"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"strings"
	"time"

	"text/template"

	errors2 "errors"

	//"github.com/Telmate/proxmox-api-go/proxmox"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"os"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	infrastructurev1alpha1 "github.com/launchboxio/cluster-api-provider-proxmox/api/v1alpha1"
)

// ProxmoxMachineReconciler reconciles a ProxmoxMachine object
type ProxmoxMachineReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

const (
	VirtualMachineInitializing = "VirtualMachineInitializing"
)

//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=proxmoxmachines,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=proxmoxmachines/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=proxmoxmachines/finalizers,verbs=update
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=*,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;machines,verbs=get;list;watch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines;machines/status,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

const machineFinalizer = "infrastructure.cluster.x-k8s.io/finalizer"

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ProxmoxMachine object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *ProxmoxMachineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	contextLogger := log.FromContext(ctx)

	insecureHTTPClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	proxmoxClient := proxmox.NewClient(os.Getenv("PM_API_URL"),
		proxmox.WithAPIToken(os.Getenv("PM_API_TOKEN_ID"), os.Getenv("PM_API_TOKEN")),
		proxmox.WithHTTPClient(insecureHTTPClient),
	)

	proxmoxMachine := &infrastructurev1alpha1.ProxmoxMachine{}
	if err := r.Get(ctx, req.NamespacedName, proxmoxMachine); err != nil {
		if errors.IsNotFound(err) {
			contextLogger.Info("ProxmoxMachine resource not found, must be deleted")
			return ctrl.Result{}, nil
		}
		contextLogger.Error(err, "Failed to get ProxmoxMachine")
		return ctrl.Result{}, err
	}

	machine, err := util.GetOwnerMachine(ctx, r.Client, proxmoxMachine.ObjectMeta)
	if err != nil {
		contextLogger.Error(err, "getting owning machine")
		return ctrl.Result{}, fmt.Errorf("unable to get machine owner: %w", err)
	}

	if machine == nil {
		contextLogger.Info("Machine controller has not set OwnerRef")
		return ctrl.Result{}, nil
	}

	cluster, err := util.GetClusterFromMetadata(ctx, r.Client, machine.ObjectMeta)
	if err != nil {
		contextLogger.Info("Machine is missing cluster label or cluster does not exist")
		return ctrl.Result{}, nil //nolint:nilerr // We ignore it intentionally.
	}

	if annotations.IsPaused(cluster, proxmoxMachine) {
		contextLogger.Info("MicrovmMachine or linked Cluster is marked as paused. Won't reconcile")
		return ctrl.Result{}, nil
	}

	proxmoxCluster := &infrastructurev1alpha1.ProxmoxCluster{}
	proxmoxClusterName := client.ObjectKey{
		Namespace: cluster.Spec.InfrastructureRef.Namespace,
		Name:      cluster.Spec.InfrastructureRef.Name,
	}

	if getErr := r.Client.Get(ctx, proxmoxClusterName, proxmoxCluster); getErr != nil {
		if errors.IsNotFound(getErr) {
			contextLogger.Info("ProxmoxCluster is not ready yet")
			return ctrl.Result{}, nil
		}
		contextLogger.Error(getErr, "error getting proxmoxcluster", "id", proxmoxClusterName)
		return ctrl.Result{}, fmt.Errorf("error getting proxmoxcluster: %w", getErr)
	}

	if proxmoxMachine.GetDeletionTimestamp() != nil {
		if controllerutil.ContainsFinalizer(proxmoxMachine, machineFinalizer) {
			// TODO: Perform out of band cleanup
			contextLogger.Info("Cleaning up machine for finalizer")
			vm, err := loadVm(proxmoxClient, proxmoxMachine.Status.Vmid)
			if err != nil {
				return ctrl.Result{}, err
			}
			err = remove(vm)
			if err != nil {
				return ctrl.Result{}, err
			}

			controllerutil.RemoveFinalizer(proxmoxMachine, machineFinalizer)
			err = r.Update(context.TODO(), proxmoxMachine)
			return ctrl.Result{}, err
		}
	}

	// Fetch our VM template
	clusterTemplate, err := getVmTemplate(proxmoxClient, proxmoxMachine.Spec.Template)
	if err != nil || clusterTemplate == nil {
		contextLogger.Error(err, "Failed to get VM template")
		return ctrl.Result{}, err
	}

	node, err := proxmoxClient.Node(clusterTemplate.Node)
	if err != nil {
		contextLogger.Error(err, "Failed getting base node")
		return ctrl.Result{}, err
	}

	template, err := node.VirtualMachine(int(clusterTemplate.VMID))
	if err != nil {
		contextLogger.Error(err, "Failed getting base template")
		return ctrl.Result{}, err
	}

	// Clone the machine, set the VMID in the status, and then
	// simply return. We want to keep creation / and CRD updates
	// as idempotent as possible
	if proxmoxMachine.Status.Vmid == 0 {

		vmid, task, err := template.Clone(&proxmox.VirtualMachineCloneOptions{
			Name: fmt.Sprintf("%s-%s", proxmoxMachine.Namespace, proxmoxMachine.Name),
		})
		contextLogger.Info("Creating VM")
		if err != nil {
			contextLogger.Error(err, "Failed creating VM")
			return ctrl.Result{}, err
		}

		meta.SetStatusCondition(&proxmoxMachine.Status.Conditions, metav1.Condition{
			Type:    VirtualMachineInitializing,
			Status:  metav1.ConditionTrue,
			Reason:  "Creating",
			Message: "VM Created, Initializing for first launch",
		})

		proxmoxMachine.Status.Vmid = vmid
		err = r.Status().Update(ctx, proxmoxMachine)
		if err != nil {
			contextLogger.Error(err, "Failed updating ProxmoxMachine status")
			return ctrl.Result{}, err
		}

		err = task.WaitFor(10)
		if err != nil {
			contextLogger.Error(err, "Task didn't complete in time")
			return ctrl.Result{}, err
		}

		contextLogger.Info("Initial creation finished, requeuing for configuration")
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(proxmoxMachine, machineFinalizer) {
		contextLogger.Info("Attaching finalizer")
		controllerutil.AddFinalizer(proxmoxMachine, machineFinalizer)
		err = r.Update(ctx, proxmoxMachine)
		return ctrl.Result{}, err
	}

	// We should always have a new VM here. We query it to make sure
	vm, err := loadVm(proxmoxClient, proxmoxMachine.Status.Vmid)
	if err != nil {
		contextLogger.Error(err, "Failed getting VM status")
		return ctrl.Result{}, err
	}

	// If VM is still showing as initializing, we want to perform further configuration
	// - Configure network, disks, etc
	// - Setup cicustom
	// - Migrate to a target node
	// - Start the Virtual Machine
	if meta.IsStatusConditionTrue(proxmoxMachine.Status.Conditions, VirtualMachineInitializing) {
		task, err := vm.Config(vmInitializationOptions(proxmoxCluster, proxmoxMachine)...)
		if err != nil {
			contextLogger.Error(err, "Failed to reconfigure VM")
			return ctrl.Result{}, err
		}

		if err = task.Wait(time.Second*5, time.Minute*10); err != nil {
			contextLogger.Error(err, "Timed out waiting for VM to finish configuring")
			return ctrl.Result{}, err
		}

		if proxmoxMachine.Spec.TargetNode != "" && vm.Node != proxmoxMachine.Spec.TargetNode {
			contextLogger.Info(fmt.Sprintf("Moving VM to node %s", proxmoxMachine.Spec.TargetNode))
			task, err := vm.Migrate(proxmoxMachine.Spec.TargetNode, "")
			if err != nil {
				return ctrl.Result{}, err
			}

			if err = task.Wait(time.Second*5, time.Minute*10); err != nil {
				contextLogger.Error(err, "Timed out waiting for VM to migrate")
				return ctrl.Result{}, err
			}
		}

		meta.SetStatusCondition(&proxmoxMachine.Status.Conditions, metav1.Condition{
			Type:    VirtualMachineInitializing,
			Status:  metav1.ConditionFalse,
			Reason:  "Completed",
			Message: "VM initialization completed",
		})
		err = r.Status().Update(ctx, proxmoxMachine)
		if err != nil {
			contextLogger.Error(err, "Failed updating ProxmoxMachine status")
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	// Lastly, check runtime settings. Things like CPU, memory, etc
	// can be changed on an instance that has been initialized already
	options := pendingChanges(proxmoxMachine, vm)
	if len(options) > 0 {
		fmt.Println(options)
		contextLogger.Info("VM out of sync, updating configuration")
		task, err := vm.Config(options...)
		if err != nil {
			contextLogger.Error(err, "Failed to reconfigure VM")
			return ctrl.Result{}, err
		}

		if err = task.Wait(time.Second*5, time.Minute*10); err != nil {
			contextLogger.Error(err, "Timed out waiting for VM to finish configuring")
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	contextLogger.Info("We have a created VM :shrug:")
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ProxmoxMachineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrastructurev1alpha1.ProxmoxMachine{}).
		Complete(r)
}

func vmInitializationOptions(cluster *infrastructurev1alpha1.ProxmoxCluster, machine *infrastructurev1alpha1.ProxmoxMachine) []proxmox.VirtualMachineOption {
	options := []proxmox.VirtualMachineOption{
		{Name: "memory", Value: machine.Spec.Resources.Memory},
		{Name: "sockets", Value: machine.Spec.Resources.CpuSockets},
		{Name: "cores", Value: machine.Spec.Resources.CpuCores},
		{Name: "cicustom", Value: strings.Join([]string{
			fmt.Sprintf("user=%s/%s", cluster.Spec.SnippetStorageUri, ""),
			fmt.Sprintf("network=%s/%s", cluster.Spec.SnippetStorageUri, ""),
		}, ",")},
	}

	for idx, network := range machine.Spec.Networks {
		options = append(options, proxmox.VirtualMachineOption{
			Name: fmt.Sprintf("net%d", idx),
			Value: strings.Join([]string{
				fmt.Sprintf("model=%s", network.Model),
				fmt.Sprintf("bridge=%s", network.Bridge),
			}, ","),
		})
	}

	return options
}

func pendingChanges(machine *infrastructurev1alpha1.ProxmoxMachine, vm *proxmox.VirtualMachine) []proxmox.VirtualMachineOption {
	opts := []proxmox.VirtualMachineOption{}

	if vm.CPUs != machine.Spec.Resources.CpuCores {
		opts = append(opts, proxmox.VirtualMachineOption{
			Name:  "cores",
			Value: machine.Spec.Resources.CpuCores,
		})
	}

	// TODO: Due to differences in storage unit, memory
	// always shows as a change. Fine for now, but should
	// be addressed
	if int(vm.MaxMem) != machine.Spec.Resources.Memory {
		opts = append(opts, proxmox.VirtualMachineOption{
			Name:  "memory",
			Value: machine.Spec.Resources.Memory,
		})
	}

	return opts
}

// TODO: Remove a requirement for Cluster setup. Folks should be able
// to run ClusterAPI on a single proxmox instance
func getVmTemplate(px *proxmox.Client, templateName string) (*proxmox.ClusterResource, error) {
	cluster, err := px.Cluster()
	if err != nil {
		return nil, err
	}

	virtualMachines, err := cluster.Resources("vm")
	if err != nil {
		return nil, err
	}

	template := &proxmox.ClusterResource{}
	for _, virtualMachine := range virtualMachines {
		if virtualMachine.Name == templateName {
			template = virtualMachine
		}
	}

	return template, nil
}

func loadVm(px *proxmox.Client, vmid int) (*proxmox.VirtualMachine, error) {
	cluster, err := px.Cluster()
	if err != nil {
		return nil, err
	}

	virtualMachines, err := cluster.Resources("vm")
	if err != nil {
		return nil, err
	}

	template := &proxmox.ClusterResource{}
	for _, virtualMachine := range virtualMachines {
		if int(virtualMachine.VMID) == vmid {
			template = virtualMachine
		}
	}

	if template == nil {
		return nil, errors2.New("Failed to find VM in cluster")
	}

	node, err := px.Node(template.Node)
	if err != nil {
		return nil, err
	}

	return node.VirtualMachine(int(template.VMID))
}

func remove(vm *proxmox.VirtualMachine) error {
	task, err := vm.Stop()
	if err != nil {
		return err
	}
	if err = task.Wait(time.Second*5, time.Minute*10); err != nil {
		return err
	}

	task, err = vm.Delete()
	if err != nil {
		return err
	}
	if err = task.Wait(time.Second*5, time.Minute*10); err != nil {
		return err
	}
	return nil
}

func writeUserDataFile(cluster *infrastructurev1alpha1.ProxmoxCluster, path string, contents []byte) error {
	handle, err := vfssimple.NewFile(filepath.Join(cluster.Spec.SnippetStorageUri, path))
	if err != nil {
		return err
	}

	_, err = handle.Write(contents)
	return err
}

func generateUserNetworkData(networks []infrastructurev1alpha1.ProxmoxNetwork) ([]byte, error) {
	tmpl, err := template.New("network").Parse(`
#cloud-config
version: 1 
config: 
  {{range $item, $key := .Networks}}
  - type: physical
    subnets: 
      - type: dhcp
  {{end}}
`)

	if err != nil {
		return []byte{}, err
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, struct {
		Networks []infrastructurev1alpha1.ProxmoxNetwork
	}{
		Networks: networks,
	})
	return buf.Bytes(), err
}

var defaultKubernetesScript = template.Must(template.New("install").Parse(`
# Setup sysctl and modules
cat <<EOF | tee /etc/modules-load.d/containerd.conf
overlay
br_netfilter
EOF

modprobe overlay
modprobe br_netfilter

cat <<EOF | sudo tee /etc/sysctl.d/99-kubernetes-cri.conf
net.bridge.bridge-nf-call-iptables = 1
net.bridge.bridge-nf-call-ip6tables = 1
net.ipv4.ip_forward = 1
EOF
sysctl --system

# Install containerd
apt update -y
apt install -y containerd
mkdir -p /etc/containerd
containerd config default | tee /etc/containerd/config.toml
systemctl restart containerd
systemctl status containerd

# Install kubectl
curl -LO "https://dl.k8s.io/release/v{{ .KubernetesVersion }}/bin/linux/amd64/kubectl"
curl -LO "https://dl.k8s.io/v{{ .KubernetesVersion }}/bin/linux/amd64/kubectl.sha256"
echo "$(cat kubectl.sha256)  kubectl" | sha256sum --check

# Install kubeadm
`))

var packageManagerInstallScript = template.Must(template.New("packages").Parse(`
#!/usr/bin/env bash 

sudo apt-get update -y
sudo apt-get install -y apt-transport-https ca-certificates curl

sudo mkdir -m 755 /etc/apt/keyrings
curl -fsSL https://pkgs.k8s.io/core:/stable:/v1.28/deb/Release.key | sudo gpg --dearmor -o /etc/apt/keyrings/kubernetes-apt-keyring.gpg
echo 'deb [signed-by=/etc/apt/keyrings/kubernetes-apt-keyring.gpg] https://pkgs.k8s.io/core:/stable:/v1.28/deb/ /' | sudo tee /etc/apt/sources.list.d/kubernetes.list

sudo apt-get update
sudo apt-get install -y kubelet kubeadm kubectl
sudo apt-mark hold kubelet kubeadm kubectl
`))
