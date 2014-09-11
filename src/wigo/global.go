package wigo

import (
	"encoding/json"
	"sync"
	"os"
	"fmt"
	"log"
	"io"

	"code.google.com/p/go-uuid/uuid"
	"github.com/fatih/color"
	"strings"
)

// Static global object
var LocalWigo 	*Wigo

type Wigo struct {
	Uuid			string
	Version			string
	IsAlive			bool

	GlobalStatus	int
	GlobalMessage	string

	LocalHost		*Host
	RemoteWigos		map[string] *Wigo

	hostname		string
	config			*Config
	locker			*sync.RWMutex
	logfilehandle	*os.File
}

func InitWigo() ( err error ){

	if LocalWigo == nil {
		LocalWigo 				= new(Wigo)

		LocalWigo.Uuid			= uuid.New()
		LocalWigo.IsAlive 		= true
		LocalWigo.Version 		= "Wigo v0.42"
		LocalWigo.GlobalStatus 	= 100
		LocalWigo.GlobalMessage = "OK"


		// Init LocalHost and RemoteWigos list
		LocalWigo.LocalHost 	= NewLocalHost()
		LocalWigo.RemoteWigos 	= make(map[string] *Wigo)

		// Private vars
		LocalWigo.config 		= NewConfig()
		LocalWigo.hostname 		= "localhost"
		LocalWigo.locker 		= new(sync.RWMutex)


		// Log
		LocalWigo.InitOrReloadLogger()


		// Test probes directory
		_, err = os.Stat( LocalWigo.GetConfig().ProbesDirectory )
		if err != nil {
			return err
		}

		// Init channels
		InitChannels()
	}

	return nil
}

// Factory

func GetLocalWigo() ( *Wigo ){
	return LocalWigo
}


// Constructors

func NewWigoFromJson( ba []byte ) ( this *Wigo, e error ){

	this 			= new(Wigo)
	this.IsAlive 	= true

	err := json.Unmarshal( ba, this )
	if( err != nil ){
		return nil, err
	}

	this.SetParentHostsInProbes()
	this.SetRemoteWigosHostnames()

	return
}

func NewWigoFromErrorMessage( message string, isAlive bool ) ( this *Wigo ){

	this = new(Wigo)
	this.GlobalStatus 	= 500
	this.GlobalMessage	= message
	this.IsAlive		= isAlive
	this.RemoteWigos	= make(map[string] *Wigo)
	this.LocalHost		= new(Host)

	return
}


// Recompute statuses
func (this *Wigo) RecomputeGlobalStatus() {

	this.GlobalStatus = 0

	// Local probes
	for probeName := range this.LocalHost.Probes {
		if this.LocalHost.Probes[probeName].Status > this.GlobalStatus {
			this.GlobalStatus = this.LocalHost.Probes[probeName].Status
		}
	}

	// Remote wigos statuses
	for wigoName := range this.RemoteWigos {
		if this.RemoteWigos[wigoName].GlobalStatus > this.GlobalStatus {
			this.GlobalStatus = this.RemoteWigos[wigoName].GlobalStatus
		}
	}

	return
}



// Getters
func (this *Wigo) GetLocalHost() ( *Host ){
	return this.LocalHost
}

func (this *Wigo) GetConfig() (*Config){
	return this.config
}

func (this *Wigo) GetHostname() ( string ){
	return this.hostname
}


// Setters

func (this *Wigo) SetHostname( hostname string ){
	this.hostname = hostname
}

func (this *Wigo) AddOrUpdateRemoteWigo( wigoName string, remoteWigo * Wigo ){

	this.Lock()
	defer this.Unlock()

	// Test is remote is not me :D
	if remoteWigo.Uuid!= "" && this.Uuid == remoteWigo.Uuid {
		log.Printf("Try to add a remote wigo %s with same uuid as me.. Discarding..",remoteWigo.GetHostname())
		return
	}

	if oldWigo, ok := this.RemoteWigos[ wigoName ] ; ok {
		this.CompareTwoWigosAndRaiseNotifications(oldWigo,remoteWigo)
	}


	this.RemoteWigos[ wigoName ] = remoteWigo
	this.RemoteWigos[ wigoName ].SetHostname(wigoName)
	this.RecomputeGlobalStatus()
}


