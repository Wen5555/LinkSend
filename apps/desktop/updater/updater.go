// Package updater downloads official desktop releases. Installation remains an
// explicit handoff to the native installer; it never replaces a running app.
package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	latestURL             = "https://api.github.com/repos/Wen5555/LinkSend/releases/latest"
	releasesURL           = "https://github.com/Wen5555/LinkSend/releases/latest"
	maxPackageBytes int64 = 1 << 30
)

type Status struct {
	CurrentVersion  string `json:"current_version"`
	LatestVersion   string `json:"latest_version"`
	Platform        string `json:"platform"`
	Stage           string `json:"stage"`
	Message         string `json:"message"`
	Error           string `json:"error"`
	ReleaseURL      string `json:"release_url"`
	Notes           string `json:"notes"`
	PackageName     string `json:"package_name"`
	PackageBytes    int64  `json:"package_bytes"`
	DownloadedBytes int64  `json:"downloaded_bytes"`
	LastChecked     string `json:"last_checked"`
	UpdateAvailable bool   `json:"update_available"`
	Busy            bool   `json:"busy"`
	CanDownload     bool   `json:"can_download"`
	CanOpen         bool   `json:"can_open"`
}

type releaseAsset struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	Digest string `json:"digest"`
	URL    string `json:"browser_download_url"`
	State  string `json:"state"`
}

type release struct {
	Tag        string         `json:"tag_name"`
	URL        string         `json:"html_url"`
	Body       string         `json:"body"`
	Draft      bool           `json:"draft"`
	Prerelease bool           `json:"prerelease"`
	Assets     []releaseAsset `json:"assets"`
}

type Manager struct {
	mu       sync.Mutex
	ctx      context.Context
	stop     context.CancelFunc
	closed   bool
	client   *http.Client
	cacheDir string
	status   Status
	asset    *releaseAsset
	file     string
	cancel   context.CancelFunc
	done     chan struct{}
}

func New(ctx context.Context, version, goos, arch, cacheDir string) *Manager {
	ctx, stop := context.WithCancel(ctx)
	platform := goos + "-" + arch
	if goos == "darwin" {
		platform = "macos-" + arch
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 20 * time.Second
	return &Manager{ctx: ctx, stop: stop, cacheDir: cacheDir,
		client: &http.Client{Transport: transport, CheckRedirect: checkRedirect},
		status: Status{CurrentVersion: version, Platform: platform, Stage: "idle", Message: "点击检查 GitHub 上的最新正式版", ReleaseURL: releasesURL}}
}

func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.snapshot()
}

func (m *Manager) snapshot() Status {
	s := m.status
	s.Busy = m.cancel != nil
	s.CanDownload = !s.Busy && !m.closed && m.asset != nil && m.file == ""
	s.CanOpen = !s.Busy && !m.closed && m.file != ""
	return s
}

// All operations are single flight. Cancellation keeps the slot until the
// worker finishes, so its late result cannot overwrite a newer operation.
func (m *Manager) begin(stage string, timeout time.Duration) (context.Context, bool) {
	if m.cancel != nil || m.closed {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(m.ctx, timeout)
	m.cancel, m.done = cancel, make(chan struct{})
	m.status.Stage, m.status.Error = stage, ""
	return ctx, true
}

func (m *Manager) finish(ctx context.Context, err error, apply func()) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	if err != nil {
		m.status.Stage = "error"
		m.status.Message = "更新未完成，可以重试或查看 GitHub 发布页"
		m.status.Error = err.Error()
		if errors.Is(err, context.Canceled) {
			m.status.Stage, m.status.Error, m.status.Message = "cancelled", "", "操作已取消，可以重试"
		} else if errors.Is(err, context.DeadlineExceeded) {
			m.status.Error = "连接 GitHub 超时，请检查网络后重试"
		}
	} else {
		apply()
	}
	m.cancel()
	m.cancel = nil
	close(m.done)
	return err == nil
}

