
### Features

* 自动根据IP国家分流
* 支持HTTP/HTTPS 代理请求
* 支持负载均衡，backup
* 指定DNS服务器
* DoH支持

### 上游代理负载均衡

* 循环
* 后备
* 哈希
* 权重
* 延迟，这要自动测试，有点难以实现

### 简单的http(s)代理分流

```
if candirect {
  if reqmth == "CONNECT" { // https
     response HTTP/1.1 200 estab
     dial original dest addr
     bidirection io copy
  } else { // http
     recontruct req, just change first request line
     dial original dest addr
     write to original dest addr
     bidirection io copy
  }
}else{
  if reqmth == "CONNECT" { // https
     dail upstream proxy addr
     write to upstream dest addr, CONNECT command
     bidirection io copy
  } else { // http
     dail upstream proxy addr
     recontruct req, just change first request line
     write to upstream dest addr
     bidirection io copy
  }
}
```


### 通用DNS服务器
* 1.1.1.1 速度快，但现在屏蔽严重了
* 8.8.8.8 
* 9.9.9.9

### 国内DNS服务器

* 114.114.114.114

### 国内DoH服务器

* https://dns.rubyfish.cn/dns-query
* https://dns.alidns.com/dns-query
* https://doh.pub/dns-query

