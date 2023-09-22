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

### Requirements
- ZFS over ISCSI shared storage for snippets (https://github.com/launchboxio/cluster-api-provider-proxmox/issues/8)
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
  --from-literal=api_token_id="${PM_API_TOKEN_ID}" \
  --from-literal=api_token_secret="${PM_API_TOKEN_SECRET}" \
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
