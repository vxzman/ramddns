# Aiolos

[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.22+-00ADD8?logo=go)](https://golang.org)

> **Aiolos**（埃俄罗斯）是希腊神话中的风神，象征快速与灵动。
> 本项目名称致敬风神，寓意**快速响应网络变化，灵动更新 DNS 记录**。

Aiolos 是一个轻量级 DDNS 客户端，专为 IPv6 环境设计，支持 Cloudflare 和阿里云 DNS。零外部依赖，单一二进制文件。

## 特性

- **多平台**：Linux、macOS、FreeBSD、OpenBSD
- **多服务商**：Cloudflare、阿里云 DNS（架构易扩展）
- **IPv6 优先**：支持从网卡或 HTTP API 获取 IPv6 地址，自动过滤链路本地/ULA/环回地址
- **并发更新**：多条 DNS 记录同时更新
- **缓存机制**：IP 未变化时不触发 API 调用
- **敏感信息管理**：`environment` 集中定义，通过 `$变量名` 引用，日志自动脱敏
- **代理支持**：Cloudflare 支持 HTTP/SOCKS5 代理（仅 Cloudflare）

## 快速开始

### 1. 构建

```bash
./build.sh          # 开发版本
./build.sh v2.0.0   # 指定版本
```

验证：

```bash
./build/aiolos version
```

### 2. 配置

```bash
cp config.example.json config.json
```

最小配置示例（Cloudflare）：

```json
{
  "environment": {
    "cf_token": "your_cloudflare_api_token"
  },
  "general": {
    "get_ip": {
      "interface": "enp6s18"
    }
  },
  "records": [
    {
      "provider": "cloudflare",
      "zone": "example.com",
      "record": "www",
      "cloudflare": {
        "api_token": "$cf_token"
      }
    }
  ]
}
```

### 3. 运行

```bash
./build/aiolos run -c config.json -d /etc/aiolos
```

## 配置

### 结构

```json
{
  "environment": { "变量名": "值" },
  "general": { "全局设置" },
  "records": [ { "DNS记录配置" } ]
}
```

### general 字段

| 字段 | 类型 | 说明 |
|------|------|------|
| `get_ip.interface` | string | 网卡名称（与 `urls` 二选一） |
| `get_ip.urls` | []string | HTTP API 地址列表（与 `interface` 二选一） |
| `proxy` | string | 全局代理 URL（`socks5://` 或 `http://`） |

### records 字段（每条记录）

| 字段 | 类型 | 说明 |
|------|------|------|
| `provider` | string | `cloudflare` 或 `aliyun` |
| `zone` | string | 主域名（如 `example.com`） |
| `record` | string | 子域名（如 `www`、`@`） |
| `ttl` | int | TTL（秒），Cloudflare 默认 180，阿里云默认 600 |
| `proxied` | bool | Cloudflare CDN 代理（仅 Cloudflare） |
| `use_proxy` | bool | 使用全局代理（仅 Cloudflare） |
| `cloudflare` | object | Cloudflare 配置（见下表） |
| `aliyun` | object | 阿里云配置（见下表） |

### 服务商特定配置

| Cloudflare | 阿里云 |
|------------|--------|
| `api_token`（必需）— API Token | `access_key_id`（必需）— AccessKey ID |
| `zone_id`（可选）— 留空自动获取 | `access_key_secret`（必需）— AccessKey Secret |
| 需要 `Zone:DNS:Edit` 权限 | 需要 `AliyunDNSFullAccess` 权限 |

### 变量引用

在 `environment` 中定义敏感值，通过 `$变量名` 引用：

```json
{
  "environment": { "cf_token": "xxx" },
  "records": [{
    "cloudflare": { "api_token": "$cf_token" }
  }]
}
```

- 仅支持 `$变量名` 格式
- 引用的变量必须在 `environment` 中存在

## 命令行

```
aiolos <command> [options]
```

| 命令 | 说明 |
|------|------|
| `run` | 执行 DDNS 更新 |
| `version` | 显示版本信息 |

`run` 命令参数：

| 参数 | 简写 | 默认值 | 说明 |
|------|------|--------|------|
| `--config` | `-c` | 无 | 配置文件路径 |
| `--dir` | `-d` | 无 | 工作目录（存放缓存文件 `cache.lastip`） |
| `--ignore-cache` | `-i` | false | 忽略缓存，强制更新 |
| `--timeout` | `-t` | 300 | 超时时间（秒） |

## 部署

### systemd（推荐）

`/etc/systemd/system/aiolos.service`：

```ini
[Unit]
Description=Aiolos DDNS Client
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
ExecStart=/usr/local/bin/aiolos run -c /etc/aiolos/config.json -d /etc/aiolos

[Install]
WantedBy=multi-user.target
```

`/etc/systemd/system/aiolos.timer`：

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

启用：

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now aiolos.timer
```

### Crontab

```bash
*/10 * * * * /usr/local/bin/aiolos run -c /etc/aiolos/config.json -d /etc/aiolos >> /var/log/aiolos.log 2>&1
```

### Docker

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

## 故障排查

| 错误 | 解决方案 |
|------|----------|
| `无法获取 IPv6 地址` | 检查网卡名称（`ip addr`），确保 IPv6 已启用，或改用 `urls` API 方式 |
| `Invalid API Token` | 检查 Token 和 `Zone:DNS:Edit` 权限 |
| `SignatureDoesNotMatch` | 检查 AccessKey，确保系统时间准确（NTP 同步） |

日志默认输出到 stdout，systemd 用户可通过 `journalctl -u aiolos.service -f` 查看。

更多故障排查见 [TROUBLESHOOTING.md](TROUBLESHOOTING.md)。

## 贡献与许可

欢迎提交 Issue 和 Pull Request。详见 [CONTRIBUTING.md](CONTRIBUTING.md)。

项目采用 **BSD-3-Clause** 许可证。

致谢：[Cobra](https://github.com/spf13/cobra)、[netlink](https://github.com/vishvananda/netlink)
