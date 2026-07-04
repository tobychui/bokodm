![](img/banner.png)

# bokodm

bokodm is a self-contained, http based RAID and disk management system with a web UI and API supporting remote SMART health monitoring, RAID configurations and disk usage information. All within a single http server endpoint / port. 



**Disclaimer**

This project is in its very early stage of development and might not works on all systems. 

### Screenshots

![](img/1.jpg)

![](img/2.jpg)

![](img/3.jpg)

### Features

- **Status** — disk SMART health overview, real-time network IO and network interfaces
- **Analytics Report** — full printable system report (OS, hardware, disks, partitions, SMART, RAID) exportable to PDF via the browser print dialog
- **Connections** — mount remote network filesystems (WebDAV, FTP, SMB / CIFS, NFS) onto this host, with remote volume usage where the protocol supports it
- **Disks** — list disks and partitions, mount / unmount a partition to any host folder with a built-in folder picker
- **RAID** — create / delete arrays (mdadm), swap damaged disks (fail → remove → add replacement), hot spares, rebuild progress and array expansion

### Build From Source

- Require go compiler 1.23.2 installed on your system
- Currently only support Debian (and Debian based distros)
- Require the following package installed
  - smartmontools (for disk SMART)
  - mdadm (for RAID management)
  - lsblk, blkid and df (usually come with Debian but make sure you have permission to run them)
- Optional packages for network filesystem mounting (the UI hints at these when missing)
  - davfs2 (WebDAV)
  - curlftpfs (FTP)
  - cifs-utils (SMB / CIFS)
  - nfs-common (NFS)

```
cd src
go mod tidy
go build
sudo ./bokodmd
# To start even when required tools are missing, run
# ./bokodmd -skip_dep
```

### License

GPL
