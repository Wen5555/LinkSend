//go:build windows

package app

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

type cacheNativeProcessEvidence struct {
	Profile          string `json:"profile"`
	CacheID          string `json:"cache_id"`
	Owner            string `json:"owner"`
	OriginalRun      string `json:"original_run"`
	NewRun           string `json:"new_run,omitempty"`
	SourceSHA256     string `json:"source_sha256"`
	HelperHandle     uint64 `json:"helper_handle,omitempty"`
	DeadlineUnixNano int64  `json:"deadline_unix_nano"`
	Consumed         bool   `json:"consumed"`
	SourceProtected  bool   `json:"source_protected"`
}

func cacheNativeRoleContext(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	absolute, e := strconv.ParseInt(os.Getenv("LINKSEND_NATIVE_PROCESS_DEADLINE"), 10, 64)
	if e != nil {
		t.Fatal(e)
	}
	observedAt := time.Now()
	remaining := time.Unix(0, absolute).Sub(observedAt)
	if remaining <= 0 || remaining > 30*time.Second {
		t.Fatal("invalid original process deadline")
	}
	return context.WithDeadline(t.Context(), observedAt.Add(remaining))
}

// The launcher and verifier execute this role in genuinely separate test
// processes. Only the parent opt-in supplies self-owned roots and handles.
func TestClipboardCacheWindowsNativeProcessRole(t *testing.T) {
	role := os.Getenv("LINKSEND_NATIVE_PROCESS_ROLE")
	if role == "" {
		t.Skip("private process role")
	}
	ctx, cancel := cacheNativeRoleContext(t)
	defer cancel()
	switch role {
	case "launcher":
		f := newCacheNativeFixture(t)
		launch, e := f.r.prepareCacheNativeWindowsLifetime(ctx, os.Getenv("LINKSEND_NATIVE_HELPER_TEST_EXE"), os.Getenv("LINKSEND_NATIVE_HELPER_TEST_SHA256"), true)
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() { _ = launch.close() })
		d, e := f.r.sealNativeDelegation(ctx, launch)
		if e != nil {
			t.Fatal(e)
		}
		held, e := launch.detach(ctx, d)
		if e != nil {
			t.Fatal(e)
		}
		target, e := strconv.ParseUint(os.Getenv("LINKSEND_NATIVE_PROCESS_DRIVER_HANDLE"), 10, 64)
		if e != nil {
			t.Fatal(e)
		}
		var driverHelper windows.Handle
		if e = windows.DuplicateHandle(windows.CurrentProcess(), launch.process, windows.Handle(target), &driverHelper, windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, 0); e != nil {
			t.Fatal(e)
		}
		sum := sha256.Sum256(f.body)
		evidence := cacheNativeProcessEvidence{Profile: f.profile, CacheID: f.r.record.ID, Owner: f.s.identity.ID(), OriginalRun: f.s.epoch, SourceSHA256: hex.EncodeToString(sum[:]), HelperHandle: uint64(driverHelper), DeadlineUnixNano: held.DeadlineUnixNano}
		if e = json.NewEncoder(os.Stdout).Encode(evidence); e != nil {
			t.Fatal(e)
		}
		// The driver first pins the duplicated actual helper handle and checks
		// its image/liveness and empty spool, then permits this normal exit.
		ack := make(chan string, 1)
		go func() { s, _ := bufio.NewReader(os.Stdin).ReadString('\n'); ack <- s }()
		select {
		case s := <-ack:
			if s != "RELEASE\n" {
				t.Fatal("invalid driver release")
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		f.closeStorage()
		f.s.Shutdown()
		exit, e := strconv.Atoi(os.Getenv("LINKSEND_NATIVE_PROCESS_EXIT"))
		if e != nil {
			t.Fatal(e)
		}
		// Test child is launched without -test.paniconexit0. os.Exit closes
		// actual process/kernel resources; Service reopen is not substituted.
		os.Exit(exit)
	case "verifier":
		var evidence cacheNativeProcessEvidence
		if e := json.Unmarshal([]byte(os.Getenv("LINKSEND_NATIVE_PROCESS_EVIDENCE")), &evidence); e != nil {
			t.Fatal(e)
		}
		s, e := New(Config{DataDir: evidence.Profile})
		if e != nil {
			t.Fatal(e)
		}
		defer s.Shutdown()
		if s.epoch == evidence.OriginalRun || s.identity.ID() != evidence.Owner {
			t.Fatal("new process did not reload the original real profile identity")
		}
		record := loadTestCacheRecord(t, s, evidence.CacheID)
		if record.Run != evidence.OriginalRun || record.Native == nil || record.Native.Phase != "delegated" {
			t.Fatal("original delegated publication was not retained")
		}
		expect := os.Getenv("LINKSEND_NATIVE_PROCESS_EXPECT_RECEIPT") == "true"
		ok, e := s.consumeCacheNativeSpool(ctx, evidence.CacheID, cacheNativeWindowsBoot{})
		if expect {
			if !ok || e != nil {
				t.Fatal("new real main failed to consume original spool", e)
			}
			raw := cacheNativeTestRaw(t, s, evidence.CacheID)
			if again, e := s.consumeCacheNativeSpool(ctx, evidence.CacheID, cacheNativeWindowsBoot{}); again || !errors.Is(e, errCacheNativeConsumed) {
				t.Fatal("real new main accepted replay", e)
			}
			if cacheNativeTestRaw(t, s, evidence.CacheID) != raw {
				t.Fatal("replay changed first observation")
			}
		} else if ok || e == nil {
			t.Fatal("abnormal launcher exit yielded a consumable receipt")
		}
		if removed, e := s.collectClipboardCache(ctx, true, time.Now().Add(365*24*time.Hour)); e != nil || removed != 0 {
			t.Fatal("Native object was collected", e)
		}
		body, e := os.ReadFile(filepath.Join(evidence.Profile, "clipboard-files", evidence.CacheID, "owned.txt"))
		if e != nil {
			t.Fatal(e)
		}
		hash := sha256.Sum256(body)
		if hex.EncodeToString(hash[:]) != evidence.SourceSHA256 {
			t.Fatal("source bytes changed")
		}
		evidence.NewRun = s.epoch
		evidence.Consumed = ok
		evidence.SourceProtected = true
		evidence.HelperHandle = 0
		if e = json.NewEncoder(os.Stdout).Encode(evidence); e != nil {
			t.Fatal(e)
		}
	default:
		t.Fatal("unknown private process role")
	}
}

func cacheNativeProcessCommand(t *testing.T, ctx context.Context, role, root string, deadline time.Time, extra []string) *exec.Cmd {
	t.Helper()
	image, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	cmd := exec.CommandContext(ctx, image, "-test.run=^TestClipboardCacheWindowsNativeProcessRole$", "-test.paniconexit0=false")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	env := make([]string, 0, len(os.Environ())+8)
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		if strings.EqualFold(key, "TMP") || strings.EqualFold(key, "TEMP") || strings.HasPrefix(key, "LINKSEND_NATIVE_PROCESS_") {
			continue
		}
		env = append(env, item)
	}
	cmd.Env = append(env, "TMP="+root, "TEMP="+root, "LINKSEND_NATIVE_PROCESS_ROLE="+role, "LINKSEND_NATIVE_PROCESS_DEADLINE="+strconv.FormatInt(deadline.UnixNano(), 10))
	cmd.Env = append(cmd.Env, extra...)
	return cmd
}

