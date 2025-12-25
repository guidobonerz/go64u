package network

import (
	"fmt"
	"log"
	"time"

	"drazil.de/go64u/config"

	"github.com/jlaffaye/ftp"
)

func GetFtpConnection(deviceName string) *ftp.ServerConn {
	device := config.GetConfig().Devices[deviceName]
	ftpConnection := device.FtpConnection
	if ftpConnection == nil {
		var err error
		ftpConnection, err := ftp.Dial(fmt.Sprintf("%s:21", device.IpAddress), ftp.DialWithTimeout(5*time.Second))
		device.FtpConnection = ftpConnection
		if err != nil {
			log.Fatal(err)
		}
		err = ftpConnection.Login("anonymous", "anonymous")
		if err != nil {
			log.Fatal(err)
		}
	}
	return ftpConnection
}
