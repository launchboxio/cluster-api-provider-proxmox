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
	"context"
	"crypto/tls"
	"fmt"
	"github.com/luthermonson/go-proxmox"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"strings"
	"time"

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
	VirtualMachine = "VirtualMachine"
)

//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=proxmoxmachines,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=proxmoxmachines/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=proxmoxmachines/finalizers,verbs=update

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

	machine := &infrastructurev1alpha1.ProxmoxMachine{}
	if err := r.Get(ctx, req.NamespacedName, machine); err != nil {
		if errors.IsNotFound(err) {
			contextLogger.Info("ProxmoxMachine resource not found, must be deleted")
			return ctrl.Result{}, nil
		}
		contextLogger.Error(err, "Failed to get ProxmoxMachine")
		return ctrl.Result{}, err
	}

	if machine.GetDeletionTimestamp() != nil {
		if controllerutil.ContainsFinalizer(machine, machineFinalizer) {
			// TODO: Perform out of band cleanup
		}
	}

	node, err := proxmoxClient.Node("artemis")
	if err != nil {
		contextLogger.Error(err, "Failed getting base node")
		return ctrl.Result{}, err
	}

	template, err := node.VirtualMachine(9001)
	if err != nil {
		contextLogger.Error(err, "Failed getting base template")
		return ctrl.Result{}, err
	}

	// Clone the machine, set the VMID in the status, and then
	// simply return. We want to keep creation / and CRD updates
	// as idempotent as possible
	if machine.Status.Vmid == 0 {
		vmid, task, err := template.Clone(&proxmox.VirtualMachineCloneOptions{
			Name: fmt.Sprintf("%s-%s", machine.Namespace, machine.Name),
		})
		contextLogger.Info("Creating VM")
		if err != nil {
			contextLogger.Error(err, "Failed creating VM")
			return ctrl.Result{}, err
		}

		contextLogger.Info("Attaching finalizer")
		if !controllerutil.ContainsFinalizer(machine, machineFinalizer) {
			controllerutil.AddFinalizer(machine, machineFinalizer)
		}

		contextLogger.Info("Setting initializing status")
		meta.SetStatusCondition(&machine.Status.Conditions, metav1.Condition{
			Type:    VirtualMachine,
			Status:  metav1.ConditionFalse,
			Reason:  "Initializing",
			Message: "VM Created, Initializing for first launch",
		})

		contextLogger.Info("Setting VMID on CRD")
		machine.Status.Vmid = vmid
		err = r.Status().Update(ctx, machine)
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

	// We should always have a new VM here. We query it to make sure
	vm, err := node.VirtualMachine(machine.Status.Vmid)
	if err != nil {
		contextLogger.Error(err, "Failed getting VM status")
		return ctrl.Result{}, err
	}

	// If VM is still showing as initializing, we want to perform further configuration
	// - Configure network, disks, etc
	// - Migrate to a target node
	// - Start the Virtual Machine
	if meta.IsStatusConditionFalse(machine.Status.Conditions, VirtualMachine) {
		task, err := vm.Config(vmInitializationOptions(machine)...)
		if err != nil {
			contextLogger.Error(err, "Failed to reconfigure VM")
			return ctrl.Result{}, err
		}

		err = task.Wait(
			time.Second*5,
			time.Minute*10,
		)
		if err != nil {
			contextLogger.Error(err, "Timed out waiting for VM to finish configuring")
			return ctrl.Result{}, err
		}

		meta.SetStatusCondition(&machine.Status.Conditions, metav1.Condition{
			Type:    VirtualMachine,
			Status:  metav1.ConditionTrue,
			Reason:  "Initializing",
			Message: "VM initialization completed",
		})
		err = r.Status().Update(ctx, machine)
		if err != nil {
			contextLogger.Error(err, "Failed updating ProxmoxMachine status")
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	// Lastly, check runtime settings. Things like CPU, memory, etc
	// can be changed on an instance that has been initialized already
	options := pendingChanges(machine, vm)
	if len(options) > 0 {
		contextLogger.Info("VM out of sync, updating configuration")
		task, err := vm.Config(options...)
		if err != nil {
			contextLogger.Error(err, "Failed to reconfigure VM")
			return ctrl.Result{}, err
		}

		err = task.Wait(
			time.Second*5,
			time.Minute*10,
		)
		if err != nil {
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

func vmInitializationOptions(machine *infrastructurev1alpha1.ProxmoxMachine) []proxmox.VirtualMachineOption {
	options := []proxmox.VirtualMachineOption{
		{Name: "memory", Value: machine.Spec.Resources.Memory},
		{Name: "sockets", Value: machine.Spec.Resources.CpuSockets},
		{Name: "cores", Value: machine.Spec.Resources.CpuCores},
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

	if int(vm.Mem) != machine.Spec.Resources.Memory {
		opts = append(opts, proxmox.VirtualMachineOption{
			Name:  "memory",
			Value: machine.Spec.Resources.Memory,
		})
	}

	return opts
}
