package bridgeruntime

import (
	"context"
	"encoding/json"
	"github.com/Pin4sf/Waldo-Kennel/backend/internal/devicebridge"
	"io"
	"net/http"
	"testing"
	"time"
)

type commandSocket struct {
	commands [][]byte
	index    int
	writes   [][]byte
	results  chan struct{}
}

func (s *commandSocket) Read(ctx context.Context) ([]byte, error) {
	if s.index > 0 {
		select {
		case <-s.results:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if s.index == len(s.commands) {
		return nil, io.EOF
	}
	raw := s.commands[s.index]
	s.index++
	return raw, nil
}
func (s *commandSocket) Write(_ context.Context, raw []byte) error {
	s.writes = append(s.writes, append([]byte(nil), raw...))
	var frame map[string]any
	if json.Unmarshal(raw, &frame) == nil && frame["type"] == "result" {
		s.results <- struct{}{}
	}
	return nil
}
func (s *commandSocket) Close() error { return nil }
func TestHandlersShipUnknownAndDeliveryUnknown(t *testing.T) {
	repo, store, d, keys, dir := realIdentity(t)
	socket := &commandSocket{results: make(chan struct{}, 2)}
	for i, class := range []string{"machine_state_query", "notify_local"} {
		payload := map[string]any{"query_id": "q", "query_kind": "session_status"}
		id := "query"
		message := "01ARZ3NDEKTSV4RRFFQ69G5FAV"
		if i == 1 {
			payload = map[string]any{"notification_id": "n", "title": "Title", "body": "Body", "severity": "info"}
			id = "notification"
			message = "01ARZ3NDEKTSV4RRFFQ69G5FAW"
		}
		raw, e := json.Marshal(map[string]any{"contract_version": devicebridge.ContractVersion, "type": "command", "message_id": message, "device_id": string(d.DeviceID), "owner_id": d.OwnerID, "command_id": id, "idempotency_key": id, "revision": 1, "class": class, "expires_at": time.Now().Unix() + 600, "payload": payload})
		if e != nil {
			t.Fatal(e)
		}
		socket.commands = append(socket.commands, raw)
	}
	f := NewFactory(Dependencies{Fence: repo, Keys: keys, SessionStore: store, KeyDirectory: dir, ReadyProbe: func(context.Context) error { return nil }, Dialer: dialFunc(func(context.Context, string, http.Header) (devicebridge.Socket, error) { return socket, nil })})
	raw, e := f.New(context.Background(), "https://example.test", d, nil)
	if e != nil {
		t.Fatal(e)
	}
	s := raw.(*Session)
	if e = s.client.Connect(context.Background()); e != io.EOF {
		t.Fatal(e)
	}
	var unknown, delivery bool
	for _, raw := range socket.writes {
		var frame map[string]any
		if e = json.Unmarshal(raw, &frame); e != nil {
			t.Fatal(e)
		}
		if frame["type"] != "result" {
			continue
		}
		p := frame["payload"].(map[string]any)
		if frame["command_id"] == "query" {
			unknown = p["status"] == "answered" && p["answer"].(map[string]any)["state"] == "unknown"
		}
		if frame["command_id"] == "notification" {
			delivery = p["status"] == "failed" && p["reason"] == "delivery_unknown"
		}
	}
	if !unknown || !delivery {
		t.Fatal("dishonest adapters")
	}
	handlers := s.client.Executor.(devicebridge.Handlers)
	if handlers.Display != nil {
		t.Fatal("notification adapter invented")
	}
}

func TestWiredDisplayReportsDeliveredOnlyAfterShellAcknowledges(t *testing.T) {
	for _, shellShows := range []bool{true, false} {
		repo, store, d, keys, dir := realIdentity(t)
		raw, e := json.Marshal(map[string]any{"contract_version": devicebridge.ContractVersion, "type": "command", "message_id": "01ARZ3NDEKTSV4RRFFQ69G5FAW", "device_id": string(d.DeviceID), "owner_id": d.OwnerID, "command_id": "notification", "idempotency_key": "notification", "revision": 1, "class": "notify_local", "expires_at": time.Now().Unix() + 600, "payload": map[string]any{"notification_id": "n", "title": "Waldo status", "body": "Your Mac is connected.", "severity": "info"}})
		if e != nil {
			t.Fatal(e)
		}
		socket := &commandSocket{commands: [][]byte{raw}}
		relay := NewDisplayRelay()
		shell := make(chan DisplayRequest, 1)
		go func() {
			request, ok := relay.next(context.Background())
			if ok {
				relay.ack(request.RequestID, shellShows)
			}
			shell <- request
		}()
		f := NewFactory(Dependencies{Fence: repo, Keys: keys, SessionStore: store, KeyDirectory: dir, Display: relay, ReadyProbe: func(context.Context) error { return nil }, Dialer: dialFunc(func(context.Context, string, http.Header) (devicebridge.Socket, error) { return socket, nil })})
		session, e := f.New(context.Background(), "https://example.test", d, nil)
		if e != nil {
			t.Fatal(e)
		}
		if e = session.(*Session).client.Connect(context.Background()); e != io.EOF {
			t.Fatal(e)
		}
		if request := <-shell; request.Title != "Waldo status" || request.Body != "Your Mac is connected." {
			t.Fatalf("shell did not receive the exact display text: %+v", request)
		}
		var status, reason any
		for _, w := range socket.writes {
			var frame map[string]any
			if e = json.Unmarshal(w, &frame); e != nil {
				t.Fatal(e)
			}
			if frame["type"] == "result" {
				p := frame["payload"].(map[string]any)
				status, reason = p["status"], p["reason"]
			}
		}
		if shellShows && status != "delivered" || !shellShows && (status != "failed" || reason != "delivery_unknown") {
			t.Fatalf("shellShows=%v produced status=%v reason=%v", shellShows, status, reason)
		}
	}
}
