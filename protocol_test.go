package iip

import (
	"bytes"
	"testing"
)

func TestFNCreateNetPacket(t *testing.T) {
	pkt := &Packet{
		Path:      "/testpath",
		Status:    Status8,
		ChannelId: 1234,
		Data:      []byte("test data"),
	}
	bts, err := CreateNetPacket(pkt)
	if err != nil {
		t.Fatalf(err.Error())
	}
	pkt2, err := ReadPacket(bytes.NewBuffer(bts))
	if err != nil {
		t.Fatalf(err.Error())
	}
	if pkt.Path != pkt2.Path || pkt.ChannelId != pkt2.ChannelId || pkt.Status != pkt2.Status || len(pkt.Data) != len(pkt2.Data) {
		t.Fatalf("fail")
	}
	for i, v := range pkt.Data {
		if v != pkt2.Data[i] {
			t.Fatalf("fail")
		}
	}
}

func BenchmarkFNCreateNetPacket(t *testing.B) {
	pkt := &Packet{
		Path:      "/testpath",
		Status:    Status8,
		ChannelId: 1234,
		Data:      []byte("test data"),
	}
	// for i := 0; i < t.N; i++ {
	CreateNetPacket(pkt)
	// }
}

func TestReadPacket(t *testing.T) {
	TestFNCreateNetPacket(t)
}

func BenchmarkFNReadPacket(t *testing.B) {
	pkt := &Packet{
		Path:      "/testpath",
		Status:    Status8,
		ChannelId: 1234,
		Data:      []byte("test data"),
	}
	bts, _ := CreateNetPacket(pkt)
	// for i := 0; i < t.N; i++ {
	ReadPacket(bytes.NewBuffer(bts))
	// }
}
