package storage

import (
	"github.com/google/uuid"
)

const installID = "installID"

func (s *Storage) InstallID() string {
	var id []byte

	id, err := s.db.Get([]byte(installID))
	if err != nil {
		return "id-read-error"
	}
	if id == nil {
		id = []byte(newID())
		if err := s.db.Set([]byte(installID), id); err != nil {
			return "id-write-error"
		}
	}
	return string(id)
}

func newID() string {
	return uuid.New().String()
}
