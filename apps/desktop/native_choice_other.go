//go:build !windows

package main

import (
	"errors"
	"github.com/wailsapp/wails/v3/pkg/application"
	"sync"
)

func showNativeChoice(window application.Window, title, message string, labels []string, defaultIndex, cancelIndex int, done func(int, error)) {
	if len(labels) < 1 || defaultIndex < 0 || defaultIndex >= len(labels) || cancelIndex < 0 || cancelIndex >= len(labels) {
		go done(-1, errors.New("NATIVE_CHOICE_INVALID"))
		return
	}
	dialog := application.Get().Dialog.Question().SetTitle(title).SetMessage(message)
	var once sync.Once
	buttons := make([]*application.Button, len(labels))
	for i, label := range labels {
		buttons[i] = dialog.AddButton(label).OnClick(func() { once.Do(func() { go done(i, nil) }) })
	}
	dialog.SetDefaultButton(buttons[defaultIndex]).SetCancelButton(buttons[cancelIndex])
	if window != nil {
		dialog.AttachToWindow(window)
	}
	dialog.Show()
}
