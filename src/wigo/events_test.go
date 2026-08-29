package wigo

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func resetEvents(t *testing.T) {
	t.Helper()

	events.Lock()
	events.subscribers = make(map[int]chan LiveEvent)
	events.nextId = 0
	events.Unlock()
}

func TestAnEventReachesEverySubscriber(t *testing.T) {
	resetEvents(t)

	first, closeFirst := SubscribeEvents()
	defer closeFirst()
	second, closeSecond := SubscribeEvents()
	defer closeSecond()

	PublishEvent(LiveEvent{Type: EventProbe, Host: "db1", Probe: "check_load", Status: 300})

	for name, stream := range map[string]<-chan LiveEvent{"first": first, "second": second} {
		select {
		case event := <-stream:
			if event.Host != "db1" || event.Probe != "check_load" || event.Status != 300 {
				t.Errorf("%s got %+v", name, event)
			}
			if event.At == 0 {
				t.Errorf("%s got an undated event", name)
			}
		case <-time.After(time.Second):
			t.Errorf("%s received nothing", name)
		}
	}
}

// A browser that stops reading must not hold up the scheduler. Dropping is
// right : the event only says "look again", and the next one says it too.
func TestASlowSubscriberIsDroppedRatherThanWaitedFor(t *testing.T) {
	resetEvents(t)

	_, unsubscribe := SubscribeEvents()
	defer unsubscribe()

	done := make(chan struct{})
	go func() {
		for i := 0; i < eventBufferSize*4; i++ {
			PublishEvent(LiveEvent{Type: EventProbe, Host: "db1"})
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("Publishing blocked on a subscriber that reads nothing")
	}
}

// A subscriber that is never closed is a leak, and the count is how it would be
// noticed.
func TestUnsubscribingIsCountedAndSafeTwice(t *testing.T) {
	resetEvents(t)

	if EventSubscribers() != 0 {
		t.Fatalf("Got %d, expected a clean slate", EventSubscribers())
	}

	stream, unsubscribe := SubscribeEvents()
	if EventSubscribers() != 1 {
		t.Errorf("Got %d, expected one", EventSubscribers())
	}

	unsubscribe()
	if EventSubscribers() != 0 {
		t.Errorf("Got %d, expected the subscriber to be gone", EventSubscribers())
	}

	// The channel is closed, which is what ends the handler's loop
	if _, open := <-stream; open {
		t.Errorf("The stream should have been closed")
	}

	// Calling it again must not panic on an already closed channel
	unsubscribe()
}

// Publishing to nobody is the normal case : most of the time no interface is
// open, and it must cost nothing and go nowhere.
func TestPublishingWithNoSubscriber(t *testing.T) {
	resetEvents(t)

	PublishEvent(LiveEvent{Type: EventHost, Host: "db1"})

	if EventSubscribers() != 0 {
		t.Errorf("Got %d", EventSubscribers())
	}
}

// The stream ends when the client goes away, or the goroutine and the
// subscriber both leak.
func TestTheStreamEndsWithTheRequest(t *testing.T) {
	resetEvents(t)
	setupTestWigo(t, "databases")

	request := httptest.NewRequest("GET", "/api/events", nil)
	ctx, cancel := context.WithCancel(request.Context())
	request = request.WithContext(ctx)

	recorder := httptest.NewRecorder()

	finished := make(chan struct{})
	go func() {
		HttpEventsHandler(recorder, request)
		close(finished)
	}()

	// Wait for it to have subscribed
	for i := 0; i < 100 && EventSubscribers() == 0; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if EventSubscribers() != 1 {
		t.Fatalf("Got %d subscribers, expected the handler to have subscribed", EventSubscribers())
	}

	cancel()

	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatalf("The handler did not stop when the client went away")
	}

	if EventSubscribers() != 0 {
		t.Errorf("Got %d, expected the subscriber to have been dropped", EventSubscribers())
	}

	// And what it wrote is a valid stream : an event name, then its data
	body := recorder.Body.String()
	if !strings.Contains(body, "event: probe\ndata: {") {
		t.Errorf("Got %q, expected a server sent event", body)
	}
	if recorder.Header().Get("Content-Type") != "text/event-stream" {
		t.Errorf("Got %q", recorder.Header().Get("Content-Type"))
	}
}

// An event with no type would be sent as an unnamed one, which no listener is
// registered for.
func TestAnUntypedEventIsNamedMessage(t *testing.T) {
	if got := eventName(LiveEvent{}); got != "message" {
		t.Errorf("Got %q, expected message", got)
	}
	if got := eventName(LiveEvent{Type: EventSchedule}); got != EventSchedule {
		t.Errorf("Got %q", got)
	}
}
