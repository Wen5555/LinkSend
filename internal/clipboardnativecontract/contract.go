// Package clipboardnativecontract is the private, receipt-only wire contract
// between the profile owner and its separately pinned Windows helper. It is
// not a clipboard publisher, signing service, or deletion capability.
package clipboardnativecontract

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"

	"github.com/Wen5555/LinkSend/internal/transfer"
)

const MaxFrame = 256 << 10
const MaxSources = 65
const MaxSourceBytes = 64 << 20

var ErrContract = errors.New("CLIPBOARD_NATIVE_CONTRACT_REJECTED")

// These layouts deliberately match the frozen app receipt-only v1 encoding.
type Binding struct {
	Owner                   string `json:"owner"`
	OwnerRun                string `json:"owner_run"`
	BootID                  string `json:"boot_id"`
	CacheID                 string `json:"cache_id"`
	ProfileRootID           string `json:"profile_root_id"`
	CacheRootID             string `json:"cache_root_id"`
	ObjectID                string `json:"object_id"`
	StageID                 string `json:"stage_id"`
	ManifestDigest          string `json:"manifest_digest"`
	PlanDigest              string `json:"plan_digest"`
	InventoryDigest         string `json:"inventory_digest"`
	PublicationID           string `json:"publication_id"`
	PublicationGeneration   uint64 `json:"publication_generation"`
	Nonce                   string `json:"nonce"`
	NativeContract          string `json:"native_contract"`
	HelperImageSHA256       string `json:"helper_image_sha256"`
	ChildEphemeralPublicKey []byte `json:"child_ephemeral_public_key"`
}

type Delegation struct {
	Version   int     `json:"version"`
	Purpose   string  `json:"purpose"`
	Binding   Binding `json:"binding"`
	Signature []byte  `json:"signature"`
}

type Receipt struct {
	Version              int     `json:"version"`
	Purpose              string  `json:"purpose"`
	DelegationDigest     string  `json:"delegation_digest"`
	Binding              Binding `json:"binding"`
	Sequence             uint64  `json:"sequence"`
	AdmissionClosed      bool    `json:"admission_closed"`
	SourcesClosed        bool    `json:"sources_closed"`
	NativeCloseWitnessID string  `json:"native_close_witness_id"`
	Signature            []byte  `json:"signature"`
}

type Node struct {
	Identity  string `json:"identity"`
	Directory bool   `json:"directory,omitempty"`
	Size      int64  `json:"size,omitempty"`
	Modified  int64  `json:"modified,omitempty"`
}

type Inventory struct {
	Version  int               `json:"version"`
	CacheID  string            `json:"cache_id"`
	Manifest string            `json:"manifest"`
	Plan     string            `json:"plan"`
	Stage    string            `json:"stage"`
	Parts    map[string]string `json:"parts"`
	Nodes    map[string]Node   `json:"nodes"`
}

type Source struct {
	Name   string `json:"name"`
	Node   Node   `json:"node"`
	Handle uint64 `json:"handle"`
	SHA256 string `json:"sha256,omitempty"`
}

type Prepare struct {
	Version             int               `json:"version"`
	Challenge           string            `json:"challenge"`
	OwnerPublic         []byte            `json:"owner_public"`
	LauncherHandle      uint64            `json:"launcher_handle"`
	SpoolHandle         uint64            `json:"spool_handle"`
	ObjectID            string            `json:"object_id"`
	Inventory           []byte            `json:"inventory"`
	Manifest            transfer.Manifest `json:"manifest"`
	Sources             []Source          `json:"sources"`
	DeadlineUnixNano    int64             `json:"deadline_unix_nano"`
	WaitForLauncherExit bool              `json:"wait_for_launcher_exit,omitempty"`
}

type Hello struct {
	Version         int    `json:"version"`
	Challenge       string `json:"challenge"`
	BootID          string `json:"boot_id"`
	ChildPublic     []byte `json:"child_public"`
	InventoryDigest string `json:"inventory_digest"`
	LaunchDigest    string `json:"launch_digest"`
	Signature       []byte `json:"signature"`
}

type Result struct {
	Version         int    `json:"version"`
	Receipt         []byte `json:"receipt"`
	ProtectedChecks uint32 `json:"protected_checks"`
	SourcesClosed   uint32 `json:"sources_closed"`
}

// Held is emitted only after the native STA/MTA pair actually retains the
// provider, streams, clones and async operation. It cannot request a signature
// or override native counters. The original fixed deadline is never renewed.
type Held struct {
	Version          int    `json:"version"`
	DelegationDigest string `json:"delegation_digest"`
	LaunchDigest     string `json:"launch_digest"`
	DeadlineUnixNano int64  `json:"deadline_unix_nano"`
	Signature        []byte `json:"signature"`
}

