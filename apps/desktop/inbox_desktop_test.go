package main

import (
	"reflect"
	"runtime"
	"testing"
)

func TestInboxRevealUsesSeparatedArguments(t *testing.T) {
	program, args, err := inboxRevealCommand("darwin", "", "/Users/test/中文 文件;$(touch nope).txt")
	if err != nil || program != "/usr/bin/open" || !reflect.DeepEqual(args, []string{"-R", "/Users/test/中文 文件;$(touch nope).txt"}) {
		t.Fatal(program, args, err)
	}
	if runtime.GOOS == "windows" {
		program, args, err = inboxRevealCommand("windows", `C:\Windows`, `C:\收到 文件\a,b & harmless.txt`)
		if err != nil || program != `C:\Windows\explorer.exe` || !reflect.DeepEqual(args, []string{"/select,", `C:\收到 文件\a,b & harmless.txt`}) {
			t.Fatal(program, args, err)
		}
	}
}

func TestInboxRevealRejectsUnresolvedOrUnsupportedTargets(t *testing.T) {
	for _, input := range [][3]string{{"darwin", "", "relative.txt"}, {"darwin", "", "/tmp/nul\x00name"}, {"windows", "relative", `C:\file.txt`}, {"linux", "", "/tmp/file.txt"}} {
		if _, _, err := inboxRevealCommand(input[0], input[1], input[2]); err == nil {
			t.Fatal(input)
		}
	}
}
