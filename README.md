[![GoDoc](https://godoc.org/github.com/truexf/iip?status.svg)](http://godoc.org/github.com/truexf/iip) 

## IIP是什么？ 
基于TCP的多路复用的基础通讯协议及框架(IIP,Internal Interaction Protocol),该协议可作为RPC接口调用的底层协议，如同http2之于gRPC，本项目基于该协议实现了client/server的基础框架。

## 使用说明
* echo回显client示例: [example/echo_client/echo_client.go](https://github.com/truexf/iip/blob/master/example/echo_client/echo_client.go)
* echo回显server示例: [example/echo_server/echo_server.go](https://github.com/truexf/iip/blob/master/example/echo_server/echo_server.go)
* 文件下载client示例(支持断点续传): [example/file_client/file_client.go](https://github.com/truexf/iip/blob/master/example/file_client/file_client.go)
* 文件下载server示例(支持断点续传): [example/file_server/file_server.go](https://github.com/truexf/iip/blob/master/example/file_server/file_server.go)
* 文件上传client示例(支持断点续传): [example/upload_client/upload_client.go](https://github.com/truexf/iip/blob/master/example/upload_client/upload_client.go)
* 文件上传server示例(支持断点续传): [example/upload_server/upload_server.go](https://github.com/truexf/iip/blob/master/example/upload_server/upload_server.go)
* load-balance-client高并发请求示例: [loadbalanceclient_test.go](https://github.com/truexf/iip/blob/master/loadbalanceclient_test.go)
* httpClient<==>httpServer(作为反向代理)<==>iipServer [httpadapter/http_server_test.go](https://github.com/truexf/iip/blob/master/httpadapter/http_server_test.go)

## benchmark对比测试
* BenchmarkPFEchoClientServer: 普通iip client单个channel
* BenchmarkPFEchoNetHttp：标准库 net/http 
* BenchmarkPFIIPBalanceClient：iip load balance client  
* 运行benchmark前需要先编译运行启动server端，server代码在example/echo_server/  编译：$ go build ./echo_server.go  启动: $ ./echo_server
* 单核：
```
$ GOMAXPROCS=1 go test -bench=. -naddr="192.168.2.98:9091" -lbcaddr="192.168.2.98:9090#2" -iipaddr="192.168.2.98:9090" -run="PF.*" -benchmem -benchtime=10s
BenchmarkPFEchoClientServer 	    5418	   1944837 ns/op	   99428 B/op	      16 allocs/op
BenchmarkPFEchoNetHttp      	    3510	   3342903 ns/op	   51476 B/op	      64 allocs/op
BenchmarkPFIIPBalanceClient 	    6043	   1942451 ns/op	   92033 B/op	      16 allocs/op
```

* 四核:
```
$ GOMAXPROCS=4 go test -bench=. -naddr="192.168.2.98:9091" -lbcaddr="192.168.2.98:9090#2" -iipaddr="192.168.2.98:9090" -run="PF.*" -benchmem -benchtime=10s
BenchmarkPFEchoClientServer-4   	    7243	   1589468 ns/op	   99418 B/op	      16 allocs/op
BenchmarkPFEchoNetHttp-4        	   13854	    868070 ns/op	   51576 B/op	      64 allocs/op
BenchmarkPFIIPBalanceClient-4   	   19844	    590167 ns/op	   85720 B/op	      15 allocs/op
```

* 八核:
```
$ GOMAXPROCS=8 go test -bench=. -naddr="192.168.2.98:9091" -lbcaddr="192.168.2.98:9090#2" -iipaddr="192.168.2.98:9090" -run="PF.*" -benchmem -benchtime=10s
BenchmarkPFEchoClientServer-8   	    7090	   1630507 ns/op	   99443 B/op	      16 allocs/op
BenchmarkPFEchoNetHttp-8        	   27556	    428245 ns/op	   52198 B/op	      65 allocs/op
BenchmarkPFIIPBalanceClient-8   	   36670	    305865 ns/op	   84436 B/op	      15 allocs/op
```

## 典型案例 
一个靠谱的底层通讯框架组件，必然是基于一个真实生产系统，伴随着该系统的成长迭代，历史思考和实践基础上抽象发展而成。

 ![image](https://github.com/truexf/iip/blob/master/usecase/baixun.png)  
[百寻广告流量交易平台](https://www.bxadsite.com/)日处理数十亿次广告流量请求,峰值qps 4w+, 采用iip承载其内部核心交易系统的微服务。

## IIP架构 
 ![image](https://github.com/truexf/iip/blob/master/iip.jpg)  


## IIP帧格式：
 ![image](https://github.com/truexf/iip/blob/master/packet.jpg)  
   


### * 对于协议的扩展性考虑，按分层的思想，可以在上层通过对数据字段进行进一步的协议定义来满足，本协议保持简单性，不提供额外的冗余扩展字段。
比如示例中的文件下载，每次请求响应传输文件的其中一块，文件的整体组合由一个简单的上层协议实现。