func SHA(value []byte) string { h := sha256.Sum256(value); return hex.EncodeToString(h[:]) }
func Message(domain string, value any) ([]byte, error) {
	body, err := json.Marshal(value)
	if err != nil || len(body) > 4096 {
		return nil, ErrContract
	}
	return append([]byte(domain+"\x00"), body...), nil
}
func DelegationMessage(d Delegation) ([]byte, error) {
	d.Signature = nil
	return Message("LinkSend clipboard cache native delegation v1", d)
}
func ReceiptMessage(r Receipt) ([]byte, error) {
	r.Signature = nil
	return Message("LinkSend clipboard cache native quiescent v1", r)
}
func HelloMessage(h Hello) ([]byte, error) {
	h.Signature = nil
	return Message("LinkSend clipboard cache native launch challenge v2", h)
}
func HeldMessage(h Held) ([]byte, error) {
	h.Signature = nil
	return Message("LinkSend clipboard cache native consumer held v1", h)
}
func InventoryDigest(raw []byte) string {
	return SHA(append([]byte("LinkSend clipboard cache inventory v1\x00"), raw...))
}

func Decode(raw []byte, out any) error {
	if len(raw) == 0 || len(raw) > MaxFrame {
		return ErrContract
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(out) != nil || d.Decode(new(any)) != io.EOF {
		return ErrContract
	}
	canonical, err := json.Marshal(out)
	if err != nil || !bytes.Equal(raw, canonical) {
		return ErrContract
	}
	return nil
}
func Write(w io.Writer, value any) error {
	raw, err := json.Marshal(value)
	if err != nil || len(raw) > MaxFrame {
		return ErrContract
	}
	var size [4]byte
	binary.LittleEndian.PutUint32(size[:], uint32(len(raw)))
	if err = writeAll(w, size[:]); err != nil {
		return err
	}
	return writeAll(w, raw)
}
func writeAll(w io.Writer, b []byte) error {
	for len(b) > 0 {
		n, e := w.Write(b)
		if e != nil {
			return e
		}
		if n <= 0 {
			return io.ErrShortWrite
		}
		b = b[n:]
	}
	return nil
}
func Read(r io.Reader, out any) error {
	var size [4]byte
	if _, err := io.ReadFull(r, size[:]); err != nil {
		return err
	}
	n := binary.LittleEndian.Uint32(size[:])
	if n == 0 || n > MaxFrame {
		return ErrContract
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(r, b); err != nil {
		return err
	}
	return Decode(b, out)
}
func VerifyDelegation(raw []byte, p Prepare, h Hello) (Delegation, error) {
	var d Delegation
	var inv Inventory
	if Decode(raw, &d) != nil || Decode(p.Inventory, &inv) != nil || d.Version != 1 || d.Purpose != "cache-native-delegation" {
		return d, ErrContract
	}
	b := d.Binding
	launchRaw, err := json.Marshal(p)
	if err != nil || p.Version != 2 || h.Version != 2 || h.LaunchDigest != SHA(launchRaw) || p.DeadlineUnixNano <= 0 {
		return d, ErrContract
	}
	if len(p.OwnerPublic) != ed25519.PublicKeySize || b.Owner != SHA(p.OwnerPublic) || b.NativeContract != "receipt-only-v1" || b.PublicationGeneration != 1 ||
		b.BootID != h.BootID || !bytes.Equal(b.ChildEphemeralPublicKey, h.ChildPublic) || b.InventoryDigest != InventoryDigest(p.Inventory) || b.ObjectID != p.ObjectID ||
		b.CacheID != inv.CacheID || b.ManifestDigest != p.Manifest.Digest() || inv.Manifest != b.ManifestDigest || b.PlanDigest != inv.Plan || b.StageID != inv.Stage {
		return d, ErrContract
	}
	for _, v := range []struct {
		s string
		n int
	}{{b.Owner, 32}, {b.OwnerRun, 16}, {b.CacheID, 16}, {b.PublicationID, 16}, {b.Nonce, 32}, {b.HelperImageSHA256, 32}, {b.PlanDigest, 32}} {
		decoded, e := hex.DecodeString(v.s)
		if e != nil || len(decoded) != v.n || hex.EncodeToString(decoded) != v.s {
			return d, ErrContract
		}
	}
	message, err := DelegationMessage(d)
	if err != nil || !ed25519.Verify(p.OwnerPublic, message, d.Signature) {
		return d, ErrContract
	}
	return d, nil
}
