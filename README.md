# ramddns

[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.22+-00ADD8?logo=go)](https://golang.org)

用 Go 写的 IPv6 DDNS 客户端。从网卡或 HTTP 接口取出当前 IPv6，更新 Cloudflare 和阿里云的 AAAA 记录。配置和 Gecko-ddns 相同。

只支持 Linux 和 FreeBSD。两个平台的网卡读取都不使用 CGo。Linux 用 `vishvananda/netlink`，FreeBSD 14+ 用与 `ifconfig` 相同的 netlink 路径。

## 构建

```bash
./build.sh
./build/ramddns version
```

交叉编译 FreeBSD：

```bash
GOOS=freebsd GOARCH=amd64 go build -o build/ramddns ./cmd/ramddns
```

## 配置

```bash
cp config.example.json config.json
```

不用的服务商整段删掉。凭证每个服务商只写一次。记录类型固定是 AAAA，不出现在配置里。

```json
{
  "interface": "vtnet0",
  "fallback_urls": [
    "https://ipv6.icanhazip.com",
    "https://6.ipw.cn"
  ],
  "proxy": "",
  "cloudflare": {
    "token": "Cloudflare API Token",
    "records": [
      {
        "name": "www.example.com",
        "zone_id": "",
        "ttl": 180,
        "proxied": false,
        "use_proxy": false
      },
      {
        "name": "example.com",
        "zone_id": "",
        "ttl": 180,
        "proxied": false,
        "use_proxy": false
      }
    ]
  },
  "aliyun": {
    "access_key_id": "AccessKey ID",
    "access_key_secret": "AccessKey Secret",
    "records": [
      {
        "name": "www.example.cn",
        "ttl": 600,
        "use_proxy": false
      }
    ]
  }
}
```

| 字段 | 含义 |
|------|------|
| `interface` | 网卡名。Linux 上例如 `eth0`，FreeBSD 上例如 `vtnet0`。和 `fallback_urls` 至少写一个 |
| `fallback_urls` | 网卡获取失败时按顺序尝试的 HTTP 地址。这些请求不走代理，也没有真实的地址生命周期 |
| `proxy` | DNS API 代理，支持 `http`、`https`、`socks5`、`socks5h`。留空表示不使用 |
| `cloudflare.token` | Cloudflare API Token，需要 `Zone:DNS:Edit` |
| `aliyun.access_key_id` / `access_key_secret` | 阿里云密钥，需要 `AliyunDNSFullAccess` |
| `records[].name` | 完整域名。`example.com` 是根域名，`www.example.com` 的主机记录是 `www` |
| `records[].zone_id` | 仅 Cloudflare。留空则查询后写入 `cache.zoneid.json`，不回写配置文件 |
| `records[].ttl` | 可选。Cloudflare 缺省 180，阿里云缺省 600 |
| `records[].proxied` | 仅 Cloudflare，是否开启橙云，默认 false |
| `records[].use_proxy` | 为 true 时，这条记录的 API 请求走全局 `proxy`。Cloudflare 和阿里云都可以 |

`example.com.cn` 这种多段公共后缀的根域名，和它下面的主机无法从名字本身区分开。根域名请写成只有两段的名字，或者把主机写在最左边，例如 `www.example.com.cn`。

选出地址时丢掉链路本地、ULA、环回和已弃用地址。剩下的全局单播里，取 preferred lifetime 最长的。生命周期用内核原始的 pltime/vltime，和 `ifconfig` 不加 `-L` 时一致。`0xffffffff` 视为永久。

IP 没变则跳过更新。缓存文件 `cache.lastip` 只保留最后一次 IP，放在配置文件旁边；`-d` 可以改目录。旧的「时间 IP」多行缓存仍能读出最后一条。

## 运行

```bash
./build/ramddns run -c config.json
./build/ramddns run -c /etc/ramddns/config.json -d /etc/ramddns
```

| 参数 | 简写 | 默认 | 说明 |
|------|------|------|------|
| `--config` | `-c` | 无 | 配置文件 |
| `--dir` | `-d` | 配置文件所在目录 | 缓存目录 |
| `--ignore` | `-i` | false | 忽略缓存，强制更新，并且不写缓存 |
| `--timeout` | `-t` | 300 | 超时（秒） |

`version` 打印版本。

### systemd

`/etc/systemd/system/ramddns.service`：

```ini
[Unit]
Description=ramddns DDNS Client
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
Description=Run ramddns every 10 minutes
Requires=ramddns.service

[Timer]
OnBootSec=5min
OnUnitActiveSec=10min
Unit=ramddns.service

[Install]
WantedBy=timers.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now ramddns.timer
```

日志在 stdout。systemd 下用 `journalctl -u ramddns.service` 查看。

## 许可

BSD-3-Clause。见 [LICENSE](LICENSE)。

依赖：[Cobra](https://github.com/spf13/cobra)、[netlink](https://github.com/vishvananda/netlink)。
