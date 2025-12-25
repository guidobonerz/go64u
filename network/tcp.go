package network

import (
	"fmt"
	"net"

	"drazil.de/go64u/config"
)

func SendTcpData(payload []byte) {
	conn, err := net.Dial("tcp", fmt.Sprintf("%s:64", config.GetConfig().Devices[config.GetConfig().SelectedDevice].IpAddress))
	if err != nil {
		panic(err)
	}
	defer conn.Close()
	conn.Write([]byte(payload))
}
