package network

import (
	"de/drazil/go64u/config"
	"fmt"
	"net"
)

func SendTcpData(payload []byte) {
	conn, err := net.Dial("tcp", fmt.Sprintf("%s:64", config.GetConfig().IpAddress))
	if err != nil {
		panic(err)
	}
	defer conn.Close()
	conn.Write([]byte(payload))
}
