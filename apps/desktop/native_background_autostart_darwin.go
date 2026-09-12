//go:build darwin

package main

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const nativeLaunchAgentLabel = "com.linksend.desktop.autostart"

var nativeAutostartMu sync.Mutex

func nativeLaunchAgentDirectory() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents"), nil
}

func installNativeAutostart(executable string) error {
	directory, err := nativeLaunchAgentDirectory()
	if err != nil {
		return err
	}
	return installNativeAutostartAt(executable, directory)
}

func uninstallNativeAutostart(executable string) error {
	directory, err := nativeLaunchAgentDirectory()
	if err != nil {
		return err
	}
	return uninstallNativeAutostartAt(executable, directory)
}

func nativeAutostartEnabled(executable string) (bool, error) {
	directory, err := nativeLaunchAgentDirectory()
	if err != nil {
		return false, err
	}
	executable, err = nativeAutostartExecutable(executable, false)
	if err != nil {
		return false, err
	}
	_, err = readOwnedNativeLaunchAgent(filepath.Join(directory, nativeLaunchAgentLabel+".plist"), executable)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

func nativeLaunchAgentXML(executable string) []byte {
	var escaped bytes.Buffer
	_ = xml.EscapeText(&escaped, []byte(executable))
	return []byte(fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>Label</key><string>%s</string>
  <key>LinkSendOwner</key><string>%s</string>
  <key>ProgramArguments</key><array><string>%s</string><string>--background</string></array>
  <key>RunAtLoad</key><true/>
  <key>LimitLoadToSessionType</key><string>Aqua</string>
</dict></plist>
`, nativeLaunchAgentLabel, nativeAutostartOwner, escaped.String()))
}

func readOwnedNativeLaunchAgent(path, executable string) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 64<<10 {
		return nil, errors.New("AUTOSTART_NOT_OWNED: existing launch agent was preserved")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(file, (64<<10)+1))
	_ = file.Close()
	if err != nil || len(data) > 64<<10 {
		return nil, errors.New("AUTOSTART_NOT_OWNED: existing launch agent could not be verified")
	}
	values := make(map[string]string)
	var arguments []string
	decoder := xml.NewDecoder(bytes.NewReader(data))
	key := ""
	seenPlist, seenDict, closedDict, closedPlist := false, false, false, false
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, errors.New("AUTOSTART_NOT_OWNED: malformed launch agent was preserved")
		}
		if end, ok := token.(xml.EndElement); ok {
			if end.Name.Local == "dict" {
				closedDict = true
			} else if end.Name.Local == "plist" {
				closedPlist = true
			}
			continue
		}
		if text, ok := token.(xml.CharData); ok && strings.TrimSpace(string(text)) != "" {
			return nil, errors.New("AUTOSTART_NOT_OWNED: unexpected launch agent text")
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		if closedDict || closedPlist {
			return nil, errors.New("AUTOSTART_NOT_OWNED: unexpected launch agent structure")
		}
		switch start.Name.Local {
		case "plist":
			if seenPlist {
				return nil, errors.New("AUTOSTART_NOT_OWNED: duplicate plist")
			}
			seenPlist = true
			continue
		case "dict":
			if !seenPlist || seenDict {
				return nil, errors.New("AUTOSTART_NOT_OWNED: nested launch agent dictionary")
			}
			seenDict = true
			continue
		case "key":
			if !seenDict || key != "" {
				return nil, errors.New("AUTOSTART_NOT_OWNED: unexpected launch agent structure")
			}
			if err := decoder.DecodeElement(&key, &start); err != nil {
				return nil, err
			}
		case "string":
			if key == "" {
				return nil, errors.New("AUTOSTART_NOT_OWNED: unexpected launch agent string")
			}
			if _, exists := values[key]; exists {
				return nil, errors.New("AUTOSTART_NOT_OWNED: duplicate launch agent key")
			}
			var value string
			if err := decoder.DecodeElement(&value, &start); err != nil {
				return nil, err
			}
			values[key], key = value, ""
		case "array":
			if key != "ProgramArguments" || arguments != nil {
				return nil, errors.New("AUTOSTART_NOT_OWNED: unexpected launch agent arguments")
			}
			var err error
			arguments, err = decodeNativeLaunchArguments(decoder)
			if err != nil {
				return nil, err
			}
			key = ""
		case "true":
			if key != "RunAtLoad" || values[key] != "" {
				return nil, errors.New("AUTOSTART_NOT_OWNED: unexpected launch agent setting")
			}
			values[key], key = "true", ""
		default:
			return nil, errors.New("AUTOSTART_NOT_OWNED: unexpected launch agent setting")
		}
	}
	if !seenPlist || !seenDict || !closedDict || !closedPlist || key != "" || len(values) != 4 || values["Label"] != nativeLaunchAgentLabel || values["LinkSendOwner"] != nativeAutostartOwner || values["RunAtLoad"] != "true" || values["LimitLoadToSessionType"] != "Aqua" || len(arguments) != 2 || arguments[0] != executable || arguments[1] != "--background" {
		return nil, errors.New("AUTOSTART_NOT_OWNED: existing launch agent belongs to another entry or installation")
	}
	return info, nil
}

func decodeNativeLaunchArguments(decoder *xml.Decoder) ([]string, error) {
	var arguments []string
	for {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		switch element := token.(type) {
		case xml.EndElement:
			if element.Name.Local == "array" {
				return arguments, nil
			}
			return nil, errors.New("AUTOSTART_NOT_OWNED: malformed argument array")
		case xml.StartElement:
			if element.Name.Local != "string" || len(arguments) >= 2 {
				return nil, errors.New("AUTOSTART_NOT_OWNED: unexpected argument value")
			}
			var value string
			if err := decoder.DecodeElement(&value, &element); err != nil {
				return nil, err
			}
			arguments = append(arguments, value)
		case xml.CharData:
			if strings.TrimSpace(string(element)) != "" {
				return nil, errors.New("AUTOSTART_NOT_OWNED: unexpected argument text")
			}
		}
	}
}

// Writes a current-user, next-login LaunchAgent. It does not bootstrap a job
// now, and does not terminate a running application when disabled.
func installNativeAutostartAt(executable, directory string) error {
	nativeAutostartMu.Lock()
	defer nativeAutostartMu.Unlock()
	executable, err := nativeAutostartExecutable(executable, true)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	path := filepath.Join(directory, nativeLaunchAgentLabel+".plist")
	if _, err := readOwnedNativeLaunchAgent(path, executable); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	temp, err := os.CreateTemp(directory, ".linksend-autostart-*.plist")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err := temp.Write(nativeLaunchAgentXML(executable)); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Link(temp.Name(), path) // Atomic no-replace, including late competitors.
}

func uninstallNativeAutostartAt(executable, directory string) error {
	nativeAutostartMu.Lock()
	defer nativeAutostartMu.Unlock()
	executable, err := nativeAutostartExecutable(executable, false)
	if err != nil {
		return err
	}
	path := filepath.Join(directory, nativeLaunchAgentLabel+".plist")
	info, err := readOwnedNativeLaunchAgent(path, executable)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	latest, err := os.Lstat(path)
	if err != nil || !os.SameFile(info, latest) {
		return errors.New("AUTOSTART_CHANGED: launch agent changed during removal")
	}
	return os.Remove(path)
}
