package wigo

import (
	"github.com/BurntSushi/toml"
	"log"
    "strconv"
    "strings"
)


type Config struct {

    // General params
    General                         *GeneralConfig

    // OpenTSDB params
    OpenTSDB                        *OpenTSDBConfig

    // Remmote wigos params
    RemoteWigos					    *RemoteWigoConfig
    AdvancedRemoteWigosList         []AdvancedRemoteWigoConfig

    // Noticications
    Notifications                   *NotificationConfig
}

func NewConfig() ( this *Config){

	// General params
    this                                                = new(Config)
    this.General                                        = new(GeneralConfig)
    this.OpenTSDB                                       = new(OpenTSDBConfig)
    this.RemoteWigos                                    = new(RemoteWigoConfig)
    this.Notifications                                  = new(NotificationConfig)

	this.General.ListenPort                             = 4000
	this.General.ListenAddress                          = "0.0.0.0"
	this.General.ProbesDirectory                        = "/usr/local/wigo/probes"
	this.General.LogFile                                = "/var/log/wigo.log"
	this.General.ConfigFile				                = "/etc/wigo/wigo.conf"

    // OpenTSDB
	this.OpenTSDB.Group					                = ""
	this.OpenTSDB.OpenTSDBEnabled				        = false
	this.OpenTSDB.OpenTSDBAddress				        = ""
	this.OpenTSDB.OpenTSDBPort					        = 0
	this.OpenTSDB.OpenTSDBMetricPrefix			        = "wigo"


	// Remote Wigos
	this.RemoteWigos.RemoteWigosList	                = nil
	this.RemoteWigos.RemoteWigosCheckInterval           = 10
	this.RemoteWigos.RemoteWigosCheckTries	            = 3
	this.AdvancedRemoteWigosList                        = nil


	// Notifications
	this.Notifications.MinLevelToSendNotifications      = 101

	this.Notifications.NotificationsOnWigoChange	    = false
	this.Notifications.NotificationsOnHostChange	    = false
	this.Notifications.NotificationsOnProbeChange	    = false

	this.Notifications.NotificationsHttpEnabled		    = false
	this.Notifications.NotificationsHttpUrl			    = ""

	this.Notifications.NotificationsEmailEnabled	    = false
	this.Notifications.NotificationsEmailSmtpServer	    = ""
	this.Notifications.NotificationsEmailFromAddress    = ""
	this.Notifications.NotificationsEmailFromName	    = ""
	this.Notifications.NotificationsEmailRecipients	    = nil

	// Override with config file
	if _, err := toml.DecodeFile(this.General.ConfigFile, &this); err != nil {
		log.Printf("Failed to load configuration file %s : %s\n", this.General.ConfigFile, err)
	}

    // Compatiblity with old RemoteWigos lists
    if this.RemoteWigos.RemoteWigosList != nil {
        for _, remoteWigo := range this.RemoteWigos.RemoteWigosList {

            // Split data into hostname/port
            splits := strings.Split(remoteWigo, ":")

            hostname := splits[0]
            port := 0
            if len(splits) > 1 {
                port, _ = strconv.Atoi(splits[1])
            }

            if port == 0 {
                port = this.General.ListenPort
            }

            // Create new RemoteWigoConfig
            AdvancedRemoteWigo := new(AdvancedRemoteWigoConfig)
            AdvancedRemoteWigo.Hostname = hostname
            AdvancedRemoteWigo.Port = port

            // Push new AdvancedRemoteWigo to remoteWigosList
            this.AdvancedRemoteWigosList = append(this.AdvancedRemoteWigosList, *AdvancedRemoteWigo)
        }
    }


	return
}

type GeneralConfig struct {

	// General params
	ListenPort						int
	ListenAddress					string
	ProbesDirectory					string
	LogFile							string
	ConfigFile						string
}

type NotificationConfig struct {

	// Noticications
	MinLevelToSendNotifications		int

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
}

type RemoteWigoConfig struct {

	// Remmote wigos params
    RemoteWigosCheckInterval        int
    RemoteWigosCheckTries           int

	RemoteWigosList					[]string
}

type AdvancedRemoteWigoConfig struct {
	Hostname            string
	Port                int
	CheckRemotesDepth   int
	CheckInterval       int
	CheckTries          int
}

type OpenTSDBConfig struct {

	// OpenTSDB
	Group							string
	OpenTSDBEnabled					bool
	OpenTSDBAddress					string
	OpenTSDBPort					int
	OpenTSDBMetricPrefix			string
}
