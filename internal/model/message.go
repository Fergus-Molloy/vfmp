package model

import (
	"fergus.molloy.xyz/vfmp/core/messages"
	"github.com/google/uuid"
)

type Message struct {
	Topic         string    `json:"topic"`
	CorrelationID uuid.UUID `json:"correlationID"`
	Data          []byte    `json:"data"`
}

func NewMessage(msg []byte, topic string, correlationID uuid.UUID) Message {
	return Message{
		Topic:         topic,
		CorrelationID: correlationID,
		Data:          msg,
	}
}

func (m Message) ToMsgMessage() messages.MsgMessage {
	return messages.NewMsgMessage(m.Topic, m.CorrelationID, m.Data)
}
