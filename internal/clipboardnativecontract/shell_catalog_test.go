package clipboardnativecontract

import (
	"bytes"
	"crypto/ed25519"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/Wen5555/LinkSend/internal/transfer"
)

func shellCatalogFixture() (ShellPrepare, ShellCatalog, ed25519.PrivateKey) {
	owner := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{1}, ed25519.SeedSize))
	cache := strings.Repeat("1", 32)
	stage := ".linksend-" + cache
	identity := func(id int) string { return fmt.Sprintf("windows:1:%x:0:2:3", id) }
	p := ShellPrepare{Version: ShellVersion, NativeContract: ShellContract, Challenge: strings.Repeat("2", 64), OwnerPublic: owner.Public().(ed25519.PublicKey),
		SpoolHandle: 1, ObjectID: identity(1), AdmissionDeadline: 1, ExpectedGeneration: 1}
	c := ShellCatalog{Version: ShellVersion, Manifest: transfer.Manifest{Version: 1, TransferID: cache, ChunkSize: 4 << 20},
		Inventory: ShellInventory{Version: ShellVersion, CacheID: cache, Plan: strings.Repeat("3", 64), Stage: identity(2)}}
	c.Inventory.Nodes = []ShellCatalogNode{{Name: ".", Node: Node{Identity: p.ObjectID, Directory: true}},
		{Name: stage, Node: Node{Identity: c.Inventory.Stage, Directory: true}},
		{Name: stage + "/state.json", Node: Node{Identity: identity(3), Size: 1, Modified: 1}}}
	for index := 0; index < transfer.MaxEntries; index++ {
		e := transfer.FileEntry{ID: uint32(index), Path: fmt.Sprintf("file%05d.bin", index), Type: "file", Modified: 5, Hash: strings.Repeat("4", 64)}
		if index == 0 {
			e.Path = "!note.bin" // Sorts before '.', which remains the fixed root.
			e.Modified = 0
			e.Size = 65 << 20 // Metadata must not inherit controlled 64 MiB body cap.
			for part := int64(0); part < (e.Size+int64(c.Manifest.ChunkSize)-1)/int64(c.Manifest.ChunkSize); part++ {
				e.Chunks = append(e.Chunks, e.Hash)
			}
		}
		c.Manifest.Files = append(c.Manifest.Files, e)
		node := Node{Identity: identity(index + 4), Size: e.Size, Modified: e.Modified}
		c.Inventory.Nodes = append(c.Inventory.Nodes, ShellCatalogNode{Name: e.Path, Node: node}, ShellCatalogNode{Name: fmt.Sprintf("%s/%d.part", stage, index), Node: node})
	}
	sort.Slice(c.Inventory.Nodes[1:], func(a, b int) bool { return c.Inventory.Nodes[a+1].Name < c.Inventory.Nodes[b+1].Name })
	for index, item := range c.Inventory.Nodes {
		source := ShellSource{Handle: uint64(index + 2)}
		if !item.Node.Directory {
			source.SHA256 = strings.Repeat("5", 64)
		}
		c.Sources = append(c.Sources, source)
	}
	c.Inventory.Manifest = c.Manifest.Digest()
	return p, c, owner
}

