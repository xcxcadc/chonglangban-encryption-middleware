# Chonglangban Encryption Middleware

这是为 Chonglangban 前端和 V2Board 兼容面板单独编写的 API 加密转发服务，仓库内的说明、配置和部署示例均使用中文。

它兼容当前主题和 EZ 的请求协议：前端使用 16 位十六进制字符作为 AES 密钥和 IV，使用 AES-CBC/PKCS7 加密逻辑路径，再进行双层 Base64 编码后放入 URL。中间件解密后将普通请求转发到 V2Board 的 `/api/v1`，订阅请求和白名单支付回调单独处理。协议细节和可变路径见 [协议与配置](docs/协议与配置.md)。

## 功能

- AES-CBC/PKCS7 路径解密，兼容标准 Base64、Base64URL 和前端 `encodeURIComponent`。
- 默认适配 `/clb/clb` 路径前缀、`/api/v1` API 前缀。
- 支持同时配置多组加密入口、订阅标记和支付回调路径，迁移路径时不需要改程序。
- 解密路径严格限制为站内绝对路径，拒绝外部 URL、控制字符和异常路径。
- `/sub/...` 订阅路径可直接转发，不拼接 API 前缀。
- 支付回调支持配置白名单，白名单路径可以不带 `X-IV` 直接转发。
- 正确处理带凭据的 CORS，支持预检请求。
- 健康检查 `/healthz`、请求体大小限制、连接复用、超时和优雅退出。
- 纯 Go 标准库实现，提供 Linux amd64/arm64 构建脚本。

## 配置

复制配置模板：

```bash
cp .env.example .env
```

至少填写：

```dotenv
PORT=3000
BACKEND_API_URL=https://你的V2Board域名
AES_KEY=与你前端完全一致的16位十六进制密钥
PATH_PREFIX=/clb/clb
API_PREFIX=/api/v1
```

前端配置要对应：

```js
API_MIDDLEWARE_ENABLED: true,
API_MIDDLEWARE_URL: 'https://你的中间件域名',
API_MIDDLEWARE_KEY: '与 AES_KEY 相同的值',
API_MIDDLEWARE_PATH: '/clb/clb',
```

`AES_KEY` 是 16 个 ASCII 十六进制字符。为了兼容当前 Chonglangban 前端，密钥按 UTF-8 字符串使用，不进行十六进制字节解码。

### 订阅地址

如果 V2Board 返回的订阅地址需要经过本中间件，将面板的订阅基地址配置为中间件域名，并把 `PLAIN_SUBSCRIPTION_PATHS` 配置成实际订阅路径。默认的 `https://中间件域名/api/v1/client/subscribe?token=...` 可以直接访问。订阅路径和标记都能自行变更，普通加密 API 请求不受影响。

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
