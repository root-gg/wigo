package wigo

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newTestAuthority builds an authority backed by a freshly generated self
// signed certificate, as the push server would use in production.
func newTestAuthority(t *testing.T) *Authority {
	t.Helper()

	directory := t.TempDir()
	certFile := filepath.Join(directory, "wigo.crt")
	keyFile := filepath.Join(directory, "wigo.key")

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Fail to generate the test private key : %s", err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "wigo-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IsCA:         true,
	}

	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("Fail to generate the test certificate : %s", err)
	}

	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600); err != nil {
		t.Fatalf("Fail to write the test certificate : %s", err)
	}
	pemKey := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	if err := os.WriteFile(keyFile, pemKey, 0600); err != nil {
		t.Fatalf("Fail to write the test private key : %s", err)
	}

	config := new(PushServerConfig)
	config.SslCert = certFile
	config.SslKey = keyFile
	config.AllowedClientsFile = filepath.Join(directory, "allowed")
	config.MaxWaitingClients = 2

	return NewAuthority(config)
}

const testClientUuid = "7ebd737f-e424-4fd5-77d0-24205f651111"
const otherClientUuid = "7ebd737f-e424-4fd5-77d0-24205f652222"

func TestNewAuthority(t *testing.T) {

	setupTestWigo(t, "databases")
	authority := newTestAuthority(t)

	if len(authority.GetServerCertificate()) == 0 {
		t.Errorf("The server certificate has not been loaded")
	}
	if len(authority.Allowed) != 0 || len(authority.Waiting) != 0 || len(authority.Tokens) != 0 {
		t.Errorf("A brand new authority should have no client")
	}
}

func TestAuthorityWaitingList(t *testing.T) {

	setupTestWigo(t, "databases")
	authority := newTestAuthority(t)

	if err := authority.AddClientToWaitingList(testClientUuid, "db-1"); err != nil {
		t.Fatalf("AddClientToWaitingList() returned an error : %s", err)
	}

	if !authority.IsWaiting(testClientUuid) {
		t.Errorf("The client should be in the waiting list")
	}
	if authority.IsAllowed(testClientUuid) {
		t.Errorf("The client should not be allowed yet")
	}

	// A new client raises a notification carrying its hostname
	notification := <-Channels.ChanCallbacks
	if notification.GetHostname() != "db-1" {
		t.Errorf("Hostname = %s, expected db-1", notification.GetHostname())
	}

	// Registering twice is a no-op and does not notify again
	if err := authority.AddClientToWaitingList(testClientUuid, "db-1"); err != nil {
		t.Errorf("AddClientToWaitingList() returned an error : %s", err)
	}
	if len(Channels.ChanCallbacks) != 0 {
		t.Errorf("The same client has been notified twice")
	}
}

func TestAuthorityWaitingListIsBounded(t *testing.T) {

	setupTestWigo(t, "databases")
	authority := newTestAuthority(t)

	// MaxWaitingClients is two in the test configuration
	authority.AddClientToWaitingList(testClientUuid, "db-1")
	authority.AddClientToWaitingList(otherClientUuid, "db-2")

	if err := authority.AddClientToWaitingList("7ebd737f-e424-4fd5-77d0-24205f653333", "db-3"); err == nil {
		t.Errorf("Expected an error once the waiting list is full")
	}
	if len(authority.Waiting) != 2 {
		t.Errorf("Got %d waiting clients, expected the list to be capped at 2", len(authority.Waiting))
	}
}

func TestAuthorityAllowClient(t *testing.T) {

	setupTestWigo(t, "databases")
	authority := newTestAuthority(t)

	// A client has to be in the waiting list first
	if err := authority.AllowClient(testClientUuid); err == nil {
		t.Errorf("Expected an error for an unknown client")
	}

	authority.AddClientToWaitingList(testClientUuid, "db-1")
	if err := authority.AllowClient(testClientUuid); err != nil {
		t.Fatalf("AllowClient() returned an error : %s", err)
	}

	if !authority.IsAllowed(testClientUuid) {
		t.Errorf("The client should be allowed")
	}
	if authority.IsWaiting(testClientUuid) {
		t.Errorf("The client should have left the waiting list")
	}
}

