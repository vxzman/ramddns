# ipflow - 动态 DNS 客户端

[![Go Version](https://img.shields.io/badge/go-1.21-blue.svg)](https://go.dev)
[![License](https://img.shields.io/badge/license-BSD_3--Clause-blue.svg)](LICENSE)
[![Platform](https://img.shields.io/badge/platform-Linux%20%7C%20FreeBSD%20%7C%20OpenBSD%20%7C%20macOS-yellow)](README.md)

**ipflow** 是一个用 Go 编写的轻量级动态 DNS (DDNS) 客户端，支持多域名、多服务商、IPv6，具备跨平台能力和丰富的日志输出。

---

## 目录

- [特性](#特性)
- [快速开始](#快速开始)
- [配置指南](#配置指南)
- [环境变量](#环境变量)
- [自动运行](#自动运行)
- [平台支持](#平台支持)
- [项目结构](#项目结构)

---

## 特性

| 功能 | 说明 |
|------|------|
| 🌐 **多域名支持** | 一条配置可更新多个 DNS 记录 |
| ☁️ **多服务商** | 支持 Cloudflare、阿里云 DNS |
| 🔒 **安全性** | 强制使用环境变量，禁止明文密钥 |
| 🚀 **并发更新** | 多个域名并行更新，提高效率 |
| 📦 **IPv6 支持** | 原生支持 IPv6，多平台接口获取（Linux/netlink, BSD/ioctl） |
| 🔄 **IP 缓存** | 避免重复 API 调用 |
| 🎨 **彩色日志** | 终端下日志分级彩色显示，支持文件输出 |
| 🌍 **代理支持** | HTTP(S)/SOCKS5 代理，记录级控制 |

---

## 快速开始

### 1. 构建

```bash
# 基础构建
go build -o ipflow ./cmd/ipflow

# 带版本信息构建
go build -ldflags "-X main.version=v2.0.0" -o ipflow ./cmd/ipflow

# 使用构建脚本
chmod +x build.sh
./build.sh v2.0.0
```

### 2. 配置

创建 `config.json`：

```json
{
    "general": {
        "get_ip": {
            "interface": "eth0",
            "urls": ["https://ipv6.icanhazip.com"]
        },
        "work_dir": "/var/lib/ipflow",
        "log_output": "shell"
    },
    "records": [
        {
            "provider": "cloudflare",
            "zone": "example.com",
            "record": "dev",
            "cloudflare": {
                "api_token": "${CLOUDFLARE_API_TOKEN}"
            }
        }
    ]
}
```

### 3. 设置环境变量

```bash
export CLOUDFLARE_API_TOKEN="your_api_token_here"
```

### 4. 运行

```bash
# 运行（CLI 名称：aiolos，容器化友好）
./aiolos run -c /path/to/config.json -d /path/to/dir

# 如果缺少 -c，会在 -d 指定路径下查找 config.json：
./aiolos run -d /path/to/dir

# 忽略缓存强制更新
./aiolos run -c config.json -i

# 查看版本
./aiolos version
```

---

## 容器化部署与密钥/变量管理

为了容器/自动化场景更方便使用：

- 新的 CLI 名称为 `aiolos`，配置参数：`--config/-c`（配置文件路径），`--dir/-d`（工作目录，用于缓存、日志相对路径和密钥存放）。
- 配置文件中新增 `vars` 顶层字段，用于定义可在配置中复用的变量：

```json
"vars": {
    "cloudflare_var1": "xxxxxxxx"
}
```

- 在配置中引用 `vars`：

```json
"cloudflare": {
    "api_token": "${var.cloudflare_var1}"
}
```

- 密钥处理与加密：
  - 支持在配置中直接写明文（为了简化测试与容器化部署）。首次运行时，程序会将明文敏感值（`vars`、Cloudflare `api_token`、阿里云 `access_key`）自动加密，并将配置文件写回（写入形式为 `enc:...`）。
  - 加密密钥保存在 `<dir>/.aiolos.key`（若未指定 `-d`，则放在配置文件目录），权限为 0600。
  - 启动时，程序会自动解密 `enc:` 值并在运行时使用明文；如果配置中使用了 `enc:`，但缺少密钥文件，会报错并退出。

- 相对日志路径处理：如果配置中的 `log_output` 不是绝对路径，则相对于 `--dir/-d`（或配置文件所在目录）展开并创建父目录。
- 缓存文件：`cache.lastip` 将放在 `--dir/-d`（若提供）或配置文件所在目录下；`work_dir` 配置项不再必需，程序会忽略或可保留兼容性。

- 推荐流程（容器内）：
  1. 将 `config.json` 挂载到容器内，如 `/etc/aiolos/config.json`。
  2. 指定工作目录为可写目录，例如 `/data/aiolos`，并运行：
     `aiolos run -c /etc/aiolos/config.json -d /data/aiolos`
  3. 首次运行会生成 `/data/aiolos/.aiolos.key` 并将明文敏感值替换为 `enc:` 格式写回配置（覆盖原文件）。

---

## 配置指南

### 安全性要求

> ⚠️ **重要**：出于安全考虑，ipflow **禁止在配置文件中明文存储密钥信息**。所有敏感信息必须使用环境变量引用。

❌ **错误示例**（会被拒绝执行）：
```json
{
    "cloudflare": {
        "api_token": "your_actual_token_here"
    }
}
```

✅ **正确示例**：
```json
{
    "cloudflare": {
        "api_token": "${CLOUDFLARE_API_TOKEN}",
        "zone_id": "${CLOUDFLARE_ZONE_ID:-}"
    }
}
```

---

### 完整配置示例

```json
{
    "general": {
        "get_ip": {
            "interface": "eth0",
            "urls": [
                "https://ipv6.icanhazip.com",
                "https://6.ipw.cn"
            ]
        },
        "work_dir": "/var/lib/ipflow",
        "log_output": "shell",
        "proxy": ""
    },
    "records": [
        {
            "provider": "cloudflare",
            "zone": "example.com",
            "record": "dev",
            "ttl": 180,
            "proxied": false,
            "use_proxy": false,
            "cloudflare": {
                "api_token": "${CLOUDFLARE_API_TOKEN}",
                "zone_id": "${CLOUDFLARE_ZONE_ID:-}"
            }
        },
        {
            "provider": "aliyun",
            "zone": "example.cn",
            "record": "www",
            "ttl": 600,
            "aliyun": {
                "access_key_id": "${ALIYUN_ACCESS_KEY_ID}",
                "access_key_secret": "${ALIYUN_ACCESS_KEY_SECRET}"
            }
        }
    ]
}
```

---

### 配置字段说明

#### general（全局配置）

| 字段 | 类型 | 说明 | 示例 |
|------|------|------|------|
| `get_ip.interface` | string | 本地网卡名（优先使用） | `eth0` |
| `get_ip.urls` | []string | 外部 IP 检测 API 列表（降级） | `["https://ipv6.icanhazip.com"]` |
| `work_dir` | string | 缓存文件目录 | `/var/lib/goddns` |
| `log_output` | string | 日志输出，`shell` 表示终端 | `shell` 或文件路径 |
| `proxy` | string | 全局代理（可选） | `socks5://127.0.0.1:1080` |

#### records（记录数组）

| 字段 | 类型 | 说明 | 必填 |
|------|------|------|------|
| `provider` | string | 服务商：`cloudflare` 或 `aliyun` | ✅ |
| `zone` | string | 主域名 | ✅ |
| `record` | string | 子域名/记录名（`@` 表示根域） | ✅ |
| `ttl` | int | DNS 记录 TTL（秒） | ❌ |
| `proxied` | bool | Cloudflare 代理模式 | ❌ |
| `use_proxy` | bool | 是否使用全局代理 | ❌ |

#### Cloudflare 配置

| 字段 | 类型 | 说明 |
|------|------|------|
| `cloudflare.api_token` | string | API Token（环境变量引用） |
| `cloudflare.zone_id` | string | Zone ID（可选，留空自动获取） |
| `cloudflare.ttl` | int | TTL（可选，覆盖记录级） |
| `cloudflare.proxied` | bool | 代理模式（可选，覆盖记录级） |

#### 阿里云配置

| 字段 | 类型 | 说明 |
|------|------|------|
| `aliyun.access_key_id` | string | AccessKey ID（环境变量引用） |
| `aliyun.access_key_secret` | string | AccessKey Secret（环境变量引用） |
| `aliyun.ttl` | int | TTL（可选，覆盖记录级） |

---

### 服务商对比

| 特性 | Cloudflare | 阿里云 |
|------|------------|--------|
| IPv6 支持 | ✅ | ✅ |
| 代理支持 | ✅ | ❌ |
| 自动获取 ZoneID | ✅ | ✅ |
| API 认证 | API Token | AccessKey |

---

## 环境变量

### 支持的环境变量

| 变量名 | 说明 | 示例 |
|--------|------|------|
| `CLOUDFLARE_API_TOKEN` | Cloudflare API Token | `your_token_here` |
| `CLOUDFLARE_ZONE_ID` | Cloudflare Zone ID（可选） | `abc123xyz` |
| `ALIYUN_ACCESS_KEY_ID` | 阿里云 AccessKey ID | `LTAI1234567890` |
| `ALIYUN_ACCESS_KEY_SECRET` | 阿里云 AccessKey Secret | `your_secret_here` |

### 使用方式

```bash
# 设置环境变量
export CLOUDFLARE_API_TOKEN="your_token_here"
export ALIYUN_ACCESS_KEY_ID="LTAI1234567890"
export ALIYUN_ACCESS_KEY_SECRET="your_secret_here"

# 运行
./ipflow run -f config.json
```

### 环境变量默认值

支持 `${VAR:-default}` 语法：

```json
{
    "cloudflare": {
        "zone_id": "${CLOUDFLARE_ZONE_ID:-}"
    }
}
```

- `${VAR}` - 使用环境变量值
- `${VAR:-default}` - 未设置或为空时使用默认值
- `${VAR-default}` - 未设置时使用默认值

---

## 自动运行

### systemd 定时（推荐）

#### 1. 创建配置文件

```bash
sudo mkdir -p /etc/ipflow
sudo nano /etc/ipflow/config.json
```

#### 2. 创建 systemd 服务和定时器

```ini
# /etc/systemd/system/ipflow.service
[Unit]
Description=Dynamic DNS client
After=network.target

[Service]
Type=oneshot
Environment="CLOUDFLARE_API_TOKEN=your_token_here"
ExecStart=/usr/local/bin/ipflow run -f /etc/ipflow/config.json
WorkingDirectory=/etc/ipflow
StandardOutput=append:/var/log/ipflow.log
StandardError=append:/var/log/ipflow.log
Restart=no

[Install]
WantedBy=multi-user.target
```

```ini
# /etc/systemd/system/ipflow.timer
[Unit]
Description=Run ipflow every 5 minutes

[Timer]
OnBootSec=5min
OnUnitActiveSec=5min
Persistent=true

[Install]
WantedBy=timers.target
```

> **提示**：如需配置多个环境变量，在 `Environment=` 行添加，例如：
> ```ini
> Environment="CLOUDFLARE_API_TOKEN=xxx" "ALIYUN_ACCESS_KEY_ID=xxx" "ALIYUN_ACCESS_KEY_SECRET=xxx"
> ```

#### 3. 启用定时器

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now ipflow.timer
```

---

### cron 定时

#### 1. 创建执行脚本

```bash
#!/bin/bash
# /etc/ipflow/run-ipflow.sh

# 配置环境变量
export CLOUDFLARE_API_TOKEN="your_token_here"
export ALIYUN_ACCESS_KEY_ID="your_access_key_id"
export ALIYUN_ACCESS_KEY_SECRET="your_access_key_secret"

# 执行 ipflow
/usr/local/bin/ipflow run -f /etc/ipflow/config.json
```

设置权限：
```bash
sudo chmod +x /etc/ipflow/run-ipflow.sh
```

#### 2. 配置 crontab

```bash
sudo crontab -e
```

添加以下内容（每 5 分钟执行一次）：

```cron
*/5 * * * * /etc/ipflow/run-ipflow.sh >> /var/log/ipflow.log 2>&1
```

> **提示**：根据实际需求调整执行频率，参考下方的时间格式说明。

#### Crontab 时间格式说明

```
# ┌───────────── 分钟 (0 - 59)
# │ ┌───────────── 小时 (0 - 23)
# │ │ ┌───────────── 日期 (1 - 31)
# │ │ │ ┌───────────── 月份 (1 - 12)
# │ │ │ │ ┌───────────── 星期几 (0 - 7) (星期日=0 或 7)
# │ │ │ │ │
# * * * * * 命令
```

**常用配置示例：**
- `*/5 * * * *` - 每 5 分钟
- `*/10 * * * *` - 每 10 分钟
- `0 * * * *` - 每小时整点
- `0 */2 * * *` - 每 2 小时
- `0 0 * * *` - 每天午夜
- `0 0 * * 0` - 每周日凌晨
- `@reboot` - 系统启动时执行

> **注意**：cron 环境与交互式 shell 不同，建议使用绝对路径，并确保环境变量文件权限正确（`chmod 600`）。

---

### macOS launchd 定时

macOS 使用 `launchd` 管理后台任务，支持开机启动和定时执行。

#### 1. 创建配置文件

```bash
sudo mkdir -p /etc/ipflow
sudo nano /etc/ipflow/config.json
```

#### 2. 创建 launchd plist

创建 `/Library/LaunchDaemons/com.ipflow.plist`：

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.ipflow</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/ipflow</string>
        <string>run</string>
        <string>-f</string>
        <string>/etc/ipflow/config.json</string>
    </array>
    <key>EnvironmentVariables</key>
    <dict>
        <key>CLOUDFLARE_API_TOKEN</key>
        <string>your_token_here</string>
        <key>ALIYUN_ACCESS_KEY_ID</key>
        <string>your_access_key_id</string>
        <key>ALIYUN_ACCESS_KEY_SECRET</key>
        <string>your_access_key_secret</string>
    </dict>
    <key>StartInterval</key>
    <integer>300</integer>  <!-- 每 300 秒（5 分钟）执行一次 -->
    <key>RunAtLoad</key>
    <true/>  <!-- 加载时立即执行一次 -->
    <key>StandardOutPath</key>
    <string>/var/log/ipflow.log</string>
    <key>StandardErrorPath</key>
    <string>/var/log/ipflow.log</string>
</dict>
</plist>
```

#### 3. 加载并启动

```bash
# 设置正确权限
sudo chown root:wheel /Library/LaunchDaemons/com.ipflow.plist
sudo chmod 644 /Library/LaunchDaemons/com.ipflow.plist

# 加载服务
sudo launchctl load /Library/LaunchDaemons/com.ipflow.plist

# 验证服务状态
sudo launchctl list | grep ipflow
```

#### 4. 管理服务

```bash
# 停止服务
sudo launchctl unload /Library/LaunchDaemons/com.ipflow.plist

# 重新加载（修改配置后）
sudo launchctl unload /Library/LaunchDaemons/com.ipflow.plist
sudo launchctl load /Library/LaunchDaemons/com.ipflow.plist

# 查看日志
tail -f /var/log/ipflow.log
```

> **提示**：`StartInterval` 单位为秒，300 表示每 5 分钟执行一次。也可使用 `StartCalendarInterval` 实现类似 cron 的定时表达式。

---

## 平台支持

| 平台 | 状态 | 说明 |
|------|------|------|
| Linux | ✅ | 使用 netlink 接口 |
| FreeBSD | ✅ | 使用 ioctl 接口 |
| OpenBSD | ✅ | 使用 ioctl 接口 |
| macOS | ⚠️ | **实验性支持**（使用 cgo + ioctl），作者无 Mac 设备测试，欢迎反馈 |

> **macOS 说明**：macOS 支持目前为实验性质，使用 CGO 调用系统 ioctl 获取 IPv6 地址生命周期。理论上与 FreeBSD 兼容，但未经充分测试。如遇到问题欢迎提交 Issue。

---

## 项目结构

```
ipflow/
├── cmd/ipflow/           # 主程序入口
│   ├── main.go
│   └── cmd.go
├── internal/
│   ├── config/           # 配置管理
│   │   ├── config.go
│   │   └── config_test.go
│   ├── log/              # 日志系统
│   │   └── log.go
│   ├── platform/ifaddr/  # 平台相关网络工具
│   │   ├── linux_netlink.go
│   │   ├── freebsd_ioctl.go
│   │   ├── openbsd_ioctl.go
│   │   ├── darwin_ioctl.go  # macOS 实验性支持 (CGO)
│   │   ├── shared.go
│   │   ├── shared_test.go
│   │   └── util.go
│   └── provider/         # DNS 服务商实现
│       ├── provider.go
│       ├── factory/
│       ├── cloudflare/
│       └── aliyun/
├── config.example.json   # 配置示例
├── .env.example          # 环境变量示例
├── build.sh              # 构建脚本
└── README.md
```

---

## 常见问题

### 1. 配置文件 JSON 格式错误怎么办？

ipflow 会自动检测并提示 JSON 格式错误，包括：
- **缺少逗号** - 在对象或数组元素之间需要逗号分隔
- **多余逗号** - 最后一个元素后不应有逗号
- **引号问题** - 键和字符串值必须使用双引号 `""`
- **括号不匹配** - 检查 `{}` 和 `[]` 是否正确配对
- **使用了注释** - JSON 标准不支持注释

错误示例会显示具体的行号和列号，例如：
```
JSON 语法错误在第 7 行第 5 列
错误位置 (第 7 行):
  "records": [
        ^
```

**验证工具推荐：**
- 在线工具：https://jsonlint.com/
- 命令行：`python -m json.tool config.json`
- 命令行：`cat config.json | jq .`

### 2. 如何获取 Cloudflare API Token？

访问 [Cloudflare Dashboard](https://dash.cloudflare.com/profile/api-tokens)，创建具有 `Zone:DNS:Edit` 权限的 Token。

### 3. 如何获取阿里云 AccessKey？

访问 [阿里云 RAM 控制台](https://ram.console.aliyun.com/manage/ak) 创建 AccessKey。

### 4. Zone ID 如何获取？

- **Cloudflare**：Dashboard → Overview 右侧，或留空自动获取
- **阿里云**：不需要

### 5. 代理如何配置？

```json
{
    "general": {
        "proxy": "socks5://127.0.0.1:1080"
    },
    "records": [
        {
            "use_proxy": true
        }
    ]
}
```

> 注意：仅 Cloudflare 支持代理，阿里云不支持。

---

## 许可证

采用 **BSD 3-Clause License** - 详见 [LICENSE](LICENSE) 文件。

---

## 贡献

欢迎提交 Issue 和 Pull Request！
