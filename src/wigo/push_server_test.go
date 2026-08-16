package wigo

import (
	"net"
	"testing"
	"time"
)

// newTestPushServer builds a push server without binding any socket.
func newTestPushServer(t *testing.T) *PushServer {
	t.Helper()

	authority := newTestAuthority(t)

	this := new(PushServer)
	this.config = authority.config
	this.authority = authority

	return this
}

func TestNewHelloRequest(t *testing.T) {

	wigo := setupTestWigo(t, "databases")

	req := NewHelloRequest([]byte("signature"))

	if req.Hostname != wigo.GetHostname() || req.Uuid != wigo.Uuid {
		t.Errorf("Got hostname %q and uuid %q, expected the local ones", req.Hostname, req.Uuid)
	}
	if string(req.UuidSignature) != "signature" {
		t.Errorf("The signature has not been carried over")
	}
}

func TestNewUpdateRequest(t *testing.T) {

	wigo := setupTestWigo(t, "databases")
	host := wigo.GetLocalHost()
	host.Probes.Set("load", newTestProbe(host, "load", 300))

	req := NewUpdateRequest(wigo, "token")

	if req.Uuid != wigo.Uuid || req.Token != "token" {
		t.Errorf("Got uuid %q and token %q", req.Uuid, req.Token)
	}
	if req.WigoHostname != "test-host" {
		t.Errorf("WigoHostname = %s, expected test-host", req.WigoHostname)
	}

	decoded, err := NewWigoFromJson([]byte(req.WigoJson), 0)
	if err != nil {
		t.Fatalf("The embedded payload is not a valid wigo : %s", err)
	}
	if _, ok := decoded.GetLocalHost().Probes.Get("load"); !ok {
		t.Errorf("The probes are missing from the payload")
	}
}

func TestPushServerRegister(t *testing.T) {

	setupTestWigo(t, "databases")
	server := newTestPushServer(t)

	reply := false
	if err := server.Register(*NewHelloRequestFor(testClientUuid, "db-1"), &reply); err != nil {
		t.Fatalf("Register() returned an error : %s", err)
	}

	// Without auto accept the client only lands in the waiting list
	if !server.authority.IsWaiting(testClientUuid) {
		t.Errorf("The client is not in the waiting list")
	}
	if server.authority.IsAllowed(testClientUuid) {
		t.Errorf("The client should not be allowed")
	}
}

func TestPushServerRegisterWithAutoAccept(t *testing.T) {

	setupTestWigo(t, "databases")
	server := newTestPushServer(t)
	server.config.AutoAcceptClients = true

	reply := false
	if err := server.Register(*NewHelloRequestFor(testClientUuid, "db-1"), &reply); err != nil {
		t.Fatalf("Register() returned an error : %s", err)
	}

	if !server.authority.IsAllowed(testClientUuid) {
		t.Errorf("The client should have been accepted automatically")
	}
}

func TestPushServerGetServerCertificate(t *testing.T) {

	setupTestWigo(t, "databases")
	server := newTestPushServer(t)

	var cert []byte
	if err := server.GetServerCertificate(*NewHelloRequestFor(testClientUuid, "db-1"), &cert); err != nil {
		t.Fatalf("GetServerCertificate() returned an error : %s", err)
	}

	// The certificate is public, an unknown client can fetch it
	if len(cert) == 0 {
		t.Errorf("No certificate has been sent")
	}
}

func TestPushServerGetUuidSignature(t *testing.T) {

	setupTestWigo(t, "databases")
	server := newTestPushServer(t)
	req := NewHelloRequestFor(testClientUuid, "db-1")

	var sig []byte

	// An unknown client gets nothing
	if err := server.GetUuidSignature(*req, &sig); err == nil || err.Error() != "NOT ALLOWED" {
		t.Errorf("Got %v, expected NOT ALLOWED", err)
	}

	// A waiting client is told to wait
	server.authority.AddClientToWaitingList(testClientUuid, "db-1")
	if err := server.GetUuidSignature(*req, &sig); err == nil || err.Error() != "WAITING" {
		t.Errorf("Got %v, expected WAITING", err)
	}

	// An allowed client gets its uuid signed
	server.authority.AllowClient(testClientUuid)
	if err := server.GetUuidSignature(*req, &sig); err != nil {
		t.Fatalf("GetUuidSignature() returned an error : %s", err)
	}
	if err := server.authority.VerifyUuidSignature(testClientUuid, sig); err != nil {
		t.Errorf("The returned signature is not valid : %s", err)
	}
}

func TestPushServerHello(t *testing.T) {

	setupTestWigo(t, "databases")
	server := newTestPushServer(t)

	req := NewHelloRequestFor(testClientUuid, "db-1")
	token := ""

	// An unknown client cannot say hello
	if err := server.Hello(*req, &token); err == nil || err.Error() != "NOT ALLOWED" {
		t.Errorf("Got %v, expected NOT ALLOWED", err)
	}

	server.authority.AddClientToWaitingList(testClientUuid, "db-1")
	if err := server.Hello(*req, &token); err == nil || err.Error() != "WAITING" {
		t.Errorf("Got %v, expected WAITING", err)
	}

	server.authority.AllowClient(testClientUuid)

	// Allowed but without a valid signature
	req.UuidSignature = []byte("not a signature")
	if err := server.Hello(*req, &token); err == nil || err.Error() != "NOT ALLOWED" {
		t.Errorf("Got %v, expected NOT ALLOWED", err)
	}

	signature, err := server.authority.GetUuidSignature(testClientUuid, "db-1")
	if err != nil {
		t.Fatalf("GetUuidSignature() returned an error : %s", err)
	}
	req.UuidSignature = signature

	if err := server.Hello(*req, &token); err != nil {
		t.Fatalf("Hello() returned an error : %s", err)
	}
	if token == "" {
		t.Fatalf("No token has been returned")
	}
	if err := server.authority.VerifyToken(testClientUuid, token); err != nil {
		t.Errorf("The returned token is not valid : %s", err)
	}
}

