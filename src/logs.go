package main

import (
	"encoding/json"
	"net/http"
	"strings"

	"imuslab.com/bokodm/bokodmd/mod/logger"
	"imuslab.com/bokodm/bokodmd/mod/utils"
)

/*
	logs.go

	This file handles the Logs tab API routing. Log entries are produced
	by mod/logger (system tool output, scheduled SMART checks, webhook
	notifications, system events) and grouped into monthly files.

	Support APIs

	/logs/list     - GET  list all log files with size and mtime
	/logs/view     - GET  file=<name>, returns the (tail-capped) content
	/logs/download - GET  file=<name>, returns the raw file as download
	/logs/delete   - POST file=<name>, deletes an archived log file
*/

// Cap the viewer payload so a huge log file cannot freeze the browser.
const logViewMaxBytes = 2 * 1024 * 1024

func HandleLogsCalls() http.Handler {
	return http.StripPrefix("/logs/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pathParts := strings.Split(r.URL.Path, "/")
		switch pathParts[0] {
		case "list":
			files, err := logger.ListLogFiles()
			if err != nil {
				utils.SendErrorResponse(w, err.Error())
				return
			}
			js, _ := json.Marshal(files)
			utils.SendJSONResponse(w, string(js))

		case "view":
			file, err := utils.GetPara(r, "file")
			if err != nil {
				utils.SendErrorResponse(w, "log file not given")
				return
			}
			content, truncated, err := logger.ReadLogFile(file, logViewMaxBytes)
			if err != nil {
				utils.SendErrorResponse(w, err.Error())
				return
			}
			js, _ := json.Marshal(map[string]interface{}{
				"content":   content,
				"truncated": truncated,
			})
			utils.SendJSONResponse(w, string(js))

		case "download":
			file, err := utils.GetPara(r, "file")
			if err != nil {
				utils.SendErrorResponse(w, "log file not given")
				return
			}
			path, err := logger.LogFilePath(file)
			if err != nil {
				utils.SendErrorResponse(w, err.Error())
				return
			}
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Header().Set("Content-Disposition", "attachment; filename=\""+file+"\"")
			http.ServeFile(w, r, path)

		case "delete":
			file, err := utils.PostPara(r, "file")
			if err != nil {
				utils.SendErrorResponse(w, "log file not given")
				return
			}
			if err := logger.DeleteLogFile(file); err != nil {
				utils.SendErrorResponse(w, err.Error())
				return
			}
			utils.SendOK(w)

		default:
			http.Error(w, "Not Found", http.StatusNotFound)
		}
	}))
}