func (m *Manager) Check() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	ctx, ok := m.begin("checking", 30*time.Second)
	if !ok {
		return m.snapshot()
	}
	m.asset, m.file = nil, ""
	m.status.LatestVersion, m.status.Notes, m.status.PackageName = "", "", ""
	m.status.PackageBytes, m.status.DownloadedBytes, m.status.UpdateAvailable = 0, 0, false
	m.status.ReleaseURL, m.status.Message = releasesURL, "正在检查 GitHub 正式版…"
	go func() {
		r, err := m.fetchRelease(ctx)
		var asset *releaseAsset
		var newer bool
		if err == nil {
			newer, err = newerVersion(r.Tag, m.status.CurrentVersion)
		}
		if err == nil && newer {
			asset, err = selectAsset(r, m.status.Platform)
		}
		m.finish(ctx, err, func() {
			m.status.LatestVersion, m.status.ReleaseURL = strings.TrimPrefix(r.Tag, "v"), r.URL
			m.status.Notes, m.status.LastChecked = limitNotes(r.Body), time.Now().UTC().Format(time.RFC3339)
			m.status.UpdateAvailable, m.asset = newer, asset
			if !newer {
				m.status.Stage, m.status.Message = "up_to_date", "当前已是最新正式版，或正在使用更新的版本"
			} else if asset == nil {
				m.status.Stage, m.status.Message = "available", "发现新版本；此版本暂无适合本机的安装包，请查看发布页"
			} else {
				m.status.Stage, m.status.Message = "available", "发现新版本，可下载更新"
				m.status.PackageName, m.status.PackageBytes = asset.Name, asset.Size
			}
		})
	}()
	return m.snapshot()
}

func (m *Manager) Download() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.asset == nil || m.file != "" {
		return m.snapshot()
	}
	ctx, ok := m.begin("downloading", 20*time.Minute)
	if !ok {
		return m.snapshot()
	}
	asset := *m.asset
	m.status.DownloadedBytes, m.status.Message = 0, "正在从 GitHub 下载更新…"
	go func() {
		file, err := m.download(ctx, asset)
		// Check cancellation while publishing, not before taking the state lock.
		accepted := m.finish(ctx, err, func() {
			m.file = file
			m.status.DownloadedBytes = asset.Size
			m.status.Stage, m.status.Message = "ready", "下载完成，SHA256 校验通过，可以打开安装包"
		})
		if !accepted && file != "" {
			removeDownload(file)
		}
	}()
	return m.snapshot()
}

func (m *Manager) Cancel() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Once a package is being handed to the OS it cannot be revoked by Cancel.
	if m.cancel != nil && m.status.Stage != "opening" {
		m.cancel()
		m.status.Stage, m.status.Message = "cancelling", "正在取消…"
	}
	return m.snapshot()
}

func (m *Manager) Close() {
	m.mu.Lock()
	m.closed = true
	m.stop()
	if m.cancel != nil {
		m.cancel()
	}
	done := m.done
	m.mu.Unlock()
	if done != nil {
		<-done
	}
}

// No caller supplies a path or URL. Recheck the private downloaded file before
// every native launch, including repeated attempts after a launch error.
func (m *Manager) OpenPackage(open func(string) error) Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.file == "" || m.asset == nil {
		return m.snapshot()
	}
	ctx, ok := m.begin("opening", 30*time.Second)
	if !ok {
		return m.snapshot()
	}
	file, asset := m.file, *m.asset
	m.status.Message = "正在复核并打开安装包…"
	go func() {
		err := verifyFile(ctx, file, asset)
		if err != nil {
			m.mu.Lock()
			m.file = ""
			m.mu.Unlock()
		} else if err = ctx.Err(); err == nil {
			if open == nil || open(file) != nil {
				err = errors.New("无法打开安装包，请稍后重试")
			}
		}
		m.finish(ctx, err, func() {
			m.status.Stage, m.status.Message = "opened", "安装包已打开。请退出 LinkSend（包括托盘后台），再完成安装并重新启动"
		})
	}()
	return m.snapshot()
}

