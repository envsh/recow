
### Features

* 自动根据IP国家分流
* 支持HTTP/HTTPS 代理请求
* 支持负载均衡，backup
* 指定DNS服务器, udp,tcp,tls,doh,
* DoH支持
* ADblock by domain return 404

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

### ADBlock
* 对于HTTPS只能获取到域名，所以只能使用域名过滤
* 对于HTTP可以考虑采用资源路径过滤。
* 该配置文件中包含easylist格式的数据文件，以及hosts格式的数据文件，需要分别处理。
* https://github.com/AdguardTeam/AdGuardHome/blob/128229ad736fce424166dc38dcaf17486fd8f1b5/client/src/helpers/filters/filters.json

### easylist 格式，
* ! 注释
* || 匹配地址的开头
* |  指地址的开始或者结束
* / 正则
* @@ 取消拦截？

完整说明：https://www.leeyiding.com/archives/50/

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