func (this *Wigo) CompareTwoWigosAndRaiseNotifications( oldWigo *Wigo, newWigo *Wigo ) () {

	if (newWigo.GlobalStatus != oldWigo.GlobalStatus) {
		NewNotificationWigo(oldWigo, newWigo)
	}

	// Detect changes and deleted probes
	if oldWigo.LocalHost != nil {

		if oldWigo.LocalHost.Status != newWigo.LocalHost.Status {
			NewNotificationHost(oldWigo.LocalHost, newWigo.LocalHost)
		}

		for probeName := range oldWigo.LocalHost.Probes {
			oldProbe := oldWigo.LocalHost.Probes[ probeName ]

			if probeWhichStillExistInNew, ok := newWigo.LocalHost.Probes[ probeName ] ; ok {

				// Probe still exist in new
				// Status has changed ? -> Notification
				if ( oldProbe.Status != probeWhichStillExistInNew.Status ) {
					NewNotificationProbe(oldProbe, probeWhichStillExistInNew)
				}
			} else {

				// Prob disappeard !
				if newWigo.IsAlive {
					NewNotificationProbe(oldProbe, nil)
				}
			}
		}
	}

	// Detect new probes (only if new wigo is up)
	if newWigo.IsAlive && oldWigo.IsAlive {
		for probeName := range newWigo.LocalHost.Probes {
			if _,ok := oldWigo.LocalHost.Probes[probeName] ; !ok {
				NewNotificationProbe( nil, newWigo.LocalHost.Probes[probeName] )
			}
		}
	}

	// Remote Wigos
	for wigoName := range oldWigo.RemoteWigos {

		oldWigo := oldWigo.RemoteWigos[ wigoName ]

		if wigoStillExistInNew, ok := newWigo.RemoteWigos[ wigoName ]; ok {

			// Test if a remote wigo is not me
			if wigoStillExistInNew.Uuid != "" && this.Uuid == wigoStillExistInNew.Uuid {
				log.Printf("Detected myself in remote wigo %s. Discarding.", wigoStillExistInNew.GetHostname())
				return
			}

			// Recursion
			this.CompareTwoWigosAndRaiseNotifications(oldWigo, wigoStillExistInNew)
		}
	}
}


func (this *Wigo) SetParentHostsInProbes(){
	for localProbeName := range this.GetLocalHost().Probes {
		this.LocalHost.Probes[localProbeName].SetHost( this.LocalHost )
	}

	for remoteWigo := range this.RemoteWigos{
		this.RemoteWigos[remoteWigo].SetParentHostsInProbes()
	}
}

func (this *Wigo) SetRemoteWigosHostnames(){

	for remoteWigo := range this.RemoteWigos{
		this.RemoteWigos[remoteWigo].SetHostname(remoteWigo)
		this.RemoteWigos[remoteWigo].SetRemoteWigosHostnames()
	}
}

// Reloads

func (this *Wigo) InitOrReloadLogger() ( err error ){
	if this.logfilehandle != nil {
		err = this.logfilehandle.Close()
		if err != nil {
			return err
		}
	}

	f, err := os.OpenFile(LocalWigo.GetConfig().LogFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		fmt.Printf("Fail to open logfile %s : %s\n", LocalWigo.GetConfig().LogFile, err)
		return err
	} else {
		LocalWigo.logfilehandle = f
		writer := io.MultiWriter( os.Stdout, f )

		log.SetOutput(writer)
		log.SetPrefix(LocalWigo.GetLocalHost().Name + " ")
	}

	return nil
}

// Locks
func (this *Wigo) Lock(){
	this.locker.Lock()
}

func (this *Wigo) Unlock(){
	this.locker.Unlock()
}


