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
	TwitchStreamKey  string             `yaml:"TwitchStreamKey"`
}

type Device struct {
	Description   string          `yaml:"Description"`
	IsDefault     bool            `yaml:"IsDefault"`
	IpAddress     string          `yaml:"IpAddress"`
	AudioPort     int             `yaml:"AudioPort"`
	VideoPort     int             `yaml:"VideoPort"`
	DebugPort     int             `yaml:"DebugPort"`
	FtpConnection *ftp.ServerConn `yaml:"-"`
	AudioChannel  chan struct{}   `yaml:"-"`
}

func ReadConfig() {

	data, err := os.ReadFile(".\\.go64u.yaml")
	if err != nil {
		//log.Println("No config file found in application folder.")
	}
	if data == nil {
		configFile := os.Getenv("GO64U_CONFIG")
		if configFile != "" {
			data, err = os.ReadFile(configFile)
			if err != nil {
				//log.Println("No config file found in application folder.")
			}
		} else {
			log.Fatal("Environment variable GO64U_CONFIG not set.")
		}
	}
	if data != nil {
		err = yaml.Unmarshal(data, &config)
		for k, c := range config.Devices {
			if c.IsDefault {
				config.SelectedDevice = k
				break
			}
		}
		if err != nil {
			log.Fatalf("Error parsing config file: %v", err)
		}
	} else {
		log.Fatal("Unable to start application. Config not found.")
	}

}

func GetConfig() *Config {
	return &config
}
