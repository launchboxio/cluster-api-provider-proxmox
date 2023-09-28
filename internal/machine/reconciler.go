package machine

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	infrastructurev1alpha1 "github.com/launchboxio/cluster-api-provider-proxmox/api/v1alpha1"
	"github.com/launchboxio/cluster-api-provider-proxmox/internal/install"
	"github.com/launchboxio/cluster-api-provider-proxmox/internal/scope"
	"github.com/launchboxio/cluster-api-provider-proxmox/internal/storage"
	"github.com/luthermonson/go-proxmox"
	"gopkg.in/yaml.v2"
	goyaml "gopkg.in/yaml.v3"
	v1 "k8s.io/api/core/v1"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/strings/slices"
	"sigs.k8s.io/cluster-api/util/conditions"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"strconv"
	"strings"
	"time"
)

const (
	machineFinalizer           = "infrastructure.cluster.x-k8s.io/finalizer"
	VirtualMachineInitializing = "VirtualMachineInitializing"

	VmConfigurationTimeout = time.Minute * 10
	VmMigrationTimeout     = time.Minute * 10
	VmCloneTimeout         = time.Minute * 5
	VmStartTimeout         = time.Minute * 10
	VmStopTimeout          = time.Minute * 10
	VmDeleteTimeout        = time.Minute * 10
)

// TODO: In general, the operator should not be waiting for tasks
// to be completed. Instead, maybe we store the current
// tasks somewhere, utilize Conditions for the various
// actions, and simply check those tasks in the reconcile
// loop. task.Wait() is adding an unecessary amount of wait
// time between actions
// TODO: Remove a requirement for Cluster setup. Folks should be able
// to run ClusterAPI on a single proxmox instance
func (m *Machine) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	if m.MachineScope.InfraMachine.GetDeletionTimestamp() != nil {
		if controllerutil.ContainsFinalizer(m.MachineScope.InfraMachine, machineFinalizer) {
			return m.reconcileDelete(ctx, req)
		}
		return ctrl.Result{}, nil
	}
	return m.reconcileCreate(ctx, req)
}

