package wigo

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

// Telling the interface something happened, rather than making it ask.
//
// The dashboard polls, and a probe that goes critical waits for the next poll
// to appear -- up to a minute by default. That is a long time to be looking at
// a green screen about a machine that is already down.
//
// What travels is deliberately thin : what changed, not what it changed to. The
// browser refetches, which keeps one serialisation of the state instead of two
// and means an event that is missed costs nothing. A subscriber that cannot
// keep up has its events dropped rather than slowing down the scheduler, for
// the same reason : the next one, or the periodic refresh, catches it up.

// LiveEvent is something worth telling an open interface about.
//
// Named apart from the Event of event.go, which is the internal directory watch
// and has nothing to do with this.
type LiveEvent struct {
	// What kind of thing changed : a probe result, a host coming up or going
	// down, or the probes schedule.
	Type string

	Host  string
	Probe string

	Status int
	At     int64
}

const (
	EventProbe    = "probe"
	EventHost     = "host"
	EventSchedule = "schedule"
)

// A browser that stops reading must not hold anything up. Enough room for a
// burst, small enough that a dead connection cannot cost much memory.
const eventBufferSize = 32

var events = struct {
	sync.Mutex
	subscribers map[int]chan LiveEvent
	nextId      int
}{subscribers: make(map[int]chan LiveEvent)}

// SubscribeEvents opens a stream, and returns the way to close it.
//
// The unsubscribe function must be called, or the subscriber leaks : it is
// deliberately returned rather than tied to a context so the caller cannot
// forget it exists.
func SubscribeEvents() (<-chan LiveEvent, func()) {
	stream := make(chan LiveEvent, eventBufferSize)

	events.Lock()
	id := events.nextId
	events.nextId++
	events.subscribers[id] = stream
	events.Unlock()

	return stream, func() {
		events.Lock()
		defer events.Unlock()

		if existing, ok := events.subscribers[id]; ok {
			delete(events.subscribers, id)
			close(existing)
		}
	}
}

// PublishEvent tells every open interface, and blocks for none of them.
func PublishEvent(event LiveEvent) {
	if event.At == 0 {
		event.At = time.Now().Unix()
	}

	events.Lock()
	defer events.Unlock()

	for _, stream := range events.subscribers {
		select {
		case stream <- event:
		default:
			// This browser is not keeping up. Dropping is right : the event
			// only says "look again", and the next one will say it too.
		}
	}
}

// EventSubscribers is how many interfaces are currently listening, which is
// worth exposing : a stream that nobody closes is a leak, and this is how it
// would be noticed.
func EventSubscribers() int {
	events.Lock()
	defer events.Unlock()

	return len(events.subscribers)
}

// HttpEventsHandler streams what happens, as server sent events.
//
// Not a wigo.Handler : those answer a status and a body, which is exactly the
// shape a stream does not have. This one writes until the client goes away.
func HttpEventsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// The server has a write timeout, which is right for every other route and
	// fatal here : a stream is meant to outlive it. Cleared for this response
	// only, rather than turning it off for the whole server.
	//
	// Failing to clear it is not a reason to refuse : the stream then lives as
	// long as the write timeout and the browser reconnects, which is degraded
	// and still far better than never streaming at all.
	controller := http.NewResponseController(w)
	if err := controller.SetWriteDeadline(time.Time{}); err != nil {
		log.Printf("Http server : cannot clear the write deadline on the event stream, "+
			"it will be cut every write timeout : %s", err)
	}

	stream, unsubscribe := SubscribeEvents()
	defer unsubscribe()

	// Said once on connect so a browser that reconnects after a nap refetches
	// rather than waiting for the next thing to happen.
	if err := writeEvent(w, controller, LiveEvent{Type: EventProbe, At: time.Now().Unix()}); err != nil {
		return
	}

	// Long enough not to be chatty, short enough that a proxy with an idle
	// timeout does not cut the connection, and that a dead one is noticed.
	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return

		case event, open := <-stream:
			if !open {
				return
			}
			if err := writeEvent(w, controller, event); err != nil {
				return
			}

		case <-heartbeat.C:
			// A comment line : valid, ignored by the client, and enough to
			// keep the connection alive and to notice a dead one.
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
			if err := controller.Flush(); err != nil {
				return
			}
		}
	}
}

func writeEvent(w http.ResponseWriter, controller *http.ResponseController, event LiveEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventName(event), body); err != nil {
		return err
	}

	return controller.Flush()
}

func eventName(event LiveEvent) string {
	if event.Type == "" {
		return "message"
	}

	return event.Type
}
