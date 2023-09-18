# Reference: https://ronamosa.io/docs/engineer/LAB/proxmox-packer-vm/
packer {
  required_plugins {
    name = {
      version = "~> 1"
      source  = "github.com/hashicorp/proxmox"
    }
  }
}

variable "kubernetes_version" {
  type = string
  default = "1.25.11"
}

variable "containerd_version" {
  type = string
  default = ":1.7.6"
}

variable "node" {
  type = string
}

source "proxmox-iso" "base" {
  iso_file = "isos:iso/ubuntu-20.04.6-live-server-amd64.iso"
  unmount_iso = true

  template_name = "cluster-api-template"
  onboot = true


  insecure_skip_tls_verify = true

  network_adapters {
    bridge = "vmbr1"
    model = "virtio"
  }

  disks {
    type = "scsi"
    disk_size = "32G"
    storage_pool = "storage"
    storage_pool_type = "zfs"
    format = "raw"
  }

  os = "l26"
  cpu_type = "host"
  sockets = 1
  cores = 2
  memory = 2048
  scsi_controller = "virtio-scsi-pci"

  qemu_agent = true

  cloud_init = true
  cloud_init_storage_pool = "storage"

  node = var.node

  boot_command = [
    "<esc><wait><esc><wait>",
    "<f6><wait><esc><wait>",
    "<bs><bs><bs><bs><bs>",
    "autoinstall ds=nocloud-net;s=http://{{ .HTTPIP }}:{{ .HTTPPort }}/ ",
    "--- <enter>"
  ]
  boot = "c"
  boot_wait = "5s"

  # PACKER Autoinstall Settings
  http_directory = "http"
#  http_bind_address = "172.16.2.209"
  http_port_min = 8802
  http_port_max = 8802

  # PACKER SSH Settings
  ssh_username = "ubuntu"
  ssh_private_key_file = "~/.ssh/id_rsa"

  # Raise the timeout, when installation takes longer
  ssh_timeout = "55m"
}

build {
  sources = ["source.proxmox-iso.base"]

  # Provisioning the VM Template for Cloud-Init Integration in Proxmox #1
  provisioner "shell" {
    inline = [
      "while [ ! -f /var/lib/cloud/instance/boot-finished ]; do echo 'Waiting for cloud-init...'; sleep 1; done",
      "sudo rm /etc/ssh/ssh_host_*",
      "sudo truncate -s 0 /etc/machine-id",
      "sudo apt -y autoremove --purge",
      "sudo apt -y clean",
      "sudo apt -y autoclean",
      "sudo cloud-init clean",
      "sudo rm -f /etc/cloud/cloud.cfg.d/subiquity-disable-cloudinit-networking.cfg",
      "sudo sync"
    ]
  }

  # Provisioning the VM Template for Cloud-Init Integration in Proxmox #3
  provisioner "file" {
    source = "files/99-pve.cfg"
    destination = "/tmp/99-pve.cfg"
  }

  # Provisioning the VM Template for Cloud-Init Integration in Proxmox #3
  provisioner "shell" {
    inline = [ "sudo cp /tmp/99-pve.cfg /etc/cloud/cloud.cfg.d/99-pve.cfg" ]
  }
}
