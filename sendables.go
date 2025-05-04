package polycast

import (
	"errors"

	"google.golang.org/protobuf/proto"
	anypb "google.golang.org/protobuf/types/known/anypb"
)

type IncomingMessage interface {
	GetContent() proto.Message

	GetWireMeta() *WireMeta
	GetMessageID() []byte

	BasicValidation() error
}

func ToWire(s proto.Message, ID ID, wm *WireMeta) *Wireable {
	ne, err := anypb.New(s)
	if err != nil {
		panic(err)
	}

	return &Wireable{
		Content:   ne,
		MessageID: ID.Bytes(),
		Meta:      wm,
	}
}

type incomingMessage struct {
	Message  proto.Message // Embedded proto.Message
	ID       []byte
	WireMeta *WireMeta
}

// GetMessageID implements IncomingMessage.
func (i *incomingMessage) GetMessageID() []byte {
	return i.ID
}

func (i *incomingMessage) GetContent() proto.Message {
	return i.Message
}

// GetWireMeta implements IncomingMessage.
func (i *incomingMessage) GetWireMeta() *WireMeta {
	return i.WireMeta
}

// SECURITY: overwrite the sender field of the wireable with the ID of the sender according to
// TLS. For instance, if sender number 5 sends a wireable with META.Sender = 3, the receiver should
// set the sender number to 5, and then call this function.
func FromWire(w *Wireable) (IncomingMessage, error) {
	m, err := w.Content.UnmarshalNew()
	if err != nil {
		// Handle the error by returning nil as the Message
		return nil, err
	}

	return &incomingMessage{
		Message:  m,
		ID:       w.GetMessageID(),
		WireMeta: w.GetMeta(),
	}, nil
}

var (
	ErrNilMessage     = errors.New("incoming message is nil")
	ErrNilContent     = errors.New("message content is nil")
	ErrNilWireMeta    = errors.New("wire meta is nil")
	ErrEmptyMessageID = errors.New("ID is empty")
)

func (i *incomingMessage) BasicValidation() error {
	if i == nil {
		return ErrNilMessage
	}

	// Check if the message is nil
	if i.Message == nil {
		return ErrNilContent
	}

	// Check if the WireMeta is nil
	if i.WireMeta == nil {
		return ErrNilWireMeta
	}

	// Check if the MessageID is empty
	if len(i.ID) == 0 {
		return ErrEmptyMessageID
	}

	return nil
}
