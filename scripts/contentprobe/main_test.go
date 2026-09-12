package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestProbeRefusesExistingProfilesAndImplicitNetwork(t *testing.T) {
	root := t.TempDir()
	profile := filepath.Join(root, "user-profile")
	if err := os.Mkdir(profile, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(profile, "user.txt")
	if err := os.WriteFile(path, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"--mode", "init", "--profile", profile}, io.Discard); err == nil {
		t.Fatal("existing user profile was reused")
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "preserve" {
		t.Fatal("existing file changed")
	}
	for _, address := range []string{"0.0.0.0:0", "localhost:1", "127.0.0.1:1", "224.0.0.1:1", "10.0.0.1"} {
		if _, err := parseAddress(address, true, false); err == nil {
			t.Fatalf("unapproved address %q accepted", address)
		}
	}
	if _, err := decodePeer("wrong key"); err == nil {
		t.Fatal("missing identity pin accepted")
	}
}

func TestProbeLocalQUICBothRolesWithThreeNativeKinds(t *testing.T) {
	root := t.TempDir()
	sender := filepath.Join(root, "sender")
	receiver := filepath.Join(root, "receiver")
	initialiseProfile := func(path string) string {
		t.Helper()
		var out bytes.Buffer
		if err := run([]string{"--mode", "init", "--profile", path}, &out); err != nil {
			t.Fatal(err)
		}
		var value struct {
			PublicKey string `json:"public_key"`
		}
		if err := json.Unmarshal(out.Bytes(), &value); err != nil {
			t.Fatal(err)
		}
		return value.PublicKey
	}
	senderKey, receiverKey := initialiseProfile(sender), initialiseProfile(receiver)
	reader, writer := io.Pipe()
	serverDone := make(chan error, 1)
	go func() {
		err := run([]string{"--mode", "receive", "--profile", receiver, "--listen", "127.0.0.1:0", "--peer-key", senderKey, "--out", filepath.Join(receiver, "results"), "--allow-loopback", "--timeout", "15s"}, writer)
		_ = writer.Close()
		serverDone <- err
	}()
	decoder := json.NewDecoder(reader)
	var ready struct {
		Ready bool   `json:"ready"`
		Local string `json:"local_address"`
	}
	if err := decoder.Decode(&ready); err != nil || !ready.Ready {
		t.Fatal("listener not ready", err)
	}
	reportsDone := make(chan []report, 1)
	decodeErr := make(chan error, 1)
	go func() {
		reports := make([]report, 0, 3)
		for {
			var r report
			err := decoder.Decode(&r)
			if err == io.EOF {
				reportsDone <- reports
				decodeErr <- nil
				return
			}
			if err != nil {
				reportsDone <- reports
				decodeErr <- err
				return
			}
			reports = append(reports, r)
		}
	}()
	var sent bytes.Buffer
	if err := run([]string{"--mode", "send", "--profile", sender, "--listen", "127.0.0.1:0", "--remote", ready.Local, "--peer-key", receiverKey, "--out", filepath.Join(sender, "results"), "--allow-loopback", "--label", "local-fixture-only", "--timeout", "15s"}, &sent); err != nil {
		t.Fatal(err)
	}
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
	received := <-reportsDone
	if err := <-decodeErr; err != nil {
		t.Fatal(err)
	}
	if len(received) != 3 {
		t.Fatalf("received %d native kinds", len(received))
	}
	sentDecoder := json.NewDecoder(&sent)
	for _, receive := range received {
		var send report
		if err := sentDecoder.Decode(&send); err != nil {
			t.Fatal(err)
		}
		if receive.Kind != send.Kind || receive.ContentDigest != send.ContentDigest || receive.BodySHA256 != send.BodySHA256 || receive.BodyBLAKE3 != send.BodyBLAKE3 || receive.BodyBytes != send.BodyBytes || receive.Mode != "content_v1" || receive.Relay {
			t.Fatal("actual native transfer evidence mismatch", send, receive)
		}
	}
}
