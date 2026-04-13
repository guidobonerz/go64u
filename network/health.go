package network

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"drazil.de/go64u/config"
)

// IsDeviceOnline returns true if the device responds to a lightweight HTTP
// GET on /v1/version within the given timeout. Uses its own HTTP client so
// it does not interfere with the long-running connections in http.go and is
// silent on failures (suitable for GUI polling).
func IsDeviceOnline(device *config.Device, timeout time.Duration) bool {
	if device == nil || device.IpAddress == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	url := fmt.Sprintf("http://%s/v1/version", device.IpAddress)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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
