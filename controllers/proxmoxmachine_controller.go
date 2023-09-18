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
	"encoding/base64"
	"fmt"
	"github.com/luthermonson/go-proxmox"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	goyaml "gopkg.in/yaml.v3"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"

	//"k8s.io/apimachinery/pkg/types"
	"net/http"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"strings"
	"time"

	"text/template"

	//"github.com/Telmate/proxmox-api-go/proxmox"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"os"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	infrastructurev1alpha1 "github.com/launchboxio/cluster-api-provider-proxmox/api/v1alpha1"
	//bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
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
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=*,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;machines,verbs=get;list;watch
//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines;machines/status,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch;create;update;patch
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

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

	if proxmoxMachine.GetDeletionTimestamp() != nil {
		if controllerutil.ContainsFinalizer(proxmoxMachine, machineFinalizer) {
			// TODO: Perform out of band cleanup
			contextLogger.Info("Cleaning up machine for finalizer")
			vm, err := loadVm(proxmoxClient, proxmoxMachine.Status.Vmid)
			if err != nil {
				return ctrl.Result{}, err
			}
			if vm == nil {
				// Couldnt find the VM, I suppose we assume its deleted
				controllerutil.RemoveFinalizer(proxmoxMachine, machineFinalizer)
				err = r.Update(context.TODO(), proxmoxMachine)
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
		contextLogger.Info("ProxmoCluster or linked Cluster is marked as paused. Won't reconcile")
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
		// Try getting status again to prevent duplication of machines
		if err := r.Get(ctx, req.NamespacedName, proxmoxMachine); err != nil {
			contextLogger.Error(err, "Failed to get ProxmoxMachine")
			return ctrl.Result{}, err
		}
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

	if vm == nil {
		contextLogger.Info(fmt.Sprintf(
			"Resource had VMID of %d, but it didnt exist in Proxmox. Purging and starting over",
			proxmoxMachine.Status.Vmid,
		))
		return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
		// We have stored a proxmox VMID in the status of the CRD, yet the
		// machine doesnt exist in proxmox. For now, let's just purge the
		// status, ignore what exists in Proxmox, and create a fresh machine
		// We don't want to do this. Any network connectivity issue with
		// proxmox results in all of the machines being orphaned
		//proxmoxMachine.Status.Vmid = 0
		//proxmoxMachine.Status.Conditions = nil
		//err = r.Status().Update(ctx, proxmoxMachine)
		//return ctrl.Result{}, err
	}
	// If VM is still showing as initializing, we want to perform further configuration
	// - Configure network, disks, etc
	// - Setup cicustom
	// - Migrate to a target node
	// - Start the Virtual Machine
	if meta.IsStatusConditionTrue(proxmoxMachine.Status.Conditions, VirtualMachineInitializing) {
		if machine.Spec.Bootstrap.DataSecretName == nil {
			contextLogger.Info("No bootstrap secret found...")
			return ctrl.Result{}, nil
		}
		bootstrapSecret := &v1.Secret{}
		if err := r.Get(ctx, types.NamespacedName{
			Namespace: proxmoxMachine.Namespace,
			Name:      *machine.Spec.Bootstrap.DataSecretName,
		}, bootstrapSecret); err != nil {
			contextLogger.Info("Failed finding bootstrap secret")
			return ctrl.Result{}, err
		}

		credentialsSecret := &v1.Secret{}
		if err := r.Get(ctx, types.NamespacedName{
			Namespace: proxmoxMachine.Namespace,
			Name:      proxmoxCluster.Spec.Snippets.CredentialsSecretName,
		}, credentialsSecret); err != nil {
			contextLogger.Info("Failed finding storage credentials")
			return ctrl.Result{}, err
		}

		err = generateSnippets(
			bootstrapSecret,
			"v1.25.11", // TODO: Pull Kubernetes version from CRD
			credentialsSecret,
			"/mnt/default/snippets/snippets/",
			proxmoxMachine,
		)
		if err != nil {
			contextLogger.Info("Failed to generate snippet for machine")
			return ctrl.Result{}, err
		}

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

			node, err = proxmoxClient.Node(proxmoxMachine.Spec.TargetNode)
			if err != nil {
				contextLogger.Error(err, "Failed getting new node")
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

		// TODO: Need to get new node identifier
		task, err = vm.Start()
		if err != nil {
			contextLogger.Error(err, "Failed starting VM")
			return ctrl.Result{}, err
		}

		if err = task.Wait(time.Second*5, time.Minute*10); err != nil {
			contextLogger.Error(err, "Timed out waiting for VM to start")
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	// Lastly, check runtime settings. Things like CPU, memory, etc
	// can be changed on an instance that has been initialized already
	options := pendingChanges(proxmoxMachine, vm)
	if len(options) > 0 {
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
			fmt.Sprintf(
				"user=%s%s-%s-user.yaml",
				cluster.Spec.Snippets.StorageUri,
				machine.Namespace, machine.Name,
			),
			fmt.Sprintf(
				"network=%s%s-%s-network.yaml",
				cluster.Spec.Snippets.StorageUri,
				machine.Namespace, machine.Name,
			),
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

	vmTemplate := &proxmox.ClusterResource{}
	for _, virtualMachine := range virtualMachines {
		if int(virtualMachine.VMID) == vmid {
			vmTemplate = virtualMachine
		}
	}

	if vmTemplate.VMID == 0 {
		return nil, nil
	}

	node, err := px.Node(vmTemplate.Node)
	if err != nil {
		return nil, err
	}

	return node.VirtualMachine(int(vmTemplate.VMID))
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
	//handle, err := vfssimple.NewFile(filepath.Join(cluster.Spec.SnippetStorageUri, path))
	//if err != nil {
	//	return err
	//}
	//
	//_, err = handle.Write(contents)
	//return err
	return nil
}

func generateUserNetworkData(networks []infrastructurev1alpha1.ProxmoxNetwork) ([]byte, error) {
	tmpl, err := template.New("network").Parse(`
#cloud-config
version: 1
config:
- type: physical
  name: ens18
  subnets:
    - type: dhcp
- type: physical
  name: ens19
  subnets:
    - type: dhcp
- type: physical
  name: ens20
  subnets:
    - type: dhcp
`)
	//  {{range $item, $key := .Networks}}
	//  - type: physical
	//    name: ens{{ $item }}
	//    subnets:
	//      - type: dhcp
	//  {{end}}
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

var packageManagerInstallScript = template.Must(template.New("packages").Parse(`
#!/usr/bin/env bash
set -x

export repo=${1%.*}
apt-get update -y
apt-get install -y apt-transport-https ca-certificates curl gnupg qemu-guest-agent wget

# systemctl enable qemu-guest-agent
# systemctl start qemu-guest-agent

swapoff -a && sed -ri '/\sswap\s/s/^#?/#/' /etc/fstab
modprobe overlay && modprobe br_netfilter

tee /etc/sysctl.d/kubernetes.conf <<EOF
net.bridge.bridge-nf-call-ip6tables = 1
net.bridge.bridge-nf-call-iptables = 1
net.ipv4.ip_forward = 1
EOF

sysctl --system

tee /etc/modules-load.d/k8s.conf <<EOF
overlay
br_netfilter
EOF

curl -fsSL https://download.docker.com/linux/ubuntu/gpg | apt-key add -
add-apt-repository "deb [arch=amd64] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable"

apt update -y
apt install containerd.io -y
mkdir /etc/containerd
containerd config default>/etc/containerd/config.toml

modprobe overlay && modprobe br_netfilter
systemctl restart containerd
systemctl enable containerd

mkdir -p -m 755 /etc/apt/keyrings
curl -fsSL https://pkgs.k8s.io/core:/stable:/v${repo}/deb/Release.key | gpg --dearmor -o /etc/apt/keyrings/kubernetes-apt-keyring.gpg
echo "deb [signed-by=/etc/apt/keyrings/kubernetes-apt-keyring.gpg] https://pkgs.k8s.io/core:/stable:/v${repo}/deb/ /" | tee /etc/apt/sources.list.d/kubernetes.list

apt-get update -y
apt-get install -y kubelet=$1-* kubeadm=$1-* kubectl=$1-*
apt-mark hold kubelet kubeadm kubectl

swapoff -a && sed -ri '/\sswap\s/s/^#?/#/' /etc/fstab
`))

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

func generateSnippets(
	secret *v1.Secret,
	version string,
	credentialsSecret *v1.Secret,
	storagePath string,
	proxmoxMachine *infrastructurev1alpha1.ProxmoxMachine,
) error {
	bootstrapSecret := &BootstrapSecret{}
	err := yaml.Unmarshal(secret.Data["value"], bootstrapSecret)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	err = packageManagerInstallScript.Execute(&buf, struct{}{})
	// Append the runCmd for our new scripte
	bootstrapSecret.RunCmd = append([]string{
		fmt.Sprintf("sudo /init.sh %s", strings.TrimPrefix(version, "v")),
	}, bootstrapSecret.RunCmd...)
	bootstrapSecret.Write_Files = append(
		bootstrapSecret.Write_Files,
		BootstrapSecretFile{
			Path:        "/init.sh",
			Permissions: "0755",
			Owner:       "root:root",
			Content:     base64.StdEncoding.EncodeToString(buf.Bytes()),
			Encoding:    "b64",
		},
	)

	if len(proxmoxMachine.Spec.SshKeys) > 0 {
		bootstrapSecret.Users = []BootstrapUser{{
			Name:                "ubuntu",
			Lock_Passwd:         false,
			Groups:              []string{"sudo"},
			Sudo:                "ALL=(ALL) NOPASSWD:ALL",
			Ssh_Authorized_Keys: proxmoxMachine.Spec.SshKeys,
		}}
	}

	userContents, err := goyaml.Marshal(bootstrapSecret)
	if err != nil {
		return err
	}

	networkData, err := generateUserNetworkData(proxmoxMachine.Spec.Networks)
	if err != nil {
		return err
	}

	// We now have 2 snippets, let's write them to the storage
	networkFileName := fmt.Sprintf("%s-%s-network.yaml", proxmoxMachine.Namespace, proxmoxMachine.Name)
	userFileName := fmt.Sprintf("%s-%s-user.yaml", proxmoxMachine.Namespace, proxmoxMachine.Name)
	err = writeFile(
		credentialsSecret,
		storagePath,
		networkFileName,
		networkData,
	)
	if err != nil {
		return err
	}

	err = writeFile(
		credentialsSecret,
		storagePath,
		userFileName,
		[]byte(fmt.Sprintf(`
#cloud-config
%s
`, userContents)),
	)
	if err != nil {
		return err
	}

	return nil
}

func writeFile(credentials *v1.Secret, storagePath string, filePath string, contents []byte) error {
	config := &ssh.ClientConfig{
		User: string(credentials.Data["user"]),
		Auth: []ssh.AuthMethod{
			ssh.Password(string(credentials.Data["password"])),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	client, err := ssh.Dial("tcp", string(credentials.Data["host"]), config)
	if err != nil {
		return err
	}
	defer client.Close()

	fs, err := sftp.NewClient(client)
	if err != nil {
		return err
	}
	defer fs.Close()

	dstFile, err := fs.Create(fmt.Sprintf(
		"%s/%s", storagePath, filePath,
	))
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = dstFile.Write(contents)
	return err
}
