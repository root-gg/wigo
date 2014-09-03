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
}

type INotification interface {
	ToJson() 		([]byte, error)
	GetMessage()	string
	GetReceiver()	string
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
func NewNotification( receiver string ) ( this *Notification ){
	this				= new(Notification)
	this.Date			= time.Now().Format(dateLayout)
	this.Receiver		= receiver
	return
}

func NewNotificationWigo( receiver string, oldWigo *Wigo, newWigo *Wigo ) ( this *NotificationWigo ){
	this 				= new(NotificationWigo)
	this.Notification	= NewNotification(receiver)
	this.OldWigo 		= oldWigo
	this.NewWigo		= newWigo


	if oldWigo.IsAlive && !newWigo.IsAlive {
		// UP -> DOWN
		this.Message = fmt.Sprintf("Wigo %s DOWN : %s", newWigo.GetHostname(), newWigo.GlobalMessage )

	} else if !oldWigo.IsAlive && newWigo.IsAlive {
		// DOWN -> UP
		this.Message = fmt.Sprintf("Wigo %s UP", newWigo.GetHostname())

	} else if newWigo.GlobalStatus != oldWigo.GlobalStatus {
		// CHANGED STATUS
		this.Message = fmt.Sprintf("Wigo %s status switched from %d to %d", newWigo.GetHostname(), oldWigo.GlobalStatus, newWigo.GlobalStatus)

	}

	return
}

func NewNotificationHost( receiver string, oldHost *Host, newHost *Host ) ( this *NotificationHost ){
	this 				= new(NotificationHost)
	this.Notification	= NewNotification(receiver)
	this.OldHost		= oldHost
	this.NewHost		= newHost

	if newHost.Status != oldHost.Status {
		this.Message = fmt.Sprintf("Host %s changed status from %d to %d", newHost.Name, oldHost.Status, newHost.Status)
	}

	return
}

func NewNotificationProbe( receiver string, oldProbe *ProbeResult, newProbe *ProbeResult ) ( this *NotificationProbe ){
	this 				= new(NotificationProbe)
	this.Notification	= NewNotification(receiver)
	this.OldProbe		= oldProbe
	this.NewProbe		= newProbe

	if newProbe.Status != oldProbe.Status {
		this.Message = fmt.Sprintf("Probe %s status changed from %d to %d : %s", newProbe.Name, oldProbe.Status, newProbe.Status, newProbe.Message )
	}

	return
}


// Getters
func (this *Notification) ToJson() ( ba []byte, e error ) {
	return json.Marshal(this)
}

func (this *Notification) GetMessage() ( string ){
	return this.Message
}

func (this *Notification) GetReceiver() ( string ){
	return this.Receiver
}

