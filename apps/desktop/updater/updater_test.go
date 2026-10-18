package updater

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func fixture(body []byte, platform string) release {
	name := "LinkSend-v0.6.2-" + platform + "-" + strings.Repeat("a", 40) + ".dmg"
	if platform == "windows-amd64" {
		name = "LinkSend-v0.6.2-windows-amd64-installer-" + strings.Repeat("a", 40) + ".exe"
	}
	hash := sha256.Sum256(body)
	return release{Tag: "v0.6.2", URL: "https://github.com/Wen5555/LinkSend/releases/tag/v0.6.2", Body: "更新说明\n<script>不可执行</script>", Assets: []releaseAsset{{Name: name, Size: int64(len(body)), Digest: "sha256:" + hex.EncodeToString(hash[:]), State: "uploaded", URL: "https://github.com/Wen5555/LinkSend/releases/download/v0.6.2/" + name}}}
}

func response(request *http.Request, body []byte, code int) *http.Response {
	return &http.Response{StatusCode: code, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(body)), ContentLength: int64(len(body)), Request: request}
}

func managerFixture(t *testing.T, r release, body []byte) *Manager {
	t.Helper()
	m := New(context.Background(), "0.6.1", "windows", "amd64", t.TempDir())
	m.client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("Authorization") != "" {
			t.Error("updater must not send credentials")
		}
		if request.URL.String() == latestURL {
			metadata, _ := json.Marshal(r)
			return response(request, metadata, 200), nil
		}
		if request.URL.String() == r.Assets[0].URL {
			return response(request, body, 200), nil
		}
		return nil, fmt.Errorf("unexpected URL: %s", request.URL)
	})
	t.Cleanup(m.Close)
	return m
}

