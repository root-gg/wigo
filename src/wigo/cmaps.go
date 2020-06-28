package wigo

import (
	"encoding/json"
	"github.com/orcaman/concurrent-map"
)

type concurrentMapWigos struct {
	cmap.ConcurrentMap
}


func NewConcurrentMapWigos() *concurrentMapWigos {
	return &concurrentMapWigos{
		ConcurrentMap: cmap.New(),
	}
}

func (m *concurrentMapWigos) UnmarshalJSON(b []byte) (err error) {
	// Reverse process of Marshal.

	tmp := make(map[string]*Wigo)

	// Unmarshal into a single map.
	if err := json.Unmarshal(b, &tmp); err != nil {
		return nil
	}

	_m := NewConcurrentMapWigos()
	// foreach key,value pair in temporary map insert into our concurrent map.
	for key, val := range tmp {
		_m.Set(key, val)
	}
	*m = *_m
	return nil
}

type concurrentMapProbes struct {
	cmap.ConcurrentMap
}

func NewConcurrentMapProbes() *concurrentMapProbes {
	return &concurrentMapProbes{
		ConcurrentMap: cmap.New(),
	}
}


func (m *concurrentMapProbes) UnmarshalJSON(b []byte) (err error) {
	// Reverse process of Marshal.

	tmp := make(map[string]*ProbeResult)

	// Unmarshal into a single map.
	if err := json.Unmarshal(b, &tmp); err != nil 	{
		return nil
	}

	_m := NewConcurrentMapProbes()
	// foreach key,value pair in temporary map insert into our concurrent map.
	for key, val := range tmp {
		_m.Set(key, val)
	}
	*m = *_m
	return nil
}