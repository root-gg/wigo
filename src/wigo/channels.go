package wigo

import "os"


var Channels *Chans


type Chans struct {
	ChanWatch		chan Event
	ChanChecks		chan Event
	ChanCallbacks	chan INotification
	ChanSignals		chan os .Signal
}

func InitChannels(){
	Channels = new(Chans)
	Channels.ChanWatch 		= make(chan Event)
	Channels.ChanChecks 	= make(chan Event)
	Channels.ChanCallbacks 	= make(chan INotification)
	Channels.ChanSignals 	= make(chan os.Signal)
}
