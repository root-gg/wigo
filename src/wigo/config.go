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

	// Http params
	Http *HttpConfig

	// PushServer params
	PushServer *PushServerConfig

	// PushClient params
	PushClient *PushClientConfig

	// Remmote wigos params
	RemoteWigos  *RemoteWigoConfig
	AdvancedList []AdvancedRemoteWigoConfig

	// Noticications
	Notifications *NotificationConfig

	// OpenTSDB params
	OpenTSDB *OpenTSDBConfig
}

func NewConfig( configFile string ) (this *Config) {

	// General params
	this = new(Config)
	this.Global = new(GeneralConfig)
	this.Http = new(HttpConfig)
	this.PushServer = new(PushServerConfig)
	this.PushClient = new(PushClientConfig)
	this.RemoteWigos = new(RemoteWigoConfig)
	this.Notifications = new(NotificationConfig)
	this.OpenTSDB = new(OpenTSDBConfig)

	this.Global.ProbesDirectory = "/usr/local/wigo/probes"
	this.Global.LogFile = "/var/log/wigo.log"
	this.Global.UuidFile = "/var/lib/wigo/uuid"
	this.Global.LogVerbose = true
	this.Global.EventLog = "/var/lib/wigo/events.log"
	this.Global.ConfigFile = configFile
	this.Global.Group = "none"

	// Http server
	this.Http.Enabled = true
	this.Http.Address = "0.0.0.0"
	this.Http.Port = 4000
	this.Http.SslEnabled = false
	this.Http.SslCert = "/etc/wigo/ssl/wigo.crt"
	this.Http.SslKey  = "/etc/wigo/ssl/wigo.key"
	this.Http.Login = "foo"
	this.Http.Password = "bar"
	this.Http.Gzip = true

	// Push server
	this.PushServer.Enabled = false
	this.PushServer.Address = "0.0.0.0"
	this.PushServer.Port = 4001
	this.PushServer.SslEnabled = true
	this.PushServer.SslCert = "/etc/wigo/ssl/wigo.crt"
	this.PushServer.SslKey  = "/etc/wigo/ssl/wigo.key"
	this.PushServer.AllowedClientsFile = "/var/lib/wigo/allowed"
	this.PushServer.MaxWaitingClients = 100
	this.PushServer.AutoAcceptClients = false

	// Push client
	this.PushClient.Enabled = false
	this.PushClient.Address = "127.0.0.1"
	this.PushClient.Port = 4001
	this.PushClient.SslEnabled = true
	this.PushClient.SslCert = "/etc/wigo/ssl/wigo.crt"
	this.PushClient.UuidSig = "/etc/wigo/ssl/uuid.sig"
	this.PushClient.PushInterval = 15
	
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

	// OpenTSDB
	this.OpenTSDB.Enabled = false
	this.OpenTSDB.Address = ""
	this.OpenTSDB.Port = 0
	this.OpenTSDB.SslEnabled = false
	this.OpenTSDB.MetricPrefix = "wigo"

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

			AdvancedRemoteWigo.SslEnabled = true
			AdvancedRemoteWigo.Login = ""
			AdvancedRemoteWigo.Password = ""

			// Push new AdvancedRemoteWigo to remoteWigosList
			this.AdvancedList = append(this.AdvancedList, *AdvancedRemoteWigo)
		}
	}

	this.RemoteWigos.AdvancedList = this.AdvancedList
	this.AdvancedList = nil

	return
}

type GeneralConfig struct {
	Hostname		string
	ListenPort      int
	ListenAddress   string
	ProbesDirectory string
	UuidFile		string
	LogFile         string
	LogVerbose		bool
	ConfigFile      string
	Group           string
	EventLog		string
}

type HttpConfig struct {
	Enabled      bool
	Address      string
	Port         int
	SslEnabled	 bool
	SslCert		 string
	SslKey	     string
	Login		 string
	Password	 string
	Gzip	     bool
}

type PushServerConfig struct {
	Enabled      		bool
	Address      		string
	Port         		int
	SslEnabled	 		bool
	SslCert		 		string
	SslKey	     		string
	AllowedClientsFile  string
	AutoAcceptClients	bool
	MaxWaitingClients 	int
}

type PushClientConfig struct {
	Enabled      bool
	Address      string
	Port         int
	SslEnabled	 bool
	SslCert		 string
	UuidSig		 string
	PushInterval int
}

type RemoteWigoConfig struct {
	CheckInterval int
	CheckTries    int

	SslEnabled		  bool
	Login			  string
	Password		  string

	List         []string
	AdvancedList []AdvancedRemoteWigoConfig
}

type NotificationConfig struct {
	// Noticications
	MinLevelToSend 	int

	OnWigoChange  	bool
	OnHostChange  	bool
	OnProbeChange 	bool

	HttpEnabled 	int
	HttpUrl     	string

	EmailEnabled     int
	EmailSmtpServer  string
	EmailRecipients  []string
	EmailFromName    string
	EmailFromAddress string
}

type AdvancedRemoteWigoConfig struct {
	Hostname          string
	Port              int
	CheckRemotesDepth int
	CheckInterval     int
	CheckTries        int
	SslEnabled		  bool
	Login			  string
	Password		  string
}

type OpenTSDBConfig struct {
	Enabled      bool
	Address      string
	Port         int
	SslEnabled	 bool
	MetricPrefix string
}
