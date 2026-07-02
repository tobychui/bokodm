package main

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"runtime"
	"strings"

	"github.com/google/uuid"
	"github.com/gorilla/csrf"
	"imuslab.com/bokofs/bokofsd/mod/disktool/raid"
	"imuslab.com/bokofs/bokofsd/mod/netstat"
)

/*
	start.go

	This file handles the startup and initialization of the application
*/

func initialization() error {
	/* Check and generate system UUID */
	configFolderPath := "./config"
	if _, err := os.Stat(configFolderPath); os.IsNotExist(err) {
		fmt.Printf("Config folder does not exist. Creating folder at %s\n", configFolderPath)
		if err := os.Mkdir(configFolderPath, os.ModePerm); err != nil {
			return fmt.Errorf("error creating config folder: %v", err)
		}

	}

	// Check if sys.uuid exists, if not generate a unique UUID and write it to sys.uuid
	uuidFilePath := configFolderPath + "/sys.uuid"
	if _, err := os.Stat(uuidFilePath); os.IsNotExist(err) {
		newUUID := uuid.New().String()
		if err := os.WriteFile(uuidFilePath, []byte(newUUID), 0644); err != nil {
			return fmt.Errorf("error writing UUID to file: %v", err)
		}
	}

	// Read the UUID from sys.uuid
	uuidBytes, err := os.ReadFile(uuidFilePath)
	if err != nil {
		return fmt.Errorf("error reading UUID from file: %v", err)
	}
	sysuuid = string(uuidBytes)

	/* File system handler */
	if *devMode {
		fmt.Println("Development mode enabled. Serving files from ./web directory.")
		webfs = http.Dir("./web")
	} else {
		fmt.Println("Production mode enabled. Serving files from embedded filesystem.")
		subFS, err := fs.Sub(embeddedFiles, "web")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error accessing embedded subdirectory: %v\n", err)
			os.Exit(1)
		}
		webfs = http.FS(subFS)
	}

	/* Network statistics */
	nsb, err := netstat.NewNetStatBuffer(300)
	if err != nil {
		return fmt.Errorf("error creating netstat buffer: %v", err)
	}
	netstatBuffer = nsb

	/* Package / dependency check */
	report := checkRuntimeEnvironment()
	if !report.AllSatisfied {
		if *skipDep {
			report.DegradedMode = true
			log.Printf("\033[33m⚠\033[0m  Starting in degraded mode (-skip_dep): one or more required dependencies are missing")
		} else {
			return fmt.Errorf("runtime environment check failed: required dependencies are missing (use -skip_dep to start anyway in degraded mode)")
		}
	}
	runtimeDeps = report

	/* RAID Manager — Linux only (mdadm required) */
	rm, err := raid.NewRaidManager()
	if err != nil {
		if runtime.GOOS == "linux" && !*skipDep {
			return fmt.Errorf("RAID manager unavailable: %v", err)
		}
		log.Printf("RAID management not available on %s: %v", runtime.GOOS, err)
	} else {
		raidManager = rm
	}

	/* CSRF Middleware */
	csrfMiddleware = csrf.Protect(
		[]byte(sysuuid),
		csrf.CookieName(CSRF_COOKIENAME),
		csrf.Secure(false),
		csrf.Path("/"),
		csrf.SameSite(csrf.SameSiteLaxMode),
	)

	return nil
}

// tmplateMiddleware is a middleware that serves HTML files and injects the CSRF token
func tmplMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		csrfToken := csrf.Token(r)

		// Check if the request is for the root or an HTML page
		isHTML := len(r.URL.Path) >= 5 && r.URL.Path[len(r.URL.Path)-5:] == ".html"
		if r.URL.Path == "/" || isHTML {
			file, err := webfs.Open(r.URL.Path)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			defer file.Close()

			// Check if the file is a directory
			fileInfo, err := file.Stat()
			if err != nil {
				log.Println(err)
				http.Error(w, "Error retrieving file information", http.StatusInternalServerError)
				return
			}

			if fileInfo.IsDir() {
				// Build the index path without a double slash when r.URL.Path is "/".
				// Trimming the trailing slash gives "" for "/" and "/foo" for "/foo/",
				// so the result is always "/index.html" or "/foo/index.html".
				base := strings.TrimRight(r.URL.Path, "/")
				indexFile, err := webfs.Open(base + "/index.html")
				if err != nil {
					http.NotFound(w, r)
					return
				}
				defer indexFile.Close()
				file = indexFile
			}

			// Replace {{.csrfToken}} in the HTML file with the CSRF token
			content, err := io.ReadAll(file)
			if err != nil {
				log.Println(err)
				http.Error(w, "Error reading file content", http.StatusInternalServerError)
				return
			}

			// Replace {{.csrfToken}} with the actual CSRF token
			modifiedContent := bytes.Replace(content, []byte("{{.csrfToken}}"), []byte(csrfToken), -1)

			// Write the modified content to the response
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			w.Header().Set("Pragma", "no-cache")
			w.Header().Set("Expires", "0")
			w.WriteHeader(http.StatusOK)
			w.Write(modifiedContent)
			return
		}

		next.ServeHTTP(w, r)

		// Add template engine initialization here if needed

	})
}

// Cleanup function to be called on exit
func cleanup() {
	fmt.Println("Performing cleanup tasks...")
	// Close the netstat buffer if it was initialized
	if netstatBuffer != nil {
		fmt.Println("Closing netstat buffer...")
		netstatBuffer.Close()
	}

	fmt.Println("Cleanup completed.")
}
