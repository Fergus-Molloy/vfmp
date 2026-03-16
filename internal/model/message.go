package model

import (
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
