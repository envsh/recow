
### 上游代理负载均衡

* 循环
* 后备
* 哈希
* 权重
* 延迟，这要自动测试，有点难以实现

### 简单的http(s)代理分流

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

