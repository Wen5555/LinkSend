package app

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/protocol"
)

func TestSessionReuseRequiresBilateralCapability(t *testing.T) {
	old, err := protocol.NewEnvelope("connect_response", "sender", "recipient",
		"0123456789abcdef0123456789abcdef", 1, map[string]string{"ufrag": "u", "password": "p"})
	if err != nil {
		t.Fatal(err)
	}
	if envelopeSupportsSessionReuse(old) {
		t.Fatal("legacy response was treated as reusable")
	}
	modern, err := protocol.NewEnvelope("connect_response", "sender", "recipient",
		"0123456789abcdef0123456789abcdef", 1, iceDescription{Ufrag: "u", Password: "p", SessionReuse: true})
	if err != nil {
		t.Fatal(err)
	}
	if !envelopeSupportsSessionReuse(modern) {
		t.Fatal("bilateral reuse capability was lost")
	}
	var legacy struct {
		Ufrag    string `json:"ufrag"`
		Password string `json:"password"`
	}
	if err := json.Unmarshal(modern.Payload, &legacy); err != nil || legacy.Ufrag != "u" || legacy.Password != "p" {
		t.Fatalf("new optional field broke legacy ICE parsing: %+v %v", legacy, err)
	}
}

func TestDirectPoolKeyLockSerializesSamePeer(t *testing.T) {
	service := &Service{directPoolLocks: make(map[string]*sync.Mutex)}
	lockA := service.directPoolKeyLock("peer/1")
	lockB := service.directPoolKeyLock("peer/1")
	if lockA != lockB {
		t.Fatal("same pool key received different connect locks")
	}
	lockA.Lock()
	entered := make(chan struct{})
	go func() {
		lockB.Lock()
		close(entered)
		lockB.Unlock()
	}()
	select {
	case <-entered:
		t.Fatal("same-key connect was not serialized")
	case <-time.After(25 * time.Millisecond):
	}
	lockA.Unlock()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("serialized connect did not resume")
	}
}
