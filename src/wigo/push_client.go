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

type PushClient struct {
	config			*PushClientConfig
	serverAddress 	string
	autograph 		[]byte
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

		/*
		 * Load CA certificate
		 */
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
				// So we can try to socialize
				log.Println("Push client : no tls server certificate")
				log.Println("Push client : disabling tls server certificate to fetch it")
				this.tlsConfig.InsecureSkipVerify = true
		}

		listner, err = tls.Dial("tcp", address, this.tlsConfig)
		if err != nil {
			return
		}

		this.client = rpc.NewClient(listner)
		log.Println("Push client : connected to push server @ " + address)

		if ( this.tlsConfig.InsecureSkipVerify == true ) {
			// Get server certificate first
			err = this.GetServerCertificate()
			if err == nil {
				err = errors.New("RECONNECT")
			}
			return

		}

		if _, err = os.Stat(this.config.UuidSig); err == nil {
			if this.autograph, err = ioutil.ReadFile(this.config.UuidSig) ; err != nil {
				log.Fatal("Push client : unable to read autograph")
			}
		} else {
			// Ask to cartman to sign us an autograph
			log.Println("Push client : socializing with cartman @ " + address)
			err = this.Socialize()
			return
		}

	} else {
		listner, err = net.Dial("tcp", address)
		if err != nil {
			return
		}

		this.client = rpc.NewClient(listner)
		log.Println("Push client : connected to push server @ " + address)
	}

	return
}

func (this *PushClient) GetServerCertificate() (err error) {
	if ( this.client == nil ) {
		return errors.New("NOT CONNECTED")
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

func (this *PushClient) Socialize() (err error) {
	if ( this.client == nil ) {
		return errors.New("NOT CONNECTED")
	}

	log.Println("Push client : say Yo!")

	b := new(bool)
	err = this.client.Call("PushServer.Yo", NewHelloRequest(nil), b)
	if ( err != nil ) {
		return
	}

	for {
		err = this.client.Call("PushServer.Autograph", NewHelloRequest(nil), &this.autograph)
		if  err != nil {
			if err.Error() == "NOT ALLOWED" {
				time.Sleep(time.Duration(10) * time.Second)
				continue
			}
			break
		}

		log.Println("Push client : now allowed to push on push server")

		sigFile, err := os.OpenFile(this.config.UuidSig, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		if err == nil {
			defer sigFile.Close()
			sigFile.Write(this.autograph)
		} else {
			log.Fatal("Push client : failed to open " + this.config.UuidSig + " for writing push server signature", err)
		}
		break
	}
	return
}

func (this *PushClient) Hello() (err error) {
	if ( this.client == nil ) {
		return errors.New("NOT CONNECTED")
	}

	log.Println("Push client : say Hello!")
	err = this.client.Call("PushServer.Hello", NewHelloRequest(this.autograph), &this.token)
	if ( err != nil ) {
		log.Println("Push client : hello error : " + err.Error())
	}

	return
}

func (this *PushClient) Update() (err error) {
	if ( this.client == nil ) {
		return errors.New("NOT CONNECTED")
	}

	log.Println("Push client : update")
	reply := new(bool)
	err = this.client.Call("PushServer.Update", NewUpdateRequest(LocalWigo,this.token), reply)
	if err != nil {
		log.Println("Push client : update error : " + err.Error())
	}

	return
}

func (this *PushClient) Goodbye() (err error) {
	if ( this.client == nil ) {
		return errors.New("NOT CONNECTED")
	}

	log.Println("Push client : goodbye")
	reply := new(bool)
	err = this.client.Call("PushServer.Goodbye", NewUpdateRequest(LocalWigo,this.token), reply)
	if err != nil {
		log.Println("Push client : update error : " + err.Error())
	}

	return
}

func (this *PushClient) Close() (err error) {
	if ( this.client == nil ) {
		return errors.New("NOT CONNECTED")
	}

	this.client.Close()
	this.client = nil

	return
}
