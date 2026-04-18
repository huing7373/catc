package ws

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnvelope_Unmarshal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantID  string
		wantTyp string
	}{
		{
			name:    "full envelope",
			input:   `{"id":"abc-123","type":"state.tick","payload":{"steps":100}}`,
			wantID:  "abc-123",
			wantTyp: "state.tick",
		},
		{
			name:    "no payload",
			input:   `{"id":"x","type":"ping"}`,
			wantID:  "x",
			wantTyp: "ping",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var env Envelope
			require.NoError(t, json.Unmarshal([]byte(tt.input), &env))
			assert.Equal(t, tt.wantID, env.ID)
			assert.Equal(t, tt.wantTyp, env.Type)
		})
	}
}

func TestNewAckResponse(t *testing.T) {
	t.Parallel()

	payload := json.RawMessage(`{"ok":true}`)
	resp := NewAckResponse("req-1", "state.tick", payload)

	assert.Equal(t, "req-1", resp.ID)
	assert.True(t, resp.OK)
	assert.Equal(t, "state.tick.result", resp.Type)
	assert.JSONEq(t, `{"ok":true}`, string(resp.Payload))
	assert.Nil(t, resp.Error)
}

func TestNewErrorResponse(t *testing.T) {
	t.Parallel()

	resp := NewErrorResponse("req-2", "blindbox.redeem", "BLINDBOX_NOT_FOUND", "blindbox not found")

	assert.Equal(t, "req-2", resp.ID)
	assert.False(t, resp.OK)
	assert.Equal(t, "blindbox.redeem.result", resp.Type)
	assert.Nil(t, resp.Payload)
	require.NotNil(t, resp.Error)
	assert.Equal(t, "BLINDBOX_NOT_FOUND", resp.Error.Code)
	assert.Equal(t, "blindbox not found", resp.Error.Message)
}

func TestNewPush(t *testing.T) {
	t.Parallel()

	payload := json.RawMessage(`{"state":"eating"}`)
	p := NewPush("friend.state", payload)

	assert.Equal(t, "friend.state", p.Type)
	assert.JSONEq(t, `{"state":"eating"}`, string(p.Payload))
}

func TestResponse_MarshalJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		resp Response
		want string
	}{
		{
			name: "ack with payload",
			resp: NewAckResponse("1", "debug.echo", json.RawMessage(`{"msg":"hi"}`)),
			want: `{"id":"1","ok":true,"type":"debug.echo.result","payload":{"msg":"hi"}}`,
		},
		{
			name: "error without payload",
			resp: NewErrorResponse("2", "unknown.type", "UNKNOWN_MESSAGE_TYPE", "unknown message type"),
			want: `{"id":"2","ok":false,"type":"unknown.type.result","error":{"code":"UNKNOWN_MESSAGE_TYPE","message":"unknown message type"}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			data, err := json.Marshal(tt.resp)
			require.NoError(t, err)
			assert.JSONEq(t, tt.want, string(data))
		})
	}
}
