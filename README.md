# Proxmox Cluster API Provider 

## Requirements 
- S3-compliant storage for snippets
- A pre-generated template for the required Kubernetes version


### Benchmarks

When creating an image from a base cloud image (having to run init scripts)
- 45 seconds from creation to VM start
- 5m15s to kubeadm init
- 6m50s until kubeadm reported as ready
