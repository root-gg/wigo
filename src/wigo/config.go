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

	// What sits behind what. A router going down should be one message, not
	// forty. See dependency.go.
	Dependencies []DependencyConfig

	// How long individual probes get to answer, by name. Anything not named
	// here keeps the interval minus a second it has always had. See
	// probe_timeout.go.
	ProbeTimeouts map[string]int

	// What this host is, beyond its one Group : env = "prod", role = "db".
	// Group is unchanged and still published, as a label too. See label.go.
	Labels map[string]string
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
	this.Global.MetricsRetentionDays = defaultMetricsRetentionDays
	this.Global.StatusHistoryDays = defaultStatusHistoryDays
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

	// Off by default : until it is turned on the API stays read only, so an
	// upgrade never opens anything that was closed before.
	this.Http.AllowWriteActions = false

	// Left empty so it can mean "whatever this install already did", which is
	// resolved from Login. See ResolvedAnonymousRole.
	this.Http.AnonymousRole = ""

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

	// Off by default : a client never lets its master act on it unless its own
	// administrator opted in on that machine.
	this.PushClient.AllowRemoteControl = false

	// Remote Wigos
	this.RemoteWigos.List = nil
	this.RemoteWigos.CheckInterval = 10
	this.AdvancedList = nil

	// Notifications
	this.Notifications.MinLevelToSend = 101

	this.Notifications.OnHostChange = false
	this.Notifications.OnProbeChange = false
	this.Notifications.FlapDetection = true
	this.Notifications.FlapWindow = defaultFlapWindow
	this.Notifications.FlapThreshold = defaultFlapThreshold
	this.Notifications.RenotifyInterval = 0
	this.Notifications.EscalateAfter = 0
	this.Notifications.QuietHoursFrom = ""
	this.Notifications.QuietHoursTo = ""
	this.Notifications.QuietHoursMinLevelToSend = 0

	this.Notifications.HttpEnabled = 0
	this.Notifications.HttpUrl = ""

	this.Notifications.EmailEnabled = 0
	this.Notifications.EmailSmtpServer = ""
	this.Notifications.EmailFromAddress = ""
	this.Notifications.EmailFromName = ""
	this.Notifications.EmailRecipients = nil

	this.Notifications.AppriseEnabled = 0
	this.Notifications.ApprisePath = "/usr/local/bin/apprise"
	this.Notifications.AppriseUrls = nil
	this.Notifications.AppriseTargets = nil

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

	// Warn about apprise targets that would never be notified
	for i := range this.Notifications.AppriseTargets {
		target := &this.Notifications.AppriseTargets[i]
		if len(target.Groups) == 0 && len(target.Hosts) == 0 {
			log.Printf("Apprise target %s has no Groups nor Hosts filter and will never be notified, use AppriseUrls to notify every host\n", target.GetName(i))
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

	// How many days of what the probes measure to keep, in the sqlite that is
	// already there. Zero keeps none. Nothing about the monitoring depends on
	// it : losing this loses history and nothing else.
	MetricsRetentionDays int

	// How many days of status changes to keep, which is what the timeline is
	// drawn from. A change is not a sample : a handful a day, not one a minute.
	StatusHistoryDays int
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

	// Lets the API change this host : enable, disable and repitch its probes.
	// Anyone able to reach the dashboard can then switch monitoring off, so it
	// stays closed until an administrator opens it.
	AllowWriteActions bool

	// What somebody who presents no credential at all is allowed to do :
	// "operator", "readonly", or "none" to refuse them. Empty keeps whatever
	// this install already did, which is the whole point of it being empty :
	// no Login means everything was open, a Login meant everything was shut,
	// and neither may change on an upgrade.
	AnonymousRole string
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

	// Lets the push server this client connects to act on it : enable, disable
	// and repitch its probes. Separate from Http.AllowWriteActions on purpose,
	// so opening the local API never opens the machine to its master as well.
	AllowRemoteControl bool
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

	// A probe that keeps changing status buries the real incident of the
	// evening under fifty messages. Once it has changed FlapThreshold times
	// inside FlapWindow seconds it is called out once and then goes quiet
	// until it settles. On by default : a monitoring tool that spams on flap
	// is not doing its job, and the first transitions are notified anyway.
	FlapDetection bool
	FlapWindow    int
	FlapThreshold int

	// A problem that is still there is said again this often, in seconds. Zero
	// keeps the old behaviour : one message when it breaks, one when it is
	// fixed, and silence in between that looks exactly like everything being
	// fine.
	RenotifyInterval int

	// After this many seconds unattended, a problem also goes to the apprise
	// targets marked Escalation. Zero never escalates.
	EscalateAfter int

	// The window nobody wants to be woken in, as "22:00" and "08:00". Nothing
	// is dropped : a notification held here is not recorded as sent, so the
	// repeat loop says it as soon as the window closes. Anything at or above
	// QuietHoursMinLevelToSend gets through anyway.
	QuietHoursFrom           string
	QuietHoursTo             string
	QuietHoursMinLevelToSend int

	HttpEnabled int
	HttpUrl     string

	EmailEnabled     int
	EmailSmtpServer  string
	EmailRecipients  []string
	EmailFromName    string
	EmailFromAddress string

	AppriseEnabled int
	ApprisePath    string
	AppriseUrls    []string
	AppriseTargets []AppriseTargetConfig
}

// AppriseTargetConfig sends a set of apprise urls only for the notifications
// matching its filters. Groups and Hosts are OR'ed together and accept the "*"
// wildcard. Use the plain AppriseUrls list to notify every host.
type AppriseTargetConfig struct {
	Name   string
	Urls   []string
	Groups []string
	Hosts  []string

	// Only notified once a problem has gone unattended for EscalateAfter
	// seconds. The people you wake up second.
	Escalation bool
}

// GetName returns a printable name for this target, falling back on its
// position in the configuration file when Name is not set.
func (this *AppriseTargetConfig) GetName(index int) string {
	if this.Name == "" {
		return "#" + strconv.Itoa(index+1)
	}

	return "\"" + this.Name + "\""
}

// Matches tells whether a notification coming from the given host/group has to
// be sent to this target.
func (this *AppriseTargetConfig) Matches(hostname string, group string) bool {

	// A target without any filter would never be notified
	if len(this.Groups) == 0 && len(this.Hosts) == 0 {
		return false
	}

	if matchFilterList(this.Groups, group) {
		return true
	}

	return matchFilterList(this.Hosts, hostname)
}

func matchFilterList(filters []string, value string) bool {
	for _, filter := range filters {
		if filter == "*" {
			return true
		}
		if value != "" && strings.EqualFold(filter, value) {
			return true
		}
	}

	return false
}

// AppriseUrl is an apprise url to notify along with the name of the target it
// comes from, so logs can tell which part of the configuration triggered it.
type AppriseUrl struct {
	Url    string
	Target string
}

// Origin returns a printable suffix telling which target this url comes from
func (this *AppriseUrl) Origin() string {
	if this.Target == "" {
		return ""
	}

	return " (target " + this.Target + ")"
}

// GetAppriseUrls returns the deduplicated list of apprise urls to notify for a
// given host/group. The plain AppriseUrls list is always included as it has no
// filter.
func (this *NotificationConfig) GetAppriseUrls(hostname string, group string, escalated bool) (urls []AppriseUrl) {

	seen := make(map[string]bool)

	appendUrls := func(list []string, target string) {
		for _, url := range list {
			if url == "" || seen[url] {
				continue
			}
			seen[url] = true
			urls = append(urls, AppriseUrl{Url: url, Target: target})
		}
	}

	appendUrls(this.AppriseUrls, "")

	for i := range this.AppriseTargets {
		// An escalation target is deliberately left out of the normal path :
		// the whole point of being second in line is not to be woken first.
		if this.AppriseTargets[i].Escalation && !escalated {
			continue
		}
		if this.AppriseTargets[i].Matches(hostname, group) {
			appendUrls(this.AppriseTargets[i].Urls, this.AppriseTargets[i].GetName(i))
		}
	}

	return
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
