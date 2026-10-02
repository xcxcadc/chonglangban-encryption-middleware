# 安装、配置与使用

本文用于把 Chonglangban 加密中间件部署到 Linux 服务器，并与 Chonglangban / EZ 兼容主题和 V2Board 连接。

## 1. 准备后端

中间件只接受固定的 `BACKEND_API_URL`，不会根据客户端传入的 URL 转发，所以先准备好 V2Board 地址，例如：

```dotenv
BACKEND_API_URL=https://panel.example.com
```

这里填写面板根地址，不要附加 `/api/v1`、查询参数或片段。中间件本身建议放在 HTTPS 反向代理后面，主题中的 `API_MIDDLEWARE_URL` 使用 HTTPS 地址。

主题端地址配置要填写完整的 Origin，例如 `https://middleware.example.com`。不要省略协议写成 `middleware.example.com`，也不要把 `/clb/clb` 拼进 Origin；`/clb/clb` 应单独填写到主题的 `API_MIDDLEWARE_PATH`，并与中间件的 `PATH_PREFIX` 一致。省略协议会让浏览器把地址当成前端站点下的相对路径，表现为配置加载失败或请求 404。

## 2. 下载并安装运行包

从 GitHub Releases 下载与你服务器 CPU 对应的运行包或二进制文件。以 amd64 Linux 为例：

```bash
sudo mkdir -p /opt/chonglangban-middleware
sudo cp chonglangban-middleware-linux-amd64 /opt/chonglangban-middleware/chonglangban-middleware
sudo chmod 755 /opt/chonglangban-middleware/chonglangban-middleware
sudo cp .env.example /opt/chonglangban-middleware/.env
sudo chmod 600 /opt/chonglangban-middleware/.env
sudo nano /opt/chonglangban-middleware/.env
```

ARM64 服务器使用 `chonglangban-middleware-linux-arm64`。`.env` 只保存在服务器，不要提交到 GitHub。

## 3. 生成密钥并配置协议

推荐新部署直接使用 v2 AES-256-GCM：

```bash
openssl rand -hex 32
```

把输出填入 `AEAD_KEY`，并把同一值填入主题的 `API_MIDDLEWARE_AEAD_KEY`：

```dotenv
PORT=3000
BACKEND_API_URL=https://panel.example.com
ENCRYPTION_PROTOCOL=aead
AEAD_KEY=这里填64位十六进制字符串
PATH_PREFIX=/clb/clb
ENCRYPTED_PATH_PREFIXES=/clb/clb
API_PREFIX=/api/v1
ALLOWED_ORIGINS=https://你的主题域名
ALLOW_PLAIN_SUBSCRIPTIONS=false
```

主题对应配置：

```js
API_MIDDLEWARE_ENABLED: true,
API_MIDDLEWARE_URL: 'https://middleware.example.com',
API_MIDDLEWARE_PATH: '/clb/clb',
API_MIDDLEWARE_PROTOCOL: 'aead',
API_MIDDLEWARE_AEAD_KEY: '与 AEAD_KEY 相同的64位十六进制字符串',
```

需要兼容旧 EZ v1 时使用：

```dotenv
ENCRYPTION_PROTOCOL=auto
AES_KEY=16位十六进制字符
AEAD_KEY=64位十六进制字符
```

确认新主题和订阅链接都正常后，再切换到 `ENCRYPTION_PROTOCOL=aead`。v1 的 `AES_KEY` 是 16 个 ASCII 十六进制字符，不能把它当作 8 字节密钥填写。

## 4. 订阅和支付回调

v2 主题会把 V2Board 返回的完整订阅路径（包括 `token` 查询参数）加密后再访问中间件。中间件解密出 `/api/v1/client/subscribe?token=...` 后会直通后端，不会重复拼接 `/api/v1`。

如果旧客户端必须暂时使用明文订阅：

```dotenv
ALLOW_PLAIN_SUBSCRIPTIONS=true
PLAIN_SUBSCRIPTION_PATHS=/api/v1/client/subscribe
```

这只是迁移开关，完成迁移后应关闭。

支付平台回调通常不能自定义 `X-IV` 或 v2 密文，因此只对白名单路径放行：

```dotenv
PAYMENT_NOTIFY_PATHS=/api/v1/guest/payment/notify/*
```

支付平台填写：

```text
https://middleware.example.com/clb/clb/api/v1/guest/payment/notify/支付方式/回调标识
```

不要把普通用户 API 放进 `PAYMENT_NOTIFY_PATHS`。

## 5. systemd 启动

复制仓库中的 `deploy/chonglangban-middleware.service`：

```bash
sudo cp deploy/chonglangban-middleware.service /etc/systemd/system/chonglangban-middleware.service
sudo systemctl daemon-reload
sudo systemctl enable --now chonglangban-middleware
sudo systemctl status chonglangban-middleware
```

日志和更新：

```bash
sudo journalctl -u chonglangban-middleware -f
sudo systemctl restart chonglangban-middleware
```

## 6. HTTPS 反向代理与检查

反向代理把 HTTPS 请求转发到 `127.0.0.1:3000`，并保留请求方法、查询参数和请求体。部署完成后先检查：

```bash
curl -fsS https://middleware.example.com/healthz
```

正常结果包含 `"status":"ok"`。主题登录、仪表盘读取、商店下单、支付状态检查、订阅导入都应在浏览器开发者工具中看到请求发往中间件域名；v2 请求路径应以 `v2.` 开头，且不需要 `X-IV`。

## 7. 源码构建

源码构建需要 Go 1.22 或更高版本：

```bash
go test ./...
go vet ./...
go build -trimpath -ldflags='-s -w' -o chonglangban-middleware .
```

交叉构建使用：

```bash
./build.sh
```

## 8. 故障排查

- 返回 `origin not allowed`：把主题的完整 HTTPS 来源加入 `ALLOWED_ORIGINS`。
- 返回 `path not found`：检查主题 `API_MIDDLEWARE_PATH` 和中间件 `PATH_PREFIX` 是否完全一致。
- 返回 `invalid encrypted path`：检查 v2 两端的 `AEAD_KEY` 是否完全一致；v1 还要检查 `AES_KEY` 和 `X-IV`。
- 订阅 404：确认中间件 `PLAIN_SUBSCRIPTION_PATHS` 包含 `/api/v1/client/subscribe`，并确认后端根地址没有重复 `/api/v1`。
- 支付回调失败：只把支付平台实际回调路径加入 `PAYMENT_NOTIFY_PATHS`，再检查反向代理是否允许 POST 请求体。