func waitIdle(t *testing.T, m *Manager) Status {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		status := m.Status()
		if !status.Busy {
			return status
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("updater did not finish")
	return Status{}
}

func TestVersionComparisonNeverDowngrades(t *testing.T) {
	for _, tc := range []struct {
		latest, current string
		newer           bool
	}{
		{"v0.6.2", "0.6.1", true}, {"v0.10.0", "0.9.9", true},
		{"v1.0.0", "0.99.99", true}, {"v0.6.1", "0.6.1", false},
		{"v0.6.1", "0.7.0", false}, {"v0.6.1", "0.7.0-preview.1", false},
		{"v0.6.1", "0.6.1-preview.2", true}, {"v0.6.1", "0.6.1+build", false},
	} {
		t.Run(tc.latest+"-"+tc.current, func(t *testing.T) {
			got, err := newerVersion(tc.latest, tc.current)
			if err != nil || got != tc.newer {
				t.Fatalf("newer=%v error=%v", got, err)
			}
		})
	}
	for _, latest := range []string{"v0.6.2-preview.1", "v00.6.2", "../../v0.6.2", "v18446744073709551616.0.0"} {
		if _, err := newerVersion(latest, "0.6.1"); err == nil {
			t.Fatalf("accepted %q", latest)
		}
	}
}

func TestReleaseSelectionBindsPlatformAndOfficialAsset(t *testing.T) {
	for _, platform := range []string{"windows-amd64", "macos-arm64", "macos-amd64"} {
		r := fixture([]byte("installer"), platform)
		asset, err := selectAsset(r, platform)
		if err != nil || asset == nil {
			t.Fatalf("%s: asset=%v error=%v", platform, asset, err)
		}
		other, err := selectAsset(r, "linux-amd64")
		if err != nil || other != nil {
			t.Fatal("offered unsupported platform")
		}
	}
	for _, mutate := range []func(*release){
		func(r *release) { r.Assets[0].Digest = "" },
		func(r *release) { r.Assets[0].Digest = "sha256:" + strings.Repeat("z", 64) },
		func(r *release) {
			r.Assets[0].URL = strings.Replace(r.Assets[0].URL, "github.com/", "evil.example/", 1)
		},
		func(r *release) { r.Assets[0].URL += "?redirect=other" },
		func(r *release) { r.Assets[0].Size = maxPackageBytes + 1 },
		func(r *release) { r.Assets[0].State = "new" },
		func(r *release) { r.Assets = append(r.Assets, r.Assets[0]) },
	} {
		r := fixture([]byte("installer"), "windows-amd64")
		mutate(&r)
		if _, err := selectAsset(r, "windows-amd64"); err == nil {
			t.Fatalf("accepted invalid asset: %+v", r.Assets)
		}
	}
	mac := fixture([]byte("installer"), "macos-arm64")
	if asset, err := selectAsset(mac, "macos-amd64"); asset != nil || err != nil {
		t.Fatal("selected wrong Mac architecture")
	}
}

func TestCheckRejectsDraftPrereleaseForeignAndInvalidMetadata(t *testing.T) {
	for _, mutate := range []func(*release){
		func(r *release) { r.Draft = true }, func(r *release) { r.Prerelease = true },
		func(r *release) { r.URL = "https://github.com/another/repo/releases/tag/v0.6.2" },
		func(r *release) { r.Tag = "v0.6.2-preview.1" },
	} {
		r := fixture([]byte("installer"), "windows-amd64")
		mutate(&r)
		m := managerFixture(t, r, nil)
		m.Check()
		status := waitIdle(t, m)
		if status.Stage != "error" || status.CanDownload || status.CanOpen {
			t.Fatalf("unsafe metadata accepted: %+v", status)
		}
	}
}

func TestCheckDownloadAndRecheckBeforeNativeOpen(t *testing.T) {
	body := []byte("verified installer bytes")
	m := managerFixture(t, fixture(body, "windows-amd64"), body)
	m.Download()
	if m.Status().Busy {
		t.Fatal("download started before release check")
	}
	m.Check()
	checked := waitIdle(t, m)
	if !checked.CanDownload || !checked.UpdateAvailable || checked.LatestVersion != "0.6.2" {
		t.Fatalf("check: %+v", checked)
	}
	m.Download()
	ready := waitIdle(t, m)
	if !ready.CanOpen || ready.CanDownload || ready.DownloadedBytes != int64(len(body)) {
		t.Fatalf("download: %+v", ready)
	}
	var opened string
	m.OpenPackage(func(file string) error { opened = file; return nil })
	if status := waitIdle(t, m); status.Stage != "opened" || opened == "" {
		t.Fatalf("open: %+v", status)
	}
	if err := os.WriteFile(opened, bytes.Repeat([]byte("x"), len(body)), 0600); err != nil {
		t.Fatal(err)
	}
	m.OpenPackage(func(string) error { t.Error("launched modified package"); return nil })
	if status := waitIdle(t, m); status.Stage != "error" || status.CanOpen || !status.CanDownload {
		t.Fatalf("tamper: %+v", status)
	}
}

func TestBadDigestAndTruncatedDownloadNeverBecomeInstallable(t *testing.T) {
	for _, body := range [][]byte{[]byte("tampered bytes"), []byte("short")} {
		m := managerFixture(t, fixture([]byte("original bytes"), "windows-amd64"), body)
		m.Check()
		waitIdle(t, m)
		m.Download()
		status := waitIdle(t, m)
		if status.Stage != "error" || status.CanOpen || !status.CanDownload {
			t.Fatalf("bad download: %+v", status)
		}
		entries, err := os.ReadDir(m.cacheDir)
		if err != nil || len(entries) != 0 {
			t.Fatalf("partial download retained: %v %v", entries, err)
		}
	}
}

func TestCancelledDownloadIsSingleFlightAndRetryable(t *testing.T) {
	body := []byte("download content")
	r := fixture(body, "windows-amd64")
	m := managerFixture(t, r, body)
	m.Check()
	waitIdle(t, m)
	started := make(chan struct{})
	var calls atomic.Int32
	original := m.client.Transport
	m.client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls.Add(1)
		close(started)
		reader, writer := io.Pipe()
		go func() { <-request.Context().Done(); _ = writer.CloseWithError(request.Context().Err()) }()
		return &http.Response{StatusCode: 200, Body: reader, ContentLength: -1, Header: make(http.Header), Request: request}, nil
	})
	m.Download()
	<-started
	m.Check()
	m.Download()
	m.Cancel()
	status := waitIdle(t, m)
	if calls.Load() != 1 || status.Stage != "cancelled" || status.CanOpen || !status.CanDownload {
		t.Fatalf("cancel: calls=%d status=%+v", calls.Load(), status)
	}
	entries, _ := os.ReadDir(m.cacheDir)
	if len(entries) != 0 {
		t.Fatal("cancelled partial retained")
	}
	m.client.Transport = original
	m.Download()
	if status = waitIdle(t, m); !status.CanOpen {
		t.Fatalf("retry: %+v", status)
	}
}

