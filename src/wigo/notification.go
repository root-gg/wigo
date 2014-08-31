package wigo

import (
	"fmt"
)

type Notification struct {

	Type		string

	Receiver	string
	Message		string

	Hostname	string
	OldProbe	*ProbeResult
	NewProbe	*ProbeResult

}

func NewNotification( t string, receiver string, host *Host, oldProbe *ProbeResult, newProbe *ProbeResult) ( this *Notification ){

	if( t != "url" ){
		return nil
	}

	this = new(Notification)
	this.Type		= t
	this.OldProbe	= oldProbe
	this.NewProbe	= newProbe
	this.Message 	= fmt.Sprintf("Probe %s switched from %d to %d on host %s", oldProbe.Name, oldProbe.Status, newProbe.Status, host.Name)
	this.Receiver	= receiver

	return
}

func (this *Notification) SendNotification( ci chan Event ){
	ci <- Event{ SENDNOTIFICATION, this }
}