func TestPushServerUpdate(t *testing.T) {

	wigo := setupTestWigo(t, "databases")
	server := newTestPushServer(t)

	token := allowTestClient(t, server)

	client := newTestRemoteWigo(testClientUuid, "db-1", "databases")
	client.LocalHost.Probes.Set("load", newTestProbe(client.LocalHost, "load", 300))

	reply := false
	if err := server.Update(*NewUpdateRequest(client, token), &reply); err != nil {
		t.Fatalf("Update() returned an error : %s", err)
	}

	tmp, ok := wigo.RemoteWigos.Get(testClientUuid)
	if !ok {
		t.Fatalf("The client data has not been stored")
	}
	if _, ok := tmp.(*Wigo).GetLocalHost().Probes.Get("load"); !ok {
		t.Errorf("The client probes have not been stored")
	}
}

func TestPushServerUpdateWithoutValidToken(t *testing.T) {

	wigo := setupTestWigo(t, "databases")
	server := newTestPushServer(t)
	allowTestClient(t, server)

	client := newTestRemoteWigo(testClientUuid, "db-1", "databases")

	reply := false
	if err := server.Update(*NewUpdateRequest(client, "stolen-token"), &reply); err == nil || err.Error() != "NOT ALLOWED" {
		t.Errorf("Got %v, expected NOT ALLOWED", err)
	}
	if wigo.RemoteWigos.Count() != 0 {
		t.Errorf("Data has been stored for an unauthenticated client")
	}
}

// Old clients push an empty payload, they must be told to upgrade.
func TestPushServerUpdateFromLegacyClient(t *testing.T) {

	setupTestWigo(t, "databases")
	server := newTestPushServer(t)
	token := allowTestClient(t, server)

	req := NewRequest(testClientUuid, token)
	reply := false

	if err := server.Update(UpdateRequest{Request: req, WigoJson: "", WigoHostname: "db-1"}, &reply); err == nil || err.Error() != "TOO OLD WIGO CLIENT" {
		t.Errorf("Got %v, expected TOO OLD WIGO CLIENT", err)
	}
}

func TestPushServerGoodbye(t *testing.T) {

	setupTestWigo(t, "databases")
	server := newTestPushServer(t)
	token := allowTestClient(t, server)

	reply := false
	if err := server.Goodbye(*NewRequest(testClientUuid, token), &reply); err != nil {
		t.Fatalf("Goodbye() returned an error : %s", err)
	}

	// The token is revoked, the client has to say hello again
	if err := server.authority.VerifyToken(testClientUuid, token); err == nil {
		t.Errorf("The token is still valid after a goodbye")
	}

	if err := server.Goodbye(*NewRequest(testClientUuid, token), &reply); err == nil || err.Error() != "NOT ALLOWED" {
		t.Errorf("Got %v, expected NOT ALLOWED", err)
	}
}

// Accept returns a nil connection along with the error, so the failure path
// must not touch it. Closing the listener has to end the loop instead of
// spinning on it.
func TestPushServerAcceptConnections(t *testing.T) {

	setupTestWigo(t, "databases")
	server := newTestPushServer(t)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Fail to listen : %s", err)
	}

	stopped := make(chan bool)
	go func() {
		server.acceptConnections(listener)
		stopped <- true
	}()

	// A connection is served
	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("Fail to connect to the test server : %s", err)
	}
	conn.Close()

	listener.Close()

	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatalf("The accept loop is still running after the listener has been closed")
	}
}

// NewHelloRequestFor builds a hello request for an arbitrary client instead of
// the local wigo, which is what the server sees on the wire.
func NewHelloRequestFor(uuid string, hostname string) *HelloRequest {
	req := new(HelloRequest)
	req.Uuid = uuid
	req.Hostname = hostname

	return req
}

// allowTestClient runs the whole registration handshake and returns a valid
// session token.
func allowTestClient(t *testing.T, server *PushServer) string {
	t.Helper()

	req := NewHelloRequestFor(testClientUuid, "db-1")

	reply := false
	if err := server.Register(*req, &reply); err != nil {
		t.Fatalf("Register() returned an error : %s", err)
	}
	if err := server.authority.AllowClient(testClientUuid); err != nil {
		t.Fatalf("AllowClient() returned an error : %s", err)
	}

	signature, err := server.authority.GetUuidSignature(testClientUuid, "db-1")
	if err != nil {
		t.Fatalf("GetUuidSignature() returned an error : %s", err)
	}
	req.UuidSignature = signature

	token := ""
	if err := server.Hello(*req, &token); err != nil {
		t.Fatalf("Hello() returned an error : %s", err)
	}

	return token
}