// The allowed list is persisted so clients survive a restart.
func TestAuthorityAllowedListPersistence(t *testing.T) {

	setupTestWigo(t, "databases")
	authority := newTestAuthority(t)

	authority.AddClientToWaitingList(testClientUuid, "db-1")
	authority.AllowClient(testClientUuid)

	reloaded := NewAuthority(authority.config)

	if !reloaded.IsAllowed(testClientUuid) {
		t.Fatalf("The allowed client has not been reloaded")
	}
	if reloaded.Allowed[testClientUuid] != "db-1" {
		t.Errorf("Hostname = %s, expected db-1", reloaded.Allowed[testClientUuid])
	}
}

// Lines that are not a "uuid hostname" pair are skipped instead of aborting the
// whole load.
func TestAuthorityLoadAllowedListIgnoresInvalidLines(t *testing.T) {

	setupTestWigo(t, "databases")
	authority := newTestAuthority(t)

	content := "# a comment\n" + testClientUuid + " db-1\ngarbage\n\n"
	if err := os.WriteFile(authority.config.AllowedClientsFile, []byte(content), 0600); err != nil {
		t.Fatalf("Fail to write the allowed clients file : %s", err)
	}

	reloaded := NewAuthority(authority.config)

	if len(reloaded.Allowed) != 1 || !reloaded.IsAllowed(testClientUuid) {
		t.Errorf("Got %v, expected only the valid line to be loaded", reloaded.Allowed)
	}
}

func TestAuthorityUpdateAllowedHostname(t *testing.T) {

	setupTestWigo(t, "databases")
	authority := newTestAuthority(t)

	authority.AddClientToWaitingList(testClientUuid, "db-1")
	authority.AllowClient(testClientUuid)

	authority.UpdateAllowedHostname(testClientUuid, "db-1-renamed")
	if authority.Allowed[testClientUuid] != "db-1-renamed" {
		t.Errorf("Hostname = %s, expected db-1-renamed", authority.Allowed[testClientUuid])
	}

	// The change is persisted right away
	reloaded := NewAuthority(authority.config)
	if reloaded.Allowed[testClientUuid] != "db-1-renamed" {
		t.Errorf("Hostname = %s, expected the new hostname to be persisted", reloaded.Allowed[testClientUuid])
	}

	// An unknown client is ignored
	authority.UpdateAllowedHostname(otherClientUuid, "db-2")
	if _, ok := authority.Allowed[otherClientUuid]; ok {
		t.Errorf("An unknown client has been added to the allowed list")
	}
}

func TestAuthorityRevokeClient(t *testing.T) {

	wigo := setupTestWigo(t, "databases")
	authority := newTestAuthority(t)

	authority.AddClientToWaitingList(testClientUuid, "db-1")
	authority.AllowClient(testClientUuid)

	token, err := authority.GetToken(testClientUuid)
	if err != nil {
		t.Fatalf("GetToken() returned an error : %s", err)
	}

	wigo.RemoteWigos.Set(testClientUuid, newTestRemoteWigo(testClientUuid, "db-1", "databases"))

	authority.RevokeClient(testClientUuid)

	if authority.IsAllowed(testClientUuid) {
		t.Errorf("The client is still allowed")
	}
	// Its tokens are revoked and its data is dropped
	if err := authority.VerifyToken(testClientUuid, token); err == nil {
		t.Errorf("The token of a revoked client is still valid")
	}
	if _, ok := wigo.RemoteWigos.Get(testClientUuid); ok {
		t.Errorf("The data of the revoked client has not been removed")
	}

	// The revocation is persisted
	reloaded := NewAuthority(authority.config)
	if reloaded.IsAllowed(testClientUuid) {
		t.Errorf("The revoked client came back after a reload")
	}
}

func TestAuthorityRevokeWaitingClient(t *testing.T) {

	setupTestWigo(t, "databases")
	authority := newTestAuthority(t)

	authority.AddClientToWaitingList(testClientUuid, "db-1")
	authority.RevokeClient(testClientUuid)

	if authority.IsWaiting(testClientUuid) {
		t.Errorf("The client is still in the waiting list")
	}
}

