http学习笔记

#### http基础
```
http几个类别:
1.text
2.image
3.audio/video
4.application:application/json、application/javascript、application/pdf
```

```
encoding type:
1.gzip
2.deflate:zlib压缩方式
3.br:一种专门为http优化的新压缩算法
```

```
accept字段标记的是客户端可理解的MIME type，可以用逗号做分割符列出多个类型
让服务器有更多的选择余地，例如下面这个
Accept: text/html,application/xml,image/webp,image/png
```

```
Accept-Encoding标记的是客户端支持的压缩格式
服务器的Content-Encoding表示实际使用的压缩格式
```

```
Accept-Language标记了客户端可理解的自然语言，也允许用,做分隔符列出多个类型
Accept-Language: zh-CN, zh, en
服务器端Content-Language: zh-CN
```

```
字符集在http里使用的请求头字段是Accept-Charset，但是响应头里没有对应的Content-Charset，
而是在Content-Type字段的数据类型后面用charset=xxx来表示
```
```
q设置优先级，quality factor
Accept: text/html,application/xml;q=0.9,*/*;q=0.8
表示浏览器最希望使用的是html文件，权重是1，其次是XML文件，权重是0.9，任意数据权重是0.8
```

```
vary表示服务器端采用了客户端的哪个字段，不是每个客户端的accept选项服务器都会采用
```
```
浏览器最多会有六个连接并发执行，是rfc的规定。
```
```
http协议对body大小没有限制，一般都是框架自己的限制
```
```
客户端accept-encoding填写了gzip，但是服务端不一定返回的报文就是gzip的，要看它的content-encoding，不一定是压缩的。服务器端很可能无视很多字段，http1.1里唯一强制要求的就是host字段，其他都不是必须的。
```
```
客户端发送的报文，也需要要填content-*，表示自己的报文属性，如果body是空就可以不携带了
```
