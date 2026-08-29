package wigo

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"net/url"
	"os/exec"
	"time"
)

type Notification struct {
	Type     string
	Hostname string
	Group    string
	Message  string
	Date     string
	Summary  string

	// How bad it is, on the probe scale, so a suppression can reason about it.
	// A host going down defaults to the worst.
	Status int

	// The probe this is about, empty when it belongs to none. A repeat about a
	// probe has to carry it, or an ack on that probe could not hold it back.
	Probe string

	// Set on a repeat about a problem left unattended long enough, which sends
	// it to the apprise targets marked Escalation as well.
	Escalated bool
}

type INotification interface {
	ToJson() ([]byte, error)
	GetMessage() string
	GetSummary() string
	GetHostname() string
	GetGroup() string

	// What the notification is about, so an ack or a silence can be narrower
	// than a whole host. Empty for anything that belongs to no probe.
	GetProbe() string

	// The status being reported, which is what tells an ack whether the
	// situation is still the one that was acknowledged.
	GetStatus() int
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

	// How steady the probe is. Not serialised : it is about the notification,
	// not about the probe result being reported.
	flap FlapState
}

// Constructors
func NewNotification() (this *Notification) {
	this = new(Notification)
	this.Date = time.Now().Format(dateLayout)
	// Anything that does not say otherwise is treated as bad news, so a message
	// nobody thought about cannot be swallowed by an ack taken on something else
	this.Status = 500
	return
}

func NewNotificationFromMessage(message string) (this *Notification) {
	this = NewNotification()
	this.Message = message
	return
}

func NewNotificationFromMessageForHost(message string, hostname string, group string) (this *Notification) {
	this = NewNotificationFromMessage(message)
	this.Hostname = hostname
	this.Group = group
	return
}

// SetStatus says how bad the news is, on the probe scale, so an ack can tell
// whether the situation is still the one that was acknowledged.
func (this *Notification) SetStatus(status int) *Notification {
	this.Status = status
	return this
}

