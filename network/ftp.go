package network

import (
	"fmt"
	"log"
	"time"

	"drazil.de/go64u/config"

	"github.com/jlaffaye/ftp"
)

var ftpConnection *ftp.ServerConn

func GetFtpConnection() *ftp.ServerConn {
	if ftpConnection == nil {
		var err error
		ftpConnection, err = ftp.Dial(fmt.Sprintf("%s:21", config.GetConfig().IpAddress), ftp.DialWithTimeout(5*time.Second))
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