func (m *Manager) fetchRelease(ctx context.Context) (release, error) {
	var r release
	response, err := m.get(ctx, latestURL, true)
	if err != nil {
		return r, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, (2<<20)+1))
	if err != nil {
		return r, errors.New("无法读取 GitHub 发布信息，请重试")
	}
	if len(body) > 2<<20 || json.Unmarshal(body, &r) != nil || r.Draft || r.Prerelease || !stableVersion.MatchString(r.Tag) || r.URL != "https://github.com/Wen5555/LinkSend/releases/tag/"+r.Tag {
		return r, errors.New("GitHub 正式版信息无效，暂不能更新")
	}
	return r, nil
}

func (m *Manager) get(ctx context.Context, address string, api bool) (*http.Response, error) {
	if !allowedURL(address) {
		return nil, errors.New("更新下载地址无效")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, errors.New("更新请求无效")
	}
	request.Header.Set("User-Agent", "LinkSend/"+m.status.CurrentVersion+" desktop-updater")
	request.Header.Set("Accept", "application/octet-stream")
	if api {
		request.Header.Set("Accept", "application/vnd.github+json")
	}
	response, err := m.client.Do(request)
	if err != nil {
		return nil, errors.New("无法连接 GitHub，请检查网络后重试")
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		switch response.StatusCode {
		case 403, 429:
			return nil, errors.New("GitHub 暂时限制了请求，请稍后重试")
		case 404:
			return nil, errors.New("GitHub 上暂未找到正式版本或安装包")
		default:
			return nil, fmt.Errorf("GitHub 暂不可用（HTTP %d），请稍后重试", response.StatusCode)
		}
	}
	return response, nil
}

func allowedURL(address string) bool {
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || u.Fragment != "" {
		return false
	}
	switch u.Host {
	case "github.com", "api.github.com", "release-assets.githubusercontent.com", "objects.githubusercontent.com", "github-releases.githubusercontent.com":
		return true
	}
	return false
}

func checkRedirect(request *http.Request, via []*http.Request) error {
	if len(via) >= 5 || !allowedURL(request.URL.String()) {
		return errors.New("更新重定向地址无效")
	}
	if len(via) > 0 && via[0].URL.Host == "api.github.com" {
		return errors.New("更新元数据不允许重定向")
	}
	return nil
}

func selectAsset(r release, platform string) (*releaseAsset, error) {
	var suffix string
	switch platform {
	case "windows-amd64":
		suffix = "-windows-amd64-installer-[0-9a-f]{40}\\.exe"
	case "macos-arm64", "macos-amd64":
		suffix = "-" + platform + "-[0-9a-f]{40}\\.dmg"
	default:
		return nil, nil
	}
	pattern := regexp.MustCompile("^LinkSend-" + regexp.QuoteMeta(r.Tag) + suffix + "$")
	var selected *releaseAsset
	for _, a := range r.Assets {
		if !pattern.MatchString(a.Name) {
			continue
		}
		if selected != nil {
			return nil, errors.New("GitHub 安装包不唯一，请查看发布页")
		}
		if a.State != "uploaded" || a.Size <= 0 || a.Size > maxPackageBytes || !validDigest(a.Digest) || a.URL != "https://github.com/Wen5555/LinkSend/releases/download/"+r.Tag+"/"+a.Name {
			return nil, errors.New("安装包缺少有效的 SHA256、大小或官方来源信息，暂不能下载")
		}
		selected = &a
	}
	return selected, nil
}

