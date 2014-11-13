package wigo

import (
	"crypto/tls"
	"encoding/gob"
	"strconv"
	"net"
	"net/rpc"
	"log"
	"errors"

	"time"
	"io/ioutil"
	"encoding/pem"
	"crypto/x509"
	"os"
	"io"
)

// Push client connect to the push server to
// update client's data over RPCs. Data is transferred
// using binary gob serialisation over tcp connection.
// Secure TLS connection is available and highly recommended.
//
// If an error occurs during the push process the connection
// should 
type PushClient struct {
	config			*PushClientConfig
	serverAddress 	string
	uuidSignature	[]byte
	token  			string
	client 			*rpc.Client
	tlsConfig 		*tls.Config
}

func NewPushClient(config *PushClientConfig) (this *PushClient, err error){
	this = new(PushClient)
	this.config = config

	gob.Register([]interface {}{})
	gob.Register(map[string]interface {}{})

	address := config.Address + ":" + strconv.Itoa(config.Port)

	var listner io.ReadWriteCloser
	if (config.SslEnabled) {
		this.tlsConfig = &tls.Config{MinVersion: tls.VersionTLS12}
		
		// Try to load the server certificate
		if _, err := os.Stat(this.config.SslCert); err == nil {
			if certBytes, err := ioutil.ReadFile(this.config.SslCert) ; err == nil {
				if block, _ := pem.Decode(certBytes) ; block != nil {
					if cert, err := x509.ParseCertificate(block.Bytes) ; err == nil {
						this.tlsConfig.RootCAs = x509.NewCertPool()
						this.tlsConfig.RootCAs.AddCert(cert)
					} else {
						log.Fatal("Push client : unable to parse x509 certificate")
					}
				} else {
					log.Fatal("Push client : unable to decode pem certificate")
				}
			} else {
				log.Fatal("Push client : unable to read certificate")
			}
		} else {
				// If you did not copy the server certificate by hand
				// the client will download it automatically from the
				// server so it can verify it's identity.
				log.Println("Push client : no tls server certificate")
				log.Println("Push client : disabling tls server certificate check to download it")
				this.tlsConfig.InsecureSkipVerify = true
		}

		listner, err = tls.Dial("tcp", address, this.tlsConfig)
		if err != nil {
			return
		}

		this.client = rpc.NewClient(listner)
		log.Printf("Push client : connected to push server ( secure tls connection ) @ %s", address)

		if ( this.tlsConfig.InsecureSkipVerify == true ) {
			// Get server certificate first
			err = this.GetServerCertificate()
			if err == nil {
				// Ask for immediate reconnect
				err = errors.New("RECONNECT")
			}
			return
		}

		if _, err = os.Stat(this.config.UuidSig); err == nil {
			if this.uuidSignature, err = ioutil.ReadFile(this.config.UuidSig) ; err != nil {
				log.Fatal("Push client : unable to read uuid signature")
			}
		} else {
			// Ask the server's authority to sign our uuid
			log.Println("Push client : registering")
			err = this.Register()
			if err == nil {
				// Ask for immediate reconnect
				err = errors.New("RECONNECT")
			}
			return
		}

	} else {
		listner, err = net.Dial("tcp", address)
		if err != nil {
			return
		}

		this.client = rpc.NewClient(listner)
		log.Println("Push client : connected to push server ( insecure connection ) @ " + address)
	}

	return
}

// Download the server certificate from the server thus
// the client can ensure the server's identity. To avoid the small window
// of MITM vulnerability you might copy the certificate by yourself.
func (this *PushClient) GetServerCertificate() (err error) {
	if ( this.client == nil ) {
		return errors.New("Push client : Not connected")
	}

	var cert []byte
	err = this.client.Call("PushServer.GetServerCertificate", NewHelloRequest(nil), &cert)
	if err == nil {
		certFile, err := os.OpenFile(this.config.SslCert, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		if err == nil {
			defer certFile.Close()
			certFile.Write(cert)
		} else {
			log.Fatal("Push client : failed to open " + this.config.SslCert + " for writing push server certificate", err)
		}
	}

	return
}

// Registering is a three step process, first ask the server
// to add the client on the waiting list. Then wait for the
// admin to move the client from the waiting list to the allowed
// list ( this behaviour may be disabled by setting the AutoAcceptClients
// configuration setting on the push server ). Then the client will
// ask the server to sign his uuid, this way the server is able to
// verify the client identity. The uuid signature is persisted on the
// file system.
func (this *PushClient) Register() (err error) {
	if ( this.client == nil ) {
		return errors.New("Push client : Not connected")
	}

	log.Println("Push client : register")

	b := new(bool) // void response
	// Ask the server to add the client on the waiting list
	err = this.client.Call("PushServer.Register", NewHelloRequest(nil), b)
	if ( err != nil ) {
		return
	}

	for {
		// Check if the client has been allowed and ask the server to sign the client's uuid
		err = this.client.Call("PushServer.GetUuidSignature", NewHelloRequest(nil), &this.uuidSignature)
		if  err != nil {
			if err.Error() == "NOT ALLOWED" {
				time.Sleep(time.Duration(this.config.PushInterval) * time.Second)
				continue
			}
			break
		}

		log.Println("Push client : now allowed to push on push server")

		// Save the uuid signature
		sigFile, err := os.OpenFile(this.config.UuidSig, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		if err == nil {
			defer sigFile.Close()
			sigFile.Write(this.uuidSignature)
		} else {
			log.Fatal("Push client : failed to open " + this.config.UuidSig + " for writing push server signature", err)
		}
		break
	}
	return
}

// Hello is the first request of every connection.
// It sends the client's uuid and signature to the
// server as an identity proof. It returns a token
// that will be used to authenticate every subsequent
// requests at a lower cost.
func (this *PushClient) Hello() (err error) {
	if ( this.client == nil ) {
		return errors.New("Push client : Not connected")
	}

	log.Println("Push client : say Hello!")
	err = this.client.Call("PushServer.Hello", NewHelloRequest(this.uuidSignature), &this.token)
	if ( err != nil ) {
		log.Println("Push client : hello error : " + err.Error())
	}

	return
}

// Send the local data to the server
func (this *PushClient) Update() (err error) {
	if ( this.client == nil ) {
		return errors.New("Push client : Not connected")
	}

	log.Println("Push client : update")
	reply := new(bool)
	err = this.client.Call("PushServer.Update", NewUpdateRequest(LocalWigo,this.token), reply)
	if err != nil {
		log.Println("Push client : update error : " + err.Error())
	}

	return
}

// Disconnect the client gracefully
func (this *PushClient) Goodbye() (err error) {
	if ( this.client == nil ) {
		return errors.New("Push client : Not connected")
	}
	defer this.Close()

	log.Println("Push client : goodbye")
	reply := new(bool)
	err = this.client.Call("PushServer.Goodbye", NewUpdateRequest(LocalWigo,this.token), reply)
	if err != nil {
		log.Println("Push client : goodbye error : " + err.Error())
	}

	return
}

// Disconnect the client
func (this *PushClient) Close() (err error) {
	if ( this.client == nil ) {
		return errors.New("Push client : Not connected")
	}

	this.client.Close()
	this.client = nil

	return
}
