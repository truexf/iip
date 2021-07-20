package iip

import "testing"

// func TestValidatePath(t *testing.T) {
// 	pathOk := "/hello_abc-xx-dd1211234//"
// 	if !ValidatePath(pathOk) {
// 		t.Fatalf("fail")
// 	}
// 	pathNotOk := "/hello_abc-xx-d^d1211$234//"
// 	if ValidatePath(pathNotOk) {
// 		t.Fatalf("fail")
// 	}
// }

func TestDefaultContext(t *testing.T) {
	ctx := &DefaultContext{}
	if ctx.GetCtxData("testkey") != nil {
		t.Fatalf("fail")
	}
	ctx.SetCtxData("testkey", ctx)
	if ctx.GetCtxData("testkey") != ctx {
		t.Fatalf("fail")
	}
	ctx.SetCtxData("testkey", nil)
	if ctx.GetCtxData("testkey") != nil {
		t.Fatalf("fail")
	}
	ctx.SetCtxData("testkey", ctx)
	if ctx.GetCtxData("testkey") != ctx {
		t.Fatalf("fail")
	}
	ctx.RemoveCtxData("testkey")
	if ctx.GetCtxData("testkey") != nil {
		t.Fatalf("fail")
	}
}
