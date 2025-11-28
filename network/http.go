package network

import (
	"fmt"
	"io"
	"log"
	"net/http"

	"de/drazil/go64u/helper"
)

func Put(command string) {
	client := &http.Client{}
	url := getUrl(command)

	req, err := http.NewRequest(http.MethodPut, url, nil)
	req.Header.Set("Content-Type", "application/json")
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
	log.Println(string(body))
}
func Post(command string) {
	client := &http.Client{}
	url := getUrl(command)

	req, err := http.NewRequest(http.MethodPost, url, nil)
	req.Header.Set("Content-Type", "application/json")
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
	log.Println(string(body))
}

func getUrl(command string) string {
	return fmt.Sprintf("http://%s/v1/machine:%s", helper.GetConfig().IpAddress, command)
}
