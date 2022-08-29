package wigo

import (
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
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

func NewConfig(configFile string) (this *Config) {

	// General params
	this = new(Config)
	this.Global = new(GeneralConfig)
	this.Http = new(HttpConfig)
	this.PushServer = new(PushServerConfig)
	this.PushClient = new(PushClientConfig)
	this.RemoteWigos = new(RemoteWigoConfig)
	this.Notifications = new(NotificationConfig)
	this.OpenTSDB = new(OpenTSDBConfig)

	this.Global.Hostname = ""
	this.Global.Group = "none"
	this.Global.ProbesDirectory = "/usr/local/wigo/probes"
	this.Global.ProbesConfigDirectory = "/etc/wigo/conf.d"
	this.Global.LogFile = "/var/log/wigo.log"
	this.Global.UuidFile = "/var/lib/wigo/uuid"
	this.Global.Database = "/var/lib/wigo/wigo.db"
	this.Global.AliveTimeout = 60
	this.Global.ConfigFile = configFile
	this.Global.Debug = false
	this.Global.Trace = false

	// Http server
	this.Http.Enabled = true
	this.Http.Address = "0.0.0.0"
	this.Http.Port = 4000
	this.Http.SslEnabled = false
	this.Http.SslCert = "/etc/wigo/ssl/wigo.crt"
	this.Http.SslKey = "/etc/wigo/ssl/wigo.key"
	this.Http.Login = ""
	this.Http.Password = ""
	this.Http.Gzip = true

	// Push server
	this.PushServer.Enabled = false
	this.PushServer.Address = "0.0.0.0"
	this.PushServer.Port = 4001
	this.PushServer.SslEnabled = true
	this.PushServer.SslCert = "/etc/wigo/ssl/wigo.crt"
	this.PushServer.SslKey = "/etc/wigo/ssl/wigo.key"
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
	this.AdvancedList = nil

	// Notifications
	this.Notifications.MinLevelToSend = 101

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
	this.OpenTSDB.Address = nil
	this.OpenTSDB.SslEnabled = false
	this.OpenTSDB.MetricPrefix = "wigo"
	this.OpenTSDB.Deduplication = 600
	this.OpenTSDB.BufferSize = 10000
	this.OpenTSDB.Tags = make(map[string]string)

	log.Printf("Loading configuration file %s\n", this.Global.ConfigFile)

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
				port = this.Http.Port
			}

			// Create new RemoteWigoConfig
			AdvancedRemoteWigo := new(AdvancedRemoteWigoConfig)
			AdvancedRemoteWigo.Hostname = hostname
			AdvancedRemoteWigo.Port = port

			AdvancedRemoteWigo.SslEnabled = false
			AdvancedRemoteWigo.Login = ""
			AdvancedRemoteWigo.Password = ""

			// Push new AdvancedRemoteWigo to remoteWigosList
			this.AdvancedList = append(this.AdvancedList, *AdvancedRemoteWigo)
		}
	}

	this.RemoteWigos.AdvancedList = this.AdvancedList
	this.AdvancedList = nil

	os.Setenv("WIGO_PROBE_CONFIG_ROOT", this.Global.ProbesConfigDirectory)

	return
}

type GeneralConfig struct {
	Hostname              string
	ListenAddress         string
	ProbesDirectory       string
	ProbesConfigDirectory string
	UuidFile              string
	LogFile               string
	Debug                 bool
	Trace                 bool
	ConfigFile            string
	Group                 string
	Database              string
	AliveTimeout          int
}

type HttpConfig struct {
	Enabled    bool
	Address    string
	Port       int
	SslEnabled bool
	SslCert    string
	SslKey     string
	Login      string
	Password   string
	Gzip       bool
}

type PushServerConfig struct {
	Enabled            bool
	Address            string
	Port               int
	SslEnabled         bool
	SslCert            string
	SslKey             string
	AllowedClientsFile string
	AutoAcceptClients  bool
	MaxWaitingClients  int
}

type PushClientConfig struct {
	Enabled      bool
	Address      string
	Port         int
	SslEnabled   bool
	SslCert      string
	UuidSig      string
	PushInterval int
}

type RemoteWigoConfig struct {
	CheckInterval int

	SslEnabled bool
	Login      string
	Password   string

	List         []string
	AdvancedList []AdvancedRemoteWigoConfig
}

type NotificationConfig struct {
	// Noticications
	MinLevelToSend int

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

type AdvancedRemoteWigoConfig struct {
	Hostname          string
	Port              int
	CheckRemotesDepth int
	CheckInterval     int
	SslEnabled        bool
	Login             string
	Password          string
}

type OpenTSDBConfig struct {
	Enabled       bool
	Address       []string
	SslEnabled    bool
	MetricPrefix  string
	Deduplication int
	BufferSize    int
	Tags          map[string]string
}
