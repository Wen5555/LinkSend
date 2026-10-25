package signaling

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/sharegrant"
)

// ShareOperation addresses only the experimental, signed authorization API.
// It neither enrolls either actor nor enables file transfer or clipboard access.
type ShareOperation string

const (
	ShareCreate    ShareOperation = "create"
	ShareClaim     ShareOperation = "claim"
	ShareRead      ShareOperation = "status"
	ShareApprove   ShareOperation = "approve"
	ShareDeny      ShareOperation = "deny"
	ShareBind      ShareOperation = "bind"
	ShareAuthorize ShareOperation = "authorize"
	ShareComplete  ShareOperation = "complete"
	ShareRevoke    ShareOperation = "revoke"
)

// ShareRequest contains bounded metadata only. Code is sent only to claim;
// CodeDigest only to create. Neither is copied into errors or response DTOs.
// Other populated binding fields are also checked on the returned status.
type ShareRequest struct {
	ID          string `json:"id,omitempty"`
	Code        string `json:"code,omitempty"`
	CodeDigest  string `json:"code_digest,omitempty"`
	Manifest    string `json:"manifest_digest,omitempty"`
	Plan        string `json:"plan_digest,omitempty"`
	ClaimID     string `json:"claim_id,omitempty"`
	ApprovalID  string `json:"approval_id,omitempty"`
	TaskID      string `json:"task_id,omitempty"`
	Generation  uint64 `json:"generation,omitempty"`
	ReceiverKey string `json:"receiver_public_key,omitempty"`
}

// ShareStatus is an untrusted network response until ShareControl validates it.
// A status does not authorize a data operation; that requires a fresh authorize.
type ShareStatus struct {
	ID          string           `json:"id"`
	State       sharegrant.State `json:"state"`
	Generation  uint64           `json:"generation"`
	Revision    uint64           `json:"revision"`
	SenderKey   string           `json:"sender_public_key"`
	ReceiverKey string           `json:"receiver_public_key,omitempty"`
	Manifest    string           `json:"manifest_digest"`
	Plan        string           `json:"plan_digest,omitempty"`
	ClaimID     string           `json:"claim_id,omitempty"`
	TaskID      string           `json:"task_id,omitempty"`
	ExpiresAt   int64            `json:"expires_at"`
	ClaimUntil  int64            `json:"claim_until,omitempty"`
	ResumeUntil int64            `json:"resume_until,omitempty"`
}

type ShareControlInfo struct {
	Version           int    `json:"version"`
	Epoch             string `json:"epoch"`
	FileTransferReady bool   `json:"file_transfer_ready"`
}

const shareMaxCounter = uint64(1<<63 - 1)

func shareHex(value string, size int) bool {
	if len(value) != size || value != strings.ToLower(value) {
		return false
	}
	b, err := hex.DecodeString(value)
	return err == nil && len(b)*2 == size
}

func shareDigest(value string) bool {
	return shareHex(value, 64) && value != strings.Repeat("0", 64)
}

func shareKey(value string) bool {
	if len(value) != 43 {
		return false
	}
	b, err := base64.RawStdEncoding.DecodeString(value)
	return err == nil && len(b) == 32 && base64.RawStdEncoding.EncodeToString(b) == value && !bytes.Equal(b, make([]byte, 32))
}

func shareInvalid() error {
	return protocol.Fail(protocol.ShareInvalid, "invalid temporary share request")
}

func shareBadResponse() error {
	return protocol.Fail(protocol.InvalidMessage, "invalid temporary share response")
}