func (m *Machine) reconcileCreate(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	m.Logger.Info("reconcileCreate", "Name", req.Name, "Namespace", req.Namespace)
	// Set the targetNode on the spec if it doesnt exist
	if m.MachineScope.InfraMachine.Spec.TargetNode == "" {
		m.Logger.Info("Selecting node for machine")
		node, err := m.selectNode(nil)
		if err != nil {
			m.Logger.Error(err, "Failed selecting node for VM")
			return ctrl.Result{}, err
		}
		m.MachineScope.InfraMachine.Spec.TargetNode = node.Node
		err = m.Update(ctx, m.MachineScope.InfraMachine)
		return ctrl.Result{Requeue: true}, err
	}

	// Anytime we have a current task in progress, we just want to poll
	// until completion, persist the data related to it, and then
	// simply requeue the reconciliation
	currentTask := m.MachineScope.InfraMachine.GetActiveTask()
	if currentTask != nil {
		m.Logger.Info("Polling task", "task", currentTask.Action)
		return m.pollTask(currentTask)
	}

	// We don't have a Proxmox VM yet, lets go ahead and create it
	if m.MachineScope.InfraMachine.Spec.ProviderID == "" {
		m.Logger.Info("No provider ID, creating new VM")
		return m.createVm(ctx, req)
	}

	m.Logger.Info("Loading existing VM")
	vmid, err := m.MachineScope.InfraMachine.GetProxmoxId()
	if err != nil {
		m.Logger.Error(err, "Failed parsing provider ID")
		return ctrl.Result{}, err
	}

	// Always ensure we add the finalizer
	if !controllerutil.ContainsFinalizer(m.MachineScope.InfraMachine, machineFinalizer) {
		m.Logger.Info("Attaching finalizer")
		controllerutil.AddFinalizer(m.MachineScope.InfraMachine, machineFinalizer)
		err = m.Update(ctx, m.MachineScope.InfraMachine)
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
			"Resource had VMID of %d, but it didnt exist in Proxmox",
			m.MachineScope.InfraMachine.Status.Vmid,
		))
		return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
		// We have stored a proxmox VMID in the status of the CRD, yet the
		// machine doesnt exist in proxmox. For now, let's just purge the
		// status, ignore what exists in Proxmox, and create a fresh machine
		// We don't want to do this. Any network connectivity issue with
		// proxmox results in all of the machines being orphaned
		//m.MachineScope.InfraMachine.Status.Vmid = 0
		//m.MachineScope.InfraMachine.Status.Conditions = nil
		//err = r.Status().Update(ctx, m.MachineScope.InfraMachine)
		//return ctrl.Result{}, err
	}

	// Attach tags
	tags := append(m.MachineScope.InfraMachine.Spec.Tags, m.ClusterScope.InfraCluster.Spec.Tags...)
	for _, tag := range tags {
		if !vm.HasTag(tag) {
			m.Logger.Info("Adding tag to VM instance")
			_, err := vm.AddTag(tag)
			if err != nil {
				m.Logger.Error(err, "Failed adding tag to VM")
			}
		}
	}

	// If the user has provided a resource pool, attach the
	// created VM to the specified pool
	if m.ClusterScope.InfraCluster.Spec.Pool != "" {
		pool, err := m.ensurePool(m.ClusterScope.InfraCluster.Spec.Pool)
		if err != nil {
			m.Recorder.Event(m.MachineScope.InfraMachine, v1.EventTypeWarning, err.Error(), "Failed getting Resource Pool: %v")
			m.Logger.Error(err, "Failed to ensure VM pool")
			return ctrl.Result{}, err
		}

		poolMembers := []string{}
		for _, member := range pool.Members {
			poolMembers = append(poolMembers, strconv.FormatUint(member.VMID, 10))
		}

		memberId := strconv.FormatUint(uint64(vm.VMID), 10)
		if !slices.Contains(poolMembers, memberId) {
			m.Logger.Info("Adding VM to resource pool")
			poolMembers = append(poolMembers, memberId)
			err = pool.Update(&proxmox.PoolUpdateOption{
				VirtualMachines: memberId,
			})
			if err != nil {
				return ctrl.Result{}, err
			}
			m.Recorder.Event(m.MachineScope.InfraMachine, "Normal", "", "Added to resource pool "+pool.PoolID)
		}
	}

	// If VM is still showing as initializing, we want to perform further configuration
	// - Configure network, disks, etc
	// - Setup cicustom
	// - Migrate to a target node
	// - Start the Virtual Machine
	// TODO: When VM reaches "starting" state, we should simply store the task,
	// and exit out. Currently, the operator waits for machine start to complete,
	// before going on to the next step
	if conditions.IsTrue(m.MachineScope.InfraMachine, VirtualMachineInitializing) {
		m.Logger.Info("Initializing VM")
		return m.initialize(ctx, req, vm)
	}

	if vm.Status != proxmox.StatusVirtualMachineRunning {
		m.Logger.Info("VM not running, attempting to start now")
		m.Recorder.Event(m.MachineScope.InfraMachine, "Normal", "", "Starting machine")
		task, err := vm.Start()
		if err != nil {
			m.Logger.Error(err, "Failed starting VM")
			return ctrl.Result{}, err
		}
		m.MachineScope.InfraMachine.SetTask(infrastructurev1alpha1.StartVm, task)
		m.MachineScope.InfraMachine.SetActiveTask(task.ID)
		err = m.Status().Update(ctx, m.MachineScope.InfraMachine)
		return ctrl.Result{Requeue: true}, err
	}

	// Ensure our machine is marked as Ready
	if !m.MachineScope.InfraMachine.Status.Ready {
		m.Logger.Info("Updating InfraMachine.Status.Ready")
		_ = m.Get(ctx, req.NamespacedName, m.MachineScope.InfraMachine)
		m.MachineScope.InfraMachine.Status.Ready = true
		if err = m.Status().Update(ctx, m.MachineScope.InfraMachine); err != nil {
			return ctrl.Result{}, err
		}

		conditions.MarkTrue(m.MachineScope.InfraMachine, "Ready")
		err = m.Status().Update(ctx, m.MachineScope.InfraMachine)
		return ctrl.Result{Requeue: true}, nil
	}

	// Lastly, wait for our node to get the registered providerID. Once it does,
	// we find the node and annotate it with the machine ID
	if m.MachineScope.Machine.Status.NodeRef == nil {
		m.Logger.Info("Checking for nodeRef to attach")
		// Attempt to attach node until successful
		// We probably shouldnt endlessly retry this,
		// TODO: Find a better method to notice when the node is ready
		_, err := m.attachNode(ctx)
		if err != nil {
			return ctrl.Result{RequeueAfter: time.Second * 15}, nil
		}
		return ctrl.Result{Requeue: true}, nil
	}

	m.MachineScope.InfraMachine.Status.State = vm.Status
	ifaces, err := vm.AgentGetNetworkIFaces()
	if err == nil {
		m.MachineScope.InfraMachine.SetIpAddresses(ifaces)
	}

	err = m.Status().Update(ctx, m.MachineScope.InfraMachine)
	return ctrl.Result{RequeueAfter: time.Minute * 1}, err
}

