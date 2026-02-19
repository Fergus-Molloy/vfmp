package messages

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

var (
	ErrMalformedMessage = errors.New("malformed message")
)

type MessageType string

const (
	RDY MessageType = "RDY"
	ACK MessageType = "ACK"
	NCK MessageType = "NCK"
	DLQ MessageType = "DLQ"
	MSG MessageType = "MSG"
)

type Message interface {
	Bytes() []byte
}

func Parse(inp []byte) (Message, error) {
	if len(inp) < 3 {
		return nil, fmt.Errorf("%w: message too short", ErrMalformedMessage)
	}

	t := MessageType(inp[:3])
	switch t {
	case RDY:
		return NewRdyMessage(strings.TrimSpace(string(inp[4:]))), nil
	case ACK:
		topic, byteID, found := bytes.Cut(inp[4:], []byte("|"))
		if !found {
			return nil, fmt.Errorf("%w: separator not found", ErrMalformedMessage)
		}

		id, err := uuid.ParseBytes(bytes.TrimSpace(byteID))
		if err != nil {
			return nil, fmt.Errorf("%w: could not parse correlation id: %v", ErrMalformedMessage, err)
		}

		return NewAckMessage(strings.TrimSpace(string(topic)), id), nil
	case NCK:
		topic, byteID, found := bytes.Cut(inp[4:], []byte("|"))
		if !found {
			return nil, fmt.Errorf("%w: separator not found", ErrMalformedMessage)
		}

		id, err := uuid.ParseBytes(bytes.TrimSpace(byteID))
		if err != nil {
			return nil, fmt.Errorf("%w: could not parse correlation id: %v", ErrMalformedMessage, err)
		}

		return NewNckMessage(strings.TrimSpace(string(topic)), id), nil
	case DLQ:
		topic, byteID, found := bytes.Cut(inp[4:], []byte("|"))
		if !found {
			return nil, fmt.Errorf("%w: separator not found", ErrMalformedMessage)
		}

		id, err := uuid.ParseBytes(bytes.TrimSpace(byteID))
		if err != nil {
			return nil, fmt.Errorf("%w: could not parse correlation id: %v", ErrMalformedMessage, err)
		}

		return NewDlqMessage(strings.TrimSpace(string(topic)), id), nil

	case MSG:
		parts := bytes.SplitN(inp[4:], []byte("|"), 3)
		if len(parts) != 3 {
			return nil, fmt.Errorf("%w: incorrect number of message parts", ErrMalformedMessage)
		}
		topic := parts[0]
		byteID := parts[1]
		data := parts[2]

		id, err := uuid.ParseBytes(bytes.TrimSpace(byteID))
		if err != nil {
			return nil, fmt.Errorf("%w: could not parse correlation id: %v", ErrMalformedMessage, err)
		}

		return NewMsgMessage(strings.TrimSpace(string(topic)), id, data), nil

	default:
		return nil, fmt.Errorf("%w: message type not recognised: %s", ErrMalformedMessage, t)
	}
}

type RdyMessage struct {
	MsgType MessageType
	Topic   string
}

func NewRdyMessage(topic string) RdyMessage {
	return RdyMessage{
		MsgType: RDY,
		Topic:   topic,
	}
}

func (r RdyMessage) Bytes() []byte {
	return fmt.Appendf(nil, "RDY|%s", r.Topic)
}

type AckMessage struct {
	MsgType       MessageType
	Topic         string
	CorrelationID uuid.UUID
}

func NewAckMessage(topic string, correlationID uuid.UUID) AckMessage {
	return AckMessage{
		MsgType:       ACK,
		Topic:         topic,
		CorrelationID: correlationID,
	}
}

func (a AckMessage) Bytes() []byte {
	return fmt.Appendf(nil, "ACK|%s|%s", a.Topic, a.CorrelationID)
}

type NckMessage struct {
	MsgType       MessageType
	Topic         string
	CorrelationID uuid.UUID
}

func NewNckMessage(topic string, correlationID uuid.UUID) NckMessage {
	return NckMessage{
		MsgType:       NCK,
		Topic:         topic,
		CorrelationID: correlationID,
	}
}

func (a NckMessage) Bytes() []byte {
	return fmt.Appendf(nil, "NCK|%s|%s", a.Topic, a.CorrelationID)
}

type DlqMessage struct {
	MsgType       MessageType
	Topic         string
	CorrelationID uuid.UUID
}

func NewDlqMessage(topic string, correlationID uuid.UUID) DlqMessage {
	return DlqMessage{
		MsgType:       DLQ,
		Topic:         topic,
		CorrelationID: correlationID,
	}
}

func (a DlqMessage) Bytes() []byte {
	return fmt.Appendf(nil, "DLQ|%s|%s", a.Topic, a.CorrelationID)
}

type MsgMessage struct {
	MsgType       MessageType
	Topic         string
	CorrelationID uuid.UUID
	Data          []byte
}

func NewMsgMessage(topic string, correlationID uuid.UUID, data []byte) MsgMessage {
	return MsgMessage{
		MsgType:       MSG,
		Topic:         topic,
		CorrelationID: correlationID,
		Data:          data,
	}
}

func (a MsgMessage) Bytes() []byte {
	return fmt.Appendf(nil, "MSG|%s|%s|%s", a.Topic, a.CorrelationID, a.Data)
}