// SendNotification dispatches a notification unless something says not to.
//
// Every notification goes through here, which is what makes one place enough
// to hold them back. An ack or a silence stops the message and nothing else :
// the status is still computed, still displayed and still logged, so the only
// thing lost is the interruption.
func SendNotification(notification INotification) {
	hostname := notification.GetHostname()
	group := notification.GetGroup()
	probe := notification.GetProbe()
	status := notification.GetStatus()

	// A situation that changed is not the one anybody acknowledged
	clearAckOn(hostname, group, probe, status)

	if suppression, held := suppressionFor(hostname, group, probe, status); held {
		log.Printf("Notification held back by the %s on %s : %s",
			suppression.Kind, describeSuppressionTarget(suppression), notification.GetMessage())
		return
	}

	// Everything behind a router that is down is unreachable, and none of it is
	// the news. Checked before the rest because it is the reason that explains
	// the most messages at once.
	if parent, held := heldByDependencies(hostname, group, status); held {
		if parent != "" {
			log.Printf("Notification held back, %s sits behind %s which is down : %s",
				hostname, parent, notification.GetMessage())
		} else {
			log.Printf("Notification held back, nobody was told %s had a problem : %s",
				hostname, notification.GetMessage())
		}
		return
	}

	// A probe that keeps changing its mind was called out once and left alone.
	// Checked here rather than where the change is recorded, so a repeat about
	// it is held back too : saying the same unsteady thing every ten minutes is
	// the same noise, more slowly.
	if probe != "" && FlapStateOf(hostname, probe).Flapping {
		log.Printf("Notification held back, probe %s on host %s is flapping : %s",
			probe, hostname, notification.GetMessage())
		return
	}

	// Held, not dropped : nothing is recorded as sent, so the repeat loop says
	// it as soon as the window closes.
	if heldByQuietHours(status) {
		logQuietHours(notification)
		return
	}

	// Back to normal, so the next problem starts its own clock instead of
	// inheriting how long this one had been open.
	if status <= 100 {
		ForgetNotified(hostname, probe)
	} else {
		recordNotified(hostname, probe, time.Now().Unix())
	}

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
		this.Group = newProbe.GetHost().Group
		this.Message = fmt.Sprintf("New probe %s with status %d detected on host %s", newProbe.Name, newProbe.Status, this.Hostname)

		// The start of its band on a timeline : without it, a probe that
		// appeared critical and stayed there would have no transition at all
		// and would be drawn as nothing.
		RecordStatusTransition(StatusChange{
			Host: this.Hostname, Probe: newProbe.Name, Group: this.Group,
			Was: StatusAbsent, Now: newProbe.Status, Message: newProbe.Message,
		})

		this.Summary += fmt.Sprintf("A new probe %s has been detected on host %s : \n\n", newProbe.Name, this.Hostname)
		this.Summary += fmt.Sprintf("\t%s\n", newProbe.Message)

	} else if oldProbe != nil && newProbe == nil {
		this.Hostname = oldProbe.GetHost().GetParentWigo().Hostname
		this.Group = oldProbe.GetHost().Group

		// It is gone : keeping its history would have it come back flapping for
		// changes it made in a previous life.
		forgetFlapping(this.Hostname, oldProbe.Name)

		RecordStatusTransition(StatusChange{
			Host: this.Hostname, Probe: oldProbe.Name, Group: this.Group,
			Was: oldProbe.Status, Now: StatusAbsent,
			Message: "the probe is gone",
		})
		this.Message = fmt.Sprintf("Probe %s on host %s does not exist anymore. Last status was %d", oldProbe.Name, this.Hostname, oldProbe.Status)

		this.Summary += fmt.Sprintf("Probe %s has been deleted on host %s : \n\n", oldProbe.Name, this.Hostname)
		this.Summary += fmt.Sprintf("Last message was : \n\n%s\n", oldProbe.Message)

	} else if oldProbe != nil && newProbe != nil {
		if newProbe.Status != oldProbe.Status {
			this.Hostname = newProbe.GetHost().GetParentWigo().Hostname
			this.Group = newProbe.GetHost().Group

			this.Message = fmt.Sprintf("Probe %s status changed from %d to %d on host %s", newProbe.Name, oldProbe.Status, newProbe.Status, this.Hostname)

			this.Summary += fmt.Sprintf("Probe %s on host %s : \n\n", oldProbe.Name, this.Hostname)
			this.Summary += fmt.Sprintf("\tOld Status : %d\n", oldProbe.Status)
			this.Summary += fmt.Sprintf("\tNew Status : %d\n\n", newProbe.Status)
			this.Summary += fmt.Sprintf("Message :\n\n\t%s\n\n", newProbe.Message)

			// List parent host probes in error
			this.HostProbesInError = newProbe.parentHost.GetErrorsProbesList()

			// Add Log
			LocalWigo.AddLog(newProbe, INFO, fmt.Sprintf("Probe %s switched from %d to %d : %s", newProbe.Name, oldProbe.Status, newProbe.Status, newProbe.Message))

			// Every change counts towards how steady this probe is, including
			// the ones too mild to be notified about : a probe bouncing between
			// OK and WARNING is exactly as unsteady as one bouncing between OK
			// and CRITICAL, it just shouts less about it.
			RecordStatusTransition(StatusChange{
				Host: this.Hostname, Probe: newProbe.Name, Group: this.Group,
				Was: oldProbe.Status, Now: newProbe.Status, Message: newProbe.Message,
			})

			// The interface hears about it now rather than at its next poll,
			// which is up to a minute of looking at a green screen about a
			// machine that is already down.
			PublishEvent(LiveEvent{
				Type:   EventProbe,
				Host:   this.Hostname,
				Probe:  newProbe.Name,
				Status: newProbe.Status,
			})

			this.flap = RecordStatusChange(this.Hostname, newProbe.Name)
			if this.flap.JustSettled {
				logSettled(this.Hostname, newProbe.Name, this.flap)
			}
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
			// A probe that keeps changing its mind is called out once and then
			// left alone. Said before the send, because the flapping notice is
			// the last thing heard about it until it settles.
			if this.flap.JustStarted {
				log.Printf("%s", describeFlapping(this.Hostname, this.GetProbe(), this.flap))
				LocalWigo.AddLog(newProbe, WARNING, describeFlapping(this.Hostname, this.GetProbe(), this.flap))

				SendNotification(NewNotificationFromMessageForHost(
					describeFlapping(this.Hostname, this.GetProbe(), this.flap),
					this.Hostname, this.Group).SetStatus(newProbe.Status))
				return
			}

			// Through the same door as everything else : a probe notification
			// that skipped it would be the one thing an ack could not hold back
			SendNotification(this)
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

func (this *Notification) GetHostname() string {
	return this.Hostname
}

// A plain notification is usually about a host rather than a probe, but a
// repeat carries the probe it is repeating about : without it, acknowledging
// that probe would silence the change and not the repeats.
func (this *Notification) GetProbe() string {
	return this.Probe
}

func (this *Notification) GetStatus() int {
	return this.Status
}

func (this *NotificationProbe) GetProbe() string {
	if this.NewProbe != nil {
		return this.NewProbe.Name
	}
	if this.OldProbe != nil {
		return this.OldProbe.Name
	}

	return ""
}

func (this *NotificationProbe) GetStatus() int {
	if this.NewProbe != nil {
		return this.NewProbe.Status
	}

	// The probe is gone. Nothing worse can be said about it, so an ack on
	// whatever it was doing does not cover its disappearance.
	return 500
}

func (this *Notification) GetGroup() string {
	return this.Group
}

func SendMail(summary string, message string) {

	log.Printf("We're gonna launch mail notif...")

	recipients := GetLocalWigo().GetConfig().Notifications.EmailRecipients
	server := GetLocalWigo().GetConfig().Notifications.EmailSmtpServer
	from := mail.Address{
		Name:    GetLocalWigo().GetConfig().Notifications.EmailFromName,
		Address: GetLocalWigo().GetConfig().Notifications.EmailFromAddress,
	}

	for i := range recipients {

		to := mail.Address{
			Name:    "",
			Address: recipients[i],
		}

		go func() {
			title := message
			// setup a map for the headers
			header := make(map[string]string)
			header["From"] = from.String()
			header["To"] = to.String()
			header["Subject"] = title

			// setup the message
			content := ""
			for k, v := range header {
				content += fmt.Sprintf("%s: %s\r\n", k, v)
			}
			content += "\r\n"
			content += title
			content += "\r\n"
			content += summary
			content += "\r\n"
			content += fmt.Sprintf("Sent from %s on %s", LocalWigo.GetLocalHost().Name, time.Now().Format(time.RFC3339))

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

			buf := bytes.NewBufferString(content)
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

// notificationIsEscalated reports whether this one also goes to the people who
// are woken second.
func notificationIsEscalated(notification INotification) bool {
	if plain, ok := notification.(*Notification); ok {
		return plain.Escalated
	}

	return false
}

func SendApprise(notification INotification) {

	log.Printf("We're gonna launch apprise notif...")

	config := GetLocalWigo().GetConfig().Notifications

	// Check if Apprise is enabled
	if config.AppriseEnabled == 0 {
		return
	}

	// Check if URLs are configured
	if len(config.AppriseUrls) == 0 && len(config.AppriseTargets) == 0 {
		log.Printf("Apprise is enabled but no URLs are configured")
		return
	}

	summary := notification.GetSummary()
	message := notification.GetMessage()
	hostname := notification.GetHostname()
	group := notification.GetGroup()

	// Keep only the urls matching the host/group of this notification
	appriseUrls := config.GetAppriseUrls(hostname, group, notificationIsEscalated(notification))
	if len(appriseUrls) == 0 {
		log.Printf("Apprise : no target matching host \"%s\" (group \"%s\"), notification not sent", hostname, group)
		return
	}

	apprisePath := config.ApprisePath

	// Ensure summary is not empty (Apprise requires it)
	appriseSummary := summary
	if appriseSummary == "" {
		appriseSummary = message
		if appriseSummary == "" {
			appriseSummary = "Wigo notification"
		}
	}

	// Send to each URL in a separate goroutine
	for _, url := range appriseUrls {
		go func(appriseUrl string, origin string) {
			// Create context with timeout (10 seconds)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			// Create command: apprise -v -t "title" -b "body" url
			cmd := exec.CommandContext(ctx, apprisePath, "-v", "-t", message, "-b", appriseSummary, appriseUrl)

			// Execute command
			output, err := cmd.CombinedOutput()
			if err != nil {
				log.Printf("Error sending apprise notification to %s%s : %s", appriseUrl, origin, err)
				if len(output) > 0 {
					log.Printf("Apprise verbose output for %s%s : %s", appriseUrl, origin, string(output))
				}
				return
			}

			// Log verbose output even on success for debugging
			if len(output) > 0 {
				log.Printf("Apprise verbose output for %s%s : %s", appriseUrl, origin, string(output))
			}
			log.Printf(" - Sent to apprise url : %s%s", appriseUrl, origin)
		}(url.Url, url.Origin())
	}
}
