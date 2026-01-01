package config

import (
	"fmt"
	"log"
	"net"
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
	RecordingFolder  string             `yaml:"RecordingFolder"`
	SelectedDevice   string             `yaml:"SelectedDevice"`
	StreamingTargets map[string]string  `yaml:"StreamingTargets"`
	LogLevel         string             `yaml:"LogLevel"`
}

type Device struct {
	Description        string          `yaml:"Description"`
	IsDefault          bool            `yaml:"IsDefault"`
	IpAddress          string          `yaml:"IpAddress"`
	AudioPort          int             `yaml:"AudioPort"`
	VideoPort          int             `yaml:"VideoPort"`
	DebugPort          int             `yaml:"DebugPort"`
	FtpConnection      *ftp.ServerConn `yaml:"-"`
	AudioUdpConnection *net.UDPConn    `yaml:"-"`
	VideoUdpConnection *net.UDPConn    `yaml:"-"`
	AudioChannel       chan struct{}   `yaml:"-"`
}

type StreamingPlatform struct {
	RtmpUrl string `yaml:"RtmpUrl"`
}

func ReadConfig() {
	configFileName := ".go64u.yaml"
	data, err := os.ReadFile(fmt.Sprintf(".\\%s", configFileName))
	if err != nil {
		//log.Println("No config file found in application folder.")
	}
	if data == nil {
		configFile := fmt.Sprintf("%s%s", os.Getenv("GO64U_CONFIG_PATH"), configFileName)
		if configFile != "" {
			data, err = os.ReadFile(configFile)
			if err != nil {
				//log.Println("No config file found in application folder.")
			}
		} else {
			log.Fatal("Environment variable GO64U_CONFIG_PATH not set.")
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
