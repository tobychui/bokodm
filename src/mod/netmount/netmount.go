package netmount

/*
	netmount.go

	This module mounts remote network filesystems (WebDAV, FTP, SMB, NFS)
	onto the local host using the standard Linux mount helpers:

	  WebDAV — mount.davfs  (package davfs2)
	  FTP    — curlftpfs    (package curlftpfs)
	  SMB    — mount.cifs   (package cifs-utils)
	  NFS    — mount.nfs    (package nfs-common / nfs-utils)

	Configured connections are persisted as JSON inside the bokodm config
	folder. Missing mount helpers never block program start: the tool
	availability is reported to the frontend so it can hint the user to
	install the corresponding package instead.
*/

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/uuid"
	"imuslab.com/bokodm/bokodmd/mod/diskinfo/df"
	"imuslab.com/bokodm/bokodmd/mod/utils"
)

type Protocol string

const (
	ProtocolWebDAV Protocol = "webdav"
	ProtocolFTP    Protocol = "ftp"
	ProtocolSMB    Protocol = "smb"
	ProtocolNFS    Protocol = "nfs"
)

// Connection describes one configured network filesystem connection.
type Connection struct {
	UUID       string   `json:"uuid"`
	Name       string   `json:"name"`     // display name
	Protocol   Protocol `json:"protocol"` // webdav / ftp / smb / nfs
	Host       string   `json:"host"`     // remote host or IP
	Port       int      `json:"port"`     // 0 = protocol default
	Path       string   `json:"path"`     // remote path: share for smb, export for nfs, url path for webdav/ftp
	Username   string   `json:"username"`
	Password   string   `json:"password,omitempty"` // persisted to disk, stripped in API responses
	MountPoint string   `json:"mountPoint"`         // local mount point
	Options    string   `json:"options"`            // extra mount options (comma separated)
	AutoMount  bool     `json:"automount"`          // mount on bokodm startup
}

// ConnectionStatus is a Connection with its live mount state attached and
// the password removed. This is what the list API returns.
type ConnectionStatus struct {
	Connection
	IsMounted bool  `json:"isMounted"`
	SizeTotal int64 `json:"sizeTotal"` // -1 when unavailable (e.g. FTP)
	SizeUsed  int64 `json:"sizeUsed"`
	SizeFree  int64 `json:"sizeFree"`
	HasUsage  bool  `json:"hasUsage"` // false when the protocol cannot report usage
}

// SystemNetworkMount describes a network filesystem that is mounted on this
// host but not managed by bokodm (e.g. mounted via /etc/fstab).
type SystemNetworkMount struct {
	Device     string `json:"device"`
	MountPoint string `json:"mountPoint"`
	FsType     string `json:"fstype"`
}

// ToolInfo describes the availability of the mount helper of one protocol.
type ToolInfo struct {
	Protocol    Protocol `json:"protocol"`
	Binary      string   `json:"binary"`
	Package     string   `json:"package"` // Debian-style package name for hints
	Available   bool     `json:"available"`
	InstallHint string   `json:"installHint"`
}

type Manager struct {
	configFile  string
	connections []*Connection
	mu          sync.RWMutex
}

// protocolTools maps each protocol to its mount helper binary and package.
var protocolTools = map[Protocol]struct {
	binary string
	pkg    string
}{
	ProtocolWebDAV: {"mount.davfs", "davfs2"},
	ProtocolFTP:    {"curlftpfs", "curlftpfs"},
	ProtocolSMB:    {"mount.cifs", "cifs-utils"},
	ProtocolNFS:    {"mount.nfs", "nfs-common"},
}

// networkFsTypes are the /proc/mounts fstypes recognized as network filesystems.
var networkFsTypes = map[string]bool{
	"davfs":          true,
	"fuse":           true, // davfs2 registers as fuse on some distros
	"fuse.curlftpfs": true,
	"cifs":           true,
	"smb3":           true,
	"nfs":            true,
	"nfs4":           true,
}

// NewManager creates a network mount manager persisting to configFile and
// tries to mount all AutoMount connections (best-effort).
func NewManager(configFile string) (*Manager, error) {
	m := &Manager{
		configFile:  configFile,
		connections: []*Connection{},
	}

	if _, err := os.Stat(configFile); err == nil {
		content, err := os.ReadFile(configFile)
		if err != nil {
			return nil, fmt.Errorf("unable to read netmount config: %v", err)
		}
		if err := json.Unmarshal(content, &m.connections); err != nil {
			return nil, fmt.Errorf("netmount config corrupted: %v", err)
		}
	}

	// Best-effort automount of stored connections
	for _, c := range m.connections {
		if c.AutoMount {
			if mounted, _ := m.IsMounted(c); !mounted {
				if err := m.Mount(c); err != nil {
					fmt.Printf("[NetMount] Automount %s (%s) failed: %v\n", c.Name, c.Protocol, err)
				}
			}
		}
	}

	return m, nil
}

