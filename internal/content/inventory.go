package content

import (
	"context"
	"errors"
	"io"
)

// ObjectReferences is Go-only lifecycle metadata, never a frontend DTO. The
// application can reconcile its own reference namespace after a crash between
// the durable object write and its SQLite draft transaction. Unknown namespaces
// remain owned by their original caller and must not be released.
type ObjectReferences struct {
	Snapshot   Snapshot
	References []string
}

func (s *Store) OwnedObjects(ctx context.Context) ([]ObjectReferences, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.root == nil {
		return nil, ErrClosed
	}
	f, err := s.root.Open(".")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	objects := make([]ObjectReferences, 0)
	count := 0
	for {
		entries, readErr := f.ReadDir(128)
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			count++
			if count > maxObjects {
				return nil, ErrLimit
			}
			if !validID(entry.Name()) {
				continue
			}
			obj, record, err := s.load(entry.Name())
			if err != nil {
				return nil, err
			}
			_ = obj.Close()
			objects = append(objects, ObjectReferences{record.Snapshot, append([]string(nil), record.References...)})
		}
		if errors.Is(readErr, io.EOF) {
			return objects, nil
		}
		if readErr != nil {
			return nil, readErr
		}
	}
}
