// contentprobe is an explicit, isolated physical-host test entry point. It uses
// production pinned identity TLS, quic-go streams and the content_v1 transfer
// protocol. It does not discover peers, signal through HTTP/WSS, alter host
// networking, read a clipboard, launch a URL or exercise the desktop UI.
package main

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"io"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Wen5555/LinkSend/internal/content"
	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/transfer"
	"github.com/Wen5555/LinkSend/internal/transport"
	quic "github.com/quic-go/quic-go"
)

const profileMarker = "LinkSend isolated content probe v1\n"

type options struct {
	mode, profile, listen, remote, peer, out, label string
	timeout                                         time.Duration
	allowLoopback                                   bool
}
type report struct {
	Role            string       `json:"role"`
	Kind            content.Kind `json:"kind"`
	TransferID      string       `json:"transfer_id"`
	LocalAddress    string       `json:"local_address"`
	RemoteAddress   string       `json:"remote_address"`
	LocalDeviceID   string       `json:"local_device_id"`
	PinnedPeerID    string       `json:"pinned_peer_id"`
	TLSVersion      uint16       `json:"tls_version"`
	ALPN            string       `json:"alpn"`
	Mode            string       `json:"mode"`
	BodyBytes       int64        `json:"body_bytes"`
	BodyBLAKE3      string       `json:"body_blake3"`
	BodySHA256      string       `json:"body_sha256"`
	ManifestDigest  string       `json:"manifest_digest"`
	ContentDigest   string       `json:"content_digest"`
	SelectionDigest string       `json:"selection_digest"`
	RelativeFile    string       `json:"relative_file"`
	State           string       `json:"state"`
	Relay           bool         `json:"relay"`
	FinishedAt      string       `json:"finished_at"`
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "contentprobe:", err)
		os.Exit(1)
	}
}

func run(args []string, output io.Writer) error {
	flags := flag.NewFlagSet("contentprobe", flag.ContinueOnError)
	var o options
	flags.StringVar(&o.mode, "mode", "", "init, receive or send")
	flags.StringVar(&o.profile, "profile", "", "new isolated profile (init) or an existing probe profile")
	flags.StringVar(&o.listen, "listen", "", "explicit local IP:UDP port; port 0 is allowed")
	flags.StringVar(&o.remote, "remote", "", "explicit destination IP:UDP port for send")
	flags.StringVar(&o.peer, "peer-key", "", "expected peer Ed25519 public key (64 lowercase hex characters)")
	flags.StringVar(&o.out, "out", "", "new evidence/output directory; existing directories are refused")
	flags.StringVar(&o.label, "label", "physical-content-probe", "bounded fixture label, never a URL to open")
	flags.DurationVar(&o.timeout, "timeout", 2*time.Minute, "overall bound, between 5 seconds and 5 minutes")
	flags.BoolVar(&o.allowLoopback, "allow-loopback", false, "explicit local test only; never physical-host evidence")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || !filepath.IsAbs(o.profile) {
		return errors.New("an absolute isolated profile and no positional arguments are required")
	}
	if o.mode == "init" {
		return initialise(o.profile, output)
	}
	if o.mode != "send" && o.mode != "receive" {
		return errors.New("mode must be init, send or receive")
	}
	if o.timeout < 5*time.Second || o.timeout > 5*time.Minute {
		return errors.New("invalid bounded timeout")
	}
	if !filepath.IsAbs(o.out) {
		return errors.New("an absolute new output directory is required")
	}
	if len(o.label) == 0 || len(o.label) > 48 || strings.ContainsAny(o.label, "\x00\r\n\t/\\") {
		return errors.New("invalid fixture label")
	}
	marker, err := os.ReadFile(filepath.Join(o.profile, ".content-probe"))
	if err != nil || string(marker) != profileMarker {
		return errors.New("profile was not created by contentprobe init")
	}
	peer, err := decodePeer(o.peer)
	if err != nil {
		return err
	}
	listen, err := parseAddress(o.listen, true, o.allowLoopback)
	if err != nil {
		return err
	}
	var remote *net.UDPAddr
	if o.mode == "send" {
		remote, err = parseAddress(o.remote, false, o.allowLoopback)
		if err != nil {
			return err
		}
	}
	if err = os.Mkdir(o.out, 0700); err != nil {
		return fmt.Errorf("refused output directory: %w", err)
	}
	id, err := identity.LoadOrCreate(o.profile)
	if err != nil {
		return err
	}
	udp, err := net.ListenUDP("udp", listen)
	if err != nil {
		return err
	}
	defer udp.Close()
	qt := &quic.Transport{Conn: udp}
	defer qt.Close()
	ctx, cancel := context.WithTimeout(context.Background(), o.timeout)
	defer cancel()
	if o.mode == "receive" {
		return receive(ctx, qt, id, peer, o, output)
	}
	return send(ctx, qt, id, peer, remote, o, output)
}

