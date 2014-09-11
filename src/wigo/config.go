package wigo

import (
	"github.com/BurntSushi/toml"
	"log"
)


type Config struct {

	// General params
	ListenPort						int
	ListenAddress					string
	ProbesDirectory					string
	LogFile							string
	ConfigFile						string


	// Remmote wigos params
	RemoteWigosList					[]string
	RemoteWigosCheckInterval		int
	RemoteWigosCheckTries			int


	// Noticications
	NotificationsOnWigoChange		bool
	NotificationsOnHostChange		bool
	NotificationsOnProbeChange		bool

	NotificationsHttpEnabled		bool
	NotificationsHttpUrl			string

	NotificationsEmailEnabled		bool
	NotificationsEmailSmtpServer	string
	NotificationsEmailRecipients	[]string
	NotificationsEmailFromName		string
	NotificationsEmailFromAddress 	string


	// OpenTSDB
	OpenTSDBAddress					string
	OpenTSDBPort					int
}

func NewConfig() ( this *Config){

	// General params
	this = new(Config)
	this.ListenPort 					= 4000
	this.ListenAddress					= "0.0.0.0"
	this.ProbesDirectory				= "/usr/local/wigo/probes"
	this.LogFile						= "/var/log/wigo.log"
	this.ConfigFile						= "/etc/wigo.conf"


	// Remote Wigos
	this.RemoteWigosList				= nil
	this.RemoteWigosCheckInterval 		= 10
	this.RemoteWigosCheckTries	  		= 3


	// Notifications
	this.NotificationsOnWigoChange		= false
	this.NotificationsOnHostChange		= false
	this.NotificationsOnProbeChange		= false

	this.NotificationsHttpEnabled		= false
	this.NotificationsHttpUrl			= ""

	this.NotificationsEmailEnabled		= false
	this.NotificationsEmailSmtpServer	= ""
	this.NotificationsEmailFromAddress 	= ""
	this.NotificationsEmailFromName		= ""
	this.NotificationsEmailRecipients	= nil

	// OpenTSDB
	this.OpenTSDBAddress				= ""
	this.OpenTSDBPort					= 0


	// Override with config file
	if _, err := toml.DecodeFile(this.ConfigFile, &this); err != nil {
		log.Printf("Failed to load configuration file %s : %s\n", this.ConfigFile, err)
	}

	return
}
