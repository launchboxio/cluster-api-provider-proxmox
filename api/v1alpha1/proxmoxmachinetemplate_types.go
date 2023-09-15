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
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ProxmoxMachineTemplateSpec defines the desired state of ProxmoxMachineTemplate
type ProxmoxMachineTemplateSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of ProxmoxMachineTemplate. Edit proxmoxmachinetemplate_types.go to remove/update
	OnBoot bool   `json:"onboot,omitempty"`
	Scsihw string `json:"scsihw,omitempty"`

	Template  string                  `json:"template"`
	Resources ProxmoxMachineResources `json:"resources"`
	Networks  []ProxmoxNetwork        `json:"networks"`
	Disks     []ProxmoxDisk           `json:"disks"`
}

type ProxmoxMachineResources struct {
	Memory     int `json:"memory,omitempty"`
	CpuCores   int `json:"cpu_cores,omitempty"`
	CpuSockets int `json:"cpu_sockets,omitempty"`
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
	StorageType string `json:"storage_type,omitempty"`
	Backup      bool   `json:"backup,omitempty"`
}

// ProxmoxMachineTemplateStatus defines the observed state of ProxmoxMachineTemplate
type ProxmoxMachineTemplateStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// ProxmoxMachineTemplate is the Schema for the proxmoxmachinetemplates API
type ProxmoxMachineTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProxmoxMachineTemplateSpec   `json:"spec,omitempty"`
	Status ProxmoxMachineTemplateStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ProxmoxMachineTemplateList contains a list of ProxmoxMachineTemplate
type ProxmoxMachineTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ProxmoxMachineTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ProxmoxMachineTemplate{}, &ProxmoxMachineTemplateList{})
}
