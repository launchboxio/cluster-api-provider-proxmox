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
	"github.com/launchboxio/cluster-api-provider-proxmox/internal/scope"
	"github.com/luthermonson/go-proxmox"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"time"

	//"k8s.io/apimachinery/pkg/types"
	"net/http"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	//"github.com/Telmate/proxmox-api-go/proxmox"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	infrastructurev1alpha1 "github.com/launchboxio/cluster-api-provider-proxmox/api/v1alpha1"
	machinereconciler "github.com/launchboxio/cluster-api-provider-proxmox/internal/machine"
	//bootstrapv1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
)

// ProxmoxMachineReconciler reconciles a ProxmoxMachine object
type ProxmoxMachineReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=proxmoxmachines,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=proxmoxmachines/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=proxmoxmachines/finalizers,verbs=update
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=*,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;machines,verbs=get;list;watch
//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines;machines/status,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch;create;update;patch
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch

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
		contextLogger.Info("ProxmoxCluster or linked Cluster is marked as paused. Won't reconcile")
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

	credentialsSecret := &v1.Secret{}
	err = r.Get(ctx, types.NamespacedName{
		Namespace: proxmoxCluster.Spec.CredentialsRef.Namespace,
		Name:      proxmoxCluster.Spec.CredentialsRef.Name,
	}, credentialsSecret)
	if err != nil {
		contextLogger.Error(err, "Failed getting Proxmox API Credentials")
		return ctrl.Result{RequeueAfter: time.Minute * 1}, err
	}

	options := []proxmox.Option{
		proxmox.WithAPIToken(string(credentialsSecret.Data["token_id"]), string(credentialsSecret.Data["token_secret"])),
	}
	if proxmoxCluster.Spec.InsecureSkipTlsVerify {
		insecureHTTPClient := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		}
		options = append(options, proxmox.WithHTTPClient(insecureHTTPClient))
	}
	proxmoxClient := proxmox.NewClient(string(credentialsSecret.Data["api_url"]), options...)

	clusterScope := &scope.ClusterScope{
		Cluster:      cluster,
		InfraCluster: proxmoxCluster,
	}

	machineScope := &scope.MachineScope{
		Machine:      machine,
		InfraMachine: proxmoxMachine,
	}

	machineReconciler := &machinereconciler.Machine{
		ProxmoxClient: proxmoxClient,
		ClusterScope:  clusterScope,
		MachineScope:  machineScope,
		Logger:        contextLogger,
		Client:        r.Client,
		Recorder:      r.Recorder,
	}

	return machineReconciler.Reconcile(ctx, req)
}

// SetupWithManager sets up the controller with the Manager.
func (r *ProxmoxMachineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrastructurev1alpha1.ProxmoxMachine{}).
		Complete(r)
}
