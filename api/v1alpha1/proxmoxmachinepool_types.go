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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ProxmoxMachinePoolSpec defines thed desired state of ProxmoxMachinePool
type ProxmoxMachinePoolSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// The number of instances to deploy in this machine pool
	Instances          int                     `json:"instances"`
	MachineTemplateRef *corev1.ObjectReference `json:"machineTemplateRef"`
}

// ProxmoxMachinePoolStatus defines the observed state of ProxmoxMachinePool
type ProxmoxMachinePoolStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// ProxmoxMachinePool is the Schema for the proxmoxmachinepools API
type ProxmoxMachinePool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProxmoxMachinePoolSpec   `json:"spec,omitempty"`
	Status ProxmoxMachinePoolStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ProxmoxMachinePoolList contains a list of ProxmoxMachinePool
type ProxmoxMachinePoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ProxmoxMachinePool `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ProxmoxMachinePool{}, &ProxmoxMachinePoolList{})
}
