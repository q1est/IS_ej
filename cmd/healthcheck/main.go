package main

import (
	"net/http"
	"os"
	"time"
)

func main() {
	url := os.Getenv("HEALTHCHECK_URL")
	if url == "" {
		port := os.Getenv("PORT")
		if port == "" {
			port = "8080"
		}
		url = "http://127.0.0.1:" + port + "/healthz"
	}
	client := http.Client{Timeout: 3 * time.Second}
	response, err := client.Get(url)
	if err != nil {
		os.Exit(1)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		os.Exit(1)
	}
}
