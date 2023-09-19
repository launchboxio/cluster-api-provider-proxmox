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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ProxmoxMachineSpec defines the desired state of ProxmoxMachine
type ProxmoxMachineSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of ProxmoxMachine. Edit proxmoxmachine_types.go to remove/update
	TargetNode string `json:"targetNode,omitempty"`

	// Foo is an example field of ProxmoxMachineTemplate. Edit proxmoxmachinetemplate_types.go to remove/update
	OnBoot bool   `json:"onboot,omitempty"`
	Scsihw string `json:"scsihw,omitempty"`

	Template  string                  `json:"template"`
	Resources ProxmoxMachineResources `json:"resources"`
	Networks  []ProxmoxNetwork        `json:"networks"`
	Disks     []ProxmoxDisk           `json:"disks"`

	NetworkUserData string   `json:"networkUserData,omitempty"`
	SshKeys         []string `json:"sshKeys,omitempty"`

	ProviderID string `json:"providerID,omitempty"`
}

type ProxmoxMachineResources struct {
	Memory     int `json:"memory,omitempty"`
	CpuCores   int `json:"cpuCores,omitempty"`
	CpuSockets int `json:"cpuSockets,omitempty"`
}

type ProxmoxNetwork struct {
	Model    string `json:"model"`
	Bridge   string `json:"bridge"`
	Firewall bool   `json:"firewall,omitempty"`
	Backup   bool   `json:"backup,omitempty"`
	Tag      string `json:"tag,omitempty"`
}

type ProxmoxDisk struct {
	Type        string `json:"type,omitempty"`
	Storage     string `json:"storage,omitempty"`
	Size        string `json:"size,omitempty"`
	StorageType string `json:"storageType,omitempty"`
	Backup      bool   `json:"backup,omitempty"`
}

// ProxmoxMachineStatus defines the observed state of ProxmoxMachine
type ProxmoxMachineStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Vmid       int                   `json:"vmid"`
	Ready      bool                  `json:"ready,omitempty"`
	Conditions []clusterv1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// ProxmoxMachine is the Schema for the proxmoxmachines API
type ProxmoxMachine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProxmoxMachineSpec   `json:"spec,omitempty"`
	Status ProxmoxMachineStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ProxmoxMachineList contains a list of ProxmoxMachine
type ProxmoxMachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ProxmoxMachine `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ProxmoxMachine{}, &ProxmoxMachineList{})
}

// GetConditions returns the conditions of ProxmoxMachine status
func (proxmoxMachine *ProxmoxMachine) GetConditions() clusterv1.Conditions {
	return proxmoxMachine.Status.Conditions
}

// SetConditions sets the conditions of ProxmoxMachine status
func (proxmoxMachine *ProxmoxMachine) SetConditions(conditions clusterv1.Conditions) {
	proxmoxMachine.Status.Conditions = conditions
}
