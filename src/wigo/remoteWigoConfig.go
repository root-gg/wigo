package wigo

import (
)

type RemoteWigoConfig struct {
	Hostname        string
	Port            int
	CheckRemotes    bool
	CheckInterval   int
	CheckTries      int
}

// Constructors
func NewRemoteWigoConfig() (this *RemoteWigoConfig) {
	this = new(RemoteWigoConfig)
	return
}
