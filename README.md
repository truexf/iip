[![GoDoc](https://godoc.org/github.com/truexf/iip?status.svg)](http://godoc.org/github.com/truexf/iip) 
## 使用说明
* echo回显client示例: [example/echo_client/echo_client.go](https://github.com/truexf/iip/blob/master/example/echo_client/echo_client.go)
* echo回显server示例: [example/echo_server/echo_server.go](https://github.com/truexf/iip/blob/master/example/echo_server/echo_server.go)
* 文件下载client示例(支持断点续传): [example/file_client/file_client.go](https://github.com/truexf/iip/blob/master/example/file_client/file_client.go)
* 文件下载server示例(支持断点续传): [example/file_server/file_server.go](https://github.com/truexf/iip/blob/master/example/file_server/file_server.go)
* 文件上传client示例(支持断点续传): [example/upload_client/upload_client.go](https://github.com/truexf/iip/blob/master/example/upload_client/upload_client.go)
* 文件上传server示例(支持断点续传): [example/upload_server/upload_server.go](https://github.com/truexf/iip/blob/master/example/upload_server/upload_server.go)
* load-balance-client高并发请求示例: [loadbalanceclient_test.go](https://github.com/truexf/iip/blob/master/loadbalanceclient_test.go)

## benchmark对比测试
* BenchmarkPFEchoClientServer: 普通iip client单个channel
* BenchmarkPFEchoNetHttp：标准库 net/http 
* BenchmarkPFIIPBalanceClient：iip load balance client 
单核：
```
$ GOMAXPROCS=1 go test -bench=. -naddr="192.168.2.98:9091" -lbcaddr="192.168.2.98:9090#2" -iipaddr="192.168.2.98:9090" -run="PF.*" -benchmem -benchtime=10s
BenchmarkPFEchoClientServer 	    5418	   1944837 ns/op	   99428 B/op	      16 allocs/op
BenchmarkPFEchoNetHttp      	    3510	   3342903 ns/op	   51476 B/op	      64 allocs/op
BenchmarkPFIIPBalanceClient 	    6043	   1942451 ns/op	   92033 B/op	      16 allocs/op
```

四核:
```
$ GOMAXPROCS=4 go test -bench=. -naddr="192.168.2.98:9091" -lbcaddr="192.168.2.98:9090#2" -iipaddr="192.168.2.98:9090" -run="PF.*" -benchmem -benchtime=10s
BenchmarkPFEchoClientServer-4   	    7243	   1589468 ns/op	   99418 B/op	      16 allocs/op
BenchmarkPFEchoNetHttp-4        	   13854	    868070 ns/op	   51576 B/op	      64 allocs/op
BenchmarkPFIIPBalanceClient-4   	   19844	    590167 ns/op	   85720 B/op	      15 allocs/op
```

8核:
```
$ GOMAXPROCS=8 go test -bench=. -naddr="192.168.2.98:9091" -lbcaddr="192.168.2.98:9090#2" -iipaddr="192.168.2.98:9090" -run="PF.*" -benchmem -benchtime=10s
BenchmarkPFEchoClientServer-8   	    7090	   1630507 ns/op	   99443 B/op	      16 allocs/op
BenchmarkPFEchoNetHttp-8        	   27556	    428245 ns/op	   52198 B/op	      65 allocs/op
BenchmarkPFIIPBalanceClient-8   	   36670	    305865 ns/op	   84436 B/op	      15 allocs/op
```

## IIP是什么？ 
基于TCP的基础通讯协议及框架(IIP,Internal Interaction Protocol),该协议可作为RPC接口调用的底层协议，如同http2之于gRPC，本项目基于该协议实现了client/server的基础框架。

## IIP架构 
 ![image](https://github.com/truexf/iip/blob/master/iip.jpg)

## 为什么开发IIP？从http1.1说起

### 一、 http1的缺点，以及对应的http2的优化

#### 1. http1是文本协议，“文本协议”的意思是其传输的数据流（包括header和body）必须先转换为ascii码的可见字符。为什么要这样呢？因为他是以\n换行符来进行数据分隔的。如果是传输带额数据是以原本的二进制内码的形式，则会和\n产生冲突，无法解析。而采用文本形式则势必需要对原数据进行文本化编码，比如url-encode，base64,等等，无论哪种编码，都会导致数据的体积增大。
http2是二进制协议，无需进行文本化编码，不会导致体积增大。


#### 2. http1虽然支持body压缩，但不支持头部压缩（支持不了，本质上还是因为\n问题），而每一次“请求/响应”都必须有头部，这些头部有时候是不必要的（特别是像cookie和user-agent这种又臭又长又重复的头部字段），增大了数据体积。
http2只有在第一次请求的时候通过http1的头部字段upgrade来请求升级到http2协议，后续的请求响应全部以http2的二进制格式的帧进行传输，帧的精心设计，避免了如http1那样的头部信息冗余。而对应于http1的header信息，在http2的header帧中保留了一个伪首部的区域以供承载 ，且这个伪首部可以采用HPACK高效压缩。

#### 3. http1是“单路的”，意思是一条底层的tcp连接上同一时刻只能跑一个请求或响应。比如说，有r1,r2,r3三个请求，在同一个连接上，必须依次串行执行，先发出r1，收到完整的响应后，再发出r2。。。这样的单路模式，对于高并发的后端服务器来说，带来几个问题：
操作内核短时间产生大量的tcp连接，可能触发资源上限，包括连接数的上限，和大量端口占用；
tcp的连接的建立、断开分别需要连接双方进行3次、4次握手，这个过程是缓慢的。同时tcp连接关闭以后，需要经历一段时间（十几秒到几十秒）的静默期，在此期间端口是不能复用的，这是tcp协议设计的要求，为了保证可靠性；
在这两者的基础上，对上层业务带来的影响就是突发性的流量请求，或者说非持久连接型的应用类型，一方面会给服务器带来更大的负载，另一方面会显著的增加响应的时延。
http2是多路复用的，意思是一条tcp连接上可以同时跑多个请求和响应，也就是说r1,r2,r3可以同时发起，其原理是http2数据帧的格式里定义标识了不同的请求r1,r2,r3的字段。这样采用数据帧交替传输，实现逻辑上的并发。物理上当然还是串行执行的，但是他减少了tcp连接，从而大幅度降低了连接断开的延时，显著避免系统内核资源的上限，也降低了系统的负载。
值得一提的是由于多个请求在同一条tcp连接上传输，http2还在数据帧上设计了优先级字段，以使某些请求可以优先传输。同时还在数据帧上设计了流量控制字段。等等，可谓用心。

#### 4. http1只能以请求响应的模式运行，http2基于请求响应，同时支持服务端主动推送（一个客户端请求，返回多个响应）。

### 二、http2的不足

#### 1. http2是基于tcp协议的，tcp协议在设计之初并非是以现在优质、高带宽的网络基础设施基础上设计的，他充分考虑了数据的可靠性，显得小心谨慎，在传输速率的表现，也已经跟不上现时的网络基础设施。未来是否有更优化的网络层协议发展出来，可以拭目以待，包括像基于udp协议的QUIC协议。个人认为下层协议还是由os内核来实现比较好，QUIC协议实现在应用层而非操作系统的内核层，始终是一个软肋。

#### 2. 大部分的http2实现和应用（包括浏览器和web服务器），事实上都必须基于TLS(SSL)安全套接层。对于一个承载互联网内容的基础协议来说，这样的安全考量是合理的，也是必须的。有利就有弊，TLS的握手和数据加密过程必然给通信及通信双方带来一些负担。这些负担和额外的耗费，对于一些内部应用，比如说后端的微服务之间，有时候并不是必须的，是可以简化的。

#### 3. 由于现实世界已经基于http1建立起来，一些通讯链路上的基础设施，比如说http代理，暂不能支持http2，因此这会对http2的铺开造成阻碍，且这种阻碍可能是长期的过程。

#### 4. 由于http2是二进制的，传输又是多路复用的，在不同帧的设计上考虑到了压缩、优先级控制、流量控制、服务端推送，这导致http2的协议可以说比较复杂了。因此在协议的实现、应用的调试上将显然会比简单明文的http1增加一些难度。简单和直观，对于人类来说，具有天生的亲和力。

### 三、鱼和熊掌可兼得否？
我研究http2的初心起源于我想实现一组用于后端系统内部、微服务之间的通信协议，及其对应的客户端和服务器。由于手上现有的系统都是基于http1，自然是想标准的http2是否能直接满足我的需求：规避http1那些缺点，以“不支持多路复用”尤甚（这种情况在突发的网络流量下弊端尤为明显）。http2是可以满足我的需求的，且其在安全性，功能细节考虑得更完善更先进。但是，正是由于这些优势、由于http2作为互联网标准的基础协议的负担，其设计上带来的这种必然的复杂性，使得其在这种内部微服务间的确定性的应用场景中，并非最好的选择。于是，在这个场景中，基于简单，高效的原则，自行实现一个这样用于系统内部服务器节点间的应用通讯协议(IIP,Internal Interaction Protocol)的构想就产生了，该协议可作为RPC接口调用的底层协议，如同http2之于gRPC, 其主要特征:
#### * 基于tcp独立实现。
#### * 不考虑服务端推送，这种需求场景通过建立两条对向的tcp连接来实现。
#### * 协议基于“请求/响应”模式，每一个请求有且只有一个响应。请求和响应底层由一个或多个帧(packet)组成。
#### * 基于帧的“多路复用”，统一的帧格式， 依次由如下部分组成：
帧格式：
* 1字节数据帧状态标识：
	* 0表示请求首帧，请求未完成
	* 1表示请求首帧，请求完成
	* 2表示请求后续帧，请求未完成
	* 3表示请求后续帧，请求完成;
	* 4表示响应首帧，响应未完成
	* 5表示响应首帧，响应完成
	* 6表示响应后续帧，响应未完成
	* 7表示响应后续帧，响应完成
	* 8关闭连接
* 文本路径（URL兼容，用于指明请求的路径及参数， 如 /manager/picture/get?id=123&name=abc）
* \0
* 4字节channel识符（多路复用的流身份ID，无符号整数，请求方发起创建，server返回新channel的唯一ID）
* 4字节数据长度（限制一个帧的数据长度不能大于16MB）
* 数据
#### * 对于协议的扩展性考虑，按分层的思想，可以在上层通过对数据字段进行进一步的协议定义来满足，本协议保持简单性，不提供额外的冗余扩展字段。
比如示例中的文件下载，每次请求响应传输文件的其中一块，文件的整体组合由一个简单的上层协议实现。
#### * 不考虑代理协议，这种需求场景可通过成熟的socks5透明代理来实现。






