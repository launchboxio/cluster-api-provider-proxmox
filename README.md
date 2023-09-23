# Kubernetes Cluster API Provider Proxmox (CAPPVE)

<p align="left">
  <!-- Go version -->
  <img src="https://img.shields.io/github/go-mod/go-version/launchboxio/cluster-api-provider-proxmox" />
  <!-- Latest release -->
  <img src="https://img.shields.io/github/v/release/launchboxio/cluster-api-provider-proxmox" />
  <!-- Release Date -->
  <img src="https://img.shields.io/github/release-date/launchboxio/cluster-api-provider-proxmox" />
  <!-- License -->
  <img src="https://img.shields.io/github/license/launchboxio/cluster-api-provider-proxmox" />
</p>


------

## What is Cluster API Provider Proxmox

[Cluster API](https://github.com/kubernetes-sigs/cluster-api) brings
declarative, Kubernetes-style APIs to cluster creation, configuration and
management.

[Proxmox](https://www.proxmox.com/en/) is an open source hypervisor for launching
QEMU VMs and LXC containers

The Cluster API Provider Proxmox allows a Proxmox host / cluster to respond to the 
infrastructure requests from Cluster API configurations.

## Getting Started 

Before installation, be sure to check the [Requirements]() section first, to ensure your Proxmox environment
is ready
### Installation 

Add the following provider configuration to `~/.cluster-api/clusterctl.yaml`
```yaml 
providers:
  - name: "proxmox"
    url: "https://github.com/launchboxio/cluster-api-provider-proxmox/releases/latest/infrastructure-components.yaml"
    type: "InfrastructureProvider"
```

We can then install the provider alongside ClusterAPI
```bash 
clusterctl init --infrastructure proxmox
```

### Requirements
- NFS storage volume for snippets (https://github.com/launchboxio/cluster-api-provider-proxmox/issues/8)
- Proxmox deployed as a cluster (https://github.com/launchboxio/cluster-api-provider-proxmox/issues/9)

### Credentials 

#### Proxmox API 

```bash
export PM_API_URL=https://proxmox:8006/api2/json
export PM_API_TOKEN_ID=user@pve!cluster-api
export PM_API_TOKEN_SECRET="xxxxxxxxxxx-xxxxx-xxxx-xxxx"
export NAMESPACE="my-cluster"

kubectl create secret generic proxmox \
  --from-literal=api_url="${PM_API_URL}" \
  --from-literal=token_id="${PM_API_TOKEN_ID}" \
  --from-literal=token_secret="${PM_API_TOKEN_SECRET}" \
  -n "${NAMESPACE}"
```

#### Storage 

```bash 
export HOST="0.0.0.0:22"
export USER="user_id"
export PASS="password"
export NAMESPACE="my-cluster"

kubectl create secret generic storage-access \
  --from-literal=host="${HOST}" \
  --from-literal=user="${USER}" \
  --from-literal=password="${PASSWORD}" \
  -n "${NAMESPACE}"
```

### Template 

This provider requires that a base cloud init template be created, which it can 
use to start and configure Kubernetes nodes. At the moment, only Ubuntu 22.04 has been 
tested, but other Ubuntu versions may work

SSH to one of the Proxmox nodes, and perform the following
```bash
export TEMPLATE_ID=XXXX  
export STORAGE="storage"
export DISK_SIZE="32G"

wget  https://cloud-images.ubuntu.com/releases/focal/release/ubuntu-20.04-server-cloudimg-amd64.img
qm create "${TEMPLATE_ID}" --memory 2048 --net0 virtio,bridge=vmbr0 # Change other configurations if needed
qm importdisk "${TEMPLATE_ID}" ubuntu-20.04-server-cloudimg-amd64.img "${STORAGE}"
qm set "${TEMPLATE_ID}" --scsihw virtio-scsi-pci --scsi0 storage:vm-9001-disk-0
qm set "${TEMPLATE_ID}" --serial0 socket --vga --serial0
qm set "${TEMPLATE_ID}" --ide2 storage:cloudinit
qm set "${TEMPLATE_ID}" --boot c --bootdisk scsi0
qm resize "${TEMPLATE_ID}" scsi0 "${DISK_SIZE}"
```
  
This template can then be used as a base for launching the Kubernetes nodes

