package ws

import "encoding/json"

type Envelope struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type Response struct {
	ID      string          `json:"id,omitempty"`
	OK      bool            `json:"ok"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Error   *ErrorPayload   `json:"error,omitempty"`
}

type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Push struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

func NewAckResponse(id, reqType string, payload json.RawMessage) Response {
	return Response{
		ID:      id,
		OK:      true,
		Type:    reqType + ".result",
		Payload: payload,
	}
}

func NewErrorResponse(id, reqType string, code, message string) Response {
	return Response{
		ID:   id,
		OK:   false,
		Type: reqType + ".result",
		Error: &ErrorPayload{
			Code:    code,
			Message: message,
		},
	}
}

func NewPush(pushType string, payload json.RawMessage) Push {
	return Push{
		Type:    pushType,
		Payload: payload,
	}
}
