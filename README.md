# qrcode-decoder

基于 Go + [gozxing](https://github.com/makiuchi-d/gozxing) 的 HTTP 二维码解码服务。接收 Base64 编码的图片，返回识别出的二维码文本内容。

## 特性

- 基于 ZXing 的 QR 码识别引擎（gozxing）
- 支持 **Base64** 或 **图片 URL**（`http`/`https`）两种输入
- 支持带 `data:image/...;base64,` 前缀的 Data URL
- 支持 **JPEG / PNG / GIF** 图片（标准库 `image` 解码）
- 支持 JSON 与 `application/x-www-form-urlencoded` / `multipart/form-data` 两种提交方式
- 健康检查接口，便于负载均衡与容器探针
- 优雅关闭（SIGINT / SIGTERM，5 秒超时）

## 识别能力说明

以下为典型场景下的参考识别率（受拍摄质量、尺寸、对比度影响，非 SLA 承诺）：

| 场景 | 参考识别率 |
|------|------------|
| 标准黑白二维码 | 高 |
| 红色二维码 + 白底 | ~99% |
| 中间带小 Logo | ~98% |
| 模糊、倾斜、低对比 | 95%+ |

## 环境要求

- Go **1.23+**
- 无其他运行时依赖（纯 Go 编译）

## 快速开始

### 1. 克隆与依赖

```bash
git clone <your-repo-url>
cd qrcode-decoder
go mod download
```

### 2. 配置（可选）

复制环境变量示例并按需修改：

```bash
cp .env.example .env
```

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `PORT` | HTTP 监听端口（不含冒号） | `8086` |

也可直接导出环境变量：

```bash
export PORT=8086
```

### 3. 启动

**开发：**

```bash
go run main.go
```

**编译后运行：**

```bash
go build -o qrcode-decoder .
./qrcode-decoder
```

启动成功后日志示例：

```text
✅ 服务启动成功 | 监听端口: :8086
```

## API

### 健康检查

```http
GET /health
```

**响应示例：**

```json
{
  "status": "ok",
  "name": "qrcode-decoder",
  "engine": "gozxing"
}
```

### 二维码解码

```http
POST /decode
```

仅支持 **POST**。请求体二选一：

| 字段 | 说明 |
|------|------|
| `base64` | 图片 Base64 字符串（可带 `data:image/...;base64,` 前缀） |
| `url` | 图片直链地址，仅支持 `http` / `https`，最大约 **10MB** |

同时传 `url` 与 `base64` 时，**优先使用 `url`**。

**成功响应：**

```json
{
  "code": 0,
  "message": "success",
  "content": "https://example.com"
}
```

**失败响应：**

```json
{
  "code": 1,
  "message": "二维码识别失败"
}
```

| `code` | 含义 |
|--------|------|
| `0` | 识别成功，`content` 为二维码文本 |
| `1` | 失败，详见 `message` |

常见 `message`：`仅支持POST请求`、`base64或url不能为空`、`图片地址无效`、`图片下载失败`、`base64解码失败`、`图片格式不支持`、`二维码识别失败` 等。

#### 方式一：JSON

```http
POST /decode
Content-Type: application/json
```

```json
{
  "base64": "iVBORw0KGgoAAAANSUhEUgAA..."
}
```

或使用完整 Data URL（会自动去掉逗号前的 MIME 前缀）：

```json
{
  "base64": "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAA..."
}
```

**cURL 示例：**

```bash
curl -s -X POST http://127.0.0.1:8086/decode \
  -H "Content-Type: application/json" \
  -d '{"base64":"'"$(base64 -w0 qrcode.png)"'"}'
```

Windows PowerShell 可先对文件做 Base64，再填入 JSON。

**通过图片 URL 解析：**

```json
{
  "url": "https://example.com/qrcode.png"
}
```

```bash
curl -s -X POST http://127.0.0.1:8086/decode \
  -H "Content-Type: application/json" \
  -d '{"url":"https://0625d9d5e411e970.sousd.com/t/2606052332555840309.png"}'
```

#### 方式二：表单

```http
POST /decode
Content-Type: application/x-www-form-urlencoded
```

```text
base64=<Base64字符串>
```

或：

```text
url=https://example.com/qrcode.png
```

**cURL 示例：**

```bash
curl -s -X POST http://127.0.0.1:8086/decode \
  -F "base64=$(base64 -w0 qrcode.png)"
```

```bash
curl -s -X POST http://127.0.0.1:8086/decode \
  -F "url=https://example.com/qrcode.png"
```

`multipart/form-data` 同样支持字段 `base64` / `url`，单张图片上限约 **10MB**。

## 部署

### 二进制部署（Linux 服务器，推荐）

无需在服务器安装 Go 或 Docker，本地（或 CI）交叉编译出单个可执行文件，上传到 Linux 即可运行。

#### 1. 交叉编译

在项目根目录执行（`CGO_ENABLED=0` 生成无 cgo 依赖的单一可执行文件，拷到 Linux 即可运行）。

**Linux / macOS 交叉编译（目标：常见 x86_64 云主机）：**

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o qrcode-decoder .
```

**ARM64 服务器**（部分轻量云、树莓派）：将 `GOARCH=amd64` 改为 `GOARCH=arm64`。

**Windows（PowerShell）交叉编译：**

```powershell
$env:CGO_ENABLED = "0"
$env:GOOS = "linux"
$env:GOARCH = "amd64"   # ARM 服务器改为 arm64
go build -ldflags="-s -w" -o qrcode-decoder .
```

编译完成后当前目录会得到 `qrcode-decoder`（Linux 可执行文件，无扩展名）。

也可在**服务器上直接编译**（需已安装 Go 1.23+）：

```bash
cd /opt/qrcode-decoder/src   # 源码目录
go build -ldflags="-s -w" -o /opt/qrcode-decoder/qrcode-decoder .
```

#### 2. 上传到服务器

建议目录：

```text
/opt/qrcode-decoder/
├── qrcode-decoder    # 可执行文件
└── .env              # 可选，PORT=8086
```

本机上传（将 `user@your-server` 换成实际地址）：

```bash
ssh user@your-server "sudo mkdir -p /opt/qrcode-decoder && sudo chown $USER:$USER /opt/qrcode-decoder"
scp qrcode-decoder user@your-server:/opt/qrcode-decoder/
scp .env.example user@your-server:/opt/qrcode-decoder/.env   # 可选
ssh user@your-server "chmod +x /opt/qrcode-decoder/qrcode-decoder"
```

#### 3. 前台试运行

```bash
cd /opt/qrcode-decoder
export PORT=8086
./qrcode-decoder
```

另开终端验证：

```bash
curl http://127.0.0.1:8086/health
```

确认正常后 `Ctrl+C` 停止，再按下面配置 systemd 常驻。

#### 4. systemd 常驻（生产推荐）

创建 `/etc/systemd/system/qrcode-decoder.service`：

```ini
[Unit]
Description=QR Code Decoder HTTP Service
After=network.target

[Service]
Type=simple
User=nobody
Group=nogroup
WorkingDirectory=/opt/qrcode-decoder
EnvironmentFile=-/opt/qrcode-decoder/.env
Environment=PORT=8086
ExecStart=/opt/qrcode-decoder/qrcode-decoder
Restart=always
RestartSec=3
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
```

> `User=nobody` 可按需改为专用系统用户；若使用 `.env`，确保该用户有读权限。

启用并开机自启：

```bash
sudo systemctl daemon-reload
sudo systemctl enable qrcode-decoder
sudo systemctl start qrcode-decoder
sudo systemctl status qrcode-decoder
```

日志与重启：

```bash
journalctl -u qrcode-decoder -f
sudo systemctl restart qrcode-decoder
```

#### 5. 防火墙与对外访问

仅内网调用时，可只监听 `127.0.0.1`（需改代码或前面加 Nginx）。默认监听 `0.0.0.0:PORT`，开放端口示例：

```bash
# Ubuntu ufw
sudo ufw allow 8086/tcp
sudo ufw reload

# CentOS firewalld
sudo firewall-cmd --permanent --add-port=8086/tcp
sudo firewall-cmd --reload
```

对外网暴露时，建议前面加 **Nginx/Caddy** 做 HTTPS 与 IP 白名单，不要直接把解码接口裸奔到公网。

#### 6. 升级流程

```bash
# 本机重新编译后
scp qrcode-decoder user@your-server:/opt/qrcode-decoder/qrcode-decoder.new
ssh user@your-server "mv /opt/qrcode-decoder/qrcode-decoder.new /opt/qrcode-decoder/qrcode-decoder && chmod +x /opt/qrcode-decoder/qrcode-decoder && sudo systemctl restart qrcode-decoder"
```

#### 7. 无 systemd 时（临时）

```bash
cd /opt/qrcode-decoder
nohup ./qrcode-decoder >> /var/log/qrcode-decoder.log 2>&1 &
```

---

### Docker

项目根目录可新增 `Dockerfile`（多阶段构建示例）：

```dockerfile
FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o qrcode-decoder .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /app/qrcode-decoder .
ENV PORT=8086
EXPOSE 8086
CMD ["./qrcode-decoder"]
```

构建与运行：

```bash
docker build -t qrcode-decoder .
docker run -d --name qrcode-decoder -p 8086:8086 -e PORT=8086 qrcode-decoder
```

健康检查：

```bash
curl http://127.0.0.1:8086/health
```

### Docker Compose

```yaml
services:
  qrcode-decoder:
    build: .
    ports:
      - "8086:8086"
    environment:
      PORT: "8086"
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://127.0.0.1:8086/health"]
      interval: 30s
      timeout: 5s
      retries: 3
```

### Kubernetes 探针示例

```yaml
livenessProbe:
  httpGet:
    path: /health
    port: 8086
  initialDelaySeconds: 5
  periodSeconds: 10
readinessProbe:
  httpGet:
    path: /health
    port: 8086
  periodSeconds: 5
```

## 反向代理（Nginx 片段）

```nginx
location /decode {
    proxy_pass http://127.0.0.1:8086;
    client_max_body_size 12m;
    proxy_read_timeout 30s;
}

location /health {
    proxy_pass http://127.0.0.1:8086;
}
```

注意：解码接口会传输 Base64，体积约为原图的 4/3，请适当调大 `client_max_body_size`。

## 项目结构

```text
qrcode-decoder/
├── main.go          # HTTP 服务与解码逻辑
├── go.mod
├── .env.example     # 端口配置示例
└── README.md
```

## 常见问题

**Q: 返回「图片格式不支持」？**  
A: 确认图片为 JPEG/PNG/GIF，且 Base64 完整无换行错误（或已正确 URL 编码）。其他格式需在代码中增加对应 `image` 解码器的 blank import。

**Q: 返回「二维码识别失败」？**  
A: 多为图中无 QR 码、码制非 QR、分辨率过低或严重污损。可尝试提高拍摄清晰度、对比度，或裁剪二维码区域后再编码上传。

**Q: 如何修改端口？**  
A: 设置环境变量 `PORT`（如 `8080`），或在 `.env` 中配置，服务启动时会自动加载 `.env`（[godotenv](https://github.com/joho/godotenv)）。

**Q: 是否支持图片 URL？**  
A: 支持。POST `/decode` 传 `url` 字段即可，服务会拉取图片后解码；仅支持 `http`/`https`，超时 15 秒。

**Q: 是否支持批量？**  
A: 当前为单张解码；批量请在业务侧循环调用。

## 技术栈

- [gozxing](https://github.com/makiuchi-d/gozxing) — ZXing 的 Go 移植
- 标准库 `net/http`、`image`、`encoding/base64`

## License

按仓库实际许可证填写（若未声明，请自行补充）。