// pollTask takes any task we've set on the machine, and
// polls it for completion, and updating any required status fields
func (m *Machine) pollTask(currentTask *infrastructurev1alpha1.ProxmoxMachineTaskStatus) (ctrl.Result, error) {
	task := proxmox.NewTask(currentTask.TaskId, m.ProxmoxClient)
	err := task.Ping()
	if err != nil {
		return ctrl.Result{}, err
	}

	// Task isn't completed, requeue in 5 seconds to check again
	if !task.IsCompleted {
		return ctrl.Result{RequeueAfter: 5}, nil
	}

	// Always propagate the current state of the task
	m.MachineScope.InfraMachine.SetTask(currentTask.Action, task)
	m.MachineScope.InfraMachine.ClearActiveTask()
	// Task is completed, so we always remove the currently active task
	if !task.IsFailed {
		// Get logs, and add to the event
		taskOutput, _ := task.Log(0, 1000)
		var rawTaskOutput []string
		for _, log := range taskOutput {
			rawTaskOutput = append(rawTaskOutput, log)
		}
		m.Logger.Info("Task failed", "Action", currentTask.Action)
		m.Recorder.Event(m.MachineScope.InfraMachine, v1.EventTypeWarning, strings.Join(rawTaskOutput, "\n"), "Create VM task failed")
	} else {
		// Do something with success
		m.Logger.Info("Task completed", "Action", currentTask.Action)
	}
	err = m.Status().Update(context.TODO(), m.MachineScope.InfraMachine)
	return ctrl.Result{RequeueAfter: time.Second * 5}, err
}

