package o11y

type O11yError string

func (o O11yError) Error() string {
	return string(o)
}

const ErrSendFailed = O11yError("o11y client send failed")
const ErrClientClosed = O11yError("o11y client closed")
const ErrClientBacklogged = O11yError("o11y client backlogged")

type CheckConnect struct {
	CheckConnect bool   `json:"checkConnect"`
	Identifier   string `json:"identifier"`
}

type CheckResponse struct {
	Ok bool `json:"ok"`
}

type ObservationHeader struct {
	Timestamp  int64  `json:"timestamp,string"`
	Identifier string `json:"identifier"`
	Data       any    `json:"data,omitempty"`
	Action     string `json:"action,omitempty"`
}

type BadType struct {
	Typename string `json:"typename"`
}