func validateShareRequest(op ShareOperation, r ShareRequest) error {
	if (r.ID != "" && !shareHex(r.ID, 32)) || (r.ClaimID != "" && !shareHex(r.ClaimID, 32)) || (r.ApprovalID != "" && !shareHex(r.ApprovalID, 32)) || (r.TaskID != "" && !shareHex(r.TaskID, 32)) || (r.Manifest != "" && !shareDigest(r.Manifest)) || (r.Plan != "" && !shareDigest(r.Plan)) || (r.CodeDigest != "" && !shareDigest(r.CodeDigest)) || (r.ReceiverKey != "" && !shareKey(r.ReceiverKey)) || r.Generation > shareMaxCounter {
		return shareInvalid()
	}
	if (op != ShareClaim && r.Code != "") || (op != ShareCreate && r.CodeDigest != "") || (op != ShareApprove && r.ApprovalID != "") {
		return shareInvalid()
	}
	if op != ShareClaim && r.ID == "" {
		return shareInvalid()
	}
	switch op {
	case ShareCreate:
		if r.Manifest == "" || r.CodeDigest == "" {
			return shareInvalid()
		}
	case ShareClaim:
		if _, err := sharegrant.CodeDigest(r.Code); err != nil || r.ClaimID == "" {
			return shareInvalid()
		}
	case ShareRead, ShareRevoke:
	case ShareApprove:
		if r.ClaimID == "" || r.ApprovalID == "" || r.TaskID == "" || r.Generation == 0 || r.Manifest == "" || r.ReceiverKey == "" {
			return shareInvalid()
		}
	case ShareDeny:
		if r.ClaimID == "" || r.Generation == 0 || r.Generation >= shareMaxCounter {
			return shareInvalid()
		}
	case ShareBind, ShareAuthorize, ShareComplete:
		if r.TaskID == "" || r.Generation == 0 || r.Manifest == "" || r.Plan == "" {
			return shareInvalid()
		}
	default:
		return shareInvalid()
	}
	return nil
}

func uniqueShareObject(raw []byte) bool {
	// Reject ambiguous object fields rather than accepting encoding/json's
	// last-key-wins interpretation, including the outer health response.
	keys := json.NewDecoder(bytes.NewReader(raw))
	if token, err := keys.Token(); err != nil || token != json.Delim('{') {
		return false
	}
	var seen []string
	for keys.More() {
		token, err := keys.Token()
		key, ok := token.(string)
		if err != nil || !ok {
			return false
		}
		for _, previous := range seen {
			// encoding/json also matches tagged struct fields by Unicode
			// SimpleFold (for example, long s and ASCII s), not just ASCII case.
			if strings.EqualFold(previous, key) {
				return false
			}
		}
		seen = append(seen, key)
		if keys.Decode(new(json.RawMessage)) != nil {
			return false
		}
	}
	if _, err := keys.Token(); err != nil {
		return false
	}
	return true
}

func decodeShareJSON(raw []byte, out any) error {
	if !uniqueShareObject(raw) {
		return shareBadResponse()
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return shareBadResponse()
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return shareBadResponse()
	}
	return nil
}

func shareTransportFailure(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		if err == context.Canceled {
			return protocol.Wrap(protocol.Cancelled, "temporary share request cancelled", err)
		}
		return protocol.Wrap(protocol.SignalingTimeout, "temporary share request interrupted", err)
	}
	return protocol.Fail(protocol.SignalingUnreachable, "temporary share service unreachable")
}

// ShareControlInfo fetches the current public boot challenge over the same
// verified HTTPS client. It does not turn experimental control into a data-plane
// capability. Epochs are not cached across operations or server restarts.
func (c *Client) ShareControlInfo(ctx context.Context) (ShareControlInfo, error) {
	endpoint, err := c.endpoint("/healthz")
	if err != nil {
		return ShareControlInfo{}, shareInvalid()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return ShareControlInfo{}, shareInvalid()
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return ShareControlInfo{}, shareTransportFailure(ctx)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ShareControlInfo{}, decodeShareFailure(resp)
	}
	raw, err := readLimitedJSON(resp.Body, protocol.MaxPayloadBytes)
	if err != nil {
		if ctx.Err() != nil {
			return ShareControlInfo{}, shareTransportFailure(ctx)
		}
		return ShareControlInfo{}, shareBadResponse()
	}
	var health struct {
		Status       string          `json:"status"`
		Capabilities json.RawMessage `json:"capabilities"`
		Control      json.RawMessage `json:"share_control"`
	}
	if !uniqueShareObject(raw) {
		return ShareControlInfo{}, shareBadResponse()
	}
	if json.Unmarshal(raw, &health) != nil || health.Status != "ok" || len(health.Capabilities) == 0 {
		return ShareControlInfo{}, protocol.Fail(protocol.VersionIncompatible, "temporary share service incompatible")
	}
	if !uniqueShareObject(health.Capabilities) {
		return ShareControlInfo{}, shareBadResponse()
	}
	var capabilities protocol.Capabilities
	if json.Unmarshal(health.Capabilities, &capabilities) != nil || !supported(capabilities) {
		return ShareControlInfo{}, protocol.Fail(protocol.VersionIncompatible, "temporary share service incompatible")
	}
	if len(health.Control) == 0 || bytes.Equal(health.Control, []byte("null")) {
		return ShareControlInfo{}, protocol.Fail(protocol.ShareUnsupported, "temporary share control unavailable")
	}
	var control struct {
		Version int    `json:"version"`
		Epoch   string `json:"epoch"`
		Ready   *bool  `json:"file_transfer_ready"`
	}
	if decodeShareJSON(health.Control, &control) != nil || !shareHex(control.Epoch, 32) || control.Ready == nil {
		return ShareControlInfo{}, shareBadResponse()
	}
	if control.Version != 1 {
		return ShareControlInfo{}, protocol.Fail(protocol.ShareUnsupported, "temporary share control version unsupported")
	}
	return ShareControlInfo{Version: control.Version, Epoch: control.Epoch, FileTransferReady: *control.Ready}, nil
}