// save persists the connection list. Config contains credentials so it is
// written with owner-only permission.
func (m *Manager) save() error {
	js, err := json.MarshalIndent(m.connections, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.configFile, js, 0600)
}

// ToolAvailable checks if the mount helper for the given protocol exists.
func ToolAvailable(p Protocol) bool {
	tool, ok := protocolTools[p]
	if !ok {
		return false
	}
	if _, err := exec.LookPath(tool.binary); err == nil {
		return true
	}
	for _, dir := range []string{"/sbin", "/usr/sbin", "/usr/local/sbin"} {
		if _, err := os.Stat(filepath.Join(dir, tool.binary)); err == nil {
			return true
		}
	}
	return false
}

// ListTools reports the availability of every protocol mount helper.
func ListTools() []ToolInfo {
	results := []ToolInfo{}
	for _, p := range []Protocol{ProtocolWebDAV, ProtocolFTP, ProtocolSMB, ProtocolNFS} {
		tool := protocolTools[p]
		results = append(results, ToolInfo{
			Protocol:    p,
			Binary:      tool.binary,
			Package:     tool.pkg,
			Available:   ToolAvailable(p),
			InstallHint: "sudo apt-get install -y " + tool.pkg,
		})
	}
	return results
}

// GetConnection returns the connection with the given UUID.
func (m *Manager) GetConnection(uuid string) (*Connection, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, c := range m.connections {
		if c.UUID == uuid {
			return c, nil
		}
	}
	return nil, errors.New("connection not found")
}

// AddConnection validates, stores and persists a new connection.
func (m *Manager) AddConnection(c *Connection) error {
	if c.Name == "" {
		return errors.New("connection name is required")
	}
	if c.Host == "" {
		return errors.New("remote host is required")
	}
	if _, ok := protocolTools[c.Protocol]; !ok {
		return errors.New("unsupported protocol")
	}
	if c.MountPoint == "" {
		return errors.New("mount point is required")
	}
	if !strings.HasPrefix(c.MountPoint, "/") {
		return errors.New("mount point must be an absolute path")
	}

	c.UUID = uuid.New().String()

	m.mu.Lock()
	m.connections = append(m.connections, c)
	err := m.save()
	m.mu.Unlock()
	return err
}

