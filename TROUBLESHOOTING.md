# 故障排查指南

本文档提供 goddns 运行过程中常见问题的解决方案。

## 📋 目录

- [启动问题](#启动问题)
- [IP 获取问题](#ip-获取问题)
- [Cloudflare 问题](#cloudflare-问题)
- [阿里云 DNS 问题](#阿里云 dns-问题)
- [网络/代理问题](#网络代理问题)
- [性能问题](#性能问题)
- [日志相关](#日志相关)

---

## 启动问题

### 问题：`Failed to load configuration`

**可能原因：**
1. 配置文件路径错误
2. 配置文件格式不正确
3. 配置文件权限问题

**解决方案：**

```bash
# 检查文件是否存在
ls -la /path/to/config.json

# 验证 JSON 格式
jq . /path/to/config.json

# 使用绝对路径
./goddns run -f /etc/goddns/config.json
```

### 问题：`Invalid config: xxx`

**常见错误及解决：**

| 错误信息 | 解决方案 |
|---------|---------|
| `at least one record must be configured` | 在 `records` 数组中至少添加一个 DNS 记录配置 |
| `either 'get_ip.interface' or 'get_ip.urls' must be configured` | 配置网卡名或 API URLs |
| `cloudflare.api_token is required` | 添加 Cloudflare API Token |
| `aliyun access_key_id and access_key_secret are required` | 添加阿里云凭证 |
| `invalid global proxy` | 检查代理 URL 格式，必须包含 scheme (如 `socks5://`) |

### 问题：`Failed to initialize logging`

日志默认输出到标准输出，由 systemd 或 cron 管理。如遇到此错误，通常是系统环境问题，请检查程序是否有权限写入指定路径。

---

## IP 获取问题

### 问题：`no suitable DDNS Candidate found`

**可能原因：**
1. 网卡名称错误
2. 网卡没有 IPv6 地址
3. IPv6 地址已过期或被废弃

**解决方案：**

```bash
# 查看可用网卡
ip -6 addr show

# 检查网卡名称是否正确
ip link show

# 使用 API 方式获取（不依赖网卡）
# 修改配置：
"get_ip": {
    "urls": [
        "https://ipv6.icanhazip.com",
        "https://6.ipw.cn"
    ]
}
```

### 问题：`All APIs failed`

**可能原因：**
- 网络连接问题
- API 服务不可用
- 防火墙阻止访问

**解决方案：**

```bash
# 测试 API 可达性
curl -6 https://ipv6.icanhazip.com
curl -6 https://6.ipw.cn

# 检查 IPv6 连接
ping6 ipv6.google.com

# 添加更多备用 API
"get_ip": {
    "urls": [
        "https://ipv6.icanhazip.com",
        "https://6.ipw.cn",
        "https://v6.ipv6-test.com/api/myip.php",
        "https://api64.ipify.org"
    ]
}
```

### 问题：获取到错误的 IPv6 地址（链路本地/ULA）

**说明：** goddns 会自动过滤以下地址：
- 链路本地地址 (fe80::/10)
- 唯一本地地址 (fc00::/7, fd00::/8)
- 环回地址 (::1)
- 已过期的地址

**解决方案：**
- 确保网卡有全局单播 IPv6 地址
- 检查路由器 RA 配置
- 联系 ISP 确认 IPv6 服务状态

---

## Cloudflare 问题

### 问题：`failed to get Zone ID: unknown error`

**可能原因：**
1. API Token 权限不足
2. Zone 名称错误
3. API Token 已失效

**解决方案：**

```bash
# 验证 API Token 权限
# 需要以下权限：
# - Zone:DNS:Edit

# 手动测试 API
curl -X GET "https://api.cloudflare.com/client/v4/zones?name=example.com" \
  -H "Authorization: Bearer YOUR_API_TOKEN" \
  -H "Content-Type: application/json"

# 检查 Zone 名称是否正确（不包含子域名）
# 正确：example.com
# 错误：www.example.com
```

### 问题：`Cloudflare API POST failed (Code 1004)`

**错误代码含义：**

| 代码 | 含义 | 解决方案 |
|------|------|---------|
| 1004 | Authentication error | 检查 API Token 是否正确 |
| 1009 | Account not found | 确认 Token 有权限访问该 Zone |
| 1043 | Invalid API key | Token 已失效，重新生成 |

### 问题：记录更新成功但 DNS 未生效

**可能原因：**
- DNS 缓存未刷新
- TTL 设置过长
- Cloudflare 代理模式影响

**解决方案：**

```json
{
    "records": [
        {
            "provider": "cloudflare",
            "zone": "example.com",
            "record": "www",
            "ttl": 120,
            "proxied": false
        }
    ]
}
```

- 降低 TTL 值（最小 120 秒）
- 设置 `proxied: false` 禁用 Cloudflare 代理
- 等待 DNS 传播（通常 5-10 分钟）

---

## 阿里云 DNS 问题

### 问题：`API error: InvalidAccessKeyId`

**可能原因：**
- AccessKey ID 错误
- AccessKey 已被禁用或删除

**解决方案：**

1. 登录阿里云控制台
2. 访问 RAM 访问控制
3. 检查 AccessKey 状态
4. 重新生成 AccessKey

### 问题：`API error: Forbidden.Ram`

**可能原因：**
- RAM 用户权限不足
- 缺少 AliyunDNSFullAccess 权限

**解决方案：**

1. 登录 RAM 控制台
2. 找到对应的 RAM 用户
3. 添加权限：`AliyunDNSFullAccess`

### 问题：`SignatureNonce already used`

**可能原因：**
- 系统时间不准确
- 重复请求

**解决方案：**

```bash
# 同步系统时间
sudo ntpdate ntp.aliyun.com

# 或启用 NTP 服务
sudo systemctl enable --now chronyd
```

---

## 网络/代理问题

### 问题：使用代理后连接失败

**支持情况：**
- ✅ Cloudflare: 支持 HTTP/HTTPS/SOCKS5 代理
- ❌ 阿里云：不支持代理

**解决方案：**

```json
{
    "general": {
        "proxy": "socks5://127.0.0.1:1080"
    },
    "records": [
        {
            "provider": "cloudflare",
            "use_proxy": true
        },
        {
            "provider": "aliyun",
            "use_proxy": false
        }
    ]
}
```

### 问题：`unsupported proxy scheme`

**支持的代理格式：**

```
socks5://host:port
socks5h://host:port  (远程 DNS 解析)
http://host:port
https://host:port
```

**错误示例：**
```
127.0.0.1:1080      # 缺少 scheme
ftp://proxy:8080    # 不支持的协议
```

---

## 性能问题

### 问题：更新速度慢

**可能原因：**
- 网络延迟高
- API 限流
- 重试次数过多

**优化建议：**

```json
{
    "general": {
        "get_ip": {
            "urls": [
                "https://ipv6.icanhazip.com",
                "https://6.ipw.cn"
            ]
        }
    }
}
```

- 添加多个备用 API（并发请求，取第一个成功）
- 配置缓存避免频繁更新
- 使用本地网卡方式获取 IP（更快）

### 问题：频繁触发 API 限流

**解决方案：**

1. 增加检查间隔（systemd timer 或 cron）
2. 利用 IP 缓存机制
3. 增加 TTL 减少更新频率

```ini
# systemd timer 配置
[Timer]
OnBootSec=5min
OnUnitActiveSec=10min  # 增加间隔
```

---

## 日志相关

### 问题：日志输出太多

**解决方案：**

通过 systemd 或 cron 将输出重定向到文件，或调整日志级别过滤（需要代码方式设置）：

```go
import "aiolos/internal/log"

// 设置为只输出 WARNING 及以上级别
log.SetLevel(log.WarningLevel)
```

### 问题：需要调试信息

**解决方案：**

```go
// 设置 DEBUG 级别
log.SetLevel(log.DebugLevel)
```

查看 DEBUG 日志：
- 网络请求详情
- 配置加载过程
- IP 选择逻辑

### 问题：日志中出现敏感信息

**说明：** goddns 会自动脱敏以下信息：
- API Token
- AccessKey ID
- Secret Key

如果发现有其他敏感信息泄露，请提交 Issue 报告。

---

## 常见问题 FAQ

### Q: 支持 IPv4 吗？

A: 当前版本仅支持 IPv6。IPv4 支持已在计划中，欢迎提交 PR。

### Q: 可以同时更新多个域名吗？

A: 可以！在 `records` 数组中配置多个记录即可：

```json
{
    "records": [
        {
            "provider": "cloudflare",
            "zone": "example.com",
            "record": "www"
        },
        {
            "provider": "cloudflare",
            "zone": "example.com",
            "record": "api"
        },
        {
            "provider": "aliyun",
            "zone": "example.cn",
            "record": "dev"
        }
    ]
}
```

### Q: 如何查看当前版本？

```bash
./goddns version
```

### Q: 缓存文件在哪里？

- 如果配置了 `work_dir`：`{work_dir}/cache.lastip`
- 否则：与配置文件同目录下的 `cache.lastip`

### Q: 如何强制更新（忽略缓存）？

```bash
./goddns run -f config.json -i
```

---

## 获取帮助

如果以上方案无法解决您的问题：

1. **检查日志**：查看完整错误信息
2. **搜索 Issue**：可能已有类似问题
3. **提交 Issue**：提供详细信息和日志

### 提交 Issue 时请提供：

```markdown
- goddns 版本：
- 操作系统：
- 网络环境（IPv6 类型）：
- DNS 服务商：
- 配置文件（脱敏后）：
- 完整日志输出：
```

---

## 附录：健康检查脚本

```bash
#!/bin/bash
# goddns-health-check.sh

echo "=== goddns 健康检查 ==="

# 检查进程
if pgrep -x "goddns" > /dev/null; then
    echo "✓ 进程运行中"
else
    echo "✗ 进程未运行"
fi

# 检查 IPv6 连接
if ping6 -c 1 ipv6.google.com > /dev/null 2>&1; then
    echo "✓ IPv6 连接正常"
else
    echo "✗ IPv6 连接失败"
fi

# 检查 API
if curl -6 -s https://ipv6.icanhazip.com > /dev/null; then
    echo "✓ IP API 可达"
else
    echo "✗ IP API 不可达"
fi

# 检查配置文件
if [ -f "/etc/goddns/config.json" ]; then
    if jq . /etc/goddns/config.json > /dev/null 2>&1; then
        echo "✓ 配置文件有效"
    else
        echo "✗ 配置文件格式错误"
    fi
else
    echo "✗ 配置文件不存在"
fi

echo "=== 检查完成 ==="
```
