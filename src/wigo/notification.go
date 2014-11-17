package wigo

import (
	"encoding/json"
	"fmt"
	"log"
	"time"
    "bytes"
    "crypto/tls"
    "net"
    "net/http"
    "net/mail"
    "net/smtp"
    "net/url"
)

type Notification struct {
	Type    	string
	Hostname 	string
	Message 	string
	Date    	string
	Summary 	string
}

type INotification interface {
	ToJson() ([]byte, error)
	GetMessage() string
	GetSummary() string
}

type NotificationWigo struct {
	*Notification
	OldWigo *Wigo
	NewWigo *Wigo
}
type NotificationProbe struct {
	*Notification
	OldProbe          *ProbeResult
	NewProbe          *ProbeResult
	HostProbesInError []string
}

// Constructors
func NewNotification() (this *Notification) {
	this = new(Notification)
	this.Date = time.Now().Format(dateLayout)
	return
}

func NewNotificationFromMessage(message string) (this *Notification) {
	this = NewNotification()
	this.Message = message
	return
}

func SendNotification(notification INotification) {
	log.Printf("New notification : %s", notification.GetMessage())
	Channels.ChanCallbacks <- notification
}

func NewNotificationProbe(oldProbe *ProbeResult, newProbe *ProbeResult) (this *NotificationProbe) {
	this = new(NotificationProbe)
	this.Notification = NewNotification()
	this.Type = "Probe"
	this.OldProbe = oldProbe
	this.NewProbe = newProbe

	if oldProbe == nil && newProbe != nil {
		this.Hostname = newProbe.GetHost().GetParentWigo().Hostname
		this.Message  = fmt.Sprintf("New probe %s with status %d detected on host %s", newProbe.Name, newProbe.Status, this.Hostname)

		this.Summary += fmt.Sprintf("A new probe %s has been detected on host %s : \n\n", newProbe.Name, this.Hostname)
		this.Summary += fmt.Sprintf("\t%s\n", newProbe.Message)

	} else if oldProbe != nil && newProbe == nil {
		this.Hostname = oldProbe.GetHost().GetParentWigo().Hostname
		this.Message  = fmt.Sprintf("Probe %s on host %s does not exist anymore. Last status was %d", oldProbe.Name, this.Hostname, oldProbe.Status)

		this.Summary += fmt.Sprintf("Probe %s has been deleted on host %s : \n\n", oldProbe.Name, this.Hostname)
		this.Summary += fmt.Sprintf("Last message was : \n\n%s\n", oldProbe.Message)

	} else if oldProbe != nil && newProbe != nil {
		if newProbe.Status != oldProbe.Status {
			this.Hostname = newProbe.GetHost().GetParentWigo().Hostname

			this.Message  = fmt.Sprintf("Probe %s status changed from %d to %d on host %s", newProbe.Name, oldProbe.Status, newProbe.Status, this.Hostname)

			this.Summary += fmt.Sprintf("Probe %s on host %s : \n\n", oldProbe.Name, this.Hostname)
			this.Summary += fmt.Sprintf("\tOld Status : %d\n", oldProbe.Status)
			this.Summary += fmt.Sprintf("\tNew Status : %d\n\n", newProbe.Status)
			this.Summary += fmt.Sprintf("Message :\n\n\t%s\n\n", newProbe.Message)

			// List parent host probes in error
			this.HostProbesInError = newProbe.parentHost.GetErrorsProbesList()

			// Add Log
			LocalWigo.AddLog(newProbe, INFO, fmt.Sprintf("Probe %s switched from %d to %d : %s", newProbe.Name, oldProbe.Status, newProbe.Status, newProbe.Message))
		}
	}

	// Log
	log.Printf("New Probe Notification : %s", this.Message)

	// Send ?
	if GetLocalWigo().GetConfig().Notifications.OnProbeChange {
		weSend := false

		if oldProbe != nil && newProbe != nil {
			if newProbe.Status < oldProbe.Status && oldProbe.Status >= GetLocalWigo().GetConfig().Notifications.MinLevelToSend {
				// It's an UP
				weSend = true
			} else if newProbe.Status >= GetLocalWigo().GetConfig().Notifications.MinLevelToSend {
				// It's a DOWN, check if new status is > to MinLevelToSend
				weSend = true
			}
		}

		if weSend {
			Channels.ChanCallbacks <- this
		}
	}

	return
}

// Getters
func (this *Notification) ToJson() (ba []byte, e error) {
	return json.Marshal(this)
}
func (this *NotificationWigo) ToJson() (ba []byte, e error) {
	return json.Marshal(this)
}
func (this *NotificationProbe) ToJson() (ba []byte, e error) {
	return json.Marshal(this)
}

func (this *Notification) GetSummary() (s string) {
	return this.Summary
}
func (this *NotificationWigo) GetSummary() (s string) {
	return this.Summary
}
func (this *NotificationProbe) GetSummary() (s string) {
	return this.Summary
}

func (this *Notification) GetMessage() string {
	return this.Message
}

func SendMail(summary string, message string) {

    log.Printf("We're gonna launch mail notif...")

    recipients := GetLocalWigo().GetConfig().Notifications.EmailRecipients
    server := GetLocalWigo().GetConfig().Notifications.EmailSmtpServer
    from := mail.Address{
        GetLocalWigo().GetConfig().Notifications.EmailFromName,
        GetLocalWigo().GetConfig().Notifications.EmailFromAddress,
    }

    for i := range recipients {

        to := mail.Address{"", recipients[i]}

        go func() {
            // setup a map for the headers
            header := make(map[string]string)
            header["From"] = from.String()
            header["To"] = to.String()
            header["Subject"] = message

            // setup the message
            message := ""
            for k, v := range header {
                message += fmt.Sprintf("%s: %s\r\n", k, v)
            }
            message += "\r\n"
            message += summary

            // Connect to the remote SMTP server.
            c, err := smtp.Dial(server)
            if err != nil {
                log.Printf("Fail to dial connect to smtp server %s : %s", server, err)
                return
            }

            // Set the sender and recipient.
            c.Mail(from.Address)
            c.Rcpt(to.Address)

            // Send the email body.
            wc, err := c.Data()
            if err != nil {
                log.Printf("Fail to send DATA to smtp server : %s", err)
                return
            }

            buf := bytes.NewBufferString(message)
            if _, err = buf.WriteTo(wc); err != nil {
                log.Printf("Fail to send notification to %s : %s", to.String(), err)
                return
            }

            log.Printf(" - Sent to email address %s", to.String())

            wc.Close()
        }()
    }

}

func CallbackHttp(json string) (e error) {

    log.Printf("We're gonna launch http notif...")

    httpUrl := GetLocalWigo().GetConfig().Notifications.HttpUrl

    // Create http client with timeout
    c := http.Client{
        Transport: &http.Transport{
            TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
            Dial: func(netw, addr string) (net.Conn, error) {
                deadline := time.Now().Add(5 * time.Second)
                c, err := net.DialTimeout(netw, addr, time.Second*5)
                if err != nil {
                    return nil, err
                }
                c.SetDeadline(deadline)
                return c, nil
            },
        },
    }

    // Make post values
    postValues := url.Values{}
    postValues.Add("Notification", string(json))

    // Make request
    resp, reqErr := c.PostForm(httpUrl, postValues)
    if reqErr != nil {
        log.Printf("Error sending callback to url %s : %s", httpUrl, reqErr)
        return reqErr
    } else {
        log.Printf(" - Sent to http url : %s", httpUrl)
    }


    resp.Body.Close()

    return nil
}