// ShareControl sends exactly one signed operation, without automatic retries.
// Response loss must be resolved by a fresh status request or an explicit retry
// of the original stable IDs; no caller consent is inferred from a response.
func (c *Client) ShareControl(ctx context.Context, op ShareOperation, request ShareRequest) (ShareStatus, error) {
	if err := validateShareRequest(op, request); err != nil {
		return ShareStatus{}, err
	}
	info, err := c.ShareControlInfo(ctx)
	if err != nil {
		return ShareStatus{}, err
	}
	body, err := json.Marshal(request)
	if err != nil || len(body) > protocol.MaxPayloadBytes {
		return ShareStatus{}, shareInvalid()
	}
	path := "/v1/shares/" + string(op)
	endpoint, err := c.endpoint(path)
	if err != nil {
		return ShareStatus{}, shareInvalid()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return ShareStatus{}, shareInvalid()
	}
	nonce, at := protocol.RandomID(), time.Now().Unix()
	id := c.identity.ID()
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-LinkSend-Membership", "2")
	req.Header.Set("X-LinkSend-Share-Grants", "1")
	req.Header.Set("X-LinkSend-Device", id)
	req.Header.Set("X-LinkSend-Public-Key", base64.RawStdEncoding.EncodeToString(c.identity.PublicKey()))
	req.Header.Set("X-LinkSend-Share-Epoch", info.Epoch)
	req.Header.Set("X-LinkSend-Nonce", nonce)
	req.Header.Set("X-LinkSend-Time", strconv.FormatInt(at, 10))
	req.Header.Set("X-LinkSend-Signature", base64.RawStdEncoding.EncodeToString(c.identity.Sign(protocol.AuthBytes(http.MethodPost, path, id, info.Epoch+":"+nonce, at, body))))
	resp, err := c.http.Do(req)
	if err != nil {
		return ShareStatus{}, shareTransportFailure(ctx)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ShareStatus{}, decodeShareFailure(resp)
	}
	raw, err := readLimitedJSON(resp.Body, protocol.MaxPayloadBytes)
	if err != nil {
		if ctx.Err() != nil {
			return ShareStatus{}, shareTransportFailure(ctx)
		}
		return ShareStatus{}, shareBadResponse()
	}
	var status ShareStatus
	if decodeShareJSON(raw, &status) != nil || !validShareStatus(status) || !shareResponseMatches(op, request, status, base64.RawStdEncoding.EncodeToString(c.identity.PublicKey())) {
		return ShareStatus{}, shareBadResponse()
	}
	return status, nil
}

