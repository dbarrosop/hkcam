package dlink

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/brutella/hc/service"
)

type Event interface {
	Raw() string
}

type MovementDetected struct {
	Value bool
	raw   string
}

func (e *MovementDetected) Raw() string { return e.raw }

type Camera struct {
	address  string
	username string
	password string
}

func New(address string, username string, password string) *Camera {
	return &Camera{
		address:  address,
		username: username,
		password: password,
	}
}

func parseEvent(raw string) Event {
	switch {
	case strings.HasPrefix(raw, "pir="):
		return &MovementDetected{raw: raw, Value: strings.HasSuffix(raw, "on")}
	}
	return nil
}

func (c *Camera) subscribeNotifications(ctx context.Context, events chan<- Event, errc chan<- error) {
	defer close(events)
	defer close(errc)

	client := &http.Client{}
	req, err := http.NewRequest(
		"GET",
		fmt.Sprintf("%s/config/notify_stream.cgi", c.address),
		nil,
	)
	if err != nil {
		errc <- err
		return
	}
	req.SetBasicAuth(c.username, c.password)

	resp, err := client.Do(req)
	if err != nil {
		errc <- err
		return
	}

	if resp.StatusCode != http.StatusOK {
		errc <- fmt.Errorf("got %s", resp.Status)
		return
	}

	for {
		reader := bufio.NewReader(resp.Body)
		line, err := reader.ReadBytes('\r')
		if err != nil {
			errc <- fmt.Errorf("got %s", resp.Status)
			return
		}
		line = bytes.TrimSpace(line)

		event := parseEvent(string(line))
		if event != nil {
			events <- event
		}
	}
}

func (c *Camera) Monitor(motionSensor *service.MotionSensor) error {
	errc := make(chan error)
	events := make(chan Event)

	go c.subscribeNotifications(context.Background(), events, errc)

	for {
		select {
		case err := <-errc:
			return err
		case event := <-events:
			switch e := event.(type) {
			case *MovementDetected:
				motionSensor.MotionDetected.SetValue(e.Value)
			default:
				return fmt.Errorf("event '%s', unrecognized", e.Raw())
			}
		}
	}
}