// createVm will create the new Proxmox QEMU VM, and update various status / spec fields
// with the respective VMID on creation. The clone operation must block, as multiple clones
// cannot execute in parallel
func (m *Machine) createVm(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	m.Logger.Info("Creating Virtual Machine")
	node, err := m.ProxmoxClient.Node(m.MachineScope.InfraMachine.Spec.TargetNode)
	if err != nil {
		m.Logger.Error(err, "Failed getting node for VM")
		return ctrl.Result{}, err
	}

	cluster, err := m.ProxmoxClient.Cluster()
	if err != nil {
		m.Logger.Error(err, "Failed getting Proxmox cluster client")
		return ctrl.Result{}, err
	}

	vmid, err := cluster.NextID()
	if err != nil {
		m.Logger.Error(err, "Failed getting next VMID")
		return ctrl.Result{}, err
	}

	task, err := node.NewVirtualMachine(vmid, vmInitializationOptions(
		m.ClusterScope.InfraCluster,
		m.MachineScope.InfraMachine,
	)...)

	if err != nil {
		m.Logger.Error(err, "Failed creating VM")
		m.Recorder.Event(m.MachineScope.InfraMachine, v1.EventTypeWarning, err.Error(), "Failed creating VM")
		return ctrl.Result{}, err
	}

	if task.IsFailed {
		// Get logs, and add to the event
		taskOutput, _ := task.Log(0, 1000)
		var rawTaskOutput []string
		for _, log := range taskOutput {
			rawTaskOutput = append(rawTaskOutput, log)
		}
		output := strings.Join(rawTaskOutput, "\n")
		m.Logger.Info("Task failed", "Action", infrastructurev1alpha1.CreateVm)
		m.Recorder.Event(m.MachineScope.InfraMachine, v1.EventTypeWarning, output, "Create VM task failed")
		return ctrl.Result{}, errors.New(output)
	}

	// Let's just get one more time, to be sure
	_ = m.Get(context.TODO(), req.NamespacedName, m.MachineScope.InfraMachine)
	m.MachineScope.InfraMachine.Spec.ProviderID = fmt.Sprintf("%s%d", infrastructurev1alpha1.ProviderIDPrefix, vmid)
	m.Recorder.Event(m.MachineScope.InfraMachine, v1.EventTypeNormal, "", "Created VM with Provider ID "+m.MachineScope.InfraMachine.GetProviderId())
	m.MachineScope.InfraMachine.Status.Vmid = vmid

	conditions.MarkTrue(m.MachineScope.InfraMachine, VirtualMachineInitializing)
	m.Logger.Info("Updating status / conditions")
	if err = m.Status().Update(ctx, m.MachineScope.InfraMachine); err != nil {
		m.Logger.Info("Failed updating status / conditions")
		return ctrl.Result{}, err
	}
	m.Logger.Info("Status / conditions updated")

	// Get the resource again, and ensure we can update the spec
	if err := m.Get(ctx, req.NamespacedName, m.MachineScope.InfraMachine); err != nil {
		return ctrl.Result{}, err
	}
	m.Logger.Info("Updating spec")
	if err = m.Update(ctx, m.MachineScope.InfraMachine); err != nil {
		m.Logger.Info("Spec failed to update")
		return ctrl.Result{}, err
	}
	m.Logger.Info("Spec updated")
	return ctrl.Result{Requeue: true}, nil
}

// initialize generates snippets, and configures the machine to match the Machine spec
func (m *Machine) initialize(ctx context.Context, req ctrl.Request, vm *proxmox.VirtualMachine) (ctrl.Result, error) {
	if m.MachineScope.Machine.Spec.Bootstrap.DataSecretName == nil {
		m.Recorder.Event(m.MachineScope.InfraMachine, "Normal", "Bootstrap secret not created", "Waiting to start VM")
		m.Logger.Info("No bootstrap secret found...")
		return ctrl.Result{RequeueAfter: time.Second * 10}, nil
	}
	bootstrapSecret := &v1.Secret{}
	if err := m.Get(ctx, types.NamespacedName{
		Namespace: m.MachineScope.InfraMachine.Namespace,
		Name:      *m.MachineScope.Machine.Spec.Bootstrap.DataSecretName,
	}, bootstrapSecret); err != nil {
		m.Logger.Info("Failed finding bootstrap secret")
		return ctrl.Result{}, err
	}

	store := storage.NewLocal(m.ProxmoxClient, m.MachineScope.InfraMachine.Spec.TargetNode, fmt.Sprintf("/snippets/%s", m.ClusterScope.InfraCluster.Name))

	err := generateSnippets(
		store,
		bootstrapSecret,
		*m.MachineScope.Machine.Spec.Version,
		m.MachineScope,
	)
	if err != nil {
		m.Logger.Info("Failed to generate snippet for machine")
		return ctrl.Result{}, err
	}

	task, err := vm.Config(vmInitializationOptions(m.ClusterScope.InfraCluster, m.MachineScope.InfraMachine)...)
	if err != nil {
		m.Recorder.Event(m.MachineScope.InfraMachine, v1.EventTypeWarning, err.Error(), "VM Configuration failure")
		m.Logger.Error(err, "Failed to reconfigure VM")
		return ctrl.Result{Requeue: false}, err
	}

	m.MachineScope.InfraMachine.SetTask(infrastructurev1alpha1.ConfigureVm, task)
	m.MachineScope.InfraMachine.SetActiveTask(task.ID)
	err = m.Status().Update(ctx, m.MachineScope.InfraMachine)
	return ctrl.Result{RequeueAfter: time.Second * 5}, err

}

