package app

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"io"

	"github.com/Wen5555/LinkSend/internal/transfer"
)

// The queue's content digest and canonical policy are committed with the exact
// saved draft revision. A metadata preview does not freeze source file bytes.
func checkSourceDraftBinding(row *sql.Row, request EnqueueRequest, paths, selection string) error {
	var savedPaths, savedPeer, savedSelection, savedPeers string
	var revision uint64
	if err := row.Scan(&savedPaths, &savedPeer, &revision, &savedSelection, &savedPeers); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrMetadataConflict
		}
		return err
	}
	var peers []string
	if len(savedPeers) > 2048 || json.Unmarshal([]byte(savedPeers), &peers) != nil || peers == nil || len(peers) > 1 || len(peers) == 1 && peers[0] != savedPeer {
		return ErrMetadataConflict
	}
	if revision != request.DraftRevision || savedPeer != request.PeerID || savedPaths != paths || savedSelection != selection {
		return ErrMetadataConflict
	}
	return nil
}

func encodeSourceSelection(selection *transfer.SourceSelection) (string, error) {
	normalized, err := transfer.NormalizeSourceSelection(selection)
	if err != nil || normalized == nil {
		return "", err
	}
	data, err := json.Marshal(normalized)
	return string(data), err
}

func decodeSourceSelection(encoded string) (*transfer.SourceSelection, error) {
	if encoded == "" {
		return nil, nil
	}
	if len(encoded) > 8192 {
		return nil, ErrMetadataInvalid
	}
	decoder := json.NewDecoder(bytes.NewBufferString(encoded))
	decoder.DisallowUnknownFields()
	var selection transfer.SourceSelection
	if err := decoder.Decode(&selection); err != nil {
		return nil, ErrMetadataInvalid
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return nil, ErrMetadataInvalid
	}
	return transfer.NormalizeSourceSelection(&selection)
}

func cloneSourceSelection(selection *transfer.SourceSelection) *transfer.SourceSelection {
	if selection == nil {
		return nil
	}
	copy := *selection
	copy.Exclude = append([]string(nil), selection.Exclude...)
	return &copy
}

func selectedQueueSourceDigest(manifest transfer.Manifest, selection *transfer.SourceSelection) string {
	base := queueSourceDigest(manifest)
	if selection == nil {
		return base
	}
	encoded, err := encodeSourceSelection(selection)
	if err != nil {
		return ""
	}
	return transfer.Sum([]byte(base + "\nsource-selection-v1\n" + encoded))
}
