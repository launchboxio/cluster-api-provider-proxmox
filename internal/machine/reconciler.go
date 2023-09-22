package machine

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	infrastructurev1alpha1 "github.com/launchboxio/cluster-api-provider-proxmox/api/v1alpha1"
	"github.com/launchboxio/cluster-api-provider-proxmox/internal/install"
	"github.com/luthermonson/go-proxmox"
	"gopkg.in/yaml.v2"
	goyaml "gopkg.in/yaml.v3"
	v1 "k8s.io/api/core/v1"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/strings/slices"
	"os"
	"path/filepath"
	"sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/conditions"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"strconv"
	"strings"
	"time"
)

const (
	machineFinalizer           = "infrastructure.cluster.x-k8s.io/finalizer"
	ProviderIDPrefix           = "proxmox://"
	VirtualMachineInitializing = "VirtualMachineInitializing"

	VmConfigurationTimeout = time.Minute * 10
	VmMigrationTimeout     = time.Minute * 10
	VmStartTimeout         = time.Minute * 10
	VmStopTimeout          = time.Minute * 10
	VmDeleteTimeout        = time.Minute * 10
)

// TODO: Remove a requirement for Cluster setup. Folks should be able
// to run ClusterAPI on a single proxmox instance
func (m *Machine) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	if m.ProxmoxMachine.GetDeletionTimestamp() != nil {
		if controllerutil.ContainsFinalizer(m.ProxmoxMachine, machineFinalizer) {
			return m.reconcileDelete(ctx, req)
		}
		return ctrl.Result{}, nil
	}
	return m.reconcileCreate(ctx, req)
}

