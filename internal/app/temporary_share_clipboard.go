package app

import (
	"crypto/subtle"
	"encoding/hex"
	"time"

	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/sharegrant"
)

// CheckTemporaryShareCode is only a local native-copy gate. It never returns
// the secret or puts it in persistence, task state, errors or diagnostics.
func (s *Service) CheckTemporaryShareCode(id, code string) error {
	done, err := s.beginProfileWork()
	if err != nil {
		return err
	}
	defer done()
	digest, err := sharegrant.CodeDigest(code)
	if err != nil {
		return temporaryShareFailure(protocol.ShareInvalid)
	}
	e, err := s.temporaryEntry(id)
	if err != nil {
		return err
	}
	r := temporaryRecordCopy(e)
	e.mu.Lock()
	denied := e.denied
	e.mu.Unlock()
	if denied || r.Role != "sender" || r.Stopped || r.Status.State != sharegrant.Offered && r.Status.State != sharegrant.Pending && r.Status.State != sharegrant.Approved || time.Now().Unix() >= r.Status.ExpiresAt || subtle.ConstantTimeCompare([]byte(r.CodeDigest), []byte(hex.EncodeToString(digest[:]))) != 1 {
		return temporaryShareFailure(protocol.ShareRejected)
	}
	return nil
}
