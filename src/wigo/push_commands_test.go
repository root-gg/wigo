package wigo

import (
	"encoding/json"
	"strings"
	"testing"
)

const commandTestUuid = "8f14e45f-ceea-467a-9b0c-000000000001"

func resetPushCommands() {
	pushCommands.Lock()
	defer pushCommands.Unlock()

	pushCommands.pending = make(map[string][]ProbeCommand)
	pushCommands.accepted = make(map[string]bool)
	pushCommands.schedule = make(map[string][]ProbeLocation)
	pushCommands.skipped = make(map[string][]string)
	pushCommands.disabled = make(map[string][]ProbeDisableRecord)
}

// A client cannot be asked anything, so a probe it disabled is invisible from
// the server unless the client reports its whole schedule. Never having
// reported has to be distinguishable from having no probe at all, otherwise
// the server would claim a client is fully monitored when it simply predates
// this.
func TestClientProbesSchedule(t *testing.T) {
	resetPushCommands()

	if _, _, _, reported := ClientProbesSchedule(commandTestUuid); reported {
		t.Errorf("A client that never reported must not look like one with no probe")
	}

	SetClientProbesSchedule(commandTestUuid, []ProbeLocation{
		{Name: "check_load", Directory: "60", Interval: 60, Enabled: true},
		{Name: "smart", Directory: ExampleProbesDirectory, Enabled: false},
	}, []string{"check_mdadm"}, []ProbeDisableRecord{
		{Probe: "smart", Reason: "disk swapped out", Author: "germain from 10.0.0.2", Interval: 300},
	})

	locations, skipped, disabled, reported := ClientProbesSchedule(commandTestUuid)
	if !reported {
		t.Fatalf("The schedule should have been recorded")
	}
	if len(locations) != 2 || locations[1].Name != "smart" || locations[1].Enabled {
		t.Errorf("Got %+v", locations)
	}

	// A probe that asked not to be run again is scheduled and has no result, so
	// the server cannot tell it apart from one that never ran unless told
	if len(skipped) != 1 || skipped[0] != "check_mdadm" {
		t.Errorf("Got %+v, expected check_mdadm to be reported as skipped", skipped)
	}

	// And a probe somebody turned off is not one that was never turned on, so
	// the reason travels with the schedule too
	if len(disabled) != 1 || disabled[0].Probe != "smart" || disabled[0].Reason != "disk swapped out" {
		t.Errorf("Got %+v, expected the reason smart was turned off", disabled)
	}

	// A client with genuinely no probe reports an empty list, which is a report
	SetClientProbesSchedule(commandTestUuid, []ProbeLocation{}, nil, nil)
	locations, skipped, disabled, reported = ClientProbesSchedule(commandTestUuid)
	if !reported || len(locations) != 0 || len(skipped) != 0 || len(disabled) != 0 {
		t.Errorf("Got %+v %+v %+v reported=%v, expected an empty but present schedule",
			locations, skipped, disabled, reported)
	}
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

// Three things can make a host read only, and each is fixed in a different
// file. A screen that names the wrong one sends somebody editing a setting that
// was never the problem -- which is what a hardcoded "set AllowWriteActions in
// [Http]" did to every push client that had simply not opted in.
func TestTheReasonForBeingReadOnlyNamesTheRightSetting(t *testing.T) {
	setupTestWigo(t, "databases")
	LocalWigo.config.Http.AllowWriteActions = true

	// A client that pushes and has not opted into being driven
	SetClientAcceptsRemoteControl("uuid-shy", false)
	if ClientAcceptsRemoteControl("uuid-shy") {
		t.Fatalf("Expected the client to be refusing")
	}

	// And one that has
	SetClientAcceptsRemoteControl("uuid-open", true)
	if !ClientAcceptsRemoteControl("uuid-open") {
		t.Fatalf("Expected the client to accept")
	}

	// What this host says about its own writes, which is the other reason and
	// has to keep naming [Http].
	_, refusal, mayWrite := httpWriteActionsAllowed(testRequest(t))
	if !mayWrite {
		t.Fatalf("Writes are on and the caller is an operator, got %q", refusal)
	}

	LocalWigo.config.Http.AllowWriteActions = false
	_, refusal, mayWrite = httpWriteActionsAllowed(testRequest(t))
	if mayWrite {
		t.Fatalf("Expected writes to be refused")
	}
	if !strings.Contains(refusal, "AllowWriteActions") || !strings.Contains(refusal, "[Http]") {
		t.Errorf("Got %q, expected it to name the setting and its section", refusal)
	}
}

// A client that closes its door again must not keep the orders queued while it
// was open : they would be applied the day it opens for another reason.
func TestTheReasonTravelsWithTheSchedule(t *testing.T) {
	setupTestWigo(t, "databases")
	LocalWigo.config.Http.AllowWriteActions = false

	_, refusal, _ := httpWriteActionsAllowed(testRequest(t))

	schedule := ProbesSchedule{
		Hostname:            "db1",
		WriteActionsAllowed: false,
		ReadOnlyReason:      refusal,
	}

	encoded, err := json.Marshal(schedule)
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if !strings.Contains(string(encoded), "ReadOnlyReason") {
		t.Errorf("The reason has to reach the interface : %s", encoded)
	}

	// And it is left out entirely when writes are allowed, so an older wigo
	// answering without it is not mistaken for one that refused silently.
	allowed := ProbesSchedule{Hostname: "db1", WriteActionsAllowed: true}
	encoded, err = json.Marshal(allowed)
	if err != nil {
		t.Fatalf("Unexpected error : %s", err)
	}
	if strings.Contains(string(encoded), "ReadOnlyReason") {
		t.Errorf("Got %s, expected no reason when writes are allowed", encoded)
	}
}
