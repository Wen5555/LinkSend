package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	linksendapp "github.com/Wen5555/LinkSend/internal/app"
)

func TestNativeShareDevicesContainOnlySafeSelectionData(t *testing.T) {
	profile := t.TempDir()
	devices := []linksendapp.DeviceInfo{
		{ID: "trusted", Name: "远端名称", Online: false, Nearby: true, Trusted: true, PublicKey: "must-not-leak", Profile: linksendapp.DeviceProfile{Alias: "我的电脑"}},
		{ID: "blocked", Name: "blocked", Trusted: true, Blocked: true},
		{ID: "untrusted", Name: "untrusted", Nearby: true},
	}
	if err := publishNativeShareDevices(profile, devices); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(profile, "native-share-v1", "devices.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 || !json.Valid(data) {
		t.Fatal("invalid snapshot", string(data))
	}
	var snapshot nativeShareDeviceSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Version != 1 || len(snapshot.Devices) != 1 || snapshot.Devices[0].ID != "trusted" || snapshot.Devices[0].Name != "我的电脑" || !snapshot.Devices[0].Reachable {
		t.Fatalf("wrong snapshot: %+v", snapshot)
	}
	if bytes.Contains(data, []byte("must-not-leak")) {
		t.Fatal("public key leaked")
	}
	if err := publishNativeShareDevices(profile, devices); err != nil {
		t.Fatal("atomic replacement failed:", err)
	}
	repeated, err := os.ReadFile(filepath.Join(profile, "native-share-v1", "devices.json"))
	if err != nil || !bytes.Equal(data, repeated) {
		t.Fatal("unchanged device snapshot was rewritten", err)
	}
}
