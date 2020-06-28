package wigo

import (
	"errors"
	"log"

	"bufio"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"io/ioutil"
	"os"
	"regexp"

	"fmt"
	"github.com/nu7hatch/gouuid"
)

// The Authority is responsible to handle the security
// of the push system. The server certificate is used to
// allow the clients to verify the server identity and
// the private key is used to sign the clients uuid
// to allow the server to verify the clients identities.
//
// Allowed client are stored in an allowed list persisted
// on the file system, so one can easily revoke clients.
type Authority struct {
	key        []byte
	privateKey *rsa.PrivateKey

	cert        []byte
	certificate *x509.Certificate

	config *PushServerConfig

	Waiting map[string]string
	Allowed map[string]string
	Tokens  map[string]string
}

func NewAuthority(config *PushServerConfig) (this *Authority) {
	this = new(Authority)
	this.config = config

	// Load CA certificate
	var err error
	if this.cert, err = ioutil.ReadFile(this.config.SslCert); err == nil {
		if block, _ := pem.Decode(this.cert); block != nil {
			if this.certificate, err = x509.ParseCertificate(block.Bytes); err != nil {
				log.Fatal("Authority : Unable to parse x509 certificate")
			}
		} else {
			log.Fatal("Authority : Unable to decode pem certificate")
		}
	} else {
		log.Fatal("Authority : Unable to read certificate")
	}

	// Load CA private key
	if this.key, err = ioutil.ReadFile(this.config.SslKey); err == nil {
		if block, _ := pem.Decode(this.key); block != nil {
			if this.privateKey, err = x509.ParsePKCS1PrivateKey(block.Bytes); err != nil {
				err = errors.New("Authority : Unable to read decode x509 private key")
				log.Println(err)
			}
		} else {
			err = errors.New("Authority : Unable to read decode pem private key")
			log.Println(err)
		}
	} else {
		err = errors.New("Authority : Unable to read private key")
		log.Println(err)
	}

	this.Waiting = make(map[string]string)
	this.Allowed = make(map[string]string)
	this.Tokens = make(map[string]string)
	this.LoadAllowedList()

	return
}

// Check is a given uuid is in the waiting list
func (this *Authority) IsWaiting(uuid string) bool {
	_, ok := this.Waiting[uuid]
	return ok
}

// Check is a given uuid is in the allowed list
func (this *Authority) IsAllowed(uuid string) bool {
	_, ok := this.Allowed[uuid]
	return ok
}

// Return the server certificate
func (this *Authority) GetServerCertificate() []byte {
	return this.cert
}

// Add a client to the waiting list
func (this *Authority) AddClientToWaitingList(uuid string, hostname string) (err error) {
	if _, ok := this.Waiting[uuid]; !ok {
		if len(this.Waiting) < this.config.MaxWaitingClients {
			this.Waiting[uuid] = hostname
			message := fmt.Sprintf("New client %s", hostname)
			SendNotification(NewNotificationFromMessage(message))
			log.Printf("Authority : %s", message)
			LocalWigo.AddLog(LocalWigo, INFO, message)
		} else {
			err = errors.New("Authority : Too many wainting clients")
		}
	}

	return
}

// Load the allowed clients list from the file system
// The file format is one "uuid hostname" per line,
// every non matching line will be ignored
func (this *Authority) LoadAllowedList() (err error) {
	if _, err = os.Stat(this.config.AllowedClientsFile); err == nil {
		file, err := os.Open(this.config.AllowedClientsFile)
		if err != nil {
			log.Fatalf("Authority : Error opening allowed clients file %s", err)
		}
		defer file.Close()

		// Format is 7ebd737f-e424-4fd5-77d0-24205f651111 Hostname
		re, err := regexp.Compile(`([[:xdigit:]]{8}-[[:xdigit:]]{4}-[[:xdigit:]]{4}-[[:xdigit:]]{4}-[[:xdigit:]]{12}) (.+)`)
		if err != nil {
			log.Fatalf("Authority : Invalid allowed client list regexp %s", err)
		}

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {

			line := scanner.Text()
			result := re.FindStringSubmatch(line)

			if len(result) != 3 {
				log.Printf("Authority : Ignoring invalid allowed client line %s", line)
				continue
			}

			// Verify the uuid validity
			uuid, err := uuid.ParseHex(result[1])
			if err != nil {
				log.Fatalf("Authority : Unable to parse allowed client uuid %s : %s", result[1], err)
			}

			this.Allowed[uuid.String()] = result[2]
		}

		if err := scanner.Err(); err != nil {
			log.Fatalf("Authority : Error while loading allowed clients file %s", err)
		}
	}
	return
}

