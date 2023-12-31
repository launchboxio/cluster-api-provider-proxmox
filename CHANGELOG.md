# Changelog

All notable changes to this project will be documented in this file.

### [0.2.8](https://github.com/launchboxio/cluster-api-provider-proxmox/compare/v0.2.7...v0.2.8) (2023-09-25)


### Bug Fixes

* **crd:** Generate missed manifests ([8f4bad7](https://github.com/launchboxio/cluster-api-provider-proxmox/commit/8f4bad7cab6c2d0afc7fcdaea3598e0e4371737f))

### [0.2.7](https://github.com/launchboxio/cluster-api-provider-proxmox/compare/v0.2.6...v0.2.7) (2023-09-25)


### Bug Fixes

* **ci:** Add spec.userData to ProxmoxMachine for adding scripts to init ([#48](https://github.com/launchboxio/cluster-api-provider-proxmox/issues/48)) ([ebdb9bd](https://github.com/launchboxio/cluster-api-provider-proxmox/commit/ebdb9bdb08f49cead6a1575d99480a1fcdeb8b9e))

### [0.2.6](https://github.com/launchboxio/cluster-api-provider-proxmox/compare/v0.2.5...v0.2.6) (2023-09-23)


### Bug Fixes

* **node:** Rather than select 1 at launchtime, populate the CR spec ([#47](https://github.com/launchboxio/cluster-api-provider-proxmox/issues/47)) ([7e5231c](https://github.com/launchboxio/cluster-api-provider-proxmox/commit/7e5231c1eec4a831a1eb3540c01bc7308cf9ac43))

### [0.2.5](https://github.com/launchboxio/cluster-api-provider-proxmox/compare/v0.2.4...v0.2.5) (2023-09-22)


### Bug Fixes

* **storage:** Remove invalid fmt.Sprintf ([f306a2e](https://github.com/launchboxio/cluster-api-provider-proxmox/commit/f306a2ef9c05c8ef1a969de9c2c8e79bfe696739))

### [0.2.4](https://github.com/launchboxio/cluster-api-provider-proxmox/compare/v0.2.3...v0.2.4) (2023-09-22)


### Bug Fixes

* **storage:** Move storage to module, SFTP is pluggable ([f524936](https://github.com/launchboxio/cluster-api-provider-proxmox/commit/f524936397d8a8ff1f3bbb6e7676d0f6d434560d))

### [0.2.3](https://github.com/launchboxio/cluster-api-provider-proxmox/compare/v0.2.2...v0.2.3) (2023-09-22)


### Bug Fixes

* **scope:** Use scopes to encapsulate the cluster and machine references ([6e12d6c](https://github.com/launchboxio/cluster-api-provider-proxmox/commit/6e12d6c1eb9c2eaf98e3888b2eb36fcef5683eca))

### [0.2.2](https://github.com/launchboxio/cluster-api-provider-proxmox/compare/v0.2.1...v0.2.2) (2023-09-22)


### Bug Fixes

* **typo:** Committed typo in strings ([842187c](https://github.com/launchboxio/cluster-api-provider-proxmox/commit/842187ca8bd8a4c6ca4547406c12afc01f287c43))

### [0.2.1](https://github.com/launchboxio/cluster-api-provider-proxmox/compare/v0.2.0...v0.2.1) (2023-09-22)


### Bug Fixes

* **cloudinit:** Allow network data to be configurable ([ab297d4](https://github.com/launchboxio/cluster-api-provider-proxmox/commit/ab297d4f080a0db3a2defd2fdf7e309215aa5c2b))

## [0.2.0](https://github.com/launchboxio/cluster-api-provider-proxmox/compare/v0.1.1...v0.2.0) (2023-09-22)


### Features

* **revert:** Were not ready to mount storage yet ([1b13070](https://github.com/launchboxio/cluster-api-provider-proxmox/commit/1b130701e854de0c99d04e1aa2edf8327fbe8279))

### [0.1.1](https://github.com/launchboxio/cluster-api-provider-proxmox/compare/v0.1.0...v0.1.1) (2023-09-22)


### Bug Fixes

* **storage:** Rather than unlink, reconfigure the VM with an empty IDE2 ([#45](https://github.com/launchboxio/cluster-api-provider-proxmox/issues/45)) ([d1c2d54](https://github.com/launchboxio/cluster-api-provider-proxmox/commit/d1c2d54523b6fa91422b9b874719d09d93fb66bf))

## [0.1.0](https://github.com/launchboxio/cluster-api-provider-proxmox/compare/v0.0.14...v0.1.0) (2023-09-22)


### Features

* **storage:** Require snippet storage be locally mounted, rather than handle several storage mechanisms ([#44](https://github.com/launchboxio/cluster-api-provider-proxmox/issues/44)) ([f273500](https://github.com/launchboxio/cluster-api-provider-proxmox/commit/f2735006c3636d104b4af9d3a3b4c9951f599002))

### [0.0.14](https://github.com/launchboxio/cluster-api-provider-proxmox/compare/v0.0.13...v0.0.14) (2023-09-21)


### Bug Fixes

* **ci:** Prefix built images with v ([#43](https://github.com/launchboxio/cluster-api-provider-proxmox/issues/43)) ([a0865b1](https://github.com/launchboxio/cluster-api-provider-proxmox/commit/a0865b134ccfb565b225248fe3a252418def5f97))

### [0.0.13](https://github.com/launchboxio/cluster-api-provider-proxmox/compare/v0.0.12...v0.0.13) (2023-09-21)


### Bug Fixes

* **ci:** Use ref_name, not ref ([bf2500c](https://github.com/launchboxio/cluster-api-provider-proxmox/commit/bf2500cded350912de975460856cd5c2b379c26b))

### [0.0.12](https://github.com/launchboxio/cluster-api-provider-proxmox/compare/v0.0.11...v0.0.12) (2023-09-21)


### Bug Fixes

* **ci:** Fix image name generation ([100d85c](https://github.com/launchboxio/cluster-api-provider-proxmox/commit/100d85c5e05f5a06fd88b819a6e4359939a6dc4f))

### [0.0.11](https://github.com/launchboxio/cluster-api-provider-proxmox/compare/v0.0.10...v0.0.11) (2023-09-21)


### Bug Fixes

* **ci:** Build infrastructure components using the github.ref ([c5adc86](https://github.com/launchboxio/cluster-api-provider-proxmox/commit/c5adc86ea8d1a9778fa928a0d1873ab6995c2a4c))

### [0.0.10](https://github.com/launchboxio/cluster-api-provider-proxmox/compare/v0.0.9...v0.0.10) (2023-09-21)


### Bug Fixes

* **ci:** Use 0.0.x versions" ([2b494e0](https://github.com/launchboxio/cluster-api-provider-proxmox/commit/2b494e0759325e30d99f725976f893b5e91622e9))

### [0.0.9](https://github.com/launchboxio/cluster-api-provider-proxmox/compare/v0.0.8...v0.0.9) (2023-09-21)


### Bug Fixes

* **ci:** Update metadata.yaml to support Cluster API installation ([0f0faff](https://github.com/launchboxio/cluster-api-provider-proxmox/commit/0f0faff4a174df8b44e18e08b1ce435fce4ad0f3))

### [0.0.8](https://github.com/launchboxio/cluster-api-provider-proxmox/compare/v0.0.7...v0.0.8) (2023-09-21)


### Bug Fixes

* **ci:** Publish metadata.yaml as well ([6701664](https://github.com/launchboxio/cluster-api-provider-proxmox/commit/6701664a60ea2c580ca004352de0e0af2a83c05b))

### [0.0.7](https://github.com/launchboxio/cluster-api-provider-proxmox/compare/v0.0.6...v0.0.7) (2023-09-21)


### Bug Fixes

* **ci:** Run make infra-yaml on release events ([7251e0a](https://github.com/launchboxio/cluster-api-provider-proxmox/commit/7251e0af54e68373b0b1f06b674378afeb06fa40))

### [0.0.6](https://github.com/launchboxio/cluster-api-provider-proxmox/compare/v0.0.5...v0.0.6) (2023-09-21)


### Bug Fixes

* **ci:** Build infrastructure components ([aaab644](https://github.com/launchboxio/cluster-api-provider-proxmox/commit/aaab64479f90465ebcac90d92dcb27e2250daf0b))

### [0.0.5](https://github.com/launchboxio/cluster-api-provider-proxmox/compare/v0.0.4...v0.0.5) (2023-09-21)


### Bug Fixes

* **ci:** Remove 1.0.0 version, fix docker build ([1153394](https://github.com/launchboxio/cluster-api-provider-proxmox/commit/1153394d8ec8aed4c664542eb443f189aa2096ff))

### [0.0.4](https://github.com/launchboxio/cluster-api-provider-proxmox/compare/v0.0.3...v0.0.4) (2023-09-21)


### Bug Fixes

* **ci:** Fix image metadata ([36232f7](https://github.com/launchboxio/cluster-api-provider-proxmox/commit/36232f764f2f2bf2dc40c4e2755e78ac2f5db3c1))

### [0.0.3](https://github.com/launchboxio/cluster-api-provider-proxmox/compare/v0.0.2...v0.0.3) (2023-09-21)


### Bug Fixes

* **ci:** Add Docker image builds on release event ([865c083](https://github.com/launchboxio/cluster-api-provider-proxmox/commit/865c083b37cf12402a4c6e27fa143543d369184f))

### [0.0.2](https://github.com/launchboxio/cluster-api-provider-proxmox/compare/v0.0.1...v0.0.2) (2023-09-21)


### Bug Fixes

* **ci:** Publish starting from 0.0.1 ([23e9671](https://github.com/launchboxio/cluster-api-provider-proxmox/commit/23e96711e9b9bc170a9e5801992e7686b6c92d19))

### **0.0.1** (2023-09-21)

### Bug Fixes

* **ci:** Add branches and auth token ([b5a5790](https://github.com/launchboxio/cluster-api-provider-proxmox/commit/b5a57905f0b1108a87c19300c581bdd902e73064))
* **ci:** Add conventional-changelog-conventionalcommits ([0aa1446](https://github.com/launchboxio/cluster-api-provider-proxmox/commit/0aa14464ba66414d6d8bae2134c5f9e557aab7c7))
* **ci:** Add missing runs-on ([17d2cef](https://github.com/launchboxio/cluster-api-provider-proxmox/commit/17d2cef643a37c1a3f684c01e7b6103e1804692a))
* **ci:** Add release configuration ([d68c34c](https://github.com/launchboxio/cluster-api-provider-proxmox/commit/d68c34cf6ff19f00832fc09af2132c7434d7a4c2))
* **ci:** Just use JSON ([1f30035](https://github.com/launchboxio/cluster-api-provider-proxmox/commit/1f30035305a3f734c190b3f960035cbbc0ac8c7f))