// UpdateConnection replaces the stored settings of an existing connection.
// The connection must be unmounted first. An empty password keeps the
// previously stored one so the user does not have to re-type it on edit.
func (m *Manager) UpdateConnection(uuid string, updated *Connection) error {
	existing, err := m.GetConnection(uuid)
	if err != nil {
		return err
	}

	if mounted, _ := m.IsMounted(existing); mounted {
		return errors.New("unmount the connection before editing")
	}

	if updated.Name == "" {
		return errors.New("connection name is required")
	}
	if updated.Host == "" {
		return errors.New("remote host is required")
	}
	if _, ok := protocolTools[updated.Protocol]; !ok {
		return errors.New("unsupported protocol")
	}
	if !strings.HasPrefix(updated.MountPoint, "/") {
		return errors.New("mount point must be an absolute path")
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	existing.Name = updated.Name
	existing.Protocol = updated.Protocol
	existing.Host = updated.Host
	existing.Port = updated.Port
	existing.Path = updated.Path
	existing.Username = updated.Username
	if updated.Password != "" {
		existing.Password = updated.Password
	}
	existing.MountPoint = updated.MountPoint
	existing.Options = updated.Options
	existing.AutoMount = updated.AutoMount
	return m.save()
}

// RemoveConnection unmounts (if needed) and deletes a stored connection.
func (m *Manager) RemoveConnection(uuid string) error {
	c, err := m.GetConnection(uuid)
	if err != nil {
		return err
	}

	if mounted, _ := m.IsMounted(c); mounted {
		if err := m.Unmount(c); err != nil {
			return fmt.Errorf("unable to unmount before removal: %v", err)
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	newList := []*Connection{}
	for _, conn := range m.connections {
		if conn.UUID != uuid {
			newList = append(newList, conn)
		}
	}
	m.connections = newList
	return m.save()
}

// remoteSource builds the mount source string for the given connection.
func remoteSource(c *Connection) string {
	remotePath := "/" + strings.TrimPrefix(c.Path, "/")
	switch c.Protocol {
	case ProtocolWebDAV:
		// Allow the user to specify the scheme in Host (e.g. https://server)
		hostURL := c.Host
		if !strings.Contains(hostURL, "://") {
			hostURL = "http://" + hostURL
		}
		u, err := url.Parse(hostURL)
		if err != nil {
			return hostURL + remotePath
		}
		if c.Port != 0 {
			u.Host = fmt.Sprintf("%s:%d", u.Hostname(), c.Port)
		}
		u.Path = remotePath
		return u.String()
	case ProtocolFTP:
		host := c.Host
		host = strings.TrimPrefix(host, "ftp://")
		if c.Port != 0 {
			host = fmt.Sprintf("%s:%d", host, c.Port)
		}
		return "ftp://" + host + remotePath
	case ProtocolSMB:
		share := strings.TrimPrefix(c.Path, "/")
		return "//" + c.Host + "/" + share
	case ProtocolNFS:
		host := c.Host
		if c.Port != 0 {
			// mount.nfs takes port as an option, handled in Mount()
			host = c.Host
		}
		return host + ":" + remotePath
	}
	return ""
}

// Mount mounts the given connection onto its mount point.
func (m *Manager) Mount(c *Connection) error {
	if !ToolAvailable(c.Protocol) {
		tool := protocolTools[c.Protocol]
		return fmt.Errorf("mount tool %s not installed (install package %s)", tool.binary, tool.pkg)
	}

	if mounted, _ := m.IsMounted(c); mounted {
		return errors.New("already mounted")
	}

	// Create the mount point if it does not exist
	if err := os.MkdirAll(c.MountPoint, 0755); err != nil {
		return fmt.Errorf("unable to create mount point: %v", err)
	}

	source := remoteSource(c)
	var cmd *exec.Cmd

	switch c.Protocol {
	case ProtocolWebDAV:
		opts := []string{}
		if c.Username != "" {
			opts = append(opts, "username="+c.Username)
		}
		if c.Options != "" {
			opts = append(opts, c.Options)
		}
		args := []string{"mount", "-t", "davfs", source, c.MountPoint}
		if len(opts) > 0 {
			args = append(args, "-o", strings.Join(opts, ","))
		}
		cmd = exec.Command("sudo", args...)
		// mount.davfs asks for the password interactively when a username
		// is given via option; feed it through stdin
		if c.Username != "" {
			cmd.Stdin = strings.NewReader(c.Password + "\n")
		} else {
			cmd.Stdin = strings.NewReader("\n\n")
		}

	case ProtocolFTP:
		opts := []string{}
		if c.Username != "" {
			// curlftpfs reads credentials from the user option
			opts = append(opts, "user="+c.Username+":"+c.Password)
		}
		if c.Options != "" {
			opts = append(opts, c.Options)
		}
		args := []string{"curlftpfs", source, c.MountPoint}
		if len(opts) > 0 {
			args = append(args, "-o", strings.Join(opts, ","))
		}
		cmd = exec.Command("sudo", args...)

	case ProtocolSMB:
		opts := []string{}
		if c.Username != "" {
			opts = append(opts, "username="+c.Username)
		} else {
			opts = append(opts, "guest")
		}
		if c.Port != 0 {
			opts = append(opts, fmt.Sprintf("port=%d", c.Port))
		}
		if c.Options != "" {
			opts = append(opts, c.Options)
		}
		cmd = exec.Command("sudo", "--preserve-env=PASSWD", "mount", "-t", "cifs", source, c.MountPoint, "-o", strings.Join(opts, ","))
		// Pass the password via the PASSWD env var so it never shows up
		// in the process list
		if c.Password != "" {
			cmd.Env = append(os.Environ(), "PASSWD="+c.Password)
		}

	case ProtocolNFS:
		opts := []string{}
		if c.Port != 0 {
			opts = append(opts, fmt.Sprintf("port=%d", c.Port))
		}
		if c.Options != "" {
			opts = append(opts, c.Options)
		}
		args := []string{"mount", "-t", "nfs", source, c.MountPoint}
		if len(opts) > 0 {
			args = append(args, "-o", strings.Join(opts, ","))
		}
		cmd = exec.Command("sudo", args...)

	default:
		return errors.New("unsupported protocol")
	}

	output, err := utils.RunAndStream(cmd)
	if err != nil {
		msg := strings.TrimSpace(string(output))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("mount failed: %s", msg)
	}
	return nil
}

// Unmount unmounts the given connection from its mount point.
func (m *Manager) Unmount(c *Connection) error {
	mounted, err := m.IsMounted(c)
	if err != nil {
		return err
	}
	if !mounted {
		return errors.New("not mounted")
	}

	cmd := exec.Command("sudo", "umount", c.MountPoint)
	output, err := utils.RunAndStream(cmd)
	if err != nil {
		// Fall back to lazy unmount so a hanging remote server cannot
		// leave the mount point stuck forever
		cmd = exec.Command("sudo", "umount", "-l", c.MountPoint)
		lazyOutput, lazyErr := utils.RunAndStream(cmd)
		if lazyErr != nil {
			msg := strings.TrimSpace(string(output) + " " + string(lazyOutput))
			return fmt.Errorf("unmount failed: %s", msg)
		}
	}
	return nil
}

// IsMounted checks /proc/mounts for the connection's mount point.
func (m *Manager) IsMounted(c *Connection) (bool, error) {
	mounts, err := listProcMounts()
	if err != nil {
		return false, err
	}
	target := filepath.Clean(c.MountPoint)
	for _, mount := range mounts {
		if filepath.Clean(mount.MountPoint) == target {
			return true, nil
		}
	}
	return false, nil
}

// GetStatus returns the connection with live mount state and usage info.
func (m *Manager) GetStatus(c *Connection) *ConnectionStatus {
	status := &ConnectionStatus{
		Connection: *c,
		SizeTotal:  -1,
		SizeUsed:   -1,
		SizeFree:   -1,
	}
	//Never leak the credentials through the API
	status.Password = ""

	mounted, err := m.IsMounted(c)
	if err != nil {
		return status
	}
	status.IsMounted = mounted

	// FTP (curlftpfs) reports fake volume numbers, skip usage reporting
	if mounted && c.Protocol != ProtocolFTP {
		if total, used, free, err := getUsageByMountPoint(c.MountPoint); err == nil {
			status.SizeTotal = total
			status.SizeUsed = used
			status.SizeFree = free
			status.HasUsage = true
		}
	}

	return status
}

// ListConnections returns all configured connections with their live status.
func (m *Manager) ListConnections() []*ConnectionStatus {
	m.mu.RLock()
	conns := make([]*Connection, len(m.connections))
	copy(conns, m.connections)
	m.mu.RUnlock()

	results := []*ConnectionStatus{}
	for _, c := range conns {
		results = append(results, m.GetStatus(c))
	}
	return results
}

// getUsageByMountPoint reports total / used / free bytes of the filesystem
// mounted at the given mount point. Network filesystems that support volume
// statistics (SMB, NFS, WebDAV) report their remote volume size here.
func getUsageByMountPoint(mountPoint string) (total int64, used int64, free int64, err error) {
	usages, err := df.GetDiskUsage()
	if err != nil {
		return -1, -1, -1, err
	}
	target := filepath.Clean(mountPoint)
	for _, usage := range usages {
		if filepath.Clean(usage.MountedOn) == target {
			return usage.Blocks * 1024, usage.Used, usage.Available, nil
		}
	}
	return -1, -1, -1, errors.New("mount point not found in df output")
}

// procMountEntry is one line of /proc/mounts.
type procMountEntry struct {
	Device     string
	MountPoint string
	FsType     string
}

func listProcMounts() ([]procMountEntry, error) {
	file, err := os.Open("/proc/mounts")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	entries := []procMountEntry{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}
		entries = append(entries, procMountEntry{
			Device:     unescapeProcMount(fields[0]),
			MountPoint: unescapeProcMount(fields[1]),
			FsType:     fields[2],
		})
	}
	return entries, scanner.Err()
}

// unescapeProcMount decodes the octal escapes (\040 etc.) used in /proc/mounts
func unescapeProcMount(s string) string {
	replacer := strings.NewReplacer("\\040", " ", "\\011", "\t", "\\012", "\n", "\\134", "\\")
	return replacer.Replace(s)
}

// ListSystemNetworkMounts lists network filesystems mounted on this host
// that are not managed by bokodm (e.g. fstab mounts).
func (m *Manager) ListSystemNetworkMounts() ([]SystemNetworkMount, error) {
	mounts, err := listProcMounts()
	if err != nil {
		return nil, err
	}

	managedMountPoints := map[string]bool{}
	m.mu.RLock()
	for _, c := range m.connections {
		managedMountPoints[filepath.Clean(c.MountPoint)] = true
	}
	m.mu.RUnlock()

	results := []SystemNetworkMount{}
	for _, mount := range mounts {
		if !networkFsTypes[mount.FsType] {
			continue
		}
		// Plain "fuse" fstype covers many non-network mounts, only accept
		// entries whose device looks like a remote source
		if mount.FsType == "fuse" && !strings.Contains(mount.Device, "://") && !strings.Contains(mount.Device, "@") {
			continue
		}
		if managedMountPoints[filepath.Clean(mount.MountPoint)] {
			continue
		}
		results = append(results, SystemNetworkMount{
			Device:     mount.Device,
			MountPoint: mount.MountPoint,
			FsType:     mount.FsType,
		})
	}
	return results, nil
}