// One full-size metadata fixture and necessary rejection cases; no native
// process, source I/O, system clipboard, format registration or GUI is used.
func TestShellCatalogV3RoundtripAndRejection(t *testing.T) {
	p, catalog, owner := shellCatalogFixture()
	var wire bytes.Buffer
	p, err := WriteShellCatalog(&wire, p, catalog)
	if err != nil {
		t.Fatal(err)
	}
	if p.Catalog.Bytes <= MaxFrame || p.Catalog.Sources != MaxShellSources || p.Catalog.Entries != transfer.MaxEntries || catalog.Manifest.TotalBytes() <= MaxSourceBytes {
		t.Fatal("fixture did not exceed controlled frame/source/body limits")
	}
	gotPrepare, gotCatalog, err := ReadShellCatalog(bytes.NewReader(wire.Bytes()))
	if err != nil || !reflect.DeepEqual(gotPrepare, p) || !reflect.DeepEqual(gotCatalog, catalog) {
		t.Fatal("full paginated catalog roundtrip failed", err)
	}

	child := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{2}, ed25519.SeedSize))
	launch, _ := json.Marshal(p)
	hello := Hello{Version: ShellVersion, Challenge: p.Challenge, BootID: "windows-boot-v1-" + strings.Repeat("6", 32), ChildPublic: child.Public().(ed25519.PublicKey),
		InventoryDigest: p.Catalog.InventoryDigest, LaunchDigest: SHA(launch)}
	message, _ := ShellHelloMessage(hello)
	hello.Signature = ed25519.Sign(child, message)
	d := Delegation{Version: ShellVersion, Purpose: "cache-native-delegation", Binding: Binding{Owner: SHA(p.OwnerPublic), OwnerRun: strings.Repeat("7", 32), BootID: hello.BootID,
		CacheID: catalog.Inventory.CacheID, ProfileRootID: "windows:1:a:0:2:3", CacheRootID: "windows:1:b:0:2:3", ObjectID: p.ObjectID, StageID: catalog.Inventory.Stage,
		ManifestDigest: p.Catalog.ManifestDigest, PlanDigest: catalog.Inventory.Plan, InventoryDigest: p.Catalog.InventoryDigest, PublicationID: strings.Repeat("8", 32),
		PublicationGeneration: 1, Nonce: strings.Repeat("9", 64), NativeContract: ShellContract, HelperImageSHA256: strings.Repeat("a", 64), ChildEphemeralPublicKey: hello.ChildPublic}}
	message, _ = DelegationMessage(d)
	d.Signature = ed25519.Sign(owner, message)
	delegation, _ := json.Marshal(d)
	if _, err = VerifyShellDelegation(delegation, p, catalog, hello); err != nil {
		t.Fatal("fixed-purpose signed v3 handoff rejected", err)
	}

	reader := bytes.NewReader(wire.Bytes())
	var header ShellPrepare
	if err = Read(reader, &header); err != nil {
		t.Fatal(err)
	}
	var first ShellCatalogPage
	if err = Read(reader, &first); err != nil {
		t.Fatal(err)
	}
	tail := make([]byte, reader.Len())
	_, _ = reader.Read(tail)
	t.Run("wrong_page", func(t *testing.T) {
		var wrong bytes.Buffer
		_ = Write(&wrong, header)
		page := first
		page.Index++
		_ = Write(&wrong, page)
		wrong.Write(tail)
		assertShellRejected(t, wrong.Bytes())
	})
	t.Run("truncated", func(t *testing.T) {
		assertShellRejected(t, wire.Bytes()[:wire.Len()-1])
	})
	t.Run("total_digest", func(t *testing.T) {
		var wrong bytes.Buffer
		_ = Write(&wrong, header)
		page := first
		page.Data = bytes.Clone(first.Data)
		page.Data[len(page.Data)/2] ^= 1
		_ = Write(&wrong, page)
		wrong.Write(tail)
		assertShellRejected(t, wrong.Bytes())
	})
	t.Run("over_budget_before_allocation", func(t *testing.T) {
		var wrong bytes.Buffer
		header := header
		header.Catalog.Bytes = MaxShellCatalogBytes + 1
		_ = Write(&wrong, header)
		assertShellRejected(t, wrong.Bytes())
	})
	t.Run("duplicate_key", func(t *testing.T) {
		raw, _ := json.Marshal(header)
		raw = append([]byte(`{"version":3,`), raw[1:]...)
		var wrong bytes.Buffer
		_ = binary.Write(&wrong, binary.LittleEndian, uint32(len(raw)))
		wrong.Write(raw)
		assertShellRejected(t, wrong.Bytes())
	})
	t.Run("v2_is_independent", func(t *testing.T) {
		var old Prepare
		if Read(bytes.NewReader(wire.Bytes()), &old) == nil {
			t.Fatal("controlled v2 decoder accepted v3 header")
		}
		var oldWire bytes.Buffer
		legacy := Prepare{Version: 2, Challenge: p.Challenge, OwnerPublic: p.OwnerPublic, DeadlineUnixNano: 1}
		if Write(&oldWire, legacy) != nil || Read(bytes.NewReader(oldWire.Bytes()), &old) != nil || !reflect.DeepEqual(old, legacy) {
			t.Fatal("old codec roundtrip changed")
		}
		assertShellRejected(t, oldWire.Bytes())
		if _, err := VerifyDelegation(delegation, legacy, Hello{Version: 2}); err == nil {
			t.Fatal("v2 accepted signed v3 delegation")
		}
		legacyDelegation := d
		legacyDelegation.Version = 1
		legacyDelegation.Binding.NativeContract = "receipt-only-v1"
		message, _ := DelegationMessage(legacyDelegation)
		legacyDelegation.Signature = ed25519.Sign(owner, message)
		raw, _ := json.Marshal(legacyDelegation)
		if _, err := VerifyShellDelegation(raw, p, catalog, hello); err == nil {
			t.Fatal("v3 accepted signed v1 receipt-only delegation")
		}
	})
}

func assertShellRejected(t *testing.T, raw []byte) {
	t.Helper()
	p, catalog, err := ReadShellCatalog(bytes.NewReader(raw))
	if err == nil || p.Version != 0 || catalog.Version != 0 {
		t.Fatal("invalid input returned a partially admitted catalog", err)
	}
}