var stableVersion = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
var currentVersion = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$`)

// Only stable releases are eligible. Numeric comparison also handles an older
// stable release being marked Latest, and never offers a version downgrade.
func newerVersion(latest, current string) (bool, error) {
	l, c := stableVersion.FindStringSubmatch(latest), currentVersion.FindStringSubmatch(current)
	if l == nil || c == nil {
		return false, errors.New("应用版本格式无法比较，请查看 GitHub 发布页")
	}
	for i := 1; i <= 3; i++ {
		left, errL := strconv.ParseUint(l[i], 10, 64)
		right, errR := strconv.ParseUint(c[i], 10, 64)
		if errL != nil || errR != nil {
			return false, errors.New("应用版本超出支持范围")
		}
		if left != right {
			return left > right, nil
		}
	}
	return c[4] != "", nil
}

func validDigest(digest string) bool {
	if !strings.HasPrefix(digest, "sha256:") || len(digest) != 71 {
		return false
	}
	_, err := hex.DecodeString(digest[7:])
	return err == nil
}

func limitNotes(notes string) string {
	if len(notes) <= 32<<10 {
		return notes
	}
	notes = notes[:32<<10]
	for !utf8.ValidString(notes) {
		notes = notes[:len(notes)-1]
	}
	return notes + "\n（完整更新说明见 GitHub 发布页）"
}

func (m *Manager) download(ctx context.Context, asset releaseAsset) (file string, err error) {
	if m.cacheDir == "" {
		return "", errors.New("无法定位应用缓存目录")
	}
	if err = os.MkdirAll(m.cacheDir, 0700); err != nil {
		return "", errors.New("无法创建更新缓存，请检查磁盘权限和空间")
	}
	info, err := os.Lstat(m.cacheDir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("更新缓存目录无效")
	}
	dir, err := os.MkdirTemp(m.cacheDir, "download-")
	if err != nil {
		return "", errors.New("无法创建更新下载文件")
	}
	part := filepath.Join(dir, "package.part")
	defer func() {
		_ = os.Remove(part)
		if err != nil {
			_ = os.Remove(dir)
		}
	}()
	response, err := m.get(ctx, asset.URL, false)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.ContentLength >= 0 && response.ContentLength != asset.Size {
		return "", errors.New("安装包大小与 GitHub 发布信息不一致")
	}
	out, err := os.OpenFile(part, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", errors.New("无法写入更新缓存")
	}
	defer out.Close()
	hash := sha256.New()
	progress := &progressWriter{ctx: ctx, manager: m}
	written, err := io.Copy(io.MultiWriter(out, hash, progress), io.LimitReader(response.Body, asset.Size+1))
	if err != nil {
		return "", errors.New("下载中断或磁盘写入失败，可以重试")
	}
	if written != asset.Size || !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), asset.Digest[7:]) {
		return "", errors.New("安装包 SHA256 或大小校验失败，请重新下载")
	}
	if err = ctx.Err(); err != nil {
		return "", err
	}
	if err = out.Sync(); err != nil {
		return "", errors.New("无法保存更新安装包")
	}
	if err = out.Close(); err != nil {
		return "", errors.New("无法关闭更新安装包")
	}
	file = filepath.Join(dir, asset.Name)
	if err = os.Rename(part, file); err != nil {
		return "", errors.New("无法保存校验后的安装包")
	}
	return file, nil
}

type progressWriter struct {
	ctx     context.Context
	manager *Manager
	total   int64
	last    time.Time
}

func (p *progressWriter) Write(body []byte) (int, error) {
	if err := p.ctx.Err(); err != nil {
		return 0, err
	}
	p.total += int64(len(body))
	if time.Since(p.last) >= 200*time.Millisecond {
		p.manager.mu.Lock()
		p.manager.status.DownloadedBytes = p.total
		p.manager.mu.Unlock()
		p.last = time.Now()
	}
	return len(body), nil
}

func verifyFile(ctx context.Context, file string, asset releaseAsset) error {
	info, err := os.Lstat(file)
	if err != nil || !info.Mode().IsRegular() || info.Size() != asset.Size {
		return errors.New("安装包缺失或发生变化，请重新下载")
	}
	f, err := os.Open(file)
	if err != nil {
		return errors.New("无法读取安装包，请重新下载")
	}
	defer f.Close()
	hash := sha256.New()
	buffer := make([]byte, 64<<10)
	var size int64
	for {
		if err = ctx.Err(); err != nil {
			return err
		}
		n, readErr := f.Read(buffer)
		size += int64(n)
		_, _ = hash.Write(buffer[:n])
		if readErr == io.EOF {
			break
		}
		if readErr != nil || size > asset.Size {
			return errors.New("无法校验安装包，请重新下载")
		}
	}
	if size != asset.Size || !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), asset.Digest[7:]) {
		return errors.New("安装包校验失败，请重新下载")
	}
	return nil
}

func removeDownload(file string) { _ = os.Remove(file); _ = os.Remove(filepath.Dir(file)) }
