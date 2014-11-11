package wigo

import (
	"log"
	"errors"

	"crypto/x509"
	"crypto/rand"
	"encoding/pem"
	"io/ioutil"
	"crypto/rsa"
	"crypto"
	"os"
	"bufio"
	"github.com/nu7hatch/gouuid"
	"regexp"
)

type Cartman struct {
	key				[]byte
	privateKey  	*rsa.PrivateKey
	
	cert			[]byte
	certificate     *x509.Certificate

	config			*PushServerConfig

	Waiting  	 	map[string]string
	Allowed			map[string]string
}

func NewCartman(config *PushServerConfig) (this *Cartman) {
	this = new(Cartman)
	this.config = config

	/*
	 * Load CA certificate
	 */
	var err error
    if this.cert, err = ioutil.ReadFile(this.config.SslCert) ; err == nil {
		if block, _ := pem.Decode(this.cert) ; block != nil {
			if this.certificate, err = x509.ParseCertificate(block.Bytes) ; err != nil {
				log.Fatal("Cartman : unable to parse x509 certificate")
			}
		} else {
			log.Fatal("Cartman : unable to decode pem certificate")
		}
	} else {
		log.Fatal("Cartman : unable to read certificate")
	}

	/*
	 * Load CA private key
	 */
	if this.key, err = ioutil.ReadFile(this.config.SslKey) ; err == nil {
		if block, _ := pem.Decode(this.key) ; block != nil {
			if this.privateKey, err = x509.ParsePKCS1PrivateKey(block.Bytes) ; err != nil {
				err = errors.New("Cartman : unable to read decode x509 private key")
				log.Println(err)
			}
		} else {
			err = errors.New("Cartman : unable to read decode pem private key")
			log.Println(err)
		}
	} else {
		err = errors.New("Cartman : unable to read private key")
		log.Println(err)
	}

	this.Waiting 	= make(map[string]string)
	this.Allowed	= make(map[string]string)

	this.LoadAllowedList()

	log.Println("Cartman : You will respect my authoritah !")

	return
}

func ( this *Cartman) IsWaiting(uuid string) bool {
	_, ok := this.Waiting[uuid]
	return ok
}

func ( this *Cartman) IsAllowed(uuid string) bool {
	_, ok := this.Allowed[uuid]
	return ok
}

func ( this *Cartman) GetServerCertificate() []byte{
	return this.cert
}

func ( this *Cartman ) AddToWaitingList(uuid string,hostname string ) (err error){
	if len(this.Waiting) < this.config.MaxWaitingClients {
		this.Waiting[uuid] = hostname
		log.Println("Cartman : added " + hostname + " to waiting list")
	} else {
		err = errors.New("Cartman : too many wainting clients")
	}

	return
}

func ( this *Cartman ) LoadAllowedList() (err error) {
	if _, err = os.Stat(this.config.AllowedClientsFile); err == nil {
		file, err := os.Open(this.config.AllowedClientsFile)
		if err != nil {
			log.Fatalf("Error opening allowed clients file %s",err)
		}
		defer file.Close()

		re, err := regexp.Compile(`([[:xdigit:]]{8}-[[:xdigit:]]{4}-[[:xdigit:]]{4}-[[:xdigit:]]{4}-[[:xdigit:]]{12}) (.+)`)
		if err != nil {
			log.Fatalf("Invalid allowed client list regexp %s", err)
		}

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {

			line := scanner.Text()
			result := re.FindStringSubmatch(line)

			if len(result) != 3 {
				log.Printf("Ignoring invalid allowed client line %s", line)
				continue
			}

			uuid, err := uuid.ParseHex(result[1])
			if err != nil {
				log.Fatalf("Unable to parse allowed client uuid %s : %s", result[1], err)
			}

			this.Allowed[uuid.String()] = result[2]
		}
		if err := scanner.Err(); err != nil {
			log.Fatalf("Error while loading allowed clients file %s",err)
		}
	}
	return
}

