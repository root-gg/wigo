package wigo

import (
	"github.com/BurntSushi/toml"
	"log"
	"strconv"
	"strings"
)

type Config struct {

	// General params
	Global *GeneralConfig

	// OpenTSDB params
	OpenTSDB *OpenTSDBConfig

	// Remmote wigos params
	RemoteWigos  *RemoteWigoConfig
	AdvancedList []AdvancedRemoteWigoConfig

	// Noticications
	Notifications *NotificationConfig
}

func NewConfig( configFile string ) (this *Config) {

	// General params
	this = new(Config)
	this.Global = new(GeneralConfig)
	this.OpenTSDB = new(OpenTSDBConfig)
	this.RemoteWigos = new(RemoteWigoConfig)
	this.Notifications = new(NotificationConfig)

	this.Global.ListenPort = 4000
	this.Global.ListenAddress = "0.0.0.0"
	this.Global.ProbesDirectory = "/usr/local/wigo/probes"
	this.Global.LogFile = "/var/log/wigo.log"
	this.Global.LogVerbose = true
	this.Global.EventLog = "/var/lib/wigo/events.log"
	this.Global.ConfigFile = configFile
	this.Global.Group = "none"

	// OpenTSDB
	this.OpenTSDB.Enabled = false
	this.OpenTSDB.Address = ""
	this.OpenTSDB.Port = 0
	this.OpenTSDB.SslEnabled = false
	this.OpenTSDB.MetricPrefix = "wigo"

	// Remote Wigos
	this.RemoteWigos.List = nil
	this.RemoteWigos.CheckInterval = 10
	this.RemoteWigos.CheckTries = 3
	this.AdvancedList = nil

	// Notifications
	this.Notifications.MinLevelToSend = 101

	this.Notifications.OnWigoChange = false
	this.Notifications.OnHostChange = false
	this.Notifications.OnProbeChange = false

	this.Notifications.HttpEnabled = 0
	this.Notifications.HttpUrl = ""

	this.Notifications.EmailEnabled = 0
	this.Notifications.EmailSmtpServer = ""
	this.Notifications.EmailFromAddress = ""
	this.Notifications.EmailFromName = ""
	this.Notifications.EmailRecipients = nil

	// Override with config file
	if _, err := toml.DecodeFile(this.Global.ConfigFile, &this); err != nil {
		log.Printf("Failed to load configuration file %s : %s\n", this.Global.ConfigFile, err)
	}

	// Compatiblity with old RemoteWigos lists
	if this.RemoteWigos.List != nil {
		for _, remoteWigo := range this.RemoteWigos.List {

			// Split data into hostname/port
			splits := strings.Split(remoteWigo, ":")

			hostname := splits[0]
			port := 0
			if len(splits) > 1 {
				port, _ = strconv.Atoi(splits[1])
			}

			if port == 0 {
				port = this.Global.ListenPort
			}

			// Create new RemoteWigoConfig
			AdvancedRemoteWigo := new(AdvancedRemoteWigoConfig)
			AdvancedRemoteWigo.Hostname = hostname
			AdvancedRemoteWigo.Port = port

			// Push new AdvancedRemoteWigo to remoteWigosList
			this.AdvancedList = append(this.AdvancedList, *AdvancedRemoteWigo)
		}
	}

	this.RemoteWigos.AdvancedList = this.AdvancedList
	this.AdvancedList = nil

	return
}

type GeneralConfig struct {

	// General params
	ListenPort      int
	ListenAddress   string
	ProbesDirectory string
	LogFile         string
	LogVerbose		bool
	ConfigFile      string
	Group           string
	EventLog		string
}

type NotificationConfig struct {

	// Noticications
	MinLevelToSend int

	OnWigoChange  bool
	OnHostChange  bool
	OnProbeChange bool

	HttpEnabled int
	HttpUrl     string

	EmailEnabled     int
	EmailSmtpServer  string
	EmailRecipients  []string
	EmailFromName    string
	EmailFromAddress string
}

type RemoteWigoConfig struct {

	// Remmote wigos params
	CheckInterval int
	CheckTries    int

	List         []string
	AdvancedList []AdvancedRemoteWigoConfig
}

type AdvancedRemoteWigoConfig struct {
	Hostname          string
	Port              int
	CheckRemotesDepth int
	CheckInterval     int
	CheckTries        int
}

type OpenTSDBConfig struct {

	// OpenTSDB
	Enabled      bool
	Address      string
	Port         int
	SslEnabled	 bool
	MetricPrefix string
}
