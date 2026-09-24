# Chonglangban 中间件协议与可变路由

## 与 EZ 的兼容边界

当前主题和 EZ 前端发送加密请求的顺序是：

```text
原始路径 → AES-128-CBC + PKCS7 → 标准 Base64
→ btoa → encodeURIComponent → 放入中间件路由最后一段
```

请求同时携带 `X-IV`，值为 16 位十六进制字符。`AES_KEY` 和前端的 `API_MIDDLEWARE_KEY` 必须完全相同。当前前端把它们作为 16 个 ASCII 字符使用，服务端也按字符字节使用，不把 `2c` 解读为一个十六进制字节，这保证了和现有 EZ 前端的字节级兼容。

本项目在兼容线格式的基础上增加了这些校验：

- 接受标准 Base64、无填充 Base64、Base64URL 和浏览器编码后的路径。
- IV、密文长度和 PKCS7 填充错误会立即拒绝，不会把空路径转发到后端。
- 明文只允许站内路径，拒绝绝对 URL、主机名、片段和控制字符，避免形成开放代理。
- 后端目标只由服务端环境变量决定，客户端不能通过密文修改目标主机。
- CORS 使用实际请求来源回显，带凭据时不会返回无效的 `*`。

## 所有路由都可以修改

逗号可以配置多个值，适合换路径时保留旧路径进行平滑迁移。

### 加密入口

```dotenv
PATH_PREFIX=/clb/clb
ENCRYPTED_PATH_PREFIXES=/clb/clb,/gateway/v2
```

前端当前的 `API_MIDDLEWARE_PATH` 要填写其中一个入口。以后改成 `/gateway/v2` 时，同时改前端配置和服务端环境变量即可。

### 订阅标记

```dotenv
SUBSCRIPTION_PREFIX=/sub
SUBSCRIPTION_PREFIXES=/sub,/feed
```

解密后的路径命中任意订阅标记时，会直接拼到后端根地址，不再拼接 `API_PREFIX`。

### V2Board 普通订阅入口

```dotenv
ALLOW_PLAIN_SUBSCRIPTIONS=true
PLAIN_SUBSCRIPTION_PATHS=/api/v1/client/subscribe,/api/v1/client/subscribe/*
```

这样 V2Board 可以返回：

```text
https://中间件域名/api/v1/client/subscribe?token=用户订阅令牌
```

订阅路径可以换成自己的路径，只需要把新路径放进 `PLAIN_SUBSCRIPTION_PATHS`。

### 支付回调

```dotenv
PAYMENT_NOTIFY_PATHS=/api/v1/guest/payment/notify/*,/payment/callback/*
```

命中白名单的回调不要求 `X-IV`，但仍然只会被转发到固定的 `BACKEND_API_URL`。路径白名单只支持末尾 `*` 前缀匹配，避免规则过宽。

## 前端匹配配置

```js
API_MIDDLEWARE_ENABLED: true,
API_MIDDLEWARE_URL: 'https://中间件域名',
API_MIDDLEWARE_KEY: '与 AES_KEY 相同',
API_MIDDLEWARE_PATH: '/clb/clb',
```

前端密钥、中间件 `AES_KEY` 和真实后端地址属于私有配置，不应写入公开仓库。仓库只提供 `.env.example`。

## 修改配置后的检查

```bash
go test ./...
go vet ./...
go build ./...
curl https://中间件域名/healthz
```

`/healthz` 只返回服务状态，不返回密钥、后端地址或其它敏感配置。
