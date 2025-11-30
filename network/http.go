package network

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"

	"de/drazil/go64u/helper"
)

func Execute(action string, method string, data []byte) []byte {
	client := &http.Client{}
	url := getUrl(action)
	log.Println(url)
	req, err := http.NewRequest(method, url, bytes.NewBuffer(data))
	//req.Header.Set("Content-Type", "application/json")
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
	return fmt.Sprintf("http://%s/v1/%s", helper.GetConfig().IpAddress, action)
}
