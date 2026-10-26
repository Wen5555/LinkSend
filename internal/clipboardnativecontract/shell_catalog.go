package clipboardnativecontract

// This is an unwired v3 handoff codec. It neither starts a helper nor grants
// publication/deletion. The controlled v2 Prepare/Read path stays unchanged.

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"io"
	"path"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/Wen5555/LinkSend/internal/transfer"
)

const (
	ShellVersion           = 3
	ShellContract          = "windows-virtual-files-v1"
	ShellPageBytes         = 64 << 10
	MaxShellCatalogBytes   = 64 << 20 // Metadata only; never file contents.
	MaxShellSources        = 2*transfer.MaxEntries + 3
	MaxShellNameUTF16Units = 4 << 20
)

// ShellPrepare contains no inline inventory/manifest and no post-export TTL.
// Its absolute deadline limits admission only. A future owner must still use
// NewShellLifetime before committing and independently prove the final OS gate.
type ShellPrepare struct {
	Version            int              `json:"version"`
	NativeContract     string           `json:"native_contract"`
	Challenge          string           `json:"challenge"`
	OwnerPublic        []byte           `json:"owner_public"`
	SpoolHandle        uint64           `json:"spool_handle"`
	ObjectID           string           `json:"object_id"`
	AdmissionDeadline  int64            `json:"admission_deadline_unix_nano"`
	ExpectedGeneration uint32           `json:"expected_generation"`
	Catalog            ShellCatalogInfo `json:"catalog"`
}

type ShellCatalogInfo struct {
	Bytes           uint64 `json:"bytes"`
	Pages           uint32 `json:"pages"`
	Entries         uint32 `json:"entries"`
	Sources         uint32 `json:"sources"`
	Digest          string `json:"digest"`
	ManifestDigest  string `json:"manifest_digest"`
	InventoryDigest string `json:"inventory_digest"`
}

// The root is first; remaining names are ordered, unique and relative. Sources
// have the same fixed order as Nodes; pages never add an index or authority.
type ShellCatalogNode struct {
	Name string `json:"name"`
	Node Node   `json:"node"`
}

type ShellInventory struct {
	Version  int                `json:"version"`
	CacheID  string             `json:"cache_id"`
	Manifest string             `json:"manifest"`
	Plan     string             `json:"plan"`
	Stage    string             `json:"stage"`
	Nodes    []ShellCatalogNode `json:"nodes"`
}

type ShellSource struct {
	Handle uint64 `json:"handle"`
	SHA256 string `json:"sha256,omitempty"`
}

type ShellCatalog struct {
	Version   int               `json:"version"`
	Manifest  transfer.Manifest `json:"manifest"`
	Inventory ShellInventory    `json:"inventory"`
	Sources   []ShellSource     `json:"sources"`
}

type ShellCatalogPage struct {
	Version int    `json:"version"`
	Index   uint32 `json:"index"`
	Digest  string `json:"digest"`
	Data    []byte `json:"data"`
}

func shellHex(value string, size int) bool {
	b, err := hex.DecodeString(value)
	return err == nil && len(b) == size && hex.EncodeToString(b) == value
}

func shellIdentity(value string) bool {
	parts := strings.Split(value, ":")
	if len(parts) != 6 || parts[0] != "windows" {
		return false
	}
	for _, part := range parts[1:] {
		n, err := strconv.ParseUint(part, 16, 32)
		if err != nil || strconv.FormatUint(n, 16) != part {
			return false
		}
	}
	return true
}