func initialise(profile string, output io.Writer) error {
	if err := os.Mkdir(profile, 0700); err != nil {
		return fmt.Errorf("refused profile (init requires a new directory): %w", err)
	}
	id, err := identity.LoadOrCreate(profile)
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(profile, ".content-probe"), []byte(profileMarker), 0600); err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(struct {
		DeviceID  string `json:"device_id"`
		PublicKey string `json:"public_key"`
	}{id.ID(), hex.EncodeToString(id.PublicKey())})
}

func decodePeer(value string) (ed25519.PublicKey, error) {
	key, err := hex.DecodeString(value)
	if err != nil || len(key) != ed25519.PublicKeySize || strings.ToLower(value) != value {
		return nil, errors.New("a valid lowercase peer Ed25519 public key is required")
	}
	return ed25519.PublicKey(key), nil
}

func parseAddress(value string, zero, loopback bool) (*net.UDPAddr, error) {
	address, err := netip.ParseAddrPort(value)
	if err != nil || address.Addr().IsUnspecified() || address.Addr().IsMulticast() || address.Addr().Zone() != "" || (!zero && address.Port() == 0) || (!loopback && address.Addr().IsLoopback()) {
		return nil, errors.New("an explicit unicast IP:port is required; loopback needs --allow-loopback")
	}
	return net.UDPAddrFromAddrPort(address), nil
}

func receive(ctx context.Context, qt *quic.Transport, id *identity.Identity, peer ed25519.PublicKey, o options, output io.Writer) error {
	tlsConfig, err := id.TLSConfig(peer, true)
	if err != nil {
		return err
	}
	listener, err := qt.Listen(tlsConfig, transport.QUICConfig())
	if err != nil {
		return err
	}
	defer listener.Close()
	if err = json.NewEncoder(output).Encode(struct {
		Ready  bool   `json:"ready"`
		Local  string `json:"local_address"`
		Pinned string `json:"pinned_peer_id"`
	}{true, listener.Addr().String(), identity.DeviceID(peer)}); err != nil {
		return err
	}
	seen := make(map[content.Kind]bool)
	for range 3 {
		conn, err := listener.Accept(ctx)
		if err != nil {
			return err
		}
		result, err := receiveOne(ctx, conn, id, peer, o)
		_ = conn.CloseWithError(0, "")
		if err != nil {
			return err
		}
		if seen[result.Kind] {
			return errors.New("duplicate content kind")
		}
		seen[result.Kind] = true
		if err = saveReport(o.out, result, output); err != nil {
			return err
		}
	}
	return nil
}

func receiveOne(ctx context.Context, conn *quic.Conn, id *identity.Identity, peer ed25519.PublicKey, o options) (report, error) {
	var result report
	if conn.ConnectionState().TLS.Version != tls.VersionTLS13 {
		return result, errors.New("TLS 1.3 required")
	}
	stream, err := conn.AcceptStream(ctx)
	if err != nil {
		return result, err
	}
	var descriptor *transfer.ContentDescriptor
	var manifest transfer.Manifest
	var directory string
	transferred, err := transfer.ReceiveWithOptions(ctx, transport.WrapStream(stream), transfer.ReceiveOptions{Directory: o.out, Peer: identity.DeviceID(peer), AcceptNativeContent: true, Plan: func(ctx context.Context, offer transfer.Offer) (transfer.ReceivePlan, error) {
		if offer.Content == nil || offer.FileFallback {
			return transfer.ReceivePlan{}, transfer.ErrContentUnsupported
		}
		manifest = offer.Manifest
		copy := *offer.Content
		descriptor = &copy
		directory = filepath.Join(o.out, "received-"+string(copy.Kind))
		if err := os.Mkdir(directory, 0700); err != nil {
			return transfer.ReceivePlan{}, err
		}
		plan, err := transfer.BuildReceivePlan(ctx, directory, manifest, transfer.PlanRequest{})
		return plan, err
	}})
	if err != nil {
		return result, err
	}
	if descriptor == nil {
		return result, transfer.ErrContentMismatch
	}
	file := filepath.Join(directory, manifest.Files[descriptor.EntryID].Path)
	return verifyResult(conn, id, peer, "receive", file, o.out, transferred)
}

