package install

import "text/template"

type PackageManagerInstallArgs struct {
	KubernetesVersion string
	Hostname          string
}

var PackageManagerInstallScript = template.Must(template.New("packages").Parse(`
#!/usr/bin/env bash
set -x

hostnamectl set-hostname {{ .Hostname }}
KUBERNETES_VERSION="{{ .KubernetesVersion }}"
export repo=${KUBERNETES_VERSION%.*}
apt-get update -y
apt-get install -y apt-transport-https ca-certificates curl gnupg qemu-guest-agent wget

systemctl enable qemu-guest-agent
systemctl start qemu-guest-agent

swapoff -a && sed -ri '/\sswap\s/s/^#?/#/' /etc/fstab
modprobe overlay && modprobe br_netfilter

tee /etc/sysctl.d/kubernetes.conf <<EOF
net.bridge.bridge-nf-call-ip6tables = 1
net.bridge.bridge-nf-call-iptables = 1
net.ipv4.ip_forward = 1
EOF

sysctl --system

tee /etc/modules-load.d/k8s.conf <<EOF
overlay
br_netfilter
EOF

curl -fsSL https://download.docker.com/linux/ubuntu/gpg | apt-key add -
add-apt-repository "deb [arch=amd64] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable"

apt update -y
apt install containerd.io -y
mkdir /etc/containerd
containerd config default>/etc/containerd/config.toml

modprobe overlay && modprobe br_netfilter
systemctl restart containerd
systemctl enable containerd

mkdir -p -m 755 /etc/apt/keyrings
curl -fsSL https://pkgs.k8s.io/core:/stable:/v${repo}/deb/Release.key | gpg --dearmor -o /etc/apt/keyrings/kubernetes-apt-keyring.gpg
echo "deb [signed-by=/etc/apt/keyrings/kubernetes-apt-keyring.gpg] https://pkgs.k8s.io/core:/stable:/v${repo}/deb/ /" | tee /etc/apt/sources.list.d/kubernetes.list

apt-get update -y
apt-get install -y kubelet=$KUBERNETES_VERSION-* kubeadm=$KUBERNETES_VERSION-* kubectl=$KUBERNETES_VERSION-*
apt-mark hold kubelet kubeadm kubectl

swapoff -a && sed -ri '/\sswap\s/s/^#?/#/' /etc/fstab
`))