func TestClipboardCacheWindowsNativeProcessLifetime(t *testing.T) {
	image, expected := os.Getenv("LINKSEND_NATIVE_HELPER_TEST_EXE"), os.Getenv("LINKSEND_NATIVE_HELPER_TEST_SHA256")
	if image == "" || expected == "" {
		t.Skip("requires an explicitly source-verified helper and digest")
	}
	for _, exitCode := range []int{0, 9} {
		t.Run(fmt.Sprintf("launcher_exit_%d", exitCode), func(t *testing.T) {
			root := t.TempDir()
			deadline := time.Now().Add(30 * time.Second)
			ctx, cancel := context.WithDeadline(t.Context(), deadline)
			defer cancel()
			var driver windows.Handle
			if e := windows.DuplicateHandle(windows.CurrentProcess(), windows.CurrentProcess(), windows.CurrentProcess(), &driver, windows.PROCESS_DUP_HANDLE|windows.PROCESS_QUERY_LIMITED_INFORMATION, true, 0); e != nil {
				t.Fatal(e)
			}
			defer windows.CloseHandle(driver)
			cmd := cacheNativeProcessCommand(t, ctx, "launcher", root, deadline, []string{"LINKSEND_NATIVE_PROCESS_DRIVER_HANDLE=" + strconv.FormatUint(uint64(driver), 10), "LINKSEND_NATIVE_PROCESS_EXIT=" + strconv.Itoa(exitCode)})
			cmd.SysProcAttr.AdditionalInheritedHandles = []syscall.Handle{syscall.Handle(driver)}
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			stdout, e := cmd.StdoutPipe()
			if e != nil {
				t.Fatal(e)
			}
			stdin, e := cmd.StdinPipe()
			if e != nil {
				t.Fatal(e)
			}
			if e = cmd.Start(); e != nil {
				t.Fatal(e)
			}
			waited := false
			defer func() {
				if !waited {
					_ = cmd.Process.Kill()
					_ = cmd.Wait()
				}
			}()
			var evidence cacheNativeProcessEvidence
			if e = json.NewDecoder(io.LimitReader(stdout, 16<<10)).Decode(&evidence); e != nil {
				t.Fatal("launcher did not reach real consumer hold", e, stderr.String())
			}
			rel, e := filepath.Rel(root, evidence.Profile)
			if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
				t.Fatal("child profile escaped the owned test root")
			}
			if evidence.DeadlineUnixNano != deadline.UnixNano() || !validClipboardFileID(evidence.CacheID, 16) || evidence.HelperHandle == 0 {
				t.Fatal("original fixed deadline or publication handle missing")
			}
			helper := windows.Handle(evidence.HelperHandle)
			defer windows.CloseHandle(helper)
			// This is a directly duplicated kernel process object from the
			// verified launcher, not OpenProcess(reportedPID).
			if state, e := windows.WaitForSingleObject(helper, 0); e != nil || state != uint32(windows.WAIT_TIMEOUT) {
				t.Fatal("helper was not alive with consumer refs held", e)
			}
			name := make([]uint16, 32768)
			size := uint32(len(name))
			if e = windows.QueryFullProcessImageName(helper, 0, &name[0], &size); e != nil {
				t.Fatal(e)
			}
			actual, e := os.Open(windows.UTF16ToString(name[:size]))
			if e != nil {
				t.Fatal(e)
			}
			hash := sha256.New()
			_, e = io.Copy(hash, actual)
			_ = actual.Close()
			if e != nil || hex.EncodeToString(hash.Sum(nil)) != expected {
				t.Fatal("duplicated helper object has wrong image", e)
			}
			spool := filepath.Join(evidence.Profile, cacheNativeSpoolDirectory, evidence.CacheID+".json")
			if info, e := os.Stat(spool); e != nil || info.Size() != 0 {
				t.Fatal("held consumer already produced receipt", e)
			}
			if _, e = io.WriteString(stdin, "RELEASE\n"); e != nil {
				t.Fatal(e)
			}
			_ = stdin.Close()
			e = cmd.Wait()
			waited = true
			if exitCode == 0 && e != nil {
				t.Fatal("actual launcher exit was not zero", e, stderr.String())
			}
			if exitCode != 0 {
				var failed *exec.ExitError
				if !errors.As(e, &failed) || failed.ExitCode() != exitCode {
					t.Fatal("actual nonzero launcher exit was not observed", e)
				}
			}
			for {
				state, e := windows.WaitForSingleObject(helper, 10)
				if e != nil {
					t.Fatal(e)
				}
				if state == windows.WAIT_OBJECT_0 {
					break
				}
				if ctx.Err() != nil {
					_ = windows.TerminateProcess(helper, 1)
					t.Fatal(ctx.Err())
				}
			}
			var helperExit uint32
			if e = windows.GetExitCodeProcess(helper, &helperExit); e != nil {
				t.Fatal(e)
			}
			if (exitCode == 0 && helperExit != 0) || (exitCode != 0 && helperExit != 1) {
				t.Fatalf("helper exit=%d after launcher exit=%d", helperExit, exitCode)
			}
			info, e := os.Stat(spool)
			if e != nil || (exitCode == 0 && info.Size() == 0) || (exitCode != 0 && info.Size() != 0) {
				t.Fatal("spool did not reflect actual successful/failed launcher exit", e)
			}
			raw, _ := json.Marshal(evidence)
			verifier := cacheNativeProcessCommand(t, ctx, "verifier", root, deadline, []string{"LINKSEND_NATIVE_PROCESS_EVIDENCE=" + string(raw), "LINKSEND_NATIVE_PROCESS_EXPECT_RECEIPT=" + strconv.FormatBool(exitCode == 0)})
			output, e := verifier.CombinedOutput()
			if e != nil {
				t.Fatal("independent new main failed", e, string(output))
			}
			var verified cacheNativeProcessEvidence
			if e = json.NewDecoder(bytes.NewReader(output)).Decode(&verified); e != nil || !verified.SourceProtected || verified.Consumed != (exitCode == 0) || verified.NewRun == evidence.OriginalRun {
				t.Fatal("independent main evidence invalid", e, string(output))
			}
			helperPID, _ := windows.GetProcessId(helper)
			t.Logf("launcher_pid=%d actual_exit=%d helper_pid=%d actual_exit=%d verifier_pid=%d original_deadline_preserved=true source_protected=true consumed=%t", cmd.Process.Pid, exitCode, helperPID, helperExit, verifier.Process.Pid, verified.Consumed)
		})
	}
}
