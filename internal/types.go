package signaling

type SignalMessage struct {
	Type    string      `json: "type"`
	FormId  string      `json: "formId"`
	ToId    string      `json: "toId"`
	Payload interface{} `json: "payload"`
}
