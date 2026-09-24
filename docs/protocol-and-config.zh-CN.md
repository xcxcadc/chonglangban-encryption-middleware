# Chonglangban 中间件协议与可变路由

## 两代协议

本项目保留 EZ 兼容协议 v1，同时增加推荐使用的 v2。两者可以通过 `ENCRYPTION_PROTOCOL=auto` 并行运行，方便逐步迁移。

### v1：兼容 EZ 的 AES-CBC

当前主题和 EZ 前端发送加密请求的顺序是：

```text
原始路径 → AES-128-CBC + PKCS7 → 标准 Base64
→ btoa → encodeURIComponent → 放入中间件路由最后一段
```

请求同时携带 `X-IV`，值为 16 位十六进制字符。`AES_KEY` 和前端的 `API_MIDDLEWARE_KEY` 必须完全相同。当前前端把它们作为 16 个 ASCII 字符使用，服务端也按字符字节使用，不把 `2c` 解读为一个十六进制字节，这保证了和现有 EZ 前端的字节级兼容。

v1 的 AES-CBC 本身不提供认证标签，因此只能作为兼容协议。服务端仍会严格验证 IV、密文长度、填充和明文路径，但这不能等同于 AEAD 的篡改认证。

### v2：AES-256-GCM 认证加密

v2 使用 64 位十六进制的 `AEAD_KEY`，解码后为 32 字节 AES-256 密钥。每次加密生成新的 12 字节随机 nonce，URL 段格式为：

```text
v2.<Base64URL(随机 nonce || 密文 || GCM 认证标签)>
```

v2 不需要 `X-IV`，GCM 认证标签会在解密时验证路径没有被篡改。普通 API、`/sub/...` 订阅路径以及完整的 V2Board `/api/v1/client/subscribe?token=...` 都可以使用同一种加密路径。

这是本项目相对 EZ 中间件的主要升级。OWASP 建议优先使用带认证的加密模式（例如 GCM/CCM），而 CBC 在没有额外 MAC 时不提供完整性保护。[OWASP 加密存储建议](https://cheatsheetseries.owasp.org/cheatsheets/Cryptographic_Storage_Cheat_Sheet.html)

## 协议配置

```dotenv
# auto=同时兼容 v1/v2；legacy=仅 v1；aead=仅 v2
ENCRYPTION_PROTOCOL=auto

# v1：16 个十六进制字符，必须和 API_MIDDLEWARE_KEY 相同
AES_KEY=16位十六进制密钥

# v2：64 个十六进制字符，必须和前端 API_MIDDLEWARE_AEAD_KEY 相同
AEAD_KEY=64位十六进制密钥
```

建议生产环境使用：

```dotenv
ENCRYPTION_PROTOCOL=aead
ALLOW_PLAIN_SUBSCRIPTIONS=false
```

迁移期间使用 `auto`，确认新前端和新订阅链接都正常后，再切换到 `aead`。

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

解密后的路径命中任意订阅标记时，会直接拼到后端根地址，不再拼接 `API_PREFIX`。例如加密 `/sub/user-token`，最终会转发到后端 `/sub/user-token`。

### 加密 V2Board 订阅

推荐把完整订阅路径加密：

```text
原始逻辑路径：/api/v1/client/subscribe?token=用户订阅令牌
外部访问地址：https://中间件域名/clb/clb/v2.加密内容
```

中间件会识别 `PLAIN_SUBSCRIPTION_PATHS` 中的逻辑路径，解密后直接转发到后端，不会重复拼接 `/api/v1`。

```dotenv
PLAIN_SUBSCRIPTION_PATHS=/api/v1/client/subscribe,/api/v1/client/subscribe/*
```

这里的变量名保留了兼容命名；它同时用于识别“加密后应该直通”的订阅路径。若希望旧版客户端继续访问明文订阅，额外打开：

```dotenv
ALLOW_PLAIN_SUBSCRIPTIONS=true
```

不打开时，明文订阅会被拒绝，v2 加密订阅仍然可以正常工作。

## 支付回调

```dotenv
PAYMENT_NOTIFY_PATHS=/api/v1/guest/payment/notify/*,/payment/callback/*
```

命中白名单的回调不要求 `X-IV`，但仍然只会被转发到固定的 `BACKEND_API_URL`。支付平台通常不能自定义加密请求头，因此支付回调保留独立白名单是必要的；不要把普通 API 路径加入该白名单。

## 前端匹配配置

旧版 v1：

```js
API_MIDDLEWARE_ENABLED: true,
API_MIDDLEWARE_URL: 'https://中间件域名',
API_MIDDLEWARE_KEY: '与 AES_KEY 相同',
API_MIDDLEWARE_PATH: '/clb/clb',
API_MIDDLEWARE_PROTOCOL: 'legacy',
```

推荐 v2：

```js
API_MIDDLEWARE_ENABLED: true,
API_MIDDLEWARE_URL: 'https://中间件域名',
API_MIDDLEWARE_PATH: '/clb/clb',
API_MIDDLEWARE_PROTOCOL: 'aead',
API_MIDDLEWARE_AEAD_KEY: '与 AEAD_KEY 相同的64位十六进制密钥',
```

前端密钥、中间件 `AES_KEY`/`AEAD_KEY` 和真实后端地址属于私有配置，不应写入公开仓库。仓库只提供 `.env.example`。

## 安全边界

- 加密不是 TLS 的替代品，生产环境仍必须使用 HTTPS。
- 前端密钥会随浏览器代码提供给客户端，因此它主要用于隐藏路径、保护传输内容和防止中间人篡改，不应被当作用户身份认证机制。
- v2 的 nonce 每次随机生成，不能固定、复用或从时间戳推导。
- 后端目标始终由服务端环境变量决定，客户端不能借密文修改转发目标。
- 明文订阅兼容开关默认关闭；支付回调只能按最小路径白名单放行。

## 修改配置后的检查

```bash
go test ./...
go vet ./...
go build ./...
curl https://中间件域名/healthz
```

`/healthz` 只返回服务状态，不返回密钥、后端地址或其它敏感配置。
