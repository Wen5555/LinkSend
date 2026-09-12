//go:build windows

package main

import (
	"errors"
	"os"
	"strings"

	"golang.org/x/sys/windows/registry"
)

func prepareNativeNotificationHarness(name string) (func(), error) {
	if !strings.HasPrefix(name, "LinkSend Native Test ") {
		return nil, errors.New("native test registry name must be isolated")
	}
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	appKey := `Software\Classes\AppUserModelId\` + name
	if existing, err := registry.OpenKey(registry.CURRENT_USER, appKey, registry.QUERY_VALUE); err == nil {
		_ = existing.Close()
		return nil, errors.New("native test app registration already exists")
	} else if !errors.Is(err, registry.ErrNotExist) {
		return nil, err
	}
	const defaultActivation = `Software\Classes\CLSID\{0F82E845-CB89-4039-BDBF-67CA33254C76}`
	defaultExisted := false
	if key, err := registry.OpenKey(registry.CURRENT_USER, defaultActivation, registry.READ); err == nil {
		defaultExisted = true
		_ = key.Close()
	}
	return func() {
		guid, _ := nativeAutostartReadValue(appKey, "CustomActivator")
		removeActivation := func(root string) {
			key, err := registry.OpenKey(registry.CURRENT_USER, root+`\LocalServer32`, registry.QUERY_VALUE)
			if err != nil {
				return
			}
			value, _, valueErr := key.GetStringValue("")
			names, namesErr := key.ReadValueNames(-1)
			_ = key.Close()
			if valueErr == nil && namesErr == nil && len(names) == 1 && (value == executable || value == `"`+executable+`" %1`) {
				_ = registry.DeleteKey(registry.CURRENT_USER, root+`\LocalServer32`)
				_ = registry.DeleteKey(registry.CURRENT_USER, root)
			}
		}
		if len(guid) == 38 && strings.HasPrefix(guid, "{") && strings.HasSuffix(guid, "}") {
			removeActivation(`Software\Classes\CLSID\` + guid)
		}
		if !defaultExisted {
			removeActivation(defaultActivation)
		}
		_ = registry.DeleteKey(registry.CURRENT_USER, appKey)
		_ = registry.DeleteKey(registry.CURRENT_USER, `Software\`+name+`\NotificationCategories`)
		_ = registry.DeleteKey(registry.CURRENT_USER, `Software\`+name)
	}, nil
}