// Serialize
func (this *Wigo) ToJsonString() ( string, error ){

	// Send json to socket channel
	j, e := json.MarshalIndent( this, "", "    ")
	if ( e != nil ) {
		return "", e
	}


	return string(j), nil
}


func (this *Wigo) GenerateSummary( showOnlyErrors bool ) ( summary string ){

	red 	:= color.New( color.FgRed ).SprintfFunc()
	yellow 	:= color.New( color.FgYellow ).SprintfFunc()

	summary += fmt.Sprintf("%s running on %s \n", this.Version, this.LocalHost.Name)
	summary += fmt.Sprintf("Local Status 	: %d\n", this.LocalHost.Status)
	summary += fmt.Sprintf("Global Status	: %d\n\n", this.GlobalStatus)

	if showOnlyErrors && this.LocalHost.Status != 100 {
		summary += "Local probes : \n\n"

		for probeName := range this.LocalHost.Probes {
			if this.LocalHost.Probes[probeName].Status > 100 && this.LocalHost.Probes[probeName].Status < 300 {
				summary += yellow("\t%-25s : %d\n", this.LocalHost.Probes[probeName].Name, this.LocalHost.Probes[probeName].Status)
			} else if this.LocalHost.Probes[probeName].Status >= 300 {
				summary += red("\t%-25s : %d\n", this.LocalHost.Probes[probeName].Name, this.LocalHost.Probes[probeName].Status)
			} else {
				summary += fmt.Sprintf("\t%-25s : %d\n", this.LocalHost.Probes[probeName].Name, this.LocalHost.Probes[probeName].Status)
			}
		}

		summary += "\n"
	}


	if this.GlobalStatus > 100 {
		summary += "Remote Wigos : \n\n"
	}

	summary += this.GenerateRemoteWigosSummary(0 , showOnlyErrors)

	return
}

func (this *Wigo) GenerateRemoteWigosSummary( level int , showOnlyErrors bool ) ( summary string ) {

	red 	:= color.New( color.FgRed ).SprintfFunc()
	yellow 	:= color.New( color.FgYellow ).SprintfFunc()

	for remoteWigo := range this.RemoteWigos {

		if(showOnlyErrors && this.RemoteWigos[remoteWigo].GlobalStatus == 100){
			continue;
		}


		// Nice align
		tabs := ""
		for i := 0; i <= level; i++{
			tabs += "\t"
		}


		// Host down ?
		if ! this.RemoteWigos[remoteWigo].IsAlive {
			summary += tabs + red(this.RemoteWigos[remoteWigo].GetHostname() + " DOWN : \n")
			summary += tabs + red("\t" + this.RemoteWigos[remoteWigo].GlobalMessage + "\n")

		} else {
			summary += tabs + this.RemoteWigos[remoteWigo].GetHostname() + " ( " + this.RemoteWigos[remoteWigo].LocalHost.Name + " ) : \n"
		}


		// Iterate on probes
		for probeName := range this.RemoteWigos[remoteWigo].GetLocalHost().Probes {

			currentProbe := this.RemoteWigos[remoteWigo].GetLocalHost().Probes[probeName]
			summary += tabs

			if currentProbe.Status > 100 && currentProbe.Status < 300 {
				summary += yellow("\t%-25s : %d  %s\n", currentProbe.Name, currentProbe.Status, strings.Replace(currentProbe.Message, "%", "%%", -1))
			} else if currentProbe.Status >= 300 {
				summary += red("\t%-25s : %d  %s\n", currentProbe.Name, currentProbe.Status, strings.Replace(currentProbe.Message, "%", "%%", -1))
			} else {
				summary += fmt.Sprintf("\t%-25s : %d  %s\n", currentProbe.Name, currentProbe.Status, strings.Replace(currentProbe.Message, "%", "%%", -1))
			}
		}

		summary += "\n"
		summary += this.RemoteWigos[remoteWigo].GenerateRemoteWigosSummary(level, showOnlyErrors)
	}

	return
}
