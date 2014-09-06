package wigo

import (
	"github.com/BurntSushi/toml"
	"log"
)


type Config struct {

	ListenPort		int
	ListenAddress	string

	ProbesDirectory	string
	LogFile			string
	ConfigFile		string

	HostsToCheck 	[]string

	CallbackUrl		string


	// Remmote wigos params
	RemoteWigosCheckInterval	int
	RemoteWigosCheckTries		int


	// Noticications
	NotificationsHttpEnabled		bool
	NotificationsHttpUrl			string

	NotificationsEmailEnabled		bool
	NotificationsEmailSmtpServer	string
	NotificationsEmailRecipients	[]string
	NotificationsEmailFromName		string
	NotificationsEmailFromAddress 	string

}

func NewConfig() ( this *Config){

	// Default conf
	this = new(Config)
	this.ListenPort 					= 4000
	this.ListenAddress					= "0.0.0.0"

	this.ProbesDirectory				= "/usr/local/wigo/probes"
	this.LogFile						= "/var/log/wigo.log"
	this.ConfigFile						= "/etc/wigo.conf"

	this.HostsToCheck					= nil
	this.CallbackUrl					= ""

	// Remote Wigos
	this.RemoteWigosCheckInterval 		= 10
	this.RemoteWigosCheckTries	  		= 3


	// Notifications
	this.NotificationsHttpEnabled		= false
	this.NotificationsHttpUrl			= ""
	this.NotificationsEmailEnabled		= false
	this.NotificationsEmailSmtpServer	= ""
	this.NotificationsEmailFromAddress 	= ""
	this.NotificationsEmailFromName		= ""
	this.NotificationsEmailRecipients	= nil


	// Override with config file
	if _, err := toml.DecodeFile(this.ConfigFile, &this); err != nil {
		log.Printf("Failed to load configuration file %s : %s\n", this.ConfigFile, err)
	}

	return
}
