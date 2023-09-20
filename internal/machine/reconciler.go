package machine

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	infrastructurev1alpha1 "github.com/launchboxio/cluster-api-provider-proxmox/api/v1alpha1"
	"github.com/launchboxio/cluster-api-provider-proxmox/internal/install"
	"github.com/luthermonson/go-proxmox"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"gopkg.in/yaml.v2"
	goyaml "gopkg.in/yaml.v3"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/strings/slices"
	"sigs.k8s.io/cluster-api/api/v1beta1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
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
		// Try getting status again to prevent duplication of machines
		if err := m.Get(ctx, req.NamespacedName, m.ProxmoxMachine); err != nil {
			m.Logger.Error(err, "Failed to get ProxmoxMachine")
			return ctrl.Result{}, err
		}
		// TODO: We need to monitor the task for failure.
		// TODO: Ocassionally, we are getting "the object has been modified", causing
		// extra VM instances to be created
		vmid, task, err := template.Clone(&proxmox.VirtualMachineCloneOptions{
			Name: fmt.Sprintf("%s-%s", m.ProxmoxMachine.Namespace, m.ProxmoxMachine.Name),
		})
		m.Logger.Info("Creating VM")
		if err != nil {
			m.Logger.Error(err, "Failed creating VM")
			return ctrl.Result{}, err
		}

		m.ProxmoxMachine.Spec.ProviderID = fmt.Sprintf("%s%d", ProviderIDPrefix, vmid)
		err = m.Update(ctx, m.ProxmoxMachine)
		if err != nil {
			m.Logger.Error(err, "Failed updating ProxmoxMachine status")
			return ctrl.Result{}, err
		}

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
	for _, tag := range m.ProxmoxMachine.Spec.Tags {
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

		credentialsSecret := &v1.Secret{}
		if err := m.Get(ctx, types.NamespacedName{
			Namespace: m.ProxmoxMachine.Namespace,
			Name:      m.ProxmoxCluster.Spec.Snippets.CredentialsSecretName,
		}, credentialsSecret); err != nil {
			m.Logger.Info("Failed finding storage credentials")
			return ctrl.Result{}, err
		}

		err = generateSnippets(
			bootstrapSecret,
			*m.Machine.Spec.Version,
			credentialsSecret,
			"/mnt/default/snippets/snippets/",
			m.ProxmoxMachine,
			m.Machine,
		)
		if err != nil {
			m.Logger.Info("Failed to generate snippet for machine")
			return ctrl.Result{}, err
		}

		task, err := vm.Config(vmInitializationOptions(m.ProxmoxCluster, m.ProxmoxMachine)...)
		if err != nil {
			m.Logger.Error(err, "Failed to reconfigure VM")
			return ctrl.Result{}, err
		}

		if err = task.Wait(time.Second*5, VmConfigurationTimeout); err != nil {
			m.Logger.Error(err, "Timed out waiting for VM to finish configuring")
			return ctrl.Result{}, err
		}

		node, err := m.selectNode(nil)
		if err != nil {
			m.Logger.Error(err, "Failed selecting node for VM")
			return ctrl.Result{}, err
		}

		if vm.Node != node.Node {
			m.Logger.Info(fmt.Sprintf("Moving VM to node %s", node.Node))
			task, err := vm.Migrate(node.Node, "")
			if err != nil {
				return ctrl.Result{}, err
			}

			if err = task.Wait(time.Second*5, VmMigrationTimeout); err != nil {
				m.Logger.Error(err, "Timed out waiting for VM to migrate")
				return ctrl.Result{}, err
			}

			// Reload the VM to get the appropriate node
			vm, err = loadVm(m.ProxmoxClient, vmid)
			if err != nil {
				return ctrl.Result{}, err
			}
		}

		conditions.MarkFalse(
			m.ProxmoxMachine, VirtualMachineInitializing, "Completed",
			v1beta1.ConditionSeverityNone, "VM initialization completed")

		err = m.Status().Update(ctx, m.ProxmoxMachine)
		if err != nil {
			m.Logger.Error(err, "Failed updating ProxmoxMachine status")
			return ctrl.Result{}, err
		}

		task, err = vm.Start()
		if err != nil {
			m.Logger.Error(err, "Failed starting VM")
			return ctrl.Result{}, err
		}

		if err = task.Wait(time.Second*5, VmStartTimeout); err != nil {
			m.Logger.Error(err, "Timed out waiting for VM to start")
			return ctrl.Result{}, err
		}

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
			m.Logger.Info("Unlinking disk", "Disk", disk)
			task, err := vm.UnlinkDisk(disk, true)
			// Unexpectedly, this does throw an error. However, it does
			// allow the volume to be cleaned up when the instance
			// is terminated. For now, that's acceptable enough
			if err != nil {
				m.Logger.Error(err, "Failed unlinking disk")
				return ctrl.Result{}, err
			}
			if err = task.Wait(time.Second*5, VmStopTimeout); err != nil {
				m.Logger.Error(err, "Timeout exceeded waiting for disk removal")
				return ctrl.Result{}, err
			}
		}
	}

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

func generateSnippets(
	secret *v1.Secret,
	version string,
	credentialsSecret *v1.Secret,
	storagePath string,
	proxmoxMachine *infrastructurev1alpha1.ProxmoxMachine,
	machine *clusterv1.Machine,
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
	var registerScript bytes.Buffer
	err = install.RegisterNodeScript.Execute(&registerScript, install.RegisterNodeScriptArgs{
		NodeName:   hostname,
		Machine:    machine.Name,
		ProviderId: proxmoxMachine.Spec.ProviderID,
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
		BootstrapSecretFile{
			Path:        "/register.sh",
			Permissions: "0755",
			Owner:       "root:root",
			Content:     base64.StdEncoding.EncodeToString(registerScript.Bytes()),
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
		credentialsSecret,
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

// ensurePool lists the current pools for the cluster, and if a matching
// pool is found, returns a handle to it. If its not found, we create a
// new resource pool, returning that to the caller
func (m *Machine) ensurePool(poolId string) (*proxmox.Pool, error) {
	pool, err := m.ProxmoxClient.Pool(poolId)
	if err != nil {
		fmt.Println(err)
		err = m.ProxmoxClient.NewPool(poolId, m.Cluster.Name)
		if err != nil {
			return nil, err
		}

		return m.ProxmoxClient.Pool(poolId)
	}
	return pool, nil
}