func TestAuthorityUuidSignature(t *testing.T) {

	setupTestWigo(t, "databases")
	authority := newTestAuthority(t)

	signature, err := authority.GetUuidSignature(testClientUuid, "db-1")
	if err != nil {
		t.Fatalf("GetUuidSignature() returned an error : %s", err)
	}
	if len(signature) == 0 {
		t.Fatalf("The signature is empty")
	}

	if err := authority.VerifyUuidSignature(testClientUuid, signature); err != nil {
		t.Errorf("The signature of the uuid is not valid : %s", err)
	}

	// A signature is bound to one uuid and cannot be replayed for another
	if err := authority.VerifyUuidSignature(otherClientUuid, signature); err == nil {
		t.Errorf("A signature has been accepted for another uuid")
	}

	// A forged signature is rejected
	forged := make([]byte, len(signature))
	copy(forged, signature)
	forged[0] ^= 0xff
	if err := authority.VerifyUuidSignature(testClientUuid, forged); err == nil {
		t.Errorf("A forged signature has been accepted")
	}

	// A signature made by another authority is rejected
	other := newTestAuthority(t)
	otherSignature, err := other.GetUuidSignature(testClientUuid, "db-1")
	if err != nil {
		t.Fatalf("GetUuidSignature() returned an error : %s", err)
	}
	if err := authority.VerifyUuidSignature(testClientUuid, otherSignature); err == nil {
		t.Errorf("A signature from another authority has been accepted")
	}
}

func TestAuthorityTokens(t *testing.T) {

	setupTestWigo(t, "databases")
	authority := newTestAuthority(t)

	token, err := authority.GetToken(testClientUuid)
	if err != nil {
		t.Fatalf("GetToken() returned an error : %s", err)
	}

	if err := authority.VerifyToken(testClientUuid, token); err != nil {
		t.Errorf("The token is not valid : %s", err)
	}
	// A token belongs to a single client
	if err := authority.VerifyToken(otherClientUuid, token); err == nil {
		t.Errorf("The token has been accepted for another client")
	}
	if err := authority.VerifyToken(testClientUuid, "unknown-token"); err == nil {
		t.Errorf("An unknown token has been accepted")
	}

	// Asking for a new token invalidates the previous one, a client can only
	// hold one connection at a time
	renewed, err := authority.GetToken(testClientUuid)
	if err != nil {
		t.Fatalf("GetToken() returned an error : %s", err)
	}
	if renewed == token {
		t.Errorf("The renewed token is the same as the previous one")
	}
	if err := authority.VerifyToken(testClientUuid, token); err == nil {
		t.Errorf("The previous token is still valid")
	}
	if err := authority.VerifyToken(testClientUuid, renewed); err != nil {
		t.Errorf("The renewed token is not valid : %s", err)
	}
}

func TestAuthorityRevokeToken(t *testing.T) {

	setupTestWigo(t, "databases")
	authority := newTestAuthority(t)

	token, err := authority.GetToken(testClientUuid)
	if err != nil {
		t.Fatalf("GetToken() returned an error : %s", err)
	}

	// Another client cannot revoke someone else's token
	if err := authority.RevokeToken(otherClientUuid, token); err == nil {
		t.Errorf("A token has been revoked by another client")
	}
	if err := authority.VerifyToken(testClientUuid, token); err != nil {
		t.Errorf("The token should still be valid : %s", err)
	}

	if err := authority.RevokeToken(testClientUuid, token); err != nil {
		t.Errorf("RevokeToken() returned an error : %s", err)
	}
	if err := authority.VerifyToken(testClientUuid, token); err == nil {
		t.Errorf("The revoked token is still valid")
	}

	// Revoking an unknown token is an error
	if err := authority.RevokeToken(testClientUuid, "unknown-token"); err == nil {
		t.Errorf("Expected an error when revoking an unknown token")
	}
}

func TestAuthoritySaveAllowedListFormat(t *testing.T) {

	setupTestWigo(t, "databases")
	authority := newTestAuthority(t)

	authority.AddClientToWaitingList(testClientUuid, "db-1")
	authority.AllowClient(testClientUuid)

	content, err := os.ReadFile(authority.config.AllowedClientsFile)
	if err != nil {
		t.Fatalf("Fail to read the allowed clients file : %s", err)
	}

	if strings.TrimSpace(string(content)) != testClientUuid+" db-1" {
		t.Errorf("Got %q, expected one \"uuid hostname\" line", string(content))
	}

	// The file may hold credentials of the monitoring fleet, keep it private
	info, err := os.Stat(authority.config.AllowedClientsFile)
	if err != nil {
		t.Fatalf("Fail to stat the allowed clients file : %s", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("Mode = %v, expected 0600", info.Mode().Perm())
	}
}