func TestRedirectsCannotLeaveGitHubOrDowngradeTLS(t *testing.T) {
	for _, target := range []string{"http://github.com/file", "https://github.com.evil.example/file", "https://127.0.0.1/file", "https://user@github.com/file", "https://github.com:444/file"} {
		m := managerFixture(t, fixture([]byte("x"), "windows-amd64"), nil)
		var calls int
		m.client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
			calls++
			r := response(request, nil, 302)
			r.Header.Set("Location", target)
			return r, nil
		})
		if _, err := m.get(context.Background(), "https://github.com/Wen5555/LinkSend/releases/download/v0.6.2/package.exe", false); err == nil || calls != 1 {
			t.Fatalf("unsafe redirect followed: %s", target)
		}
	}
	for _, target := range []string{"https://release-assets.githubusercontent.com/a?signature=fixture", "https://objects.githubusercontent.com/a"} {
		request, _ := http.NewRequest("GET", target, nil)
		if err := checkRedirect(request, []*http.Request{{URL: mustURL(t, "https://github.com/file")}}); err != nil {
			t.Fatal(err)
		}
	}
	request, _ := http.NewRequest("GET", "https://github.com/other", nil)
	if err := checkRedirect(request, []*http.Request{{URL: mustURL(t, latestURL)}}); err == nil {
		t.Fatal("metadata redirect allowed")
	}
}

func TestAlreadyCurrentAndGitHubFailureStayDistinct(t *testing.T) {
	m := managerFixture(t, fixture([]byte("x"), "windows-amd64"), nil)
	m.status.CurrentVersion = "0.6.2"
	m.Check()
	if s := waitIdle(t, m); s.Stage != "up_to_date" || s.CanDownload {
		t.Fatalf("current: %+v", s)
	}
	m.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) { return response(r, nil, 403), nil })
	m.Check()
	if s := waitIdle(t, m); s.Stage != "error" || !strings.Contains(s.Error, "限制") || s.CanDownload {
		t.Fatalf("failure: %+v", s)
	}
}

// Opt-in real HTTPS test. It checks and downloads the current official package
// into a temporary test cache, verifies it, and NEVER starts an installer.
func TestLiveGitHubCheckAndDownload(t *testing.T) {
	if os.Getenv("LINKSEND_TEST_GITHUB_UPDATES") != "1" {
		t.Skip("explicit live GitHub test only")
	}
	m := New(context.Background(), "0.0.0", runtime.GOOS, runtime.GOARCH, t.TempDir())
	defer m.Close()
	m.Check()
	wait := func(timeout time.Duration) Status {
		deadline := time.Now().Add(timeout)
		for time.Now().Before(deadline) {
			s := m.Status()
			if !s.Busy {
				return s
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatal("live GitHub operation timed out")
		return Status{}
	}
	s := wait(35 * time.Second)
	if !s.CanDownload {
		t.Fatalf("live check: %+v", s)
	}
	m.Download()
	s = wait(3 * time.Minute)
	if !s.CanOpen {
		t.Fatalf("live download: %+v", s)
	}
	if err := verifyFile(context.Background(), m.file, *m.asset); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(m.file), m.asset.Name))
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(data)
	t.Logf("LIVE_CHECK_DOWNLOAD_VERIFIED version=%s platform=%s bytes=%d sha256=%x installer_launched=false", s.LatestVersion, s.Platform, len(data), hash)
}
