package wigo

import "time"


type Log struct {
	Date		string
	Timestamp	int64
	Level		uint8
	Message 	string
	Host		string
	Probe   	string
	Group		string
}

func NewLog( level uint8, message string ) ( this *Log ){
	this 			= new(Log)
	this.Date 		= time.Now().Format(dateLayout)
	this.Timestamp	= time.Now().Unix()
	this.Level  	= level
	this.Message	= message
	this.Host		= GetLocalWigo().GetLocalHost().Name
	this.Group		= GetLocalWigo().GetLocalHost().Group
	this.Probe		= ""

	return
}

// Setters
func ( this *Log ) SetHost( hostname string ){
	this.Host = hostname
}
func ( this *Log ) SetProbe( probename string ){
	this.Host = probename
}
func ( this *Log ) SetGroup( group string ){
	this.Group = group
}

// Log levels
const (
	DEBUG		= 1
	NOTICE		= 2
	INFO		= 3
	ERROR		= 4
	WARNING		= 5
	CRITICAL 	= 6
	EMERGENCY 	= 7
)
