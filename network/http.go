package network

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"

	"drazil.de/go64u/config"
)

type HttpConfig struct {
	URL         string
	Method      string
	Payload     []byte
	SetClientId bool
}

func SendHttpRequest(httpConfig *HttpConfig) []byte {
	client := &http.Client{}

	req, err := http.NewRequest(httpConfig.Method, httpConfig.URL, bytes.NewBuffer(httpConfig.Payload))

	if httpConfig.SetClientId {
		req.Header.Set("Client-Id", config.GetConfig().DatabaseClient)
		req.Header.Set("User-Agent", "Assembly Query")
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

func GetUrl(action string) string {
	return fmt.Sprintf("http://%s/v1/%s", config.GetConfig().Devices[config.GetConfig().SelectedDevice].IpAddress, action)
}
