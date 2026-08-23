package wigo

import (
	"fmt"
	"log"
	"sync"
)

// Orders a push server holds for its clients.
//
// A client sits behind a NAT and cannot be called, so it comes and asks for its
// orders on the connection it already keeps open. Nothing is ever pushed at it.
//
// Two things have to be true before an order is even recorded : the client must
// have said it accepts being driven, which it reports on every update, and the
// server must be allowed to perform write actions at all. A client that never
// said anything -- including one running a version that does not know about any
// of this -- counts as refusing.

const (
	CommandDisableProbe     = "disable"
	CommandSetProbeInterval = "interval"
)

// A client that stops asking for its orders must not make the server grow
// without end. Beyond this the oldest are dropped.
const maxPendingCommandsPerClient = 32

// ProbeCommand is one order for one probe.
type ProbeCommand struct {
	Action   string
	Probe    string
	Interval int
}

// CommandBatch is what a client gets back when it asks for its orders.
// A struct rather than a slice so fields can be added later without breaking
// the wire in either direction.
type CommandBatch struct {
	Commands []ProbeCommand
}

var pushCommands = struct {
	sync.Mutex
	pending  map[string][]ProbeCommand
	accepted map[string]bool
}{
	pending:  make(map[string][]ProbeCommand),
	accepted: make(map[string]bool),
}

// SetClientAcceptsRemoteControl records what a client said about being driven.
// Reported on every update, so it follows a configuration change on the client
// without anything to do here.
func SetClientAcceptsRemoteControl(uuid string, accepts bool) {
	if uuid == "" {
		return
	}

	pushCommands.Lock()
	defer pushCommands.Unlock()

	pushCommands.accepted[uuid] = accepts

	// Orders already queued for a client that just closed its door would sit
	// there forever, and would be applied if it opened it again for another
	// reason. Drop them.
	if !accepts {
		delete(pushCommands.pending, uuid)
	}
}

// ClientAcceptsRemoteControl reports whether a client said it accepts being
// driven. Unknown clients count as refusing.
func ClientAcceptsRemoteControl(uuid string) bool {
	pushCommands.Lock()
	defer pushCommands.Unlock()

	return pushCommands.accepted[uuid]
}

// QueueProbeCommand records an order for a client to pick up.
func QueueProbeCommand(uuid string, command ProbeCommand) error {
	if uuid == "" {
		return fmt.Errorf("no client to queue this for")
	}

	pushCommands.Lock()
	defer pushCommands.Unlock()

	if !pushCommands.accepted[uuid] {
		return fmt.Errorf("this client does not accept being driven")
	}

	queue := append(pushCommands.pending[uuid], command)
	if len(queue) > maxPendingCommandsPerClient {
		dropped := len(queue) - maxPendingCommandsPerClient
		log.Printf("Push server : dropping %d order(s) for client %s, it is not asking for them", dropped, uuid)
		queue = queue[dropped:]
	}
	pushCommands.pending[uuid] = queue

	return nil
}

// takeProbeCommands hands a client its orders and forgets them.
func takeProbeCommands(uuid string) []ProbeCommand {
	pushCommands.Lock()
	defer pushCommands.Unlock()

	commands := pushCommands.pending[uuid]
	delete(pushCommands.pending, uuid)

	return commands
}

// PendingProbeCommands counts the orders waiting for a client.
func PendingProbeCommands(uuid string) int {
	pushCommands.Lock()
	defer pushCommands.Unlock()

	return len(pushCommands.pending[uuid])
}

// ApplyProbeCommand carries out an order received from a push server.
//
// It goes through the same functions the local API uses, so a probe name
// arriving over the wire is validated exactly as one arriving over HTTP and an
// interval is bounded the same way. A server has no way to reach outside the
// probes directory of a client, however badly it behaves.
func ApplyProbeCommand(command ProbeCommand) error {
	switch command.Action {

	case CommandDisableProbe:
		return UnscheduleProbe(command.Probe)

	case CommandSetProbeInterval:
		return ScheduleProbe(command.Probe, command.Interval)

	default:
		return fmt.Errorf("unknown order %q", command.Action)
	}
}
