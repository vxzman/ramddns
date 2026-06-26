# Ramddns

[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.22+-00ADD8?logo=go)](https://golang.org)

轻量级 DDNS 客户端，专为 IPv6 环境设计。从本地网卡或远程 API 获取 IPv6 地址，自动更新 Cloudflare / 阿里云 DNS 的 AAAA 记录。

**单二进制，零 CGo 依赖（Linux / FreeBSD / OpenBSD）**。

## 平台支持

| 平台 | 实现方式 | CGo |
|------|----------|-----|
| Linux | netlink (`vishvananda/netlink`) | 否 |
| FreeBSD | netlink (`AF_NETLINK` + `RTM_GETADDR`) | 否 |
| OpenBSD | ioctl (`SIOCGIFALIFETIME_IN6`) | 否 |
| macOS | ioctl (CGo) | 是 |

FreeBSD 14+ 使用与 `ifconfig` 相同的 netlink 路径获取 IPv6 地址及生命周期。
OpenBSD 使用原生 `ioctl` 获取 SLAAC 地址的 `pltime` / `vltime`。

## 快速开始

### 1. 构建

```bash
./build.sh          # 开发版本
./build.sh v2.0.0   # 指定版本
```

交叉编译：

```bash
GOOS=freebsd GOARCH=amd64 go build -o build/ramddns ./cmd/ramddns
GOOS=openbsd GOARCH=amd64 go build -o build/ramddns ./cmd/ramddns
```

验证：

```bash
./build/ramddns version
```

### 2. 配置

```bash
cp config.example.json config.json
```

完整配置说明见下方 [配置](#配置) 一节。

### 3. 运行

```bash
./build/ramddns run -c config.json -d /etc/ramddns
```

## 配置

配置文件为 JSON 格式。以下示例使用 JSONC 语法讲解字段，实际使用时请复制 `config.example.json` 并填入自己的值。

```jsonc
{
  // ── env ──────────────────────────────────────────────────
  // 敏感信息集中存放，通过 $变量名 在 records 中引用。
  // 仅支持 $name 语法，不支持 ${name} 或系统环境变量。
  "env": {
    "cf_token": "your_cloudflare_api_token",
    "cf_zone": "your_cloudflare_zone_id",
    "ak_id": "your_aliyun_access_key_id",
    "ak_secret": "your_aliyun_access_key_secret"
  },

  // ── ip_source ────────────────────────────────────────────
  "ip_source": {
    "interface": "enp6s18",                       // 网卡名（与 fallback_urls 二选一，interface 优先）
    "fallback_urls": [                            // HTTP API 回退方案
      "https://ipv6.icanhazip.com",
      "https://6.ipw.cn",
      "https://v6.ipv6-test.com/api/myip.php"
    ]
  },
  "proxy": "",                                    // 全局代理 socks5:// 或 http://（仅 Cloudflare 生效）

  // ── records ──────────────────────────────────────────────
  "records": [
    {
      // 基础字段（所有服务商通用）
      "provider": "cloudflare",                   // cloudflare | aliyun
      "zone": "example.com",                      // 主域名
      "name": "www",                              // 子域名，@ 表示根域名
      "type": "AAAA",                             // DNS 记录类型，默认 AAAA
      "ttl": 180,                                 // 可选，Cloudflare 默认 180，阿里云默认 600
      "proxied": false,                           // 可选，Cloudflare CDN 代理
      "use_proxy": false,                         // 可选，是否使用全局 proxy

      // Cloudflare 专属字段
      "api_token": "$cf_token",                   // 必需，$ 引用 env
      "zone_id": "$cf_zone"                       // 可选，留空自动获取
    },
    {
      "provider": "aliyun",
      "zone": "example.cn",
      "name": "www",
      "type": "AAAA",
      "ttl": 600,
      "use_proxy": false,

      // 阿里云专属字段
      "access_key_id": "$ak_id",                  // 必需
      "access_key_secret": "$ak_secret"           // 必需
    }
  ]
}
```

### 服务商对比

| | Cloudflare | 阿里云 |
|--|-----------|--------|
| **认证** | API Token | AccessKey ID + Secret |
| **权限** | `Zone:DNS:Edit` | `AliyunDNSFullAccess` |
| **代理** | ✅ HTTP/SOCKS5 | ❌ 不支持 |
| **Zone ID** | 留空自动获取 | — |

## 命令行

```
ramddns <command> [options]
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

## 工作原理

```
┌─────────────┐     ┌──────────────┐     ┌─────────────┐
│ 获取 IPv6   │ ──▶ │ 筛选最佳地址  │ ──▶ │ 更新 DNS    │
│ 网卡 / API  │     │ 全局单播+最长 │     │ Cloudflare  │
│             │     │ 首选生命周期  │     │ 阿里云      │
└─────────────┘     └──────────────┘     └─────────────┘
                            │
                    缓存 IP 不变化
                    则跳过 API 调用
```

## 部署

### systemd（推荐）

`/etc/systemd/system/ramddns.service`：

```ini
[Unit]
Description=Ramddns DDNS Client
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
ExecStart=/usr/local/bin/ramddns run -c /etc/ramddns/config.json -d /etc/ramddns

[Install]
WantedBy=multi-user.target
```

`/etc/systemd/system/ramddns.timer`：

```ini
[Unit]
Description=Run Ramddns DDNS every 10 minutes
Requires=ramddns.service

[Timer]
OnBootSec=5min
OnUnitActiveSec=10min
Unit=ramddns.service

[Install]
WantedBy=timers.target
```

启用：

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now ramddns.timer
```

### Crontab

```bash
*/10 * * * * /usr/local/bin/ramddns run -c /etc/ramddns/config.json -d /etc/ramddns >> /var/log/ramddns.log 2>&1
```

## 故障排查

| 错误 | 解决方案 |
|------|----------|
| `无可用 IPv6 地址` | 检查网卡名称（`ip addr` 或 `ifconfig`），确保 IPv6 已启用；或改用 `urls` API 方式 |
| `Invalid API Token` | 检查 Cloudflare Token 和 `Zone:DNS:Edit` 权限 |
| `SignatureDoesNotMatch` | 检查阿里云 AccessKey，确保系统时间准确（NTP 同步） |

日志默认输出到 stdout，systemd 用户可通过 `journalctl -u ramddns.service -f` 查看。

更多故障排查见 [TROUBLESHOOTING.md](TROUBLESHOOTING.md)。

## 贡献与许可

欢迎提交 Issue 和 Pull Request。详见 [CONTRIBUTING.md](CONTRIBUTING.md)。

项目采用 **BSD-3-Clause** 许可证。

致谢：[Cobra](https://github.com/spf13/cobra)、[netlink](https://github.com/vishvananda/netlink)
