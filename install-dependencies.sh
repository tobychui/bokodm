#!/usr/bin/env bash
#
# install-dependencies.sh
#
# Installs every external tool bokodm uses. Required tools are needed for
# the server to start; optional tools enable extra features (network
# filesystem mounting, NTFS support, partitioning) and the web UI will
# hint at them when missing.
#
# Usage:  sudo ./install-dependencies.sh
#
set -u

# ---- package lists ----------------------------------------------------
# name mapping is identical across apt/dnf/yum/zypper unless overridden
# below for a specific package manager.
REQUIRED_PKGS=(
    util-linux      # lsblk, blkid
    coreutils       # df
    mdadm           # RAID management
    smartmontools   # smartctl - SMART health monitoring
)

OPTIONAL_PKGS=(
    parted          # disk partitioning tool
    ntfs-3g         # NTFS mounting / formatting
    dosfstools      # mkfs.vfat for FAT32 formatting
    davfs2          # WebDAV network filesystem
    curlftpfs       # FTP network filesystem
    cifs-utils      # SMB / CIFS network filesystem
    nfs-common      # NFS network filesystem (Debian name)
)

# ---- helpers -----------------------------------------------------------
info()  { printf '\033[36m[i]\033[0m %s\n' "$1"; }
ok()    { printf '\033[32m[✔]\033[0m %s\n' "$1"; }
warn()  { printf '\033[33m[⚠]\033[0m %s\n' "$1"; }
fail()  { printf '\033[31m[✘]\033[0m %s\n' "$1"; }

if [ "$(id -u)" -ne 0 ]; then
    fail "This script must be run as root. Try: sudo $0"
    exit 1
fi

# ---- detect package manager --------------------------------------------
PM=""
if command -v apt-get >/dev/null 2>&1; then PM="apt"
elif command -v dnf >/dev/null 2>&1; then PM="dnf"
elif command -v yum >/dev/null 2>&1; then PM="yum"
elif command -v pacman >/dev/null 2>&1; then PM="pacman"
elif command -v zypper >/dev/null 2>&1; then PM="zypper"
elif command -v apk >/dev/null 2>&1; then PM="apk"
else
    fail "No supported package manager found (apt/dnf/yum/pacman/zypper/apk)"
    exit 1
fi
info "Detected package manager: $PM"

# translate Debian-centric package names for other distros
translate_pkg() {
    local pkg="$1"
    case "$PM" in
        dnf|yum|zypper)
            case "$pkg" in
                nfs-common) echo "nfs-utils" ;;
                *) echo "$pkg" ;;
            esac
            ;;
        pacman)
            case "$pkg" in
                nfs-common) echo "nfs-utils" ;;
                ntfs-3g)    echo "ntfs-3g" ;;
                *) echo "$pkg" ;;
            esac
            ;;
        apk)
            case "$pkg" in
                nfs-common)    echo "nfs-utils" ;;
                smartmontools) echo "smartmontools" ;;
                *) echo "$pkg" ;;
            esac
            ;;
        *) echo "$pkg" ;;
    esac
}

install_pkg() {
    local pkg
    pkg="$(translate_pkg "$1")"
    case "$PM" in
        apt)    DEBIAN_FRONTEND=noninteractive apt-get install -y "$pkg" ;;
        dnf)    dnf install -y "$pkg" ;;
        yum)    yum install -y "$pkg" ;;
        pacman) pacman -S --noconfirm --needed "$pkg" ;;
        zypper) zypper --non-interactive install "$pkg" ;;
        apk)    apk add "$pkg" ;;
    esac
}

# ---- refresh package index ----------------------------------------------
info "Updating package index..."
case "$PM" in
    apt)    apt-get update -qq ;;
    dnf)    dnf makecache -q ;;
    yum)    yum makecache -q ;;
    pacman) pacman -Sy --noconfirm >/dev/null ;;
    zypper) zypper --non-interactive refresh >/dev/null ;;
    apk)    apk update >/dev/null ;;
esac

# ---- install ------------------------------------------------------------
FAILED_REQUIRED=()
FAILED_OPTIONAL=()

info "Installing required packages..."
for pkg in "${REQUIRED_PKGS[@]}"; do
    if install_pkg "$pkg" >/dev/null 2>&1; then
        ok "$pkg"
    else
        fail "$pkg"
        FAILED_REQUIRED+=("$pkg")
    fi
done

info "Installing optional packages..."
for pkg in "${OPTIONAL_PKGS[@]}"; do
    if install_pkg "$pkg" >/dev/null 2>&1; then
        ok "$pkg"
    else
        warn "$pkg (optional, feature will be disabled in the UI)"
        FAILED_OPTIONAL+=("$pkg")
    fi
done

# ---- summary --------------------------------------------------------------
echo
if [ ${#FAILED_REQUIRED[@]} -eq 0 ]; then
    ok "All required dependencies installed."
else
    fail "Required packages failed to install: ${FAILED_REQUIRED[*]}"
    fail "bokodm will refuse to start until these are installed (or use -skip_dep)."
fi
if [ ${#FAILED_OPTIONAL[@]} -gt 0 ]; then
    warn "Optional packages not installed: ${FAILED_OPTIONAL[*]}"
fi
echo
info "Build the server with:  cd src && go mod tidy && go build"
info "Then start it with:     sudo ./bokodmd"
