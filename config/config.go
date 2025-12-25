package config

import (
	"log"
	"os"

	"github.com/jlaffaye/ftp"
	"gopkg.in/yaml.v3"
)

var config Config

type Config struct {
	Password         string             `yaml:"Password"`
	Devices          map[string]*Device `yaml:"Devices"`
	ScreenshotFolder string             `yaml:"ScreenshotFolder"`
	DumpFolder       string             `yaml:"DumpFolder"`
	SelectedDevice   string             `yaml:"SelectedDevice"`
}

type Device struct {
	Description   string          `yaml:"Description"`
	IsDefault     bool            `yaml:"IsDefault"`
	IpAddress     string          `yaml:"IpAddress"`
	AudioPort     int             `yaml:"AudioPort"`
	VideoPort     int             `yaml:"VideoPort"`
	DebugPort     int             `yaml:"DebugPort"`
	FtpConnection *ftp.ServerConn `yaml:"-"`
}

func ReadConfig() {
	configFile := os.Getenv("GO64U_CONFIG")
	if configFile != "" {
		data, err := os.ReadFile(configFile)
		if err != nil {
			log.Fatalf("Error reading config file: %v", err)
		}

		err = yaml.Unmarshal(data, &config)
		if err != nil {
			log.Fatalf("Error parsing config file: %v", err)
		}
	} else {
		panic("Environment variable GO64U_CONFIG not set.")
	}
}

func GetConfig() *Config {
	return &config
}
