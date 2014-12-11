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

		dialer := &net.Dialer{
			Timeout : 5 * time.Second,
		}
		log.Printf("Push client : connecting to push server @ %s", address)
		listner, err = tls.DialWithDialer(dialer, "tcp", address, this.tlsConfig)
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

		// Check if the client is allowed to push to the server. If it's not
		// the case add the client to a waiting appoval list. This beahviour may
		// be disabled by setting the AutoAcceptClients configuration parametter
		// to true on the push server.
		log.Println("Push client : Register")
		b := new(bool) // void response
		err = this.CallWithTimeout("PushServer.Register", NewHelloRequest(nil), b, time.Duration(5) * time.Second)
		if ( err != nil ) {
			return
		}

		if _, err = os.Stat(this.config.UuidSig); err == nil {
			if this.uuidSignature, err = ioutil.ReadFile(this.config.UuidSig) ; err != nil {
				log.Fatal("Push client : Unable to read uuid signature")
			}
		} else {
			err = this.SignUuid()
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

	log.Println("Push client : Downloading server certificate")
	var cert []byte
	err = this.CallWithTimeout("PushServer.GetServerCertificate", NewHelloRequest(nil), &cert, time.Duration(5) * time.Second)
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

// Ask the server to sign the client's uuid, this way the server is able to
// verify the client identity. The uuid signature is persisted on the
// file system.
func (this *PushClient) SignUuid() (err error) {
	if ( this.client == nil ) {
		return errors.New("Push client : Not connected")
	}

	log.Println("Push client : Ask for uuid signature!")
	for {
		// Check if the client has been allowed and ask the server to sign the client's uuid
		err = this.CallWithTimeout("PushServer.GetUuidSignature", NewHelloRequest(nil), &this.uuidSignature,time.Duration(5) * time.Second)
		if  err != nil {
			if err.Error() == "WAITING" {
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

	log.Println("Push client : Hello")
	for {
		err = this.CallWithTimeout("PushServer.Hello", NewHelloRequest(this.uuidSignature), &this.token, time.Duration(5) * time.Second)
		if ( err != nil ) {
			if err.Error() == "WAITING" {
				time.Sleep(time.Duration(this.config.PushInterval) * time.Second)
				continue
			}
			log.Println("Push client : hello error : " + err.Error())
		}
		break
	}

	return
}

// Send the local data to the server
func (this *PushClient) Update() (err error) {
	if ( this.client == nil ) {
		return errors.New("Push client : Not connected")
	}

	log.Println("Push client : Update")

	reply := new(bool)
	err = this.CallWithTimeout("PushServer.Update", NewUpdateRequest(LocalWigo,this.token), reply, time.Duration(5) * time.Second)
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

	log.Println("Push client : Goodbye")

	reply := new(bool)
	err = this.CallWithTimeout("PushServer.Goodbye", NewUpdateRequest(LocalWigo,this.token), reply, time.Duration(5) * time.Second)
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

func (this *PushClient) CallWithTimeout( serviceMethod string, args interface{}, reply interface{}, timeout time.Duration) error{
	c := make(chan error, 1)
	go func() { c <- this.client.Call(serviceMethod,args,reply) } ()
	select {
	case err := <-c:
		return err
	case <-time.After(timeout):
		log.Printf("Push client : rpc %s timed out after %.3fs", serviceMethod, timeout.Seconds())
		return errors.New("timeout")
	}
}