func (m *Machine) reconcileCreate(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	clusterTemplate, err := getVmTemplate(m.ProxmoxClient, m.ProxmoxMachine.Spec.Template)
	if err != nil || clusterTemplate == nil {
		m.Logger.Error(err, "Failed to get VM template")
		return ctrl.Result{}, err
	}

	node, err := m.ProxmoxClient.Node(clusterTemplate.Node)
	if err != nil {
		m.Logger.Error(err, "Failed getting base node")
		return ctrl.Result{}, err
	}

	template, err := node.VirtualMachine(int(clusterTemplate.VMID))
	if err != nil {
		m.Logger.Error(err, "Failed getting base template")
		return ctrl.Result{}, err
	}

	if m.ProxmoxMachine.Spec.ProviderID == "" {
		node, err := m.selectNode(nil)
		if err != nil {
			m.Logger.Error(err, "Failed selecting node for VM")
			return ctrl.Result{}, err
		}

		// Try getting status again to prevent duplication of machines
		if err := m.Get(ctx, req.NamespacedName, m.ProxmoxMachine); err != nil {
			m.Logger.Error(err, "Failed to get ProxmoxMachine")
			return ctrl.Result{}, err
		}
		// TODO: We need to monitor the task for failure.
		// TODO: Ocassionally, we are getting "the object has been modified", causing
		// extra VM instances to be created //

		vmid, task, err := template.Clone(&proxmox.VirtualMachineCloneOptions{
			Name:   fmt.Sprintf("%s-%s", m.ProxmoxMachine.Namespace, m.ProxmoxMachine.Name),
			Target: node.Node,
		})
		m.Logger.Info("Creating VM")
		if err != nil {
			m.Logger.Error(err, "Failed creating VM")
			m.Recorder.Event(m.ProxmoxMachine, v1.EventTypeWarning, err.Error(), "Failed creating VM")
			return ctrl.Result{}, err
		}

		m.ProxmoxMachine.Spec.ProviderID = fmt.Sprintf("%s%d", ProviderIDPrefix, vmid)
		err = m.Update(ctx, m.ProxmoxMachine)
		if err != nil {
			m.Logger.Error(err, "Failed updating ProxmoxMachine status")
			return ctrl.Result{}, err
		}

		m.Recorder.Event(m.ProxmoxMachine, v1.EventTypeNormal, "", "Created VM with Provider ID "+m.ProxmoxMachine.Spec.ProviderID)

		m.ProxmoxMachine.Status.Vmid = vmid
		conditions.MarkTrue(m.ProxmoxMachine, VirtualMachineInitializing)

		err = m.Status().Update(ctx, m.ProxmoxMachine)
		if err != nil {
			return ctrl.Result{}, err
		}

		err = task.WaitFor(10)
		if err != nil {
			m.Logger.Error(err, "Task didn't complete in time")
			return ctrl.Result{}, err
		}

		m.Logger.Info("Initial creation finished, requeuing for configuration")
		return ctrl.Result{}, nil
	}

	vmid, err := strconv.Atoi(
		strings.TrimPrefix(m.ProxmoxMachine.Spec.ProviderID, ProviderIDPrefix),
	)
	if err != nil {
		m.Logger.Error(err, "Failed parsing provider ID")
		return ctrl.Result{}, err
	}

	if !controllerutil.ContainsFinalizer(m.ProxmoxMachine, machineFinalizer) {
		m.Logger.Info("Attaching finalizer")
		controllerutil.AddFinalizer(m.ProxmoxMachine, machineFinalizer)
		err = m.Update(ctx, m.ProxmoxMachine)
		return ctrl.Result{}, err
	}

	// We should always have a new VM here. We query it to make sure
	vm, err := loadVm(m.ProxmoxClient, vmid)
	if err != nil {
		m.Logger.Error(err, "Failed getting VM status")
		return ctrl.Result{}, err
	}

	if vm == nil {
		m.Logger.Info(fmt.Sprintf(
			"Resource had VMID of %d, but it didnt exist in Proxmox. Purging and starting over",
			m.ProxmoxMachine.Status.Vmid,
		))
		return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
		// We have stored a proxmox VMID in the status of the CRD, yet the
		// machine doesnt exist in proxmox. For now, let's just purge the
		// status, ignore what exists in Proxmox, and create a fresh machine
		// We don't want to do this. Any network connectivity issue with
		// proxmox results in all of the machines being orphaned
		//m.ProxmoxMachine.Status.Vmid = 0
		//m.ProxmoxMachine.Status.Conditions = nil
		//err = r.Status().Update(ctx, m.ProxmoxMachine)
		//return ctrl.Result{}, err
	}

	// Attach tags
	tags := append(m.ProxmoxMachine.Spec.Tags, m.ProxmoxCluster.Spec.Tags...)
	for _, tag := range tags {
		if !vm.HasTag(tag) {
			_, err := vm.AddTag(tag)
			if err != nil {
				m.Logger.Error(err, "Failed adding tag to VM")
			}
		}
	}

	// If the user has provided a resource pool, attach the
	// created VM to the specified pool
	if m.ProxmoxCluster.Spec.Pool != "" {
		pool, err := m.ensurePool(m.ProxmoxCluster.Spec.Pool)
		if err != nil {
			m.Recorder.Event(m.ProxmoxMachine, v1.EventTypeWarning, err.Error(), "Failed getting Resource Pool: %v")
			m.Logger.Error(err, "Failed to ensure VM pool")
			return ctrl.Result{}, err
		}

		poolMembers := []string{}
		for _, member := range pool.Members {
			poolMembers = append(poolMembers, strconv.FormatUint(member.VMID, 10))
		}

		memberId := strconv.FormatUint(uint64(vm.VMID), 10)
		if !slices.Contains(poolMembers, memberId) {
			poolMembers = append(poolMembers, memberId)
			err = pool.Update(&proxmox.PoolUpdateOption{
				VirtualMachines: memberId,
			})
			if err != nil {
				return ctrl.Result{}, err
			}
			m.Recorder.Event(m.ProxmoxMachine, "Normal", "", "Added to resource pool "+pool.PoolID)
		}
	}

	// If VM is still showing as initializing, we want to perform further configuration
	// - Configure network, disks, etc
	// - Setup cicustom
	// - Migrate to a target node
	// - Start the Virtual Machine
	if conditions.IsTrue(m.ProxmoxMachine, VirtualMachineInitializing) {
		if m.Machine.Spec.Bootstrap.DataSecretName == nil {
			m.Logger.Info("No bootstrap secret found...")
			return ctrl.Result{}, nil
		}
		bootstrapSecret := &v1.Secret{}
		if err := m.Get(ctx, types.NamespacedName{
			Namespace: m.ProxmoxMachine.Namespace,
			Name:      *m.Machine.Spec.Bootstrap.DataSecretName,
		}, bootstrapSecret); err != nil {
			m.Logger.Info("Failed finding bootstrap secret")
			return ctrl.Result{}, err
		}

		err = generateSnippets(
			bootstrapSecret,
			*m.Machine.Spec.Version,
			"/mnt/default/snippets/snippets/",
			m.ProxmoxMachine,
		)
		if err != nil {
			m.Logger.Info("Failed to generate snippet for machine")
			return ctrl.Result{}, err
		}

		task, err := vm.Config(vmInitializationOptions(m.ProxmoxCluster, m.ProxmoxMachine)...)
		if err != nil {
			m.Recorder.Event(m.ProxmoxMachine, v1.EventTypeWarning, err.Error(), "VM Configuration failure")
			m.Logger.Error(err, "Failed to reconfigure VM")
			return ctrl.Result{Requeue: false}, err
		}

		if err = task.Wait(time.Second*5, VmConfigurationTimeout); err != nil {
			m.Recorder.Event(m.ProxmoxMachine, v1.EventTypeWarning, err.Error(), "VM Configuration timeout")
			m.Logger.Error(err, "Timed out waiting for VM to finish configuring")
			return ctrl.Result{}, err
		}

		conditions.MarkFalse(
			m.ProxmoxMachine, VirtualMachineInitializing, "Completed",
			v1beta1.ConditionSeverityNone, "VM initialization completed")

		err = m.Status().Update(ctx, m.ProxmoxMachine)
		if err != nil {
			m.Logger.Error(err, "Failed updating ProxmoxMachine status")
			return ctrl.Result{}, err
		}

		m.Recorder.Event(m.ProxmoxMachine, "Normal", "", "Starting machine")
		task, err = vm.Start()
		if err != nil {
			m.Logger.Error(err, "Failed starting VM")
			return ctrl.Result{}, err
		}

		if err = task.Wait(time.Second*5, VmStartTimeout); err != nil {
			m.Recorder.Event(m.ProxmoxMachine, v1.EventTypeWarning, err.Error(), "VM Start timeout")
			m.Logger.Error(err, "Timed out waiting for VM to start")
			return ctrl.Result{}, err
		}

		m.Recorder.Event(m.ProxmoxMachine, v1.EventTypeNormal, "", "Machine started")
		return ctrl.Result{}, nil
	}

	// Ensure our machine is marked as Ready
	if !m.ProxmoxMachine.Status.Ready {
		_ = m.Get(ctx, req.NamespacedName, m.ProxmoxMachine)
		m.ProxmoxMachine.Status.Ready = true
		if err = m.Status().Update(ctx, m.ProxmoxMachine); err != nil {
			return ctrl.Result{}, err
		}

		conditions.MarkTrue(m.ProxmoxMachine, "Ready")
		err = m.Status().Update(ctx, m.ProxmoxMachine)
		return ctrl.Result{}, nil
	}

	// Lastly, wait for our node to get the registered providerID. Once it does,
	// we find the node and annotate it with the machine ID
	if m.Machine.Status.NodeRef == nil {
		// Attempt to attach node until successful
		// We probably shouldnt endlessly retry this,
		// TODO: Find a better method to notice when the node is ready
		_, err := m.attachNode(ctx)
		if err != nil {
			return ctrl.Result{RequeueAfter: time.Second * 15}, nil
		}
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

func (m *Machine) getRESTClient(clusterName string) (*kubernetes.Clientset, error) {
	secret := &v1.Secret{}
	err := m.Get(context.TODO(), types.NamespacedName{
		Namespace: m.Cluster.Namespace,
		Name:      fmt.Sprintf("%s-kubeconfig", clusterName),
	}, secret)
	if err != nil {
		return nil, err
	}
	config, err := clientcmd.RESTConfigFromKubeConfig(secret.Data["value"])
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(config)
}

func (m *Machine) attachNode(ctx context.Context) (ctrl.Result, error) {
	client, err := m.getRESTClient(m.Cluster.Name)
	if err != nil {
		return ctrl.Result{}, err
	}

	nodeName := fmt.Sprintf("%s-%s", m.ProxmoxMachine.Namespace, m.ProxmoxMachine.Name)
	node, err := client.
		CoreV1().
		Nodes().
		Get(ctx, nodeName, v12.GetOptions{})
	if err != nil {
		return ctrl.Result{}, err
	}

	type Patch struct {
		Op    string `json:"op"`
		Path  string `json:"path"`
		Value string `json:"value"`
	}

	var patches []Patch

	if node.Spec.ProviderID != m.ProxmoxMachine.Spec.ProviderID {
		patches = append(patches, Patch{
			Op:    "add",
			Path:  "/spec/providerID",
			Value: m.ProxmoxMachine.Spec.ProviderID,
		})
	}

	if _, ok := node.ObjectMeta.Annotations["cluster.x-k8s.io"]; !ok {
		patches = append(patches, Patch{
			Op:    "add",
			Path:  "/metadata/annotations/cluster.x-k8s.io",
			Value: m.Machine.Name,
		})
	}

	if len(patches) > 0 {
		payloadBytes, _ := json.Marshal(patches)

		_, err = client.
			CoreV1().
			Nodes().
			Patch(ctx, node.Name, types.JSONPatchType, payloadBytes, v12.PatchOptions{})
		if err != nil {
			m.Recorder.Event(m.ProxmoxMachine, v1.EventTypeWarning, err.Error(), "Failed attaching node to machine")
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

func (m *Machine) reconcileDelete(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// TODO: Perform out of band cleanup
	m.Logger.Info("Cleaning up machine for finalizer")
	vmid, err := strconv.Atoi(
		strings.TrimPrefix(m.ProxmoxMachine.Spec.ProviderID, ProviderIDPrefix),
	)
	if err != nil {
		m.Logger.Error(err, "Failed parsing provider ID")
		return ctrl.Result{}, err
	}
	vm, err := loadVm(m.ProxmoxClient, vmid)
	if err != nil {
		return ctrl.Result{}, err
	}
	if vm == nil {
		// Couldnt find the VM, I suppose we assume its deleted
		controllerutil.RemoveFinalizer(m.ProxmoxMachine, machineFinalizer)
		err = m.Update(context.TODO(), m.ProxmoxMachine)
		return ctrl.Result{}, err
	}

	// TODO: We also need to cleanup the cloudinit volume. For some reason
	// deleting a VM sometimes doesnt remove the cloudinit volume

	// First, we stop the VM
	if vm.Status == "running" {
		m.Recorder.Event(m.ProxmoxMachine, v1.EventTypeNormal, "", "Stopping VM")
		task, err := vm.Stop()
		if err != nil {
			return ctrl.Result{}, err
		}
		if err = task.Wait(time.Second*5, VmStopTimeout); err != nil {
			return ctrl.Result{}, err
		}

	}

	disks := vm.VirtualMachineConfig.MergeIDEs()
	for disk, mount := range disks {
		if strings.Contains(mount, "cloudinit") {
			m.Recorder.Event(m.ProxmoxMachine, v1.EventTypeNormal, "", "Unlinking Disk "+disk)
			m.Logger.Info("Unlinking disk", "Disk", disk)

			task, err := vm.Config(proxmox.VirtualMachineOption{
				Name:  disk,
				Value: "none,media=cdrom",
			})
			if err != nil {
				m.Recorder.Event(m.ProxmoxMachine, v1.EventTypeWarning, err.Error(), "VM Configuration failure")
				m.Logger.Error(err, "Failed to reconfigure VM")
				return ctrl.Result{Requeue: false}, err
			}

			if err = task.Wait(time.Second*5, VmConfigurationTimeout); err != nil {
				m.Recorder.Event(m.ProxmoxMachine, v1.EventTypeWarning, err.Error(), "VM Configuration timeout")
				m.Logger.Error(err, "Timed out waiting for VM to finish configuring")
				return ctrl.Result{}, err
			}
		}
	}

	m.Recorder.Event(m.ProxmoxMachine, v1.EventTypeNormal, "", "Deleting VM")
	task, err := vm.Delete()
	if err != nil {
		return ctrl.Result{}, err
	}
	if err = task.Wait(time.Second*5, VmDeleteTimeout); err != nil {
		return ctrl.Result{}, err
	}

	controllerutil.RemoveFinalizer(m.ProxmoxMachine, machineFinalizer)
	err = m.Update(context.TODO(), m.ProxmoxMachine)
	return ctrl.Result{}, err
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
		{Name: "agent", Value: 1},
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

// https://kubernetes.io/docs/reference/config-api/kubelet-config.v1beta1/
// providerID: proxmoxMachine.Spec.ProviderID
func generateSnippets(
	secret *v1.Secret,
	version string,
	storagePath string,
	proxmoxMachine *infrastructurev1alpha1.ProxmoxMachine,
) error {
	bootstrapSecret := &BootstrapSecret{}
	err := yaml.Unmarshal(secret.Data["value"], bootstrapSecret)
	if err != nil {
		return err
	}

	var packageInstall bytes.Buffer
	hostname := proxmoxMachine.Namespace + "-" + proxmoxMachine.Name
	err = install.PackageManagerInstallScript.Execute(&packageInstall, install.PackageManagerInstallArgs{
		Hostname:          hostname,
		KubernetesVersion: strings.Trim(version, "v"),
	})
	bootstrapSecret.Write_Files = append(
		bootstrapSecret.Write_Files,
		BootstrapSecretFile{
			Path:        "/init.sh",
			Permissions: "0755",
			Owner:       "root:root",
			Content:     base64.StdEncoding.EncodeToString(packageInstall.Bytes()),
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

	// We now have 2 snippets, let's write them to the storage
	networkFileName := fmt.Sprintf("%s-%s-network.yaml", proxmoxMachine.Namespace, proxmoxMachine.Name)
	userFileName := fmt.Sprintf("%s-%s-user.yaml", proxmoxMachine.Namespace, proxmoxMachine.Name)
	err = writeFile(
		storagePath,
		networkFileName,
		[]byte(fmt.Sprintf(`
#cloud-config
%s
`, proxmoxMachine.Spec.NetworkUserData)),
	)
	if err != nil {
		return err
	}

	err = writeFile(
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

func writeFile(storagePath string, filePath string, contents []byte) error {
	fullPath := filepath.Join(storagePath, filePath)
	err := os.WriteFile(fullPath, contents, 0644)
	return err
}

// ensurePool lists the current pools for the cluster, and if a matching
// pool is found, returns a handle to it. If its not found, we create a
// new resource pool, returning that to the caller
func (m *Machine) ensurePool(poolId string) (*proxmox.Pool, error) {
	pool, err := m.ProxmoxClient.Pool(poolId)
	if err != nil {
		err = m.ProxmoxClient.NewPool(poolId, m.Cluster.Name)
		if err != nil {
			return nil, err
		}

		return m.ProxmoxClient.Pool(poolId)
	}
	return pool, nil
}
