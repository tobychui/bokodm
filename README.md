![](img/banner.png)

# bokodm

bokodm (ボコdm) is a lightweight, self-contained HTTP server for disk monitoring and RAID management on Linux. It combines a static web UI with a REST API for RAID configuration (via `mdadm`), SMART health monitoring, and disk usage information — all served from a single HTTP endpoint.

**Disclaimer**

This project is in its early stage of development and may not work on all systems. It must be run with sufficient privileges (e.g. as root or with appropriate capabilities) to access block devices and manage RAID arrays.

### Features

- **RAID Management** — Create, delete, assemble, and monitor software RAID arrays via `mdadm`
- **SMART Monitoring** — Query disk health and SMART attributes via `smartctl`
- **Disk Information** — List block devices, partitions, filesystem types, and usage via `lsblk`, `blkid`, and `df`
- **Network Statistics** — Monitor network interface throughput in real time
- **Static Web UI** — Embedded web interface for RAID status and disk overview

### Requirements

- Linux (Debian-based distros recommended)
- Run as root or with sufficient permissions to access block devices
- The following tools must be available in `PATH`:
  - `mdadm` — RAID management
  - `smartctl` (smartmontools) — SMART health monitoring
  - `lsblk`, `blkid`, `df` — disk information (usually pre-installed)
  - `ffmpeg` — optional, only needed for media transcoding features

### Build From Source

Requires Go 1.23.2 or later.

```bash
cd src
go mod tidy
go build -o bokodm
sudo ./bokodm
```

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `-p` | `9000` | HTTP port to listen on |
| `-dev` | `false` | Serve web files from `./web` instead of embedded FS |
| `-c` | `./config` | Path to the config folder |
| `-skip_dep_check` | `false` | Skip dependency check on startup |

### Configuration

The mdadm config file path defaults to `/etc/mdadm/mdadm.conf`. It is defined as the `MdadmConfPath` constant in `src/mod/disktool/raid/mdadmConf.go` and can be changed there if needed.

### License

GPL