func validateShellPrepare(p ShellPrepare) error {
	i := p.Catalog
	if p.Version != ShellVersion || p.NativeContract != ShellContract || !shellHex(p.Challenge, 32) || len(p.OwnerPublic) != ed25519.PublicKeySize ||
		p.SpoolHandle == 0 || !shellIdentity(p.ObjectID) || p.AdmissionDeadline <= 0 || p.ExpectedGeneration == 0 ||
		i.Bytes == 0 || i.Bytes > MaxShellCatalogBytes || i.Pages != uint32((i.Bytes+ShellPageBytes-1)/ShellPageBytes) ||
		i.Entries == 0 || i.Entries > transfer.MaxEntries || i.Sources < i.Entries+2 || i.Sources > MaxShellSources ||
		!shellHex(i.Digest, 32) || !shellHex(i.ManifestDigest, 32) || !shellHex(i.InventoryDigest, 32) {
		return ErrContract
	}
	return nil
}

// ValidateShellCatalog checks metadata consistency only. It does not inspect
// handles, verify contents or attest actual source lifetime/ownership.
func ValidateShellCatalog(p ShellPrepare, c ShellCatalog) error {
	if validateShellPrepare(p) != nil || c.Version != ShellVersion || c.Manifest.Validate() != nil ||
		c.Inventory.Version != ShellVersion || c.Inventory.CacheID != c.Manifest.TransferID || c.Inventory.Manifest != c.Manifest.Digest() ||
		!shellHex(c.Inventory.Plan, 32) || !shellIdentity(c.Inventory.Stage) ||
		len(c.Manifest.Files) != int(p.Catalog.Entries) || len(c.Sources) != int(p.Catalog.Sources) || len(c.Sources) != len(c.Inventory.Nodes) {
		return ErrContract
	}
	files := make(map[string]transfer.FileEntry, len(c.Manifest.Files))
	for _, entry := range c.Manifest.Files {
		files[entry.Path] = entry
	}
	stage := ".linksend-" + c.Inventory.CacheID
	nodes := make(map[string]Node, len(c.Inventory.Nodes))
	fileHashes := make(map[string]string, len(c.Inventory.Nodes))
	usedHandles := map[uint64]bool{p.SpoolHandle: true}
	nameUnits := 0
	for index, item := range c.Inventory.Nodes {
		n, source := item.Node, c.Sources[index]
		if (index > 1 && c.Inventory.Nodes[index-1].Name >= item.Name) || !shellIdentity(n.Identity) || n.Size < 0 ||
			(n.Directory && (n.Size != 0 || n.Modified != 0 || source.SHA256 != "")) ||
			(!n.Directory && !shellHex(source.SHA256, 32)) || source.Handle == 0 || usedHandles[source.Handle] {
			return ErrContract
		}
		usedHandles[source.Handle] = true
		nameUnits += len(utf16.Encode([]rune(item.Name)))
		if nameUnits > MaxShellNameUTF16Units {
			return ErrContract
		}
		switch {
		case item.Name == ".":
			if index != 0 || !n.Directory || n.Identity != p.ObjectID {
				return ErrContract
			}
		case item.Name == stage:
			if !n.Directory || n.Identity != c.Inventory.Stage {
				return ErrContract
			}
		case strings.HasPrefix(item.Name, stage+"/"):
			base := strings.TrimPrefix(item.Name, stage+"/")
			if n.Directory || !shellPartName(base, c.Manifest.Files) {
				return ErrContract
			}
		default:
			entry, ok := files[item.Name]
			if !ok || transfer.ValidatePath(item.Name) != nil || (entry.Type == "directory") != n.Directory || n.Size != entry.Size {
				return ErrContract
			}
		}
		if item.Name != "." {
			parent, ok := nodes[path.Dir(item.Name)]
			if !ok || !parent.Directory {
				return ErrContract
			}
		}
		nodes[item.Name] = n
		fileHashes[item.Name] = source.SHA256
	}
	if !nodes["."].Directory || !nodes[stage].Directory || nodes[stage+"/state.json"].Directory || nodes[stage+"/state.json"].Identity == "" {
		return ErrContract
	}
	// The committed file and the engine part must be the same original inode.
	// All advertised manifest entries and all file parts must be present.
	for _, entry := range c.Manifest.Files {
		node, ok := nodes[entry.Path]
		if !ok {
			return ErrContract
		}
		if entry.Type == "file" {
			part, exists := nodes[stage+"/"+strconv.FormatUint(uint64(entry.ID), 10)+".part"]
			if !exists || part != node || fileHashes[entry.Path] != fileHashes[stage+"/"+strconv.FormatUint(uint64(entry.ID), 10)+".part"] {
				return ErrContract
			}
		}
	}
	raw, err := json.Marshal(c)
	inv, invErr := json.Marshal(c.Inventory)
	if err != nil || invErr != nil || uint64(len(raw)) != p.Catalog.Bytes || SHA(raw) != p.Catalog.Digest ||
		c.Manifest.Digest() != p.Catalog.ManifestDigest || InventoryDigest(inv) != p.Catalog.InventoryDigest {
		return ErrContract
	}
	return nil
}

