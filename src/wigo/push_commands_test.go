package wigo

import (
	"testing"
)

const commandTestUuid = "8f14e45f-ceea-467a-9b0c-000000000001"

func resetPushCommands() {
	pushCommands.Lock()
	defer pushCommands.Unlock()

	pushCommands.pending = make(map[string][]ProbeCommand)
	pushCommands.accepted = make(map[string]bool)
}

// A client that never said anything -- including one running a version that
// knows nothing about orders -- must not be queued for. That is what makes an
// upgrade of the server alone leave every client closed.
func TestQueueRefusesSilentClient(t *testing.T) {
	resetPushCommands()

	if ClientAcceptsRemoteControl(commandTestUuid) {
		t.Errorf("An unknown client must not be reported as accepting orders")
	}

	err := QueueProbeCommand(commandTestUuid, ProbeCommand{Action: CommandDisableProbe, Probe: "check_load"})
	if err == nil {
		t.Fatalf("Queueing for a client that never accepted should fail")
	}
	if PendingProbeCommands(commandTestUuid) != 0 {
		t.Errorf("Nothing should have been queued")
	}
}

func TestQueueAndTakeCommands(t *testing.T) {
	resetPushCommands()
	SetClientAcceptsRemoteControl(commandTestUuid, true)

	if err := QueueProbeCommand(commandTestUuid, ProbeCommand{Action: CommandDisableProbe, Probe: "check_load"}); err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if err := QueueProbeCommand(commandTestUuid, ProbeCommand{Action: CommandSetProbeInterval, Probe: "smart", Interval: 900}); err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}

	if PendingProbeCommands(commandTestUuid) != 2 {
		t.Errorf("Got %d orders waiting, expected 2", PendingProbeCommands(commandTestUuid))
	}

	commands := takeProbeCommands(commandTestUuid)
	if len(commands) != 2 || commands[0].Probe != "check_load" || commands[1].Interval != 900 {
		t.Errorf("Got %+v", commands)
	}

	// Handed over once, never twice
	if PendingProbeCommands(commandTestUuid) != 0 {
		t.Errorf("The orders should have been forgotten once handed over")
	}
	if len(takeProbeCommands(commandTestUuid)) != 0 {
		t.Errorf("Asking again should return nothing")
	}
}

// Orders waiting for a client that closes its door would otherwise be applied
// the day it opens it again for an unrelated reason.
func TestClosingTheDoorDropsPendingCommands(t *testing.T) {
	resetPushCommands()
	SetClientAcceptsRemoteControl(commandTestUuid, true)

	if err := QueueProbeCommand(commandTestUuid, ProbeCommand{Action: CommandDisableProbe, Probe: "check_load"}); err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}

	SetClientAcceptsRemoteControl(commandTestUuid, false)

	if PendingProbeCommands(commandTestUuid) != 0 {
		t.Errorf("The waiting orders should have been dropped")
	}
	if err := QueueProbeCommand(commandTestUuid, ProbeCommand{Action: CommandDisableProbe, Probe: "check_load"}); err == nil {
		t.Errorf("Queueing should be refused again")
	}
}

// A client that stops asking must not make the server grow without end.
func TestQueueIsBounded(t *testing.T) {
	resetPushCommands()
	SetClientAcceptsRemoteControl(commandTestUuid, true)

	for i := 0; i < maxPendingCommandsPerClient+10; i++ {
		if err := QueueProbeCommand(commandTestUuid, ProbeCommand{Action: CommandDisableProbe, Probe: "check_load"}); err != nil {
			t.Fatalf("Unexpected error : %s", err)
		}
	}

	if PendingProbeCommands(commandTestUuid) != maxPendingCommandsPerClient {
		t.Errorf("Got %d orders waiting, expected the queue to stop at %d",
			PendingProbeCommands(commandTestUuid), maxPendingCommandsPerClient)
	}
}

func TestQueueRefusesEmptyUuid(t *testing.T) {
	resetPushCommands()

	if err := QueueProbeCommand("", ProbeCommand{Action: CommandDisableProbe, Probe: "check_load"}); err == nil {
		t.Errorf("Queueing without a client should fail")
	}
}

// An order carries a probe name and an interval straight from another machine.
// They go through the same checks as anything arriving over HTTP, so a badly
// behaved server cannot reach outside the probes directory.
func TestApplyProbeCommandValidatesItsInput(t *testing.T) {

	config := new(Config)
	config.Global = new(GeneralConfig)
	config.Global.ProbesDirectory = newTestProbesDirectory(t, "60/check_load")
	LocalWigo.config = config

	if err := ApplyProbeCommand(ProbeCommand{Action: CommandDisableProbe, Probe: "../../etc/passwd"}); err == nil {
		t.Errorf("A traversing probe name should be refused")
	}
	if err := ApplyProbeCommand(ProbeCommand{Action: CommandSetProbeInterval, Probe: "check_load", Interval: 1}); err == nil {
		t.Errorf("An out of range interval should be refused")
	}
	if err := ApplyProbeCommand(ProbeCommand{Action: "reboot", Probe: "check_load"}); err == nil {
		t.Errorf("An unknown order should be refused")
	}

	// A well formed one goes through
	if err := ApplyProbeCommand(ProbeCommand{Action: CommandSetProbeInterval, Probe: "check_load", Interval: 300}); err != nil {
		t.Errorf("Unexpected error : %s", err)
	}
	if !probeIsIn(t, config.Global.ProbesDirectory, "300", "check_load") {
		t.Errorf("The probe has not been moved")
	}
}