func getVmOptions(m *Machine) []proxmox.VirtualMachineOption {
	return []proxmox.VirtualMachineOption{}
}

func (m *Machine) getRESTClient(clusterName string) (*kubernetes.Clientset, error) {
	secret := &v1.Secret{}
	err := m.Get(context.TODO(), types.NamespacedName{
		Namespace: m.ClusterScope.Cluster.Namespace,
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
	client, err := m.getRESTClient(m.ClusterScope.Cluster.Name)
	if err != nil {
		return ctrl.Result{}, err
	}

	nodeName := fmt.Sprintf("%s-%s", m.MachineScope.InfraMachine.Namespace, m.MachineScope.InfraMachine.Name)
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

	if node.Spec.ProviderID != m.MachineScope.InfraMachine.Spec.ProviderID {
		patches = append(patches, Patch{
			Op:    "add",
			Path:  "/spec/providerID",
			Value: m.MachineScope.InfraMachine.Spec.ProviderID,
		})
	}

	if _, ok := node.ObjectMeta.Annotations["cluster.x-k8s.io"]; !ok {
		patches = append(patches, Patch{
			Op:    "add",
			Path:  "/metadata/annotations/cluster.x-k8s.io",
			Value: m.MachineScope.Machine.Name,
		})
	}

	if len(patches) > 0 {
		payloadBytes, _ := json.Marshal(patches)

		_, err = client.
			CoreV1().
			Nodes().
			Patch(ctx, node.Name, types.JSONPatchType, payloadBytes, v12.PatchOptions{})
		if err != nil {
			m.Recorder.Event(m.MachineScope.InfraMachine, v1.EventTypeWarning, err.Error(), "Failed attaching node to machine")
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{Requeue: true}, nil
}

func (m *Machine) reconcileDelete(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// TODO: Perform out of band cleanup
	m.Logger.Info("Cleaning up machine for finalizer")
	vmid, err := m.MachineScope.InfraMachine.GetProxmoxId()
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
		controllerutil.RemoveFinalizer(m.MachineScope.InfraMachine, machineFinalizer)
		err = m.Update(context.TODO(), m.MachineScope.InfraMachine)
		return ctrl.Result{}, err
	}

	// TODO: We also need to cleanup the cloudinit volume. For some reason
	// deleting a VM sometimes doesnt remove the cloudinit volume

	// First, we stop the VM
	if vm.Status == "running" {
		m.Recorder.Event(m.MachineScope.InfraMachine, v1.EventTypeNormal, "", "Stopping VM")
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
			m.Recorder.Event(m.MachineScope.InfraMachine, v1.EventTypeNormal, "", "Unlinking Disk "+disk)
			m.Logger.Info("Unlinking disk", "Disk", disk)

			task, err := vm.UnlinkDisk(disk, true)
			if err != nil {
				m.Recorder.Event(m.MachineScope.InfraMachine, v1.EventTypeWarning, err.Error(), "Failed unlinking disk")
				m.Logger.Error(err, "Failed unlinking disk")
				return ctrl.Result{Requeue: false}, err
			}

			if task != nil {
				if err = task.Wait(time.Second*5, VmConfigurationTimeout); err != nil {
					m.Recorder.Event(m.MachineScope.InfraMachine, v1.EventTypeWarning, err.Error(), "Timeout unlinking disk")
					m.Logger.Error(err, "Timed out waiting for disk to unlink")
					return ctrl.Result{}, err
				}
			}
		}
	}

	m.Recorder.Event(m.MachineScope.InfraMachine, v1.EventTypeNormal, "", "Deleting VM")
	task, err := vm.Delete()
	if err != nil {
		return ctrl.Result{}, err
	}
	if err = task.Wait(time.Second*5, VmDeleteTimeout); err != nil {
		return ctrl.Result{}, err
	}

	controllerutil.RemoveFinalizer(m.MachineScope.InfraMachine, machineFinalizer)
	err = m.Update(context.TODO(), m.MachineScope.InfraMachine)
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

func vmInitializationOptions(cluster *infrastructurev1alpha1.ProxmoxCluster, machine *infrastructurev1alpha1.ProxmoxMachine) []proxmox.VirtualMachineOption {
	storageBase := fmt.Sprintf("%s:snippets/", cluster.Name)
	cicustom := []string{
		fmt.Sprintf(
			"user=%s%s-%s-user.yaml",
			storageBase,
			machine.Namespace, machine.Name,
		),
	}

	if machine.Spec.NetworkUserData != "" {
		cicustom = append(cicustom, fmt.Sprintf(
			"network=%s%s-%s-network.yaml",
			storageBase,
			machine.Namespace, machine.Name,
		))
	}
	options := []proxmox.VirtualMachineOption{
		{Name: "memory", Value: machine.Spec.Resources.Memory},
		{Name: "sockets", Value: machine.Spec.Resources.CpuSockets},
		{Name: "cores", Value: machine.Spec.Resources.CpuCores},
		{Name: "cicustom", Value: strings.Join(cicustom, ",")},
		{Name: "agent", Value: 1},
		{Name: "name", Value: machine.Name},
		{Name: "pool", Value: cluster.Spec.Pool},
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
	store storage.Storage,
	secret *v1.Secret,
	version string,
	machineScope *scope.MachineScope,
) error {
	bootstrapSecret := &BootstrapSecret{}
	err := yaml.Unmarshal(secret.Data["value"], bootstrapSecret)
	if err != nil {
		return err
	}

	var packageInstall bytes.Buffer
	hostname := machineScope.InfraMachine.Namespace + "-" + machineScope.InfraMachine.Name
	err = install.PackageManagerInstallScript.Execute(&packageInstall, install.PackageManagerInstallArgs{
		Hostname:           hostname,
		KubernetesVersion:  strings.Trim(version, "v"),
		AdditionalUserData: machineScope.InfraMachine.Spec.UserData,
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

	if len(machineScope.InfraMachine.Spec.SshKeys) > 0 {
		bootstrapSecret.Users = []BootstrapUser{{
			Name:                "ubuntu",
			Lock_Passwd:         false,
			Groups:              []string{"sudo"},
			Sudo:                "ALL=(ALL) NOPASSWD:ALL",
			Ssh_Authorized_Keys: machineScope.InfraMachine.Spec.SshKeys,
		}}
	}

	userContents, err := goyaml.Marshal(bootstrapSecret)
	if err != nil {
		return err
	}

	if machineScope.InfraMachine.Spec.NetworkUserData != "" {
		networkFileName := fmt.Sprintf("%s-%s-network.yaml", machineScope.InfraMachine.Namespace, machineScope.InfraMachine.Name)
		err = store.WriteFile(networkFileName, []byte(fmt.Sprintf(`
#cloud-config
%s
`, machineScope.InfraMachine.Spec.NetworkUserData)))
		if err != nil {
			return err
		}
	}

	userFileName := fmt.Sprintf("%s-%s-user.yaml", machineScope.InfraMachine.Namespace, machineScope.InfraMachine.Name)
	err = store.WriteFile(userFileName, []byte(fmt.Sprintf(`
#cloud-config
%s
`, userContents)),
	)
	if err != nil {
		return err
	}

	return nil
}

// ensurePool lists the current pools for the cluster, and if a matching
// pool is found, returns a handle to it. If its not found, we create a
// new resource pool, returning that to the caller
func (m *Machine) ensurePool(poolId string) (*proxmox.Pool, error) {
	pool, err := m.ProxmoxClient.Pool(poolId)
	if err != nil {
		err = m.ProxmoxClient.NewPool(poolId, m.ClusterScope.Cluster.Name)
		if err != nil {
			return nil, err
		}

		return m.ProxmoxClient.Pool(poolId)
	}
	return pool, nil
}
