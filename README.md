# Chonglangban Encryption Middleware

这是为 Chonglangban 前端和 V2Board 兼容面板单独编写的 API 加密转发服务，仓库内的说明、配置和部署示例均使用中文。

它兼容当前主题和 EZ 的 v1 请求协议：前端使用 16 位十六进制字符作为 AES 密钥和 IV，使用 AES-CBC/PKCS7 加密逻辑路径，再进行双层 Base64 编码后放入 URL。与此同时，它提供 v2 AES-256-GCM 认证加密协议，密文自带随机 nonce 和认证标签，可以让普通 API 和订阅链接都不携带明文路径。协议细节和可变路径见 [协议与配置](docs/protocol-and-config.zh-CN.md)。

## 功能

- AES-CBC/PKCS7 路径解密，兼容标准 Base64、Base64URL 和前端 `encodeURIComponent`。
- AES-256-GCM v2 路径加密，随机 nonce、认证标签和篡改检测；支持 `auto` 平滑迁移。
- 默认适配 `/clb/clb` 路径前缀、`/api/v1` API 前缀。
- 支持同时配置多组加密入口、订阅标记和支付回调路径，迁移路径时不需要改程序。
- 解密路径严格限制为站内绝对路径，拒绝外部 URL、控制字符和异常路径。
- 订阅路径既可以使用 `/sub/...` 加密转发，也可以将 `/api/v1/client/subscribe?token=...` 整体使用 v2 加密后转发。
- 支付回调支持配置白名单，白名单路径可以不带 `X-IV` 直接转发。
- 正确处理带凭据的 CORS，支持预检请求。
- 健康检查 `/healthz`、请求体大小限制、连接复用、超时和优雅退出。
- 纯 Go 标准库实现，提供 Linux amd64/arm64 构建脚本。

## 配置

复制配置模板：

```bash
cp .env.example .env
```

至少填写（兼容旧主题）：

```dotenv
PORT=3000
BACKEND_API_URL=https://你的V2Board域名
AES_KEY=与你前端完全一致的16位十六进制密钥
PATH_PREFIX=/clb/clb
API_PREFIX=/api/v1
```

推荐启用 v2：

```dotenv
ENCRYPTION_PROTOCOL=auto
AEAD_KEY=64位十六进制随机密钥
ALLOW_PLAIN_SUBSCRIPTIONS=false
```

v2 前端配置使用 `API_MIDDLEWARE_PROTOCOL: 'aead'` 和同一份 `API_MIDDLEWARE_AEAD_KEY`。如果设置为 `auto`，中间件和前端可以在迁移期间同时处理 v1 与 v2；不要在生产环境把旧的明文订阅作为长期方案。

前端配置要对应：

```js
API_MIDDLEWARE_ENABLED: true,
API_MIDDLEWARE_URL: 'https://你的中间件域名',
API_MIDDLEWARE_KEY: '与 AES_KEY 相同的值',
API_MIDDLEWARE_PATH: '/clb/clb',
API_MIDDLEWARE_PROTOCOL: 'aead',
API_MIDDLEWARE_AEAD_KEY: '与 AEAD_KEY 相同的64位十六进制密钥',
```

`AES_KEY` 是 16 个 ASCII 十六进制字符。为了兼容当前 Chonglangban 前端，密钥按 UTF-8 字符串使用，不进行十六进制字节解码。

### 订阅地址与加密订阅

如果 V2Board 返回的订阅地址需要经过本中间件，推荐由前端将返回地址的路径和查询参数加密为 v2 URL，再把中间件域名和 `API_MIDDLEWARE_PATH` 作为外层地址。中间件解密后会把完整的 `/api/v1/client/subscribe?token=...` 直通后端，不会重复拼接 `/api/v1`。

旧版客户端无法携带加密协议时，仍可临时打开 `ALLOW_PLAIN_SUBSCRIPTIONS=true`，并配置 `PLAIN_SUBSCRIPTION_PATHS`；该选项只是兼容开关，不是推荐的安全模式。

### 支付回调

支付平台不能生成 Chonglangban 的加密路径，因此把实际回调路径加入白名单，例如：

```dotenv
PAYMENT_NOTIFY_PATHS=/api/v1/guest/payment/notify/*
```

然后让支付平台回调到：

```text
https://中间件域名/clb/clb/api/v1/guest/payment/notify/支付方式/UUID
```

白名单只绕过加密校验，仍然会经过后端地址和请求方法转发。

## 本地运行

```bash
go test ./...
go run .
```

启动后检查：

```bash
curl http://127.0.0.1:3000/healthz
```

## 构建

```bash
./build.sh
```

输出：

- `dist/chonglangban-middleware-linux-amd64`
- `dist/chonglangban-middleware-linux-arm64`
- `dist/.env.example`

也可以构建容器：

```bash
docker build -t chonglangban-encryption-middleware .
docker run --env-file .env -p 3000:3000 chonglangban-encryption-middleware
```

## 部署建议

建议用 Nginx 或 Caddy 为服务提供 HTTPS，再把 HTTPS 地址写入前端 `API_MIDDLEWARE_URL`。生产环境将 `ALLOWED_ORIGINS` 改成实际前端域名，不要把 `.env`、AES 密钥或真实后端地址提交到 GitHub。
