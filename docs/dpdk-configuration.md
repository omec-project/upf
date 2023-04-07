<!--
SPDX-License-Identifier: Apache-2.0
Copyright 2022 Intel Corporation
-->

# DPDK Configuration

The following steps are required to properly configure the devices to deploy the
UPF in DPDK mode. Let's assume that interfaces `ens801f0` and `ens801f1` are the
ones to be used for this purpose.

- Get their MAC addresses
```bash
$ ip a
1: lo: <LOOPBACK,UP,LOWER_UP> mtu 65536 qdisc noqueue state UNKNOWN group default qlen 1000
    link/loopback 00:00:00:00:00:00 brd 00:00:00:00:00:00
    inet 127.0.0.1/8 scope host lo
       valid_lft forever preferred_lft forever
    inet6 ::1/128 scope host
       valid_lft forever preferred_lft forever
...
3: ens801f0: <NO-CARRIER,BROADCAST,MULTICAST,UP> mtu 1500 qdisc mq state UP group default qlen 1000
    link/ether b4:96:91:b1:ff:f0 brd ff:ff:ff:ff:ff:ff
4: ens801f1: <NO-CARRIER,BROADCAST,MULTICAST,UP> mtu 1500 qdisc mq state UP group default qlen 1000
    link/ether b4:96:91:b1:ff:f1 brd ff:ff:ff:ff:ff:ff
...
```

- Download a copy of dpdk-devbind script

The dpdk-devbind script from DPDK is used for this purpose. To get a copy of it,
execute the following command from the UPF's root directory:
```bash
$ wget https://raw.githubusercontent.com/DPDK/dpdk/main/usertools/dpdk-devbind.py -O dpdk-devbind.py
$ chmod +x dpdk-devbind.py
```

- Get the PCI addresses of interest
```bash
$ ./dpdk-devbind.py -s
Network devices using kernel driver
===================================
0000:17:00.0 'Ethernet Controller X710 for 10GBASE-T 15ff' if=ens260f0 drv=i40e unused=vfio-pci *Active*
0000:17:00.1 'Ethernet Controller X710 for 10GBASE-T 15ff' if=ens260f1 drv=i40e unused=vfio-pci
0000:4b:00.0 'Ethernet Controller E810-C for QSFP 1592' if=ens785f0 drv=ice unused=vfio-pci
0000:4b:00.1 'Ethernet Controller E810-C for QSFP 1592' if=ens785f1 drv=ice unused=vfio-pci
0000:b1:00.0 'Ethernet Controller E810-C for QSFP 1592' if=ens801f0 drv=ice unused=vfio-pci
0000:b1:00.1 'Ethernet Controller E810-C for QSFP 1592' if=ens801f1 drv=ice unused=vfio-pci

No 'Baseband' devices detected
==============================

...
```

- Bind devices to `DPDK-compatible driver`

```bash
$ sudo ./dpdk-devbind.py -b vfio-pci 0000:b1:00.0
$ sudo ./dpdk-devbind.py -b vfio-pci 0000:b1:00.1
```

- Verify that the binding was successful
```bash
$ ./dpdk-devbind.py -s

Network devices using DPDK-compatible driver
============================================
0000:b1:00.0 'Ethernet Controller E810-C for QSFP 1592' drv=vfio-pci unused=ice
0000:b1:00.1 'Ethernet Controller E810-C for QSFP 1592' drv=vfio-pci unused=ice

Network devices using kernel driver
===================================
0000:17:00.0 'Ethernet Controller X710 for 10GBASE-T 15ff' if=ens260f0 drv=i40e unused=vfio-pci *Active*
0000:17:00.1 'Ethernet Controller X710 for 10GBASE-T 15ff' if=ens260f1 drv=i40e unused=vfio-pci
0000:4b:00.0 'Ethernet Controller E810-C for QSFP 1592' if=ens785f0 drv=ice unused=vfio-pci
0000:4b:00.1 'Ethernet Controller E810-C for QSFP 1592' if=ens785f1 drv=ice unused=vfio-pci

No 'Baseband' devices detected
==============================

...
```

- Now, check the group that these two interfaces got assigned
```bash
$ ls /dev/vfio/
184  185  vfio
```
