package model

type Message struct {
	Topic         string `json:"topic"`
	CorrelationID string `json:"correlationID"`
	Data          []byte `json:"data"`
}

func NewMessage(msg []byte, topic, correlationID string) Message {
	return Message{
		Topic:         topic,
		CorrelationID: correlationID,
		Data:          msg,
	}
}
