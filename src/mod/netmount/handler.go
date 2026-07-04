package netmount

/*
	handler.go

	HTTP API handlers for the network mount module. Routed from
	/api/netmount/* in the main package.
*/

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"

	"imuslab.com/bokodm/bokodmd/mod/utils"
)

// HandleListTools returns the availability of every protocol mount helper
// so the frontend can warn about missing packages instead of failing.
func HandleListTools(w http.ResponseWriter, r *http.Request) {
	js, _ := json.Marshal(ListTools())
	utils.SendJSONResponse(w, string(js))
}

// HandleListConnections returns all configured connections with live status.
func (m *Manager) HandleListConnections(w http.ResponseWriter, r *http.Request) {
	js, _ := json.Marshal(m.ListConnections())
	utils.SendJSONResponse(w, string(js))
}

// HandleListSystemMounts returns network filesystems mounted outside bokodm.
func (m *Manager) HandleListSystemMounts(w http.ResponseWriter, r *http.Request) {
	mounts, err := m.ListSystemNetworkMounts()
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}
	js, _ := json.Marshal(mounts)
	utils.SendJSONResponse(w, string(js))
}

// parseConnectionForm reads the connection fields shared by the add and
// edit APIs from the POST form.
func parseConnectionForm(r *http.Request) (*Connection, error) {
	name, err := utils.PostPara(r, "name")
	if err != nil {
		return nil, errors.New("connection name not given")
	}

	protocol, err := utils.PostPara(r, "protocol")
	if err != nil {
		return nil, errors.New("protocol not given")
	}

	host, err := utils.PostPara(r, "host")
	if err != nil {
		return nil, errors.New("remote host not given")
	}

	mountPoint, err := utils.PostPara(r, "mountPoint")
	if err != nil {
		return nil, errors.New("mount point not given")
	}

	// Optional fields
	remotePath, _ := utils.PostPara(r, "path")
	username, _ := utils.PostPara(r, "username")
	password, _ := utils.PostPara(r, "password")
	options, _ := utils.PostPara(r, "options")
	portStr, _ := utils.PostPara(r, "port")
	port := 0
	if portStr != "" {
		port, err = strconv.Atoi(portStr)
		if err != nil || port < 0 || port > 65535 {
			return nil, errors.New("invalid port number")
		}
	}
	autoMount, err := utils.PostBool(r, "automount")
	if err != nil {
		autoMount = false
	}

	return &Connection{
		Name:       name,
		Protocol:   Protocol(protocol),
		Host:       host,
		Port:       port,
		Path:       remotePath,
		Username:   username,
		Password:   password,
		MountPoint: mountPoint,
		Options:    options,
		AutoMount:  autoMount,
	}, nil
}

// HandleAddConnection creates a new connection and mounts it immediately.
func (m *Manager) HandleAddConnection(w http.ResponseWriter, r *http.Request) {
	thisConnection, err := parseConnectionForm(r)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	if !ToolAvailable(thisConnection.Protocol) {
		tool := protocolTools[thisConnection.Protocol]
		utils.SendErrorResponse(w, "mount tool "+tool.binary+" not installed. Install package "+tool.pkg+" first")
		return
	}

	if err := m.AddConnection(thisConnection); err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	// Mount it right away; keep the connection stored even when the first
	// mount fails so the user can fix the settings / network and retry
	if err := m.Mount(thisConnection); err != nil {
		log.Println("[NetMount] Mount failed:", err)
		utils.SendErrorResponse(w, "connection saved but mount failed: "+err.Error())
		return
	}

	log.Println("[NetMount] Mounted " + string(thisConnection.Protocol) + " filesystem " + thisConnection.Name + " on " + thisConnection.MountPoint)
	utils.SendOK(w)
}

// HandleEditConnection updates a stored connection, require "uuid" plus the
// same fields as the add API. Empty password keeps the stored one. The
// connection must be unmounted before editing.
func (m *Manager) HandleEditConnection(w http.ResponseWriter, r *http.Request) {
	uuid, err := utils.PostPara(r, "uuid")
	if err != nil {
		utils.SendErrorResponse(w, "connection uuid not given")
		return
	}

	updated, err := parseConnectionForm(r)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	if err := m.UpdateConnection(uuid, updated); err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	log.Println("[NetMount] Connection " + updated.Name + " (" + uuid + ") updated")
	utils.SendOK(w)
}

// HandleSetAutoMount toggles the automount flag of a stored connection,
// require "uuid" and "automount" (bool). Works while mounted.
func (m *Manager) HandleSetAutoMount(w http.ResponseWriter, r *http.Request) {
	uuid, err := utils.PostPara(r, "uuid")
	if err != nil {
		utils.SendErrorResponse(w, "connection uuid not given")
		return
	}
	autoMount, err := utils.PostBool(r, "automount")
	if err != nil {
		utils.SendErrorResponse(w, "automount flag not given")
		return
	}

	c, err := m.GetConnection(uuid)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	m.mu.Lock()
	c.AutoMount = autoMount
	err = m.save()
	m.mu.Unlock()
	if err != nil {
		utils.SendErrorResponse(w, "unable to save: "+err.Error())
		return
	}
	utils.SendOK(w)
}

// HandleMountConnection mounts a stored connection, require "uuid"
func (m *Manager) HandleMountConnection(w http.ResponseWriter, r *http.Request) {
	uuid, err := utils.PostPara(r, "uuid")
	if err != nil {
		utils.SendErrorResponse(w, "connection uuid not given")
		return
	}

	c, err := m.GetConnection(uuid)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	if err := m.Mount(c); err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}
	utils.SendOK(w)
}

// HandleUnmountConnection unmounts a stored connection, require "uuid"
func (m *Manager) HandleUnmountConnection(w http.ResponseWriter, r *http.Request) {
	uuid, err := utils.PostPara(r, "uuid")
	if err != nil {
		utils.SendErrorResponse(w, "connection uuid not given")
		return
	}

	c, err := m.GetConnection(uuid)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	if err := m.Unmount(c); err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}
	utils.SendOK(w)
}

// HandleRemoveConnection unmounts and deletes a stored connection, require "uuid"
func (m *Manager) HandleRemoveConnection(w http.ResponseWriter, r *http.Request) {
	uuid, err := utils.PostPara(r, "uuid")
	if err != nil {
		utils.SendErrorResponse(w, "connection uuid not given")
		return
	}

	if err := m.RemoveConnection(uuid); err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}
	utils.SendOK(w)
}

// HandleConnectionStatus returns the live status of one connection, require "uuid"
func (m *Manager) HandleConnectionStatus(w http.ResponseWriter, r *http.Request) {
	uuid, err := utils.GetPara(r, "uuid")
	if err != nil {
		utils.SendErrorResponse(w, "connection uuid not given")
		return
	}

	c, err := m.GetConnection(uuid)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	js, _ := json.Marshal(m.GetStatus(c))
	utils.SendJSONResponse(w, string(js))
}
