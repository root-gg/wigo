package wigo

import (
	"fmt"
	"encoding/json"
	"time"
)

type Notification struct {
	Type		string
	Receiver	string
	Message		string
	Date		string
	Ressource	string
}

type INotification interface {
	ToJson() 		([]byte, error)
	GetMessage()	string
	GetReceiver()	string
	GetRessource()	string
}

type NotificationWigo struct {
	*Notification
	OldWigo		*Wigo
	NewWigo		*Wigo
}
type NotificationHost struct {
	*Notification
	OldHost		*Host
	NewHost		*Host
}
type NotificationProbe struct {
	*Notification
	OldProbe	*ProbeResult
	NewProbe	*ProbeResult
}


// Constructors
func NewNotification() ( this *Notification ){
	this				= new(Notification)
	this.Date			= time.Now().Format(dateLayout)
	return
}

func NewNotificationWigo( oldWigo *Wigo, newWigo *Wigo ) ( this *NotificationWigo ){
	this 				= new(NotificationWigo)
	this.Notification	= NewNotification()
	this.OldWigo 		= oldWigo
	this.NewWigo		= newWigo
	this.Ressource		= oldWigo.GetHostname()

	if oldWigo.IsAlive && !newWigo.IsAlive {
		// UP -> DOWN
		this.Message = fmt.Sprintf("Host %s DOWN : %s", newWigo.GetHostname(), newWigo.GlobalMessage )

	} else if !oldWigo.IsAlive && newWigo.IsAlive {
		// DOWN -> UP
		this.Message = fmt.Sprintf("Host %s UP", newWigo.GetHostname())

	} else if newWigo.GlobalStatus != oldWigo.GlobalStatus {
		// CHANGED STATUS
		this.Message = fmt.Sprintf("Host %s status switched from %d to %d", newWigo.GetHostname(), oldWigo.GlobalStatus, newWigo.GlobalStatus)

	}

	return
}

func NewNotificationHost( oldHost *Host, newHost *Host ) ( this *NotificationHost ){
	this 				= new(NotificationHost)
	this.Notification	= NewNotification()
	this.OldHost		= oldHost
	this.NewHost		= newHost
	this.Ressource		= oldHost.Name

	if newHost.Status != oldHost.Status {
		this.Message = fmt.Sprintf("Host %s changed status from %d to %d", newHost.Name, oldHost.Status, newHost.Status)
	}

	return
}

func NewNotificationProbe( oldProbe *ProbeResult, newProbe *ProbeResult ) ( this *NotificationProbe ){
	this 				= new(NotificationProbe)
	this.Notification	= NewNotification()
	this.OldProbe		= oldProbe
	this.NewProbe		= newProbe

	if oldProbe == nil && newProbe != nil {
		this.Message = fmt.Sprintf("New probe %s with status %d detected on host %s", newProbe.Name, newProbe.Status, newProbe.GetHost().Name)
		this.Ressource = newProbe.Name

	} else if oldProbe != nil && newProbe == nil {
		this.Message = fmt.Sprintf("Probe %s on host %s does not exist anymore. Last status was %d", oldProbe.Name, oldProbe.GetHost().Name, oldProbe.Status )
		this.Ressource = oldProbe.Name

	} else if oldProbe != nil && newProbe != nil {
		this.Ressource = newProbe.Name
		if newProbe.Status != oldProbe.Status {
			this.Message = fmt.Sprintf("Probe %s status changed from %d to %d on host %s (%s)", newProbe.Name, oldProbe.Status, newProbe.Status, oldProbe.GetHost().Name, newProbe.Message)
		}
	}

	return
}


// Getters
func (this *Notification) ToJson() ( ba []byte, e error ) {
	return json.Marshal(this)
}
func (this *NotificationWigo) ToJson() ( ba []byte, e error ) {
	return json.Marshal(this)
}
func (this *NotificationHost) ToJson() ( ba []byte, e error ) {
	return json.Marshal(this)
}
func (this *NotificationProbe) ToJson() ( ba []byte, e error ) {
	return json.Marshal(this)
}


func (this *Notification) GetMessage() ( string ){
	return this.Message
}
func (this *Notification) GetReceiver() ( string ){
	return this.Receiver
}
func (this *Notification) GetRessource() ( string ){
	return this.Ressource
}