func validShareStatus(s ShareStatus) bool {
	if !shareHex(s.ID, 32) || !shareKey(s.SenderKey) || !shareDigest(s.Manifest) || s.Generation == 0 || s.Generation > shareMaxCounter || s.Revision == 0 || s.Revision > shareMaxCounter || s.ExpiresAt <= 600 || s.ExpiresAt > 7258119000 {
		return false
	}
	created := s.ExpiresAt - int64(sharegrant.OfferTTL/time.Second)
	if (s.ReceiverKey != "" && (!shareKey(s.ReceiverKey) || s.ReceiverKey == s.SenderKey)) || (s.ClaimID != "" && !shareHex(s.ClaimID, 32)) || (s.TaskID != "" && !shareHex(s.TaskID, 32)) || (s.Plan != "" && !shareDigest(s.Plan)) || (s.ClaimUntil != 0 && (s.ClaimUntil <= created || s.ClaimUntil > s.ExpiresAt)) || (s.ResumeUntil != 0 && (s.ResumeUntil < created+int64(sharegrant.ResumeTTL/time.Second) || s.ResumeUntil > s.ExpiresAt+int64(sharegrant.ResumeTTL/time.Second))) {
		return false
	}
	if (s.ClaimID == "") != (s.ReceiverKey == "") || (s.ClaimID == "") != (s.ClaimUntil == 0) || (s.TaskID != "" && s.ClaimID == "") || (s.Plan != "" && s.TaskID == "") || (s.Plan == "") != (s.ResumeUntil == 0) {
		return false
	}
	switch s.State {
	case sharegrant.Offered:
		return s.ClaimID == "" && s.TaskID == "" && s.Plan == ""
	case sharegrant.Pending:
		return s.ClaimID != "" && s.TaskID == "" && s.Plan == ""
	case sharegrant.Approved:
		return s.ClaimID != "" && s.TaskID != "" && s.Plan == ""
	case sharegrant.Bound, sharegrant.Completed:
		return s.ClaimID != "" && s.TaskID != "" && s.Plan != ""
	case sharegrant.Revoked, sharegrant.Expired:
		return true
	default:
		return false
	}
}

func shareResponseMatches(op ShareOperation, r ShareRequest, s ShareStatus, actor string) bool {
	if (r.ID != "" && r.ID != s.ID) || (r.Manifest != "" && r.Manifest != s.Manifest) || (r.Plan != "" && r.Plan != s.Plan) || (r.TaskID != "" && r.TaskID != s.TaskID) {
		return false
	}
	if op == ShareDeny {
		return actor == s.SenderKey && s.State == sharegrant.Offered && s.Generation == r.Generation+1
	}
	if (r.ClaimID != "" && r.ClaimID != s.ClaimID) || (r.ReceiverKey != "" && r.ReceiverKey != s.ReceiverKey) || (r.Generation != 0 && r.Generation != s.Generation && !(op == ShareRevoke && r.Generation+1 == s.Generation)) {
		return false
	}
	switch op {
	case ShareCreate:
		// Creating with stable IDs can return any later, including terminal,
		// state. It must never cause a second offer or reset its original TTL.
		return actor == s.SenderKey
	case ShareClaim:
		return actor == s.ReceiverKey && (s.State == sharegrant.Pending || s.State == sharegrant.Approved || s.State == sharegrant.Bound || s.State == sharegrant.Completed)
	case ShareApprove:
		return actor == s.SenderKey && (s.State == sharegrant.Approved || s.State == sharegrant.Bound || s.State == sharegrant.Completed)
	case ShareBind:
		return actor == s.ReceiverKey && (s.State == sharegrant.Bound || s.State == sharegrant.Completed)
	case ShareAuthorize:
		return (actor == s.SenderKey || actor == s.ReceiverKey) && s.State == sharegrant.Bound
	case ShareComplete:
		return (actor == s.SenderKey || actor == s.ReceiverKey) && s.State == sharegrant.Completed
	case ShareRead:
		return actor == s.SenderKey || actor == s.ReceiverKey
	case ShareRevoke:
		return actor == s.SenderKey && s.State == sharegrant.Revoked
	default:
		return false
	}
}

func decodeShareFailure(resp *http.Response) error {
	code := protocol.InvalidMessage
	raw, err := readLimitedJSON(resp.Body, protocol.MaxMessageBytes)
	if err == nil {
		var failure struct {
			Code protocol.Code `json:"code"`
		}
		if json.Unmarshal(raw, &failure) == nil {
			switch failure.Code {
			case protocol.ShareInvalid, protocol.ShareRejected, protocol.ShareExpired, protocol.ShareConflict, protocol.ShareTerminal, protocol.ShareUnsupported, protocol.ShareUnavailable, protocol.AuthenticationFailed, protocol.VersionIncompatible, protocol.Replay, protocol.RateLimited:
				code = failure.Code
			}
		}
	}
	if code == protocol.InvalidMessage {
		if resp.StatusCode == http.StatusGatewayTimeout {
			code = protocol.SignalingTimeout
		} else if resp.StatusCode >= 500 && resp.StatusCode < 600 {
			code = protocol.SignalingUnreachable
		}
	}
	return &RemoteError{Status: resp.StatusCode, Cause: protocol.Error{Code: code, Detail: "temporary share request rejected"}}
}
