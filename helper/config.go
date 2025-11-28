package helper

import (
	"log"
	"os"

	"gopkg.in/yaml.v3"
)

var config Config

type Config struct {
	IpAddress  string `yaml:"ipaddress"`
	Stream     Stream `yaml:"stream"`
	Screenshot Folder `yaml:"screenshot"`
	Dump       Folder `yaml:"dump"`
}

type Stream struct {
	Audio Audio `yaml:"audio"`
	Video Video `yaml:"video"`
	Debug Debug `yaml:"debug"`
}

type Screenshot struct {
	Folder string `yaml:"folder"`
}

type Dump struct {
	Folder string `yaml:"folder"`
}

type Folder struct {
	Folder string `yaml:"folder"`
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
	data, err := os.ReadFile(".go64u.yaml")
	if err != nil {
		log.Fatalf("Fehler beim Lesen der Datei: %v", err)
	}

	err = yaml.Unmarshal(data, &config)
	if err != nil {
		log.Fatalf("Fehler beim Parsen des YAML: %v", err)
	}
}

func GetConfig() *Config {
	return &config
}
