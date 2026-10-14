package main

import (
	"flag"
	"net/http"
	"os"
	"time"
)

func main() {
	url := flag.String("url", "http://127.0.0.1:8080/readyz", "URL que se comprobara")
	flag.Parse()

	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Get(*url)
	if err != nil {
		os.Exit(1)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		os.Exit(1)
	}
}
