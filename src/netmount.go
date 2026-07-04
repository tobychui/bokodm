package main

import (
	"net/http"
	"strings"

	"imuslab.com/bokodm/bokodmd/mod/netmount"
)

/*
	netmount.go

	This file handles the network filesystem mount API routing

	Support APIs

	/netmount/tools   - List mount helper availability for each protocol
	/netmount/list    - List all configured connections with live status
	/netmount/system  - List network fs mounted outside of bokodm
	/netmount/add     - Create a new connection and mount it
	/netmount/edit    - Update a stored connection (must be unmounted)
	/netmount/mount   - Mount a stored connection
	/netmount/unmount - Unmount a stored connection
	/netmount/remove  - Unmount & delete a stored connection
	/netmount/status  - Get the live status of one connection
*/

func HandleNetMountCalls() http.Handler {
	return http.StripPrefix("/netmount/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pathParts := strings.Split(r.URL.Path, "/")

		// Tool listing works even when the manager failed to start so the
		// frontend can always render the install hints
		if pathParts[0] == "tools" {
			netmount.HandleListTools(w, r)
			return
		}

		if netmountManager == nil {
			http.Error(w, "Network mount management is not available", http.StatusServiceUnavailable)
			return
		}

		switch pathParts[0] {
		case "list":
			netmountManager.HandleListConnections(w, r)
		case "system":
			netmountManager.HandleListSystemMounts(w, r)
		case "add":
			netmountManager.HandleAddConnection(w, r)
		case "edit":
			netmountManager.HandleEditConnection(w, r)
		case "setautomount":
			netmountManager.HandleSetAutoMount(w, r)
		case "mount":
			netmountManager.HandleMountConnection(w, r)
		case "unmount":
			netmountManager.HandleUnmountConnection(w, r)
		case "remove":
			netmountManager.HandleRemoveConnection(w, r)
		case "status":
			netmountManager.HandleConnectionStatus(w, r)
		default:
			http.Error(w, "Not Found", http.StatusNotFound)
		}
	}))
}
