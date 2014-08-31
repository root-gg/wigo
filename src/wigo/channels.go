package wigo

import "os"


var Channels *Chans


type Chans struct {
	ChanWatch		chan Event
	ChanChecks		chan Event
	ChanSocket		chan Event
	ChanResults		chan Event
	ChanCallbacks	chan Event
	ChanSignals		chan os .Signal
}

func InitChannels(){
	Channels = new(Chans)
	Channels.ChanWatch 		= make(chan Event)
	Channels.ChanChecks 	= make(chan Event)
	Channels.ChanSocket 	= make(chan Event)
	Channels.ChanResults 	= make(chan Event)
	Channels.ChanCallbacks 	= make(chan Event)
	Channels.ChanSignals 	= make(chan os.Signal)
}
