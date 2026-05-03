# Aiolos

[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.22+-00ADD8?logo=go)](https://golang.org)
[![Release](https://img.shields.io/badge/release-v2.0.0-green.svg)](https://github.com/your-org/aiolos/releases)

> **Aiolos**（埃俄罗斯）是希腊神话中的风神，象征快速与灵动。  
> 本项目名称致敬风神，寓意**快速响应网络变化，灵动更新 DNS 记录**。

Aiolos 是一个**轻量级、多平台的动态 DNS (DDNS) 客户端**，专为 IPv6 环境设计，支持多个主流 DNS 服务商。采用 Go 1.22+ 开发，具有零依赖、易部署、高并发等特性。

---

## 📑 目录

- [核心特性](#-核心特性)
- [支持的 DNS 服务商](#-支持的 dns 服务商)
- [快速开始](#-快速开始)
- [安装指南](#-安装指南)
- [配置详解](#-配置详解)
- [命令行参数](#-命令行参数)
- [使用示例](#-使用示例)
- [高级功能](#-高级功能)
- [故障排查](#-故障排查)
- [贡献与许可](#-贡献与许可)

---

## ✨ 核心特性

### 🚀 基础特性

- **多平台支持**：Linux、macOS、FreeBSD、OpenBSD
- **多 DNS 服务商**：Cloudflare、阿里云 DNS（架构易于扩展）
- **并发更新**：同时更新多条 DNS 记录，提高效率
- **IPv6 优先**：原生支持 IPv6 地址动态获取与更新
- **零外部依赖**：单一二进制文件，开箱即用

### 🔒 安全特性

- **敏感信息集中管理**：所有 API 凭证在 `environment` 字段中统一管理
- **变量引用**：支持 `$变量名` 方式引用 environment 中的值
- **日志脱敏**：自动隐藏敏感信息，防止泄露

### 🛠️ 运维友好

- **多种部署方式**：systemd、Docker、Crontab
- **缓存机制**：IP 未变化时不触发 API 更新
- **灵活日志**：支持标准输出或文件，可配置日志级别
- **代理支持**：Cloudflare 支持 HTTP/SOCKS5 代理

---

## 🌐 支持的 DNS 服务商

### Cloudflare

| 特性 | 说明 |
|------|------|
| **认证方式** | API Token |
| **所需权限** | `Zone:DNS:Edit` |
| **代理支持** | ✅ 支持 HTTP/HTTPS/SOCKS5 |
| **Zone ID** | 可自动获取或手动配置 |
| **代理模式** | 支持 Cloudflare CDN 代理（`proxied` 字段） |
| **最小 TTL** | 120 秒 |

**API Token 获取**：[https://dash.cloudflare.com/profile/api-tokens](https://dash.cloudflare.com/profile/api-tokens)

### 阿里云 DNS

| 特性 | 说明 |
|------|------|
| **认证方式** | AccessKey ID + AccessKey Secret |
| **所需权限** | `AliyunDNSFullAccess` |
| **代理支持** | ❌ 不支持 |
| **签名方式** | HMAC-SHA1 |
| **TTL 范围** | 1-86400 秒 |

**AccessKey 获取**：[https://ram.console.aliyun.com/manage/ak](https://ram.console.aliyun.com/manage/ak)

---

## 🚀 快速开始

### 1. 构建

```bash
# 构建开发版本（当前平台）
./build.sh

# 构建指定版本
./build.sh v2.0.0

# 直接构建（等同方式）
go build -o build/aiolos ./cmd/aiolos
```

构建完成后验证：

```bash
./build/aiolos version
```

### 2. 准备配置文件

复制示例配置文件：

```bash
cp config.example.json config.json
```

编辑 `config.json`，填入你的域名和凭证（详见 [配置详解](#-配置详解)）。

### 3. 运行

```bash
./aiolos run --config config.json --dir /etc/aiolos
```

或使用简写：

```bash
./aiolos run -c config.json -d /etc/aiolos
```

---

## 📦 安装指南

### systemd 部署（推荐）

适用于长期运行的服务器环境。

#### 1. 创建配置目录

```bash
sudo mkdir -p /etc/aiolos
sudo chmod 755 /etc/aiolos
```

#### 2. 创建配置文件

```bash
sudo cp config.example.json /etc/aiolos/config.json
sudo chmod 600 /etc/aiolos/config.json
```

编辑 `/etc/aiolos/config.json`，在 `environment` 字段中定义敏感信息（详见 [配置详解](#-配置详解)）：

```json
{
  "environment": {
    "cloudflare_token": "your_cloudflare_api_token_here",
    "cloudflare_zone_id": "your_zone_id_here",
    "aliyun_key_id": "your_access_key_id",
    "aliyun_key_secret": "your_access_key_secret"
  },
  "records": [
    {
      "provider": "cloudflare",
      "zone": "example.com",
      "record": "www",
      "cloudflare": {
        "api_token": "$cloudflare_token",
        "zone_id": "$cloudflare_zone_id"
      }
    }
  ]
}
```

> 💡 **提示**：你也可以使用 `.env` 文件配合 systemd 的 `EnvironmentFile` 来管理系统级环境变量，然后在配置文件中通过 `${ENV_VAR}` 语法引用。但这需要自行修改配置文件格式。推荐直接在 `environment` 字段中管理所有敏感信息。

#### 3. 创建 systemd 服务文件

创建 `/etc/systemd/system/aiolos.service`：

```ini
[Unit]
Description=Aiolos DDNS Client
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
ExecStart=/usr/local/bin/aiolos run --config /etc/aiolos/config.json --dir /etc/aiolos

[Install]
WantedBy=multi-user.target
```

#### 4. 创建 systemd 定时器

创建 `/etc/systemd/system/aiolos.timer`：

```ini
[Unit]
Description=Run Aiolos DDNS every 10 minutes
Requires=aiolos.service

[Timer]
OnBootSec=5min
OnUnitActiveSec=10min
Unit=aiolos.service

[Install]
WantedBy=timers.target
```

#### 5. 启用并启动

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now aiolos.timer
```

#### 6. 验证状态

```bash
# 查看定时器状态
systemctl status aiolos.timer

# 查看下次执行时间
systemctl list-timers aiolos.timer

# 手动触发一次
systemctl start aiolos.service

# 查看日志
journalctl -u aiolos.service -f
```

---

### Docker 部署

#### Dockerfile 示例

```dockerfile
FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY . .
RUN go build -o aiolos ./cmd/aiolos

FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/
COPY --from=builder /app/aiolos .
COPY --from=builder /app/config.json .

# 创建数据目录
RUN mkdir -p /data

CMD ["./aiolos", "run", "-c", "config.json", "-d", "/data"]
```

#### docker-compose.yml 示例

```yaml
version: '3.8'

services:
  aiolos:
    build: .
    volumes:
      - ./config.json:/root/config.json:ro
      - aiolos_data:/data
    restart: unless-stopped

volumes:
  aiolos_data:
```

> 💡 **提示**：所有敏感信息应在 `config.json` 的 `environment` 字段中定义，无需通过 Docker 环境变量传递。

运行：

```bash
docker-compose up -d
```

---

### Crontab 部署

适用于简单场景或临时测试。

#### 1. 编辑 crontab

```bash
crontab -e
```

#### 2. 添加定时任务

```bash
# 每 10 分钟执行一次
*/10 * * * * /usr/local/bin/aiolos run -c /etc/aiolos/config.json -d /etc/aiolos
```

#### 3. 查看执行日志

```bash
# 查看 cron 日志
sudo grep CRON /var/log/syslog

# 或重定向输出到日志文件
*/10 * * * * /usr/local/bin/aiolos run -c /etc/aiolos/config.json -d /etc/aiolos >> /var/log/aiolos.log 2>&1
```

---

## 📝 配置详解

### 配置文件结构

```json
{
  "environment": {
    // 环境变量和敏感值集中存放
  },
  "general": {
    // 全局配置
  },
  "records": [
    // DNS 记录列表
  ]
}
```

### 完整配置示例

```json
{
  "environment": {
    "cloudflare_var": "your_cloudflare_api_token",
    "cloudflare_zone_id": "your_cloudflare_zone_id",
    "aliyun_access_key_id": "your_aliyun_access_key_id",
    "aliyun_access_key_secret": "your_aliyun_access_key_secret"
  },
  "general": {
    "get_ip": {
      "interface": "enp6s18",
      "urls": [
        "https://ipv6.icanhazip.com",
        "https://6.ipw.cn",
        "https://v6.ipv6-test.com/api/myip.php"
      ]
    },
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
        "api_token": "$cloudflare_var",
        "zone_id": "$cloudflare_zone_id"
      }
    },
    {
      "provider": "aliyun",
      "zone": "example.cn",
      "record": "www",
      "ttl": 600,
      "use_proxy": false,
      "aliyun": {
        "access_key_id": "$aliyun_access_key_id",
        "access_key_secret": "$aliyun_access_key_secret"
      }
    }
  ]
}
```

### 字段说明

#### 顶层字段

| 字段 | 类型 | 必需 | 描述 |
|------|------|------|------|
| `environment` | `map[string]string` | 否 | 环境变量和敏感值集中存放，可在配置中引用 |
| `general` | `GeneralConfig` | 是 | 全局配置 |
| `records` | `[]RecordConfig` | 是 | DNS 记录列表（至少一条） |

#### general 字段

| 字段 | 类型 | 必需 | 描述 |
|------|------|------|------|
| `get_ip.interface` | `string` | 条件 | 网卡名称（与 `urls` 二选一） |
| `get_ip.urls` | `[]string` | 条件 | IP 获取 API URLs（与 `interface` 二选一） |
| `proxy` | `string` | 否 | 全局代理 URL（`socks5://` 或 `http://`） |

#### records 字段（每条记录）

| 字段 | 类型 | 必需 | 描述 |
|------|------|------|------|
| `provider` | `string` | 是 | 服务商：`cloudflare` 或 `aliyun` |
| `zone` | `string` | 是 | 主域名（如 `example.com`） |
| `record` | `string` | 是 | 子域名记录（如 `www`、`@`、`dev`） |
| `ttl` | `int` | 否 | TTL 值（秒） |
| `proxied` | `bool` | 否 | Cloudflare 代理模式（仅 Cloudflare） |
| `use_proxy` | `bool` | 否 | 是否使用全局代理（仅 Cloudflare 支持） |
| `cloudflare` | `object` | 条件 | Cloudflare 特定配置（`provider=cloudflare` 时必需） |
| `aliyun` | `object` | 条件 | 阿里云特定配置（`provider=aliyun` 时必需） |

#### Cloudflare 特定配置

| 字段 | 类型 | 必需 | 描述 |
|------|------|------|------|
| `api_token` | `string` | 是 | Cloudflare API Token（可使用 `$` 引用 environment） |
| `zone_id` | `string` | 否 | Zone ID（留空自动获取） |

#### 阿里云特定配置

| 字段 | 类型 | 必需 | 描述 |
|------|------|------|------|
| `access_key_id` | `string` | 是 | 阿里云 AccessKey ID |
| `access_key_secret` | `string` | 是 | 阿里云 AccessKey Secret |

---

### 变量引用语法

Aiolos 仅支持引用配置文件顶层 `environment` 中定义的变量。

**语法**：使用 `$变量名` 方式引用

```json
{
  "environment": {
    "cloudflare_var": "your_api_token",
    "aliyun_key": "your_access_key"
  },
  "records": [
    {
      "provider": "cloudflare",
      "zone": "example.com",
      "record": "www",
      "cloudflare": {
        "api_token": "$cloudflare_var",
        "zone_id": "$cloudflare_zone_id"
      }
    }
  ]
}
```

**注意**：
- 引用格式：`$变量名`（例如：`$cloudflare_var`）
- 如果引用的变量在 `environment` 中不存在，程序将报错退出
- 不支持系统环境变量引用（如 `${ENV_VAR}`）
- 不支持变量默认值语法

---

## 🖥️ 命令行参数

### 命令结构

```bash
aiolos <command> [options]
```

### 可用命令

| 命令 | 描述 |
|------|------|
| `run` | 运行 DDNS 更新 |
| `version` | 显示版本信息 |

### `run` 命令参数

| 参数 | 简写 | 类型 | 默认值 | 描述 |
|------|------|------|--------|------|
| `--config` | `-c` | `string` | 无 | 配置文件路径（JSON 格式） |
| `--dir` | `-d` | `string` | 无 | 工作目录（存放缓存、密钥、相对日志路径） |
| `--ignore-cache` | `-i` | `bool` | `false` | 忽略缓存 IP，强制更新 |
| `--timeout` | `-t` | `int` | `300` | 超时时间（秒） |

### `version` 命令

显示版本、提交哈希和构建日期：

```bash
aiolos version
```

输出示例：

```
Aiolos v2.0.0
Commit: abc123def456
Build Date: 2024-01-01T00:00:00Z
```

---

## 💡 使用示例

### 基本用法

```bash
# 指定配置文件和工作目录
aiolos run --config /etc/aiolos/config.json --dir /etc/aiolos

# 简写
aiolos run -c /etc/aiolos/config.json -d /etc/aiolos
```

### 仅指定工作目录

自动在工作目录下查找 `config.json`：

```bash
aiolos run -d /etc/aiolos
```

### 强制更新（忽略缓存）

```bash
aiolos run -c config.json -i
```

### 使用自定义超时时间

```bash
aiolos run -c config.json -d /etc/aiolos -t 600
```

### 调试模式

程序默认输出 DEBUG 级别日志到标准输出，systemd 或 cron 会将其重定向到对应日志。

> 💡 **提示**：DEBUG 级别日志会在运行时自动输出详细调试信息。

---

## 🔧 高级功能

### IP 获取策略

#### 网卡方式（优先）

直接从指定网卡获取 IPv6 地址：

```json
{
  "general": {
    "get_ip": {
      "interface": "enp6s18"
    }
  }
}
```

#### API 方式（备用）

通过多个公共 API 并发查询：

```json
{
  "general": {
    "get_ip": {
      "urls": [
        "https://ipv6.icanhazip.com",
        "https://6.ipw.cn",
        "https://v6.ipv6-test.com/api/myip.php"
      ]
    }
  }
}
```

#### 混合模式

同时指定网卡和 API，优先使用网卡：

```json
{
  "general": {
    "get_ip": {
      "interface": "enp6s18",
      "urls": [
        "https://ipv6.icanhazip.com"
      ]
    }
  }
}
```

### IP 过滤规则

自动过滤以下类型的 IPv6 地址：

- ❌ 链路本地地址（`fe80::/10`）
- ❌ 唯一本地地址（ULA, `fc00::/7`, `fd00::/8`）
- ❌ 环回地址（`::1`）
- ❌ 已过期的地址（`PreferredLft = 0`）

### 缓存机制

- **缓存文件**：`{work_dir}/cache.lastip`
- **作用**：避免 IP 未变化时频繁调用 API
- **跳过缓存**：使用 `-i` 或 `--ignore-cache` 参数

### 日志系统

#### 日志级别

| 级别 | 描述 |
|------|------|
| `DEBUG` | 调试信息（包含详细 API 请求） |
| `INFO` | 一般信息 |
| `WARNING` | 警告信息 |
| `ERROR` | 错误信息 |
| `FATAL` | 致命错误（程序退出） |
| `SUCCESS` | 成功信息 |

#### 敏感信息脱敏

日志中自动隐藏敏感信息：

```
[INFO] 使用 API Token: ***REDACTED***
[INFO] 使用 AccessKey: ***REDACTED***
```

### 代理支持

#### 全局代理配置

```json
{
  "general": {
    "proxy": "socks5://127.0.0.1:1080"
  }
}
```

或 HTTP 代理：

```json
{
  "general": {
    "proxy": "http://127.0.0.1:8080"
  }
}
```

#### 单条记录代理配置

```json
{
  "records": [
    {
      "provider": "cloudflare",
      "use_proxy": true
    }
  ]
}
```

> ⚠️ **注意**：仅 Cloudflare 支持代理，阿里云不支持。

### 并发更新

所有 DNS 记录并发更新，提高多域名场景下的更新效率：

```json
{
  "records": [
    { "provider": "cloudflare", "zone": "example.com", "record": "www" },
    { "provider": "cloudflare", "zone": "example.com", "record": "api" },
    { "provider": "aliyun", "zone": "example.cn", "record": "blog" }
  ]
}
```

三条记录将同时更新，而非串行执行。

---

## 🐛 故障排查

### 常见问题

#### 1. IP 获取失败

**错误信息**：
```
[ERROR] 无法获取 IPv6 地址
```

**解决方案**：
- 检查网卡名称是否正确（`ip addr` 查看）
- 确保系统已启用 IPv6
- 尝试使用 API 方式获取（配置 `urls` 字段）

#### 2. Cloudflare API 权限不足

**错误信息**：
```
[ERROR] Cloudflare API 错误：Invalid API Token
```

**解决方案**：
- 检查 API Token 是否正确
- 确保 Token 具有 `Zone:DNS:Edit` 权限
- 检查 Zone ID 是否正确

#### 3. 阿里云签名失败

**错误信息**：
```
[ERROR] 阿里云 API 错误：SignatureDoesNotMatch
```

**解决方案**：
- 检查 AccessKey ID 和 Secret 是否正确
- 确保系统时间准确（NTP 同步）
- 检查账号是否有 `AliyunDNSFullAccess` 权限

### 日志级别调整

程序默认输出 DEBUG 级别日志。DEBUG 级别会自动输出完整请求信息。

### 查看 systemd 日志

```bash
# 查看最近 100 行日志
journalctl -u aiolos.service -n 100

# 实时查看日志
journalctl -u aiolos.service -f

# 查看特定时间段日志
journalctl -u aiolos.service --since "2024-01-01 00:00:00" --until "2024-01-01 23:59:59"
```

### 详细故障排查指南

更多故障排查信息请参考 [TROUBLESHOOTING.md](TROUBLESHOOTING.md)。

---

## 🤝 贡献与许可

### 贡献指南

欢迎提交 Issue 与 Pull Request！

- 🐛 **报告 Bug**：[创建 Issue](https://github.com/your-org/aiolos/issues)
- 💡 **功能建议**：[创建 Issue](https://github.com/your-org/aiolos/issues)
- 🔧 **提交代码**：[创建 Pull Request](https://github.com/your-org/aiolos/pulls)

详细贡献指南请参考 [CONTRIBUTING.md](CONTRIBUTING.md)。

### 许可证

本项目采用 **BSD-3-Clause** 许可证。详见 [LICENSE](LICENSE) 文件。

### 致谢

感谢以下开源项目：

- [Cobra](https://github.com/spf13/cobra) - 命令行框架
- [netlink](https://github.com/vishvananda/netlink) - Linux 网络接口操作

---

## 📞 联系方式

- **项目主页**：[https://github.com/your-org/aiolos](https://github.com/your-org/aiolos)
- **问题反馈**：[https://github.com/your-org/aiolos/issues](https://github.com/your-org/aiolos/issues)

---

**Made with ❤️ by the Aiolos Team**
