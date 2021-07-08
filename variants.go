package iip

var (
	DefaultResponseData = []byte(`{"code": -1, "message": "unknown"}`)

	ErrPacketContinue   error = &Error{Code: 100, Message: "packet uncompleted"}
	ErrHandleNoResponse error = &Error{Code: 101, Message: "handle no response"}
	ErrHandleError      error = &Error{Code: 102, Message: "handle error"}
	ErrRequestTimeout   error = &Error{Code: 103, Message: "request timtout"}
	ErrUnknown          error = &Error{Code: 104, Message: "unknown"}
)
