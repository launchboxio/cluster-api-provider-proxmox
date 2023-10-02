package machine

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
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
	"sigs.k8s.io/cluster-api/util/conditions"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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
	clusterTemplate, err := getVmTemplate(m.ProxmoxClient, m.MachineScope.InfraMachine.Spec.Template)
	if err != nil || clusterTemplate == nil {
		m.Logger.Error(err, "Failed to get VM template")
		return ctrl.Result{}, err
	}

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
	fmt.Println(currentTask)
	if currentTask != nil {
		m.Logger.Info("Polling task", "task", currentTask.Action)
		return m.pollTask(currentTask)
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

	// We don't have a Proxmox VM yet, lets go ahead and create it
	if m.MachineScope.InfraMachine.Spec.ProviderID == "" {
		m.Logger.Info("No provider ID, creating new VM")
		return m.createVm(ctx, req, template)
	}

	// Always ensure we add the finalizer
	if !controllerutil.ContainsFinalizer(m.MachineScope.InfraMachine, machineFinalizer) {
		m.Logger.Info("Attaching finalizer")
		controllerutil.AddFinalizer(m.MachineScope.InfraMachine, machineFinalizer)
		err := m.Update(ctx, m.MachineScope.InfraMachine)
		return ctrl.Result{}, err
	}

	m.Logger.Info("Loading existing VM")
	vmid, err := m.MachineScope.InfraMachine.GetProxmoxId()
	if err != nil {
		m.Logger.Error(err, "Failed parsing provider ID")
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
	}

	if conditions.IsTrue(m.MachineScope.InfraMachine, VirtualMachineInitializing) {
		task, err := m.configureVm(vm)
		if err != nil {
			m.Logger.Error(err, "Failed parsing provider ID")
			m.Recorder.Event(m.MachineScope.InfraMachine, v1.EventTypeWarning, err.Error(), "Failed configuring VM")
		}
		m.MachineScope.InfraMachine.SetTask(infrastructurev1alpha1.StartVm, task)
		m.MachineScope.InfraMachine.SetActiveTask(task.ID)
		conditions.MarkFalse(m.MachineScope.InfraMachine, VirtualMachineInitializing, "Completed", "Normal", "")
		err = m.Status().Update(ctx, m.MachineScope.InfraMachine)
		return ctrl.Result{Requeue: true}, err
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
		output, err := m.taskOutput(task)
		if err != nil {
			m.Logger.Error(err, "Failed getting task output")
		}
		m.Logger.Error(err, "Action", currentTask.Action)
		m.Recorder.Event(m.MachineScope.InfraMachine, v1.EventTypeWarning, output, fmt.Sprintf("%s failed", currentTask.Action))
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
func (m *Machine) createVm(ctx context.Context, req ctrl.Request, template *proxmox.VirtualMachine) (ctrl.Result, error) {
	m.Logger.Info("Creating Virtual Machine")

	// Verify that our bootstrap Data Secret is created
	if m.MachineScope.Machine.Spec.Bootstrap.DataSecretName == nil {
		m.Recorder.Event(m.MachineScope.InfraMachine, "Normal", "Bootstrap secret not created", "Waiting to start VM")
		m.Logger.Info("No bootstrap secret found...")
		return ctrl.Result{RequeueAfter: time.Second * 10}, nil
	}

	// Get the bootstrap secret
	bootstrapSecret := &v1.Secret{}
	if err := m.Get(ctx, types.NamespacedName{
		Namespace: m.MachineScope.InfraMachine.Namespace,
		Name:      *m.MachineScope.Machine.Spec.Bootstrap.DataSecretName,
	}, bootstrapSecret); err != nil {
		m.Logger.Info("Failed finding bootstrap secret")
		return ctrl.Result{}, err
	}

	m.Logger.Info("Writing userdata to snippet storage")
	store, err := storage.NewLocal(m.ProxmoxClient, m.MachineScope.InfraMachine.Spec.TargetNode, fmt.Sprintf("/snippets/%s", m.ClusterScope.InfraCluster.Name))
	if err != nil {
		return ctrl.Result{}, err
	}

	// TODO: Not sure why, but this causes panic: send on closed channel
	// for now, we'll add a time.sleep and see if that helps
	defer func() {
		time.Sleep(time.Second * 1)
		store.Close()
	}()

	err = generateSnippets(
		store,
		bootstrapSecret,
		*m.MachineScope.Machine.Spec.Version,
		m.MachineScope,
	)
	if err != nil {
		m.Logger.Info("Failed to generate snippets for machine")
		return ctrl.Result{}, err
	}

	m.Logger.Info("Verifying spec.TargetNode")
	node, err := m.ProxmoxClient.Node(m.MachineScope.InfraMachine.Spec.TargetNode)
	if err != nil {
		m.Logger.Error(err, "Failed getting node for VM")
		return ctrl.Result{}, err
	}

	newId, task, err := template.Clone(&proxmox.VirtualMachineCloneOptions{
		Pool:   m.ClusterScope.InfraCluster.Spec.Pool,
		Target: node.Name,
	})

	if err != nil {
		m.Logger.Error(err, "Failed cloning VM")
		m.Recorder.Event(m.MachineScope.InfraMachine, v1.EventTypeWarning, err.Error(), "Failed creating VM")
		return ctrl.Result{}, err
	}

	if err = task.Wait(time.Second*10, time.Minute*1); err != nil {
		if err != nil {
			m.Logger.Error(err, "Failed polling task")
		}
		m.Logger.Error(err, "Failed waiting for task to complete", "Action", infrastructurev1alpha1.CreateVm)
		m.Recorder.Event(m.MachineScope.InfraMachine, v1.EventTypeWarning, "", "Create VM failed")
		return ctrl.Result{}, err
	}

	if task.IsFailed {
		output, err := m.taskOutput(task)
		if err != nil {
			m.Logger.Error(err, "Failed getting task output task")
		}
		m.Logger.Error(err, "Failed creating VM", "Action", infrastructurev1alpha1.CreateVm)
		m.Recorder.Event(m.MachineScope.InfraMachine, v1.EventTypeWarning, output, "Create VM failed")
		return ctrl.Result{}, err
	}

	// Let's just get one more time, to be sure
	_ = m.Get(context.TODO(), req.NamespacedName, m.MachineScope.InfraMachine)
	m.MachineScope.InfraMachine.Spec.ProviderID = fmt.Sprintf("%s%d", infrastructurev1alpha1.ProviderIDPrefix, newId)
	m.MachineScope.InfraMachine.Status.Vmid = newId
	m.Recorder.Event(m.MachineScope.InfraMachine, v1.EventTypeNormal, "", "Created VM with Provider ID "+m.MachineScope.InfraMachine.GetProviderId())
	conditions.MarkTrue(m.MachineScope.InfraMachine, VirtualMachineInitializing)
	m.Logger.Info("Updating resource")
	if err = m.Update(ctx, m.MachineScope.InfraMachine); err != nil {
		m.Logger.Info("Failed updating resource")
		return ctrl.Result{}, err
	}
	m.Logger.Info("Machine resource updated")

	return ctrl.Result{Requeue: true}, nil
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

	tags := append(machine.Spec.Tags, cluster.Spec.Tags...)

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
		//{Name: "pool", Value: cluster.Spec.Pool},
		{Name: "tags", Value: strings.Join(tags, ",")},
		{Name: "ide2", Value: fmt.Sprintf("file=%s:cloudinit,media=cdrom", cluster.Name)},
		{Name: "onboot", Value: 1},
		//{Name: "scsihw", Value: "virtio-scsi-pci"},
		// TODO: This ends up giving us a soft requirement on
		// scsi0 being available. Might want to adjust at some point
		//{Name: "boot", Value: "order=scsi0"},
		//{Name: "serial0", Value: "socket"},
		//{Name: "vga", Value: "serial0"},
	}

	//for idx, disk := range machine.Spec.Disks {
	//	// Use the specified storage, otherwise,
	//	// default to the storage for the cluster
	//	//diskStorage := disk.Storage
	//	//if diskStorage == "" {
	//	//	diskStorage = cluster.Name
	//	//}
	//	//options = append(options, proxmox.VirtualMachineOption{
	//	//	Name:  fmt.Sprintf("scsi%d", idx),
	//	//	Value: fmt.Sprintf("file=%s"),
	//	//})
	//}

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

func (m *Machine) configureVm(vm *proxmox.VirtualMachine) (*proxmox.Task, error) {
	options := vmInitializationOptions(
		m.ClusterScope.InfraCluster,
		m.MachineScope.InfraMachine,
	)
	return vm.Config(options...)
}

func (m *Machine) taskOutput(task *proxmox.Task) (string, error) {
	var rawTaskOutput []string
	taskOutput, err := task.Log(0, 1000)
	if err != nil {
		return "", err
	}
	for _, log := range taskOutput {
		rawTaskOutput = append(rawTaskOutput, log)
	}
	return strings.Join(rawTaskOutput, "\n"), nil
}