func send(ctx context.Context, qt *quic.Transport, id *identity.Identity, peer ed25519.PublicKey, remote *net.UDPAddr, o options, output io.Writer) error {
	store, err := content.OpenStore(o.profile)
	if err != nil {
		return err
	}
	defer store.Close()
	tlsConfig, err := id.TLSConfig(peer, false)
	if err != nil {
		return err
	}
	for _, kind := range []content.Kind{content.Text, content.URL, content.Image} {
		snapshot, err := createFixture(ctx, store, kind, o.label)
		if err != nil {
			return err
		}
		file, err := store.OwnedPath(ctx, snapshot.ID)
		if err != nil {
			return err
		}
		prepared, err := transfer.Prepare(ctx, []string{file}, 64<<10)
		if err != nil {
			return err
		}
		descriptor, err := transfer.NewContentDescriptor(prepared.Manifest, snapshot)
		if err != nil {
			_ = prepared.Close()
			return err
		}
		conn, err := qt.Dial(ctx, remote, tlsConfig, transport.QUICConfig())
		if err != nil {
			_ = prepared.Close()
			return err
		}
		stream, err := conn.OpenStreamSync(ctx)
		var transferred transfer.Result
		if err == nil {
			transferred, err = transfer.SendWithOptions(ctx, transport.WrapStream(stream), prepared, transfer.SendOptions{Content: &descriptor})
		}
		_ = prepared.Close()
		if err != nil {
			_ = conn.CloseWithError(1, "probe transfer failed")
			return err
		}
		result, err := verifyResult(conn, id, peer, "send", file, o.profile, transferred)
		_ = conn.CloseWithError(0, "")
		if err != nil {
			return err
		}
		if err = saveReport(o.out, result, output); err != nil {
			return err
		}
	}
	return nil
}

func createFixture(ctx context.Context, store *content.Store, kind content.Kind, label string) (content.Snapshot, error) {
	owner := "probe:" + string(kind) + ":" + label
	switch kind {
	case content.Text:
		return store.CreateText(ctx, kind, "LinkSend 实体双机内容验收\n"+label+"\n<script>仅作为文字传输</script>\n", owner)
	case content.URL:
		return store.CreateText(ctx, kind, "https://example.invalid/linksend-content-probe?direction="+label, owner)
	case content.Image:
		img := image.NewNRGBA(image.Rect(0, 0, 96, 64))
		seed := sha256.Sum256([]byte(label))
		for y := 0; y < 64; y++ {
			for x := 0; x < 96; x++ {
				img.SetNRGBA(x, y, color.NRGBA{R: uint8(x*2) ^ seed[0], G: uint8(y*3) ^ seed[1], B: uint8(x+y) ^ seed[2], A: 255})
			}
		}
		return store.CreateImageFromImage(ctx, img, owner)
	default:
		return content.Snapshot{}, content.ErrInvalidText
	}
}

func verifyResult(conn *quic.Conn, id *identity.Identity, peer ed25519.PublicKey, role, file, base string, result transfer.Result) (report, error) {
	var empty report
	if result.State != "Completed" || result.Content == nil || result.FileFallback || result.ContentDigest == "" || result.SelectionDigest == "" || conn.ConnectionState().TLS.Version != tls.VersionTLS13 {
		return empty, errors.New("missing bilateral native content result")
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return empty, err
	}
	if int64(len(data)) != result.Bytes || transfer.Sum(data) != result.Content.Digest {
		return empty, transfer.ErrIntegrity
	}
	sum := sha256.Sum256(data)
	relative, err := filepath.Rel(base, file)
	if err != nil {
		return empty, err
	}
	return report{Role: role, Kind: result.Content.Kind, TransferID: result.TransferID, LocalAddress: conn.LocalAddr().String(), RemoteAddress: conn.RemoteAddr().String(), LocalDeviceID: id.ID(), PinnedPeerID: identity.DeviceID(peer), TLSVersion: conn.ConnectionState().TLS.Version, ALPN: conn.ConnectionState().TLS.NegotiatedProtocol, Mode: "content_v1", BodyBytes: result.Bytes, BodyBLAKE3: result.Content.Digest, BodySHA256: hex.EncodeToString(sum[:]), ManifestDigest: result.Digest, ContentDigest: result.ContentDigest, SelectionDigest: result.SelectionDigest, RelativeFile: filepath.ToSlash(relative), State: result.State, Relay: false, FinishedAt: time.Now().UTC().Format(time.RFC3339Nano)}, nil
}

func saveReport(directory string, r report, output io.Writer) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(directory, string(r.Kind)+".json"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(append(data, '\n'))
	syncErr := f.Sync()
	closeErr := f.Close()
	if err = errors.Join(writeErr, syncErr, closeErr); err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(r)
}
