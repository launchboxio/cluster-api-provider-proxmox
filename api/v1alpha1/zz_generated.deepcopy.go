//go:build !ignore_autogenerated
// +build !ignore_autogenerated

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

// Code generated by controller-gen. DO NOT EDIT.

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/cluster-api/api/v1beta1"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ProxmoxCluster) DeepCopyInto(out *ProxmoxCluster) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ProxmoxCluster.
func (in *ProxmoxCluster) DeepCopy() *ProxmoxCluster {
	if in == nil {
		return nil
	}
	out := new(ProxmoxCluster)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ProxmoxCluster) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ProxmoxClusterList) DeepCopyInto(out *ProxmoxClusterList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]ProxmoxCluster, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ProxmoxClusterList.
func (in *ProxmoxClusterList) DeepCopy() *ProxmoxClusterList {
	if in == nil {
		return nil
	}
	out := new(ProxmoxClusterList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ProxmoxClusterList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ProxmoxClusterSnippetsConfig) DeepCopyInto(out *ProxmoxClusterSnippetsConfig) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ProxmoxClusterSnippetsConfig.
func (in *ProxmoxClusterSnippetsConfig) DeepCopy() *ProxmoxClusterSnippetsConfig {
	if in == nil {
		return nil
	}
	out := new(ProxmoxClusterSnippetsConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ProxmoxClusterSpec) DeepCopyInto(out *ProxmoxClusterSpec) {
	*out = *in
	out.ControlPlaneEndpoint = in.ControlPlaneEndpoint
	out.CredentialsRef = in.CredentialsRef
	out.Snippets = in.Snippets
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ProxmoxClusterSpec.
func (in *ProxmoxClusterSpec) DeepCopy() *ProxmoxClusterSpec {
	if in == nil {
		return nil
	}
	out := new(ProxmoxClusterSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ProxmoxClusterStatus) DeepCopyInto(out *ProxmoxClusterStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make(v1beta1.Conditions, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.FailureDomains != nil {
		in, out := &in.FailureDomains, &out.FailureDomains
		*out = make(v1beta1.FailureDomains, len(*in))
		for key, val := range *in {
			(*out)[key] = *val.DeepCopy()
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ProxmoxClusterStatus.
func (in *ProxmoxClusterStatus) DeepCopy() *ProxmoxClusterStatus {
	if in == nil {
		return nil
	}
	out := new(ProxmoxClusterStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ProxmoxDisk) DeepCopyInto(out *ProxmoxDisk) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ProxmoxDisk.
func (in *ProxmoxDisk) DeepCopy() *ProxmoxDisk {
	if in == nil {
		return nil
	}
	out := new(ProxmoxDisk)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ProxmoxMachine) DeepCopyInto(out *ProxmoxMachine) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ProxmoxMachine.
func (in *ProxmoxMachine) DeepCopy() *ProxmoxMachine {
	if in == nil {
		return nil
	}
	out := new(ProxmoxMachine)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ProxmoxMachine) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ProxmoxMachineList) DeepCopyInto(out *ProxmoxMachineList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]ProxmoxMachine, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ProxmoxMachineList.
func (in *ProxmoxMachineList) DeepCopy() *ProxmoxMachineList {
	if in == nil {
		return nil
	}
	out := new(ProxmoxMachineList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ProxmoxMachineList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ProxmoxMachinePool) DeepCopyInto(out *ProxmoxMachinePool) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ProxmoxMachinePool.
func (in *ProxmoxMachinePool) DeepCopy() *ProxmoxMachinePool {
	if in == nil {
		return nil
	}
	out := new(ProxmoxMachinePool)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ProxmoxMachinePool) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ProxmoxMachinePoolList) DeepCopyInto(out *ProxmoxMachinePoolList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]ProxmoxMachinePool, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ProxmoxMachinePoolList.
func (in *ProxmoxMachinePoolList) DeepCopy() *ProxmoxMachinePoolList {
	if in == nil {
		return nil
	}
	out := new(ProxmoxMachinePoolList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ProxmoxMachinePoolList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ProxmoxMachinePoolSpec) DeepCopyInto(out *ProxmoxMachinePoolSpec) {
	*out = *in
	if in.MachineTemplateRef != nil {
		in, out := &in.MachineTemplateRef, &out.MachineTemplateRef
		*out = new(corev1.ObjectReference)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ProxmoxMachinePoolSpec.
func (in *ProxmoxMachinePoolSpec) DeepCopy() *ProxmoxMachinePoolSpec {
	if in == nil {
		return nil
	}
	out := new(ProxmoxMachinePoolSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ProxmoxMachinePoolStatus) DeepCopyInto(out *ProxmoxMachinePoolStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ProxmoxMachinePoolStatus.
func (in *ProxmoxMachinePoolStatus) DeepCopy() *ProxmoxMachinePoolStatus {
	if in == nil {
		return nil
	}
	out := new(ProxmoxMachinePoolStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ProxmoxMachineResources) DeepCopyInto(out *ProxmoxMachineResources) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ProxmoxMachineResources.
func (in *ProxmoxMachineResources) DeepCopy() *ProxmoxMachineResources {
	if in == nil {
		return nil
	}
	out := new(ProxmoxMachineResources)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ProxmoxMachineSpec) DeepCopyInto(out *ProxmoxMachineSpec) {
	*out = *in
	out.Resources = in.Resources
	if in.Networks != nil {
		in, out := &in.Networks, &out.Networks
		*out = make([]ProxmoxNetwork, len(*in))
		copy(*out, *in)
	}
	if in.Disks != nil {
		in, out := &in.Disks, &out.Disks
		*out = make([]ProxmoxDisk, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ProxmoxMachineSpec.
func (in *ProxmoxMachineSpec) DeepCopy() *ProxmoxMachineSpec {
	if in == nil {
		return nil
	}
	out := new(ProxmoxMachineSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ProxmoxMachineStatus) DeepCopyInto(out *ProxmoxMachineStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]v1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ProxmoxMachineStatus.
func (in *ProxmoxMachineStatus) DeepCopy() *ProxmoxMachineStatus {
	if in == nil {
		return nil
	}
	out := new(ProxmoxMachineStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ProxmoxMachineTemplate) DeepCopyInto(out *ProxmoxMachineTemplate) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ProxmoxMachineTemplate.
func (in *ProxmoxMachineTemplate) DeepCopy() *ProxmoxMachineTemplate {
	if in == nil {
		return nil
	}
	out := new(ProxmoxMachineTemplate)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ProxmoxMachineTemplate) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ProxmoxMachineTemplateList) DeepCopyInto(out *ProxmoxMachineTemplateList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]ProxmoxMachineTemplate, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ProxmoxMachineTemplateList.
func (in *ProxmoxMachineTemplateList) DeepCopy() *ProxmoxMachineTemplateList {
	if in == nil {
		return nil
	}
	out := new(ProxmoxMachineTemplateList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ProxmoxMachineTemplateList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ProxmoxMachineTemplateResource) DeepCopyInto(out *ProxmoxMachineTemplateResource) {
	*out = *in
	in.Spec.DeepCopyInto(&out.Spec)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ProxmoxMachineTemplateResource.
func (in *ProxmoxMachineTemplateResource) DeepCopy() *ProxmoxMachineTemplateResource {
	if in == nil {
		return nil
	}
	out := new(ProxmoxMachineTemplateResource)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ProxmoxMachineTemplateSpec) DeepCopyInto(out *ProxmoxMachineTemplateSpec) {
	*out = *in
	in.Template.DeepCopyInto(&out.Template)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ProxmoxMachineTemplateSpec.
func (in *ProxmoxMachineTemplateSpec) DeepCopy() *ProxmoxMachineTemplateSpec {
	if in == nil {
		return nil
	}
	out := new(ProxmoxMachineTemplateSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ProxmoxMachineTemplateStatus) DeepCopyInto(out *ProxmoxMachineTemplateStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ProxmoxMachineTemplateStatus.
func (in *ProxmoxMachineTemplateStatus) DeepCopy() *ProxmoxMachineTemplateStatus {
	if in == nil {
		return nil
	}
	out := new(ProxmoxMachineTemplateStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ProxmoxNetwork) DeepCopyInto(out *ProxmoxNetwork) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ProxmoxNetwork.
func (in *ProxmoxNetwork) DeepCopy() *ProxmoxNetwork {
	if in == nil {
		return nil
	}
	out := new(ProxmoxNetwork)
	in.DeepCopyInto(out)
	return out
}
