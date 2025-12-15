package config

import (
	"log"
	"os"

	"gopkg.in/yaml.v3"
)

var config Config

type Config struct {
	IpAddress        string `yaml:"ipaddress"`
	Password         string `yaml:"password"`
	Stream           Stream `yaml:"stream"`
	ScreenshotFolder string `yaml:"screenshotFolder"`
	DumpFolder       string `yaml:"dumpFolder"`
}

type Stream struct {
	Audio Audio `yaml:"audio"`
	Video Video `yaml:"video"`
	Debug Debug `yaml:"debug"`
}

type Audio struct {
	Port int `yaml:"port"`
}
type Video struct {
	Port int `yaml:"port"`
}
type Debug struct {
	Port int `yaml:"port"`
}

func ReadConfig() {
	data, err := os.ReadFile(os.Getenv("GO64U_CONFIG"))
	if err != nil {
		log.Fatalf("Error reading config file: %v", err)
	}

	err = yaml.Unmarshal(data, &config)
	if err != nil {
		log.Fatalf("Error parsing config file: %v", err)
	}
}

func GetConfig() *Config {
	return &config
}
