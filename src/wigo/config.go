package wigo

import (
	"github.com/BurntSushi/toml"
	"log"
)


type Config struct {

	ListenPort		int
	ListenAddress	string

	HostsToCheck 	[]string

}

func NewConfig( path string ) ( this *Config){

	// Default conf
	this = new(Config)
	this.ListenPort 		= 4000
	this.ListenAddress	= "0.0.0.0"
	this.HostsToCheck		= nil

	// Override with config file
	if _, err := toml.DecodeFile(path, &this); err != nil {
		log.Printf("Failed to load configuration file %s : %s\n", path, err)
	}

	return
}