func shellPartName(name string, files []transfer.FileEntry) bool {
	if name == "state.json" {
		return true
	}
	if !strings.HasSuffix(name, ".part") {
		return false
	}
	value := strings.TrimSuffix(name, ".part")
	id, err := strconv.ParseUint(value, 10, 32)
	return err == nil && strconv.FormatUint(id, 10) == value && id < uint64(len(files)) && files[id].Type == "file"
}

func prepareShellCatalog(p ShellPrepare, c ShellCatalog) (ShellPrepare, []byte, error) {
	if len(c.Manifest.Files) == 0 || len(c.Manifest.Files) > transfer.MaxEntries || len(c.Inventory.Nodes) > MaxShellSources || len(c.Sources) != len(c.Inventory.Nodes) {
		return ShellPrepare{}, nil, ErrContract
	}
	raw, err := json.Marshal(c)
	inv, invErr := json.Marshal(c.Inventory)
	if err != nil || invErr != nil || len(raw) == 0 || len(raw) > MaxShellCatalogBytes {
		return ShellPrepare{}, nil, ErrContract
	}
	p.Catalog = ShellCatalogInfo{Bytes: uint64(len(raw)), Pages: uint32((len(raw) + ShellPageBytes - 1) / ShellPageBytes),
		Entries: uint32(len(c.Manifest.Files)), Sources: uint32(len(c.Sources)), Digest: SHA(raw), ManifestDigest: c.Manifest.Digest(), InventoryDigest: InventoryDigest(inv)}
	if ValidateShellCatalog(p, c) != nil {
		return ShellPrepare{}, nil, ErrContract
	}
	return p, raw, nil
}

// WriteShellCatalog uses bounded existing frame I/O but a distinct v3 header
// and fixed-size ordered pages. No page contains file contents or authority.
func WriteShellCatalog(w io.Writer, p ShellPrepare, c ShellCatalog) (ShellPrepare, error) {
	p, raw, err := prepareShellCatalog(p, c)
	if err != nil {
		return ShellPrepare{}, err
	}
	if err = Write(w, p); err != nil {
		return ShellPrepare{}, err
	}
	for index, start := uint32(0), 0; start < len(raw); index, start = index+1, start+ShellPageBytes {
		end := min(start+ShellPageBytes, len(raw))
		if err = Write(w, ShellCatalogPage{Version: ShellVersion, Index: index, Digest: p.Catalog.Digest, Data: raw[start:end]}); err != nil {
			return ShellPrepare{}, err
		}
	}
	return p, nil
}

