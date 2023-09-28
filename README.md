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

## Project Status
This project is currently under active development. Breaking changes may be occur, so please use at your own risk
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
- Proxmox deployed as a cluster (https://github.com/launchboxio/cluster-api-provider-proxmox/issues/9)

### Credentials 

#### Proxmox API 

```bash
export PM_API_URL=https://proxmox:8006/api2/json
export PM_USERNAME='root@pam'
export PM_PASSWORD="xxxxxxxxxxx"w
export NAMESPACE="my-cluster"

kubectl create secret generic proxmox \
  --from-literal=api_url="${PM_API_URL}" \
  --from-literal=username="${PM_USERNAME}" \
  --from-literal=password="${PM_PASSWORD}" \
  -n "${NAMESPACE}"
```


