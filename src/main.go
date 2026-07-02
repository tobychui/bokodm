package main

import (
	"embed"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

//go:embed web/*
var embeddedFiles embed.FS

func main() {
	flag.Parse()

	// Start the application
	if err := initialization(); err != nil {
		log.Fatalf("Initialization failed: %v", err)
	}

	// Capture termination signals and call cleanup
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signalChan
		fmt.Println("Received termination signal, cleaning up...")
		cleanup()
		os.Exit(0)
	}()

	/* Static Web Server — serves the disk manager UI */
	http.Handle("/", csrfMiddleware(tmplMiddleware(http.FileServer(webfs))))

	/* REST API Handlers */
	http.Handle("/api/", csrfMiddleware(HandlerAPIcalls()))

	addr := fmt.Sprintf(":%d", *httpPort)
	fmt.Printf("Starting bokodm on %s\n", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting server: %v\n", err)
		os.Exit(1)
	}
}
