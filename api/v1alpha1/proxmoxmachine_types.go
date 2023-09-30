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
	"github.com/luthermonson/go-proxmox"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"strconv"
	"strings"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ProxmoxMachineSpec defines the desired state of ProxmoxMachine
type ProxmoxMachineSpec struct {
	TargetNode string `json:"targetNode,omitempty"`

	OnBoot bool   `json:"onboot,omitempty"`
	Scsihw string `json:"scsihw,omitempty"`

	Template  string                  `json:"template"`
	Resources ProxmoxMachineResources `json:"resources"`
	Networks  []ProxmoxNetwork        `json:"networks"`
	Disks     []ProxmoxDisk           `json:"disks"`

	UserData        string   `json:"userData,omitempty"`
	NetworkUserData string   `json:"networkUserData,omitempty"`
	SshKeys         []string `json:"sshKeys,omitempty"`

	ProviderID string   `json:"providerID,omitempty"`
	Tags       []string `json:"tags,omitempty"`
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
	Size        int    `json:"size,omitempty"`
	StorageType string `json:"storageType,omitempty"`
	Backup      bool   `json:"backup,omitempty"`
}

// ProxmoxMachineStatus defines the observed state of ProxmoxMachine
type ProxmoxMachineStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Vmid        int                                  `json:"vmid"`
	Ready       bool                                 `json:"ready,omitempty"`
	Conditions  []clusterv1.Condition                `json:"conditions,omitempty"`
	State       string                               `json:"state,omitempty"`
	IpAddresses map[string][]string                  `json:"ipAddresses,omitempty"`
	CurrentTask string                               `json:"currentTask,omitempty"`
	Tasks       map[string]*ProxmoxMachineTaskStatus `json:"tasks,omitempty"`
}

type ProxmoxMachineTaskAction string

const (
	ProviderIDPrefix                          = "proxmox://"
	CreateVm         ProxmoxMachineTaskAction = "createvm"
	StartVm          ProxmoxMachineTaskAction = "startvm"
	UnlinkDisk       ProxmoxMachineTaskAction = "unlinkdisk"
	DestroyVm        ProxmoxMachineTaskAction = "destroyvm"
	ConfigureVm      ProxmoxMachineTaskAction = "configurevm"
)

type ProxmoxMachineTaskStatus struct {
	Type      string                   `json:"type,omitempty"`
	State     string                   `json:"state,omitempty"`
	StartTime string                   `json:"startTime,omitempty"`
	EndTime   string                   `json:"endTime,omitempty"`
	Success   bool                     `json:"success,omitempty"`
	Action    ProxmoxMachineTaskAction `json:"action"`
	TaskId    proxmox.UPID             `json:"taskId"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="VMID",type="string",JSONPath=".status.vmid",description="ID of the Proxmox VM"
// +kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.state",description="Last seen state of the VM"

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

func (proxmoxMachine *ProxmoxMachine) SetIpAddresses(ifaces []*proxmox.AgentNetworkIface) {
	resultIfaces := map[string][]string{}
	for _, iface := range ifaces {
		resultIfaces[iface.Name] = []string{}
		for _, addr := range iface.IPAddresses {
			resultIfaces[iface.Name] = append(resultIfaces[iface.Name], addr.IPAddress)
		}
	}
	proxmoxMachine.Status.IpAddresses = resultIfaces
}

func (proxmoxMachine *ProxmoxMachine) GetProviderId() string {
	return proxmoxMachine.Spec.ProviderID
}

func (proxmoxMachine *ProxmoxMachine) IsReady() bool {
	return proxmoxMachine.Status.Ready
}

func (proxmoxMachine *ProxmoxMachine) GetVmId() int {
	return proxmoxMachine.Status.Vmid
}

func (proxmoxMachine *ProxmoxMachine) SetTask(action ProxmoxMachineTaskAction, task *proxmox.Task) {
	if proxmoxMachine.Status.Tasks == nil {
		proxmoxMachine.Status.Tasks = map[string]*ProxmoxMachineTaskStatus{}
	}
	proxmoxMachine.Status.Tasks[task.ID] = &ProxmoxMachineTaskStatus{
		Action:    action,
		Type:      task.Type,
		StartTime: task.StartTime.String(),
		EndTime:   task.EndTime.String(),
		State:     task.Status,
		TaskId:    task.UPID,
	}
}

func (proxmoxMachine *ProxmoxMachine) SetActiveTask(taskId string) {
	proxmoxMachine.Status.CurrentTask = taskId
}

func (proxmoxMachine *ProxmoxMachine) GetActiveTask() *ProxmoxMachineTaskStatus {
	currentTaskId := proxmoxMachine.Status.CurrentTask
	if currentTaskId == "" {
		return nil
	}

	if val, ok := proxmoxMachine.Status.Tasks[currentTaskId]; ok {
		return val
	}

	return nil
}

func (proxmoxMachine *ProxmoxMachine) ClearActiveTask() {
	proxmoxMachine.Status.CurrentTask = ""
}

func (proxmoxMachine *ProxmoxMachine) GetProxmoxId() (int, error) {
	return strconv.Atoi(
		strings.TrimPrefix(proxmoxMachine.GetProviderId(), ProviderIDPrefix),
	)
}