// ReadShellCatalog never returns a partially admitted catalog. Count/byte caps
// are checked before allocation; strict canonical JSON rejects duplicate,
// reordered/unknown keys, while fixed page sizes/order reject splicing/skips.
func ReadShellCatalog(r io.Reader) (ShellPrepare, ShellCatalog, error) {
	var p ShellPrepare
	if err := Read(r, &p); err != nil {
		return ShellPrepare{}, ShellCatalog{}, err
	}
	if validateShellPrepare(p) != nil {
		return ShellPrepare{}, ShellCatalog{}, ErrContract
	}
	raw := make([]byte, 0, int(p.Catalog.Bytes))
	for index := uint32(0); index < p.Catalog.Pages; index++ {
		var page ShellCatalogPage
		if err := Read(r, &page); err != nil {
			return ShellPrepare{}, ShellCatalog{}, err
		}
		want := min(uint64(ShellPageBytes), p.Catalog.Bytes-uint64(len(raw)))
		if page.Version != ShellVersion || page.Index != index || page.Digest != p.Catalog.Digest || uint64(len(page.Data)) != want {
			return ShellPrepare{}, ShellCatalog{}, ErrContract
		}
		raw = append(raw, page.Data...)
	}
	var c ShellCatalog
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if SHA(raw) != p.Catalog.Digest || decoder.Decode(&c) != nil || decoder.Decode(new(any)) != io.EOF {
		return ShellPrepare{}, ShellCatalog{}, ErrContract
	}
	canonical, err := json.Marshal(c)
	if err != nil || !bytes.Equal(raw, canonical) || ValidateShellCatalog(p, c) != nil {
		return ShellPrepare{}, ShellCatalog{}, ErrContract
	}
	return p, c, nil
}

// ShellHelloMessage retains the existing purpose domain. Its signed v3 field
// and distinct launch digest prevent it being accepted by the v2 verifier.
func ShellHelloMessage(h Hello) ([]byte, error) {
	if h.Version != ShellVersion {
		return nil, ErrContract
	}
	return HelloMessage(h)
}

func VerifyShellDelegation(raw []byte, p ShellPrepare, c ShellCatalog, h Hello) (Delegation, error) {
	var d Delegation
	if Decode(raw, &d) != nil || ValidateShellCatalog(p, c) != nil || d.Version != ShellVersion || d.Purpose != "cache-native-delegation" {
		return Delegation{}, ErrContract
	}
	launch, err := json.Marshal(p)
	message, helloErr := ShellHelloMessage(h)
	if err != nil || helloErr != nil || h.LaunchDigest != SHA(launch) || h.Challenge != p.Challenge || h.InventoryDigest != p.Catalog.InventoryDigest ||
		!strings.HasPrefix(h.BootID, "windows-boot-v1-") || !shellHex(strings.TrimPrefix(h.BootID, "windows-boot-v1-"), 16) ||
		len(h.ChildPublic) != ed25519.PublicKeySize || !ed25519.Verify(h.ChildPublic, message, h.Signature) {
		return Delegation{}, ErrContract
	}
	b := d.Binding
	if b.Owner != SHA(p.OwnerPublic) || b.NativeContract != ShellContract || b.PublicationGeneration != 1 || b.BootID != h.BootID ||
		!bytes.Equal(b.ChildEphemeralPublicKey, h.ChildPublic) || b.InventoryDigest != p.Catalog.InventoryDigest || b.ObjectID != p.ObjectID ||
		b.CacheID != c.Inventory.CacheID || b.ManifestDigest != p.Catalog.ManifestDigest || b.PlanDigest != c.Inventory.Plan || b.StageID != c.Inventory.Stage ||
		!shellIdentity(b.ProfileRootID) || !shellIdentity(b.CacheRootID) {
		return Delegation{}, ErrContract
	}
	for _, field := range []struct {
		value string
		size  int
	}{{b.OwnerRun, 16}, {b.PublicationID, 16}, {b.Nonce, 32}, {b.HelperImageSHA256, 32}} {
		if !shellHex(field.value, field.size) {
			return Delegation{}, ErrContract
		}
	}
	message, err = DelegationMessage(d)
	if err != nil || !ed25519.Verify(p.OwnerPublic, message, d.Signature) {
		return Delegation{}, ErrContract
	}
	return d, nil
}