func ( this *Cartman ) SaveAllowedList() (err error) {
	// Save UUID
	allowedClientsFile, err := os.OpenFile(this.config.AllowedClientsFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err == nil {
		defer allowedClientsFile.Close()
		for uuid, hostname := range this.Allowed {
			allowedClientsFile.Write([]byte( uuid + " " + hostname + "\n" ))
		}
	} else {
		log.Fatalf("Failed to create uuid file : %s", err)
	}
	return
}

func ( this *Cartman ) Allow(uuid string, hostname string) (err error){
	if h, ok := this.Waiting[uuid] ; ok {
		if h == hostname {
			delete(this.Waiting,uuid)
			this.Allowed[uuid] = hostname
			this.SaveAllowedList()
			log.Println("Cartman : added " + hostname + " to ready list")
		} else {
			err = errors.New("Cartman : Invaluuid uuid")
		}
	} else {
		err = errors.New("Cartman : no uuid for " + hostname)
	}
	return
}

func ( this *Cartman ) GetAutograph(uuid string, hostname string) (autograph []byte, err error) {
	hash := crypto.SHA256.New()
	hash.Write([]byte(uuid))
	digest := hash.Sum(nil)
	autograph, err = rsa.SignPKCS1v15(rand.Reader, this.privateKey, crypto.SHA256, digest)
	if err != nil {
		err = errors.New("Cartman : Failed to sign autograph for " + hostname + " : " + err.Error())
		log.Println(err)
	}
	return
}

func ( this *Cartman ) VerifyAutograph(uuid string, autograph []byte) (err error){
	hash := crypto.SHA256.New()
	hash.Write([]byte(uuid))
	digest := hash.Sum(nil)
	err = rsa.VerifyPKCS1v15(&this.privateKey.PublicKey, crypto.SHA256, digest, autograph)
	log.Println(err)
	return
}

///*
// * Cartman Listner
// */
//
//
//type listener struct {
//	net.Listener
//
//	tlsConfig 		*tls.Config
//	rpcServer		*rpc.Server
//
//	grounded		map[net.Addr]time.Time
//}
//
//func (this *listener) Accept() (conn net.Conn, err error) {
//	conn, err = this.Listener.Accept()
//	if err != nil {
//		log.Println("Cartman : accept connection error")
//		return
//	}
//
//	if _, ok := this.grounded[conn.RemoteAddr()] ; !ok {
//		c := tls.Server(conn, this.tlsConfig)
//		if err = c.Handshake() ; err == nil {
//			log.Println("Cartman : tls handshake okay")
//			this.rpcServer.ServeConn(c)
//		} else {
//			this.grounded[conn.RemoteAddr()] = time.Now()
//			log.Println("Cartman : tls handshake error " + conn.RemoteAddr().String() + " has been grounded")
//		}
//	} else {
//		c := tls.Server(conn, this.tlsLightConfig)
//		if err = c.Handshake() ; err == nil {
//			log.Println("Cartman : tls light handshake okay")
//			this.rpcServer.ServeConn(c)
//			delete(this.grounded, conn.RemoteAddr())
//		} else {
//			log.Println("Cartman : tls light handshake error " + conn.RemoteAddr().String() + " is still grounded")
//		}
//	}
//
//	return
//}
//
//
//func (this *Cartman) NewListener(socket net.Listener, rpcServer *rpc.Server) net.Listener {
//	l := new(listener)
//	l.Listener = socket
//	l.tlsConfig = this.tlsConfig
//
//	this.server = rpc.NewServer()
//	this.server.Register(this)
//
//	go func() {
//		for {
//			listner.Accept()
//			/*
//			TLS OR NOTHING
//			if conn, err := listner.Accept() ; err == nil {
//	 			go rpc.ServeConn(conn)
//			} else {
//				log.Println(err)
//			}
//			*/
//		}
//	}()
//
//	return l
//}
