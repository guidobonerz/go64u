package network

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"

	"drazil.de/go64u/config"
)

func Execute(action string, method string, data []byte) []byte {
	client := &http.Client{}
	url := getUrl(action)

	req, err := http.NewRequest(method, url, bytes.NewBuffer(data))
	req.Header.Set("Client-Id", "Ultimate")
	if config.GetConfig().Password != "" {
		req.Header.Set("X-password", config.GetConfig().Password)
	}
	if err != nil {
		log.Fatal(err)
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)

	if err != nil {
		log.Fatal(err)
	}

	return body
}

func Get(url string) []byte {
	client := &http.Client{}

	req, err := http.NewRequest("GET", url, nil)
	req.Header.Set("Client-Id", "Ultimate")

	if err != nil {
		log.Fatal(err)
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)

	if err != nil {
		log.Fatal(err)
	}

	return body
}

func getUrl(action string) string {
	return fmt.Sprintf("http://%s/v1/%s", config.GetConfig().Devices[config.GetConfig().SelectedDevice].IpAddress, action)
}
