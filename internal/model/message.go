package model

type Message struct {
	Topic         string
	CorrelationID string
	Data          []byte
}

func NewMessage(msg []byte, topic, correlationID string) Message {
	return Message{
		Topic:         topic,
		CorrelationID: correlationID,
		Data:          msg,
	}
}
