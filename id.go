package polycast

import (
	"encoding/binary"
	"errors"
)

type ID struct {
	Unique []byte
	leader int16 // should be set by the sender.
}

type IDString string
type IDBytes []byte

var (
	errInvalidID = errors.New("invalid ID")
)

func (i ID) validID() error {
	if i.Unique == nil {
		return errInvalidID
	}

	if i.leader < 0 {
		return errInvalidID
	}

	return nil
}

func (i *ID) string() IDString {
	return IDString(i.Bytes())
}

func (i *ID) Bytes() IDBytes {
	bf := make([]byte, 2+len(i.Unique))
	binary.LittleEndian.PutUint16(bf, uint16(i.leader))

	copy(bf[2:], i.Unique)
	return bf
}

func bytesToID(b []byte) ID {
	if len(b) < 3 {
		return ID{}
	}

	id := ID{
		Unique: make([]byte, len(b)-2),
		leader: int16(binary.LittleEndian.Uint16(b[:2])),
	}

	copy(id.Unique, b[2:])

	if len(id.Unique) == 0 {
		return ID{}
	}

	if id.leader < 0 {
		return ID{}
	}

	return id
}
