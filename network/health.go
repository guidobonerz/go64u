package network

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"drazil.de/go64u/config"
)

func IsDeviceOnline(device *config.Device, timeout time.Duration) bool {
	if device == nil || device.IpAddress == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	url := fmt.Sprintf("http://%s/v1/version", device.IpAddress)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if config.GetConfig().Password != "" {
		req.Header.Set("X-password", config.GetConfig().Password)
	}
	if err != nil {
		return false
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}
