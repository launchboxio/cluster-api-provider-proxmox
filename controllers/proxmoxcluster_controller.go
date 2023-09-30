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
	infrastructurev1alpha1 "github.com/launchboxio/cluster-api-provider-proxmox/api/v1alpha1"
	"github.com/luthermonson/go-proxmox"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"net/http"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"time"
)

// ProxmoxClusterReconciler reconciles a ProxmoxCluster object
type ProxmoxClusterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

const (
	clusterFinalizer = "infrastructure.cluster.x-k8s.io/finalizer"
)

type ProxmoxStorage struct {
	Type    string `json:"type"`
	Content string `json:"content"`
	Mkdir   bool   `json:"mkdir"`
	Path    string `json:"path"`
	Storage string `json:"storage"`
}

//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=proxmoxclusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=proxmoxclusters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=proxmoxclusters/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ProxmoxCluster object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *ProxmoxClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	contextLogger := log.FromContext(ctx)

	proxmoxCluster := &infrastructurev1alpha1.ProxmoxCluster{}
	if err := r.Get(ctx, req.NamespacedName, proxmoxCluster); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		contextLogger.Error(err, "Failed getting ProxmoxCluster")
		return ctrl.Result{}, err
	}

	cluster, err := util.GetOwnerCluster(ctx, r.Client, proxmoxCluster.ObjectMeta)
	if err != nil {
		contextLogger.Error(err, "Failed getting owning cluster")
		return ctrl.Result{}, err
	}

	if cluster == nil {
		contextLogger.Info("Cluster controller doesnt have owner ref yet")
		return ctrl.Result{}, nil
	}

	if annotations.IsPaused(cluster, proxmoxCluster) {
		contextLogger.Info("Cluster / ProxmoxCluster paused")
		return ctrl.Result{}, nil
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
		proxmox.WithCredentials(&proxmox.Credentials{
			Username: string(credentialsSecret.Data["username"]),
			Password: string(credentialsSecret.Data["password"]),
		}),
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

	// Ensure the directory storage is created
	if proxmoxCluster.Status.StorageReady == false {
		var res interface{}
		// Note: For now, we aren't going to delete this storage upon cluster deletion
		err := proxmoxClient.Get(fmt.Sprintf("/storage/%s", proxmoxCluster.Name), &res)
		fmt.Println(err)
		if err != nil {
			// Create the storage mechanism
			storage := &ProxmoxStorage{
				Storage: proxmoxCluster.Name,
				Mkdir:   true,
				Type:    "dir",
				Content: "images,snippets",
				Path:    fmt.Sprintf("/snippets/%s", proxmoxCluster.Name),
			}
			err = proxmoxClient.Post("/storage", &storage, nil)
			if err != nil {
				contextLogger.Error(err, "Failed creating cluster storage")
				return ctrl.Result{}, err
			}
		}

		proxmoxCluster.Status.StorageReady = true
		err = r.Status().Update(context.TODO(), proxmoxCluster)
		return ctrl.Result{}, err
	}

	// Ensure the resource pool for this cluster is created
	if proxmoxCluster.Spec.Pool != "" {
		_, err := proxmoxClient.Pool(proxmoxCluster.Spec.Pool)
		if err != nil {
			err = proxmoxClient.NewPool(proxmoxCluster.Spec.Pool, proxmoxCluster.Name)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	proxmoxCluster.Status.Ready = true
	err = r.Status().Update(context.TODO(), proxmoxCluster)
	return ctrl.Result{}, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *ProxmoxClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrastructurev1alpha1.ProxmoxCluster{}).
		Complete(r)
}
