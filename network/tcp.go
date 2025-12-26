package network

import (
	"fmt"
	"net"
)

func SendTcpData(payload []byte, target string) {
	conn, err := net.Dial("tcp", fmt.Sprintf("%s:64", target))
	if err != nil {
		panic(err)
	}
	defer conn.Close()
	conn.Write([]byte(payload))
}
