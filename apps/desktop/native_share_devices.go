package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"

	linksendapp "github.com/Wen5555/LinkSend/internal/app"
	"github.com/Wen5555/LinkSend/internal/protocol"
)

type nativeShareDevice struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Reachable bool   `json:"reachable"`
}

type nativeShareDeviceSnapshot struct {
	Version     int                 `json:"version"`
	Revision    uint64              `json:"revision"`
	GeneratedAt string              `json:"generated_at"`
	Devices     []nativeShareDevice `json:"devices"`
}

func publishNativeShareDevices(profile string, devices []linksendapp.DeviceInfo) error {
	records := make([]nativeShareDevice, 0, len(devices))
	for _, device := range devices {
		if !device.Trusted || device.Blocked || device.ID == "" {
			continue
		}
		name := device.Profile.Alias
		if name == "" {
			name = device.Name
		}
		if name == "" {
			name = device.ID[:min(10, len(device.ID))]
		}
		records = append(records, nativeShareDevice{ID: device.ID, Name: name, Reachable: device.Online || device.Nearby})
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].Name == records[j].Name {
			return records[i].ID < records[j].ID
		}
		return records[i].Name < records[j].Name
	})
	if len(records) > 256 {
		records = records[:256]
	}
	now := time.Now().UTC()
	snapshot := nativeShareDeviceSnapshot{Version: 1, Revision: uint64(now.UnixNano()), GeneratedAt: now.Format(time.RFC3339Nano), Devices: records}
	data, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	var firstErr error
	for _, root := range nativeShareRoots(profile) {
		if err := writeNativeShareDevices(root, data); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func writeNativeShareDevices(rootPath string, data []byte) error {
	directory := filepath.Join(rootPath, "native-share-v1")
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("SYSTEM_SHARE_STORAGE: device directory must be local")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	if existing, openErr := root.Open("devices.json"); openErr == nil {
		existing.Close()
	} else if !errors.Is(openErr, os.ErrNotExist) {
		return openErr
	}
	temporary := protocol.RandomID() + ".pending"
	file, err := root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer root.Remove(temporary)
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = root.Rename(temporary, "devices.json"); err != nil {
		return err
	}
	return syncActivationDirectory(root)
}

func publishNativeShareReceipt(rootPath, requestID string) error {
	if !activationName.MatchString(requestID + ".json") {
		return errors.New("SYSTEM_SHARE_INVALID: receipt request ID")
	}
	directory := filepath.Join(rootPath, "native-share-v1", "accepted")
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	path := filepath.Join(directory, requestID)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	_, writeErr := file.WriteString("queued\n")
	if writeErr == nil {
		writeErr = file.Sync()
	}
	if closeErr := file.Close(); writeErr == nil {
		writeErr = closeErr
	}
	return writeErr
}

func removeNativeShareReceipt(rootPath, requestID string) {
	if activationName.MatchString(requestID + ".json") {
		_ = os.Remove(filepath.Join(rootPath, "native-share-v1", "accepted", requestID))
	}
}
