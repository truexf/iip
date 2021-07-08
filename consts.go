package iip

const (
	MaxPathLen            uint32 = 512
	MaxPacketSize         uint32 = 16 * 1024 * 1024
	PacketReadBufSize     uint32 = 16 * 1024
	ChannelPacketQueueLen uint32 = 1000
	ConnWriteQueueLen     int    = 1000

	PathNewChannel    string = "/sys/new_channel"
	PathDeleteChannel string = "/sys/delete_channel"

	RoleClient byte = 0
	RoleServer byte = 4

	PacketTypeRequest  byte = 0
	PacketTypeResponse byte = 4

	StatusC0 byte = 0 //请求首帧，请求未完成
	StatusC1 byte = 1 //请求首帧，请求完成
	StatusC2 byte = 2 //请求后续帧，请求未完成
	StatusC3 byte = 3 //请求后续帧，请求完成
	StatusS4 byte = 4 //响应首帧，响应未完成
	StatusS5 byte = 5 //表示响应首帧，响应完成
	StatusS6 byte = 6 //表示响应后续帧，响应未完成
	StatusS7 byte = 7 //表示响应后续帧，响应完成
	Status8  byte = 8 //关闭连接

	CtxServer       string = "/ctx/sys/server"
	CtxClient       string = "/ctx/sys/server"
	CtxResponseChan string = "/ctx/sys/response_chan"
)