// Persist the allowed clients list on the file system
// The file format is one "uuid hostname" per line,
// every non matching line will be ignored
func (this *Authority) SaveAllowedList() (err error) {
	allowedClientsFile, err := os.OpenFile(this.config.AllowedClientsFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err == nil {
		defer allowedClientsFile.Close()
		for uuid, hostname := range this.Allowed {
			allowedClientsFile.Write([]byte(uuid + " " + hostname + "\n"))
		}
	} else {
		log.Fatalf("Authority : Failed to create uuid file : %s", err)
	}
	return
}

// Add a client to the allowed list. The client have to be
// in the waiting list. // TODO remove this limitation
func (this *Authority) AllowClient(uuid string) (err error) {
	if hostname, ok := this.Waiting[uuid]; ok {
		delete(this.Waiting, uuid)
		this.Allowed[uuid] = hostname
		if err := this.SaveAllowedList(); err != nil {
			// Fatal ?
			log.Println(err)
		}
		message := hostname + " added to allowed list"
		log.Printf("Authority : %s", message)
		LocalWigo.AddLog(LocalWigo, INFO, message)
	} else {
		err = errors.New("Authority : Invalid uuid " + uuid)
	}
	return
}

// Remove a client from the allowed list, revoke tokens if any and
// remove client data.
// TODO The dedup data in gopentsdb are leaked.
func (this *Authority) RevokeClient(uuid string) (err error) {
	if hostname, ok := this.Waiting[uuid]; ok {
		delete(this.Waiting, uuid)
		log.Println("Authority : " + hostname + " removed from waiting list")
	}
	if hostname, ok := this.Allowed[uuid]; ok {
		delete(this.Allowed, uuid)
		if err := this.SaveAllowedList(); err != nil {
			// Fatal ?
			log.Println(err)
		}
		message := hostname + " removed from the allowed list"
		log.Printf("Authority : %s", message)
		LocalWigo.AddLog(LocalWigo, INFO, message)
	}
	for token, u := range this.Tokens {
		if uuid == u {
			delete(this.Tokens, token)
			log.Println("Authority : token " + token + " revoked")
		}
	}
	if tmp, ok := LocalWigo.RemoteWigos.Get(uuid); ok {
		wigo := tmp.(*Wigo)
		LocalWigo.RemoteWigos.Remove(uuid)
		log.Println("Authority : " + wigo.Hostname + " removed")
	}
	return
}

// Sign a client's uuid with the server's private key
func (this *Authority) GetUuidSignature(uuid string, hostname string) (uuidSignature []byte, err error) {
	hash := crypto.SHA256.New()
	hash.Write([]byte(uuid))
	digest := hash.Sum(nil)
	uuidSignature, err = rsa.SignPKCS1v15(rand.Reader, this.privateKey, crypto.SHA256, digest)
	if err != nil {
		err = errors.New("Authority : Failed to sign uuid for " + hostname + " : " + err.Error())
		log.Println(err)
	}
	return
}

// Verify the validity of an uuid signature
func (this *Authority) VerifyUuidSignature(uuid string, uuidSignature []byte) (err error) {
	hash := crypto.SHA256.New()
	hash.Write([]byte(uuid))
	digest := hash.Sum(nil)
	err = rsa.VerifyPKCS1v15(&this.privateKey.PublicKey, crypto.SHA256, digest, uuidSignature)
	return
}

// Generate a token to use as a proof of identity for all subsequent requests
func (this *Authority) GetToken(clientUuid string) (token string, err error) {
	if t, err := uuid.NewV4(); err == nil {
		token = t.String()
		for t, u := range this.Tokens {
			if clientUuid == u {
				delete(this.Tokens, t)
			}
		}
		this.Tokens[token] = clientUuid
	} else {
		err = errors.New("Authority : Unable to generate token : " + err.Error())
		log.Println(err)
	}

	return
}

// Verify the validity of a token
func (this *Authority) VerifyToken(uuid string, token string) (err error) {
	if u, ok := this.Tokens[token]; ok {
		if uuid != u {
			err = errors.New("Authority : Invalid token " + token + " for client with uuid " + uuid)
			log.Println(err)
		}
	} else {
		err = errors.New("Authority : Invalid token " + token + " for client with uuid " + uuid)
		log.Println(err)
	}

	return
}

// Revoke a token
func (this *Authority) RevokeToken(uuid string, token string) (err error) {
	if u, ok := this.Tokens[token]; ok {
		if uuid == u {
			delete(this.Tokens, token)
		} else {
			err = errors.New("Authority : Invalid token " + token + " for client with uuid " + uuid)
			log.Println(err)
		}
	} else {
		err = errors.New("Authority : Invalid token " + token + " for client with uuid " + uuid)
		log.Println(err)
	}

	return
}
