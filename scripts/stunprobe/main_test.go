package main

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/pion/stun/v4"
)

// Exercise false-pass boundaries with a real UDP responder. This is a control
// protocol fixture, not a substitute for external STUN or production ICE.
func TestSTUNOnlyRejectsTURNAllocation(t *testing.T) {
	for _, accepts := range []bool{false, true} {
		t.Run(map[bool]string{false: "rejected", true: "accepted"}[accepts], func(t *testing.T) {
			server := serveFixture(t, func(request *stun.Message, remote *net.UDPAddr) *stun.Message {
				setID := stun.NewTransactionIDSetter(request.TransactionID)
				if request.Type == stun.BindingRequest {
					return stun.MustBuild(setID, stun.BindingSuccess, stun.XORMappedAddress{IP: remote.IP, Port: remote.Port}, stun.Fingerprint)
				}
				if accepts {
					return stun.MustBuild(setID, stun.NewType(stun.MethodAllocate, stun.ClassSuccessResponse), stun.Fingerprint)
				}
				return stun.MustBuild(setID, stun.NewType(stun.MethodAllocate, stun.ClassErrorResponse), stun.ErrorCodeAttribute{Code: stun.CodeForbidden}, stun.Fingerprint)
			})
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			result, err := checkSTUNOnly(ctx, "127.0.0.1", server)
			if result["binding_success"] != true || result["allocation_observed"] != accepts || (err != nil) != accepts {
				t.Fatalf("result=%v err=%v", result, err)
			}
		})
	}
}

func TestSTUNOnlyFailureIsNotATURNPass(t *testing.T) {
	for _, mode := range []string{"binding_rejected", "wrong_transaction", "bad_fingerprint", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			server := serveFixture(t, func(request *stun.Message, remote *net.UDPAddr) *stun.Message {
				setID := stun.NewTransactionIDSetter(request.TransactionID)
				if mode == "binding_rejected" {
					return stun.MustBuild(setID, stun.BindingError, stun.ErrorCodeAttribute{Code: stun.CodeForbidden}, stun.Fingerprint)
				}
				if mode == "wrong_transaction" {
					setID = stun.TransactionID
				}
				response := stun.MustBuild(setID, stun.BindingSuccess, stun.XORMappedAddress{IP: remote.IP, Port: remote.Port}, stun.Fingerprint)
				if mode == "bad_fingerprint" {
					response.Raw[len(response.Raw)-1] ^= 1
				}
				return response
			})
			ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			defer cancel()
			if mode == "cancelled" {
				cancel()
			}
			result, err := checkSTUNOnly(ctx, "127.0.0.1", server)
			if err == nil || result["binding_success"] != false || result["allocation_observed"] != false {
				t.Fatalf("false pass: result=%v err=%v", result, err)
			}
		})
	}
}

func serveFixture(t *testing.T, response func(*stun.Message, *net.UDPAddr) *stun.Message) string {
	t.Helper()
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	t.Cleanup(func() { _ = conn.Close(); <-done })
	go func() {
		defer close(done)
		buffer := make([]byte, 2048)
		for {
			n, remote, readErr := conn.ReadFromUDP(buffer)
			if errors.Is(readErr, net.ErrClosed) {
				return
			}
			if readErr != nil {
				t.Errorf("fixture read: %v", readErr)
				return
			}
			request := &stun.Message{Raw: append([]byte(nil), buffer[:n]...)}
			if decodeErr := request.Decode(); decodeErr != nil {
				t.Errorf("fixture decode: %v", decodeErr)
				return
			}
			if _, writeErr := conn.WriteToUDP(response(request, remote).Raw, remote); writeErr != nil && !errors.Is(writeErr, net.ErrClosed) {
				t.Errorf("fixture write: %v", writeErr)
				return
			}
		}
	}()
	return conn.LocalAddr().String()
}
