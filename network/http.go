package network

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"drazil.de/go64u/config"
)

type HttpConfig struct {
	URL         string
	Method      string
	Payload     []byte
	SetClientId bool
}

func SendHttpRequest(httpConfig *HttpConfig) []byte {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}
	var request *http.Request
	var err error
	request, err = http.NewRequest(httpConfig.Method, httpConfig.URL, bytes.NewBuffer(httpConfig.Payload))

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	request = request.WithContext(ctx)

	if httpConfig.SetClientId {
		request.Header.Set("Client-Id", config.GetConfig().DatabaseClient)
		request.Header.Set("User-Agent", "Assembly Query")
	}
	if config.GetConfig().Password != "" {
		request.Header.Set("X-Password", config.GetConfig().Password)
	}

	resp, err := client.Do(request)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			fmt.Printf("\"%s\" seems not to be online", config.GetConfig().Devices[config.GetConfig().SelectedDevice].Description)
			return nil
		}

		fmt.Printf("Request failed: %v\n", err)
		return nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)

	if err != nil {
		log.Fatal(err)
	}

	return body
}

func GetUrl(action string) string {
	s := fmt.Sprintf("http://%s/v1/%s", config.GetConfig().Devices[config.GetConfig().SelectedDevice].IpAddress, action)
	fmt.Println(s)
	return s
}
