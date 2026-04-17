package ws

type HubStub struct{}

func NewHubStub() *HubStub { return &HubStub{} }

func (h *HubStub) GoroutineCount() int { return 0 }

func (h *HubStub) Name() string { return "ws_hub" }
