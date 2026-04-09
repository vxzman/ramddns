# 贡献指南

感谢你考虑为 goddns 做出贡献！本指南将帮助你了解如何参与项目开发。

## 📋 目录

- [行为准则](#行为准则)
- [开发环境设置](#开发环境设置)
- [开发流程](#开发流程)
- [代码规范](#代码规范)
- [提交规范](#提交规范)
- [测试要求](#测试要求)

## 行为准则

- 尊重他人观点，保持友好和建设性的沟通
- 对事不对人，专注于技术讨论
- 欢迎各种形式的贡献，无论大小

## 开发环境设置

### 前置要求

- Go 1.21 或更高版本
- Git
- Linux/macOS/FreeBSD 操作系统

### 克隆项目

```bash
git clone https://github.com/your-username/goddns.git
cd goddns
```

### 安装依赖

```bash
go mod download
```

### 构建项目

```bash
# 快速构建
go build -o goddns ./cmd/goddns

# 或使用构建脚本
./build.sh dev
```

### 运行测试

```bash
go test ./...
```

## 开发流程

### 1. Fork 项目

在 GitHub 上点击 Fork 按钮创建你自己的副本

### 2. 创建分支

```bash
git checkout -b feature/your-feature-name
# 或
git checkout -b fix/issue-123
```

**分支命名规范：**
- `feature/xxx` - 新功能
- `fix/xxx` - Bug 修复
- `docs/xxx` - 文档更新
- `refactor/xxx` - 代码重构
- `test/xxx` - 测试相关

### 3. 进行修改

进行修改时请遵循 [代码规范](#代码规范)

### 4. 运行测试

确保所有测试通过：

```bash
go test ./... -v
```

### 5. 提交更改

遵循 [提交规范](#提交规范)：

```bash
git add .
git commit -m "feat: add new DNS provider support"
```

### 6. 推送到 Fork

```bash
git push origin feature/your-feature-name
```

### 7. 创建 Pull Request

在 GitHub 上创建 Pull Request，并填写详细的描述。

**PR 描述模板：**

```markdown
## 变更类型
- [ ] 新功能
- [ ] Bug 修复
- [ ] 文档更新
- [ ] 代码重构
- [ ] 测试更新

## 描述
简要描述此 PR 的目的

## 相关 Issue
Fixes #123

## 测试
- [ ] 已添加单元测试
- [ ] 已进行手动测试
- [ ] 所有现有测试通过

## 检查清单
- [ ] 代码遵循项目规范
- [ ] 已更新相关文档
- [ ] 无新的 lint 警告
```

## 代码规范

### Go 代码风格

遵循 [Effective Go](https://golang.org/doc/effective_go.html) 和 [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)

### 格式化

使用 `go fmt` 格式化代码：

```bash
go fmt ./...
```

### Lint

推荐使用 `golint` 或 `golangci-lint`：

```bash
golangci-lint run
```

### 命名规范

- 包名：小写，无下划线
- 变量/函数：驼峰命名（导出首字母大写）
- 常量：全大写，下划线分隔
- 接口：单方法接口用 `-er` 后缀

### 错误处理

- 始终检查错误
- 使用 `fmt.Errorf` 添加上下文：`fmt.Errorf("failed to connect: %w", err)`
- 不要忽略错误（避免使用 `_`）

### 注释

- 导出的函数/类型必须有注释
- 注释以函数名开头
- 保持注释简洁明了

```go
// GetZoneID returns the Cloudflare Zone ID for the given zone name
func (p *CloudflareProvider) GetZoneID(ctx context.Context, zoneName string) (string, error) {
    // ...
}
```

## 提交规范

遵循 [Conventional Commits](https://www.conventionalcommits.org/) 规范：

### 类型

- `feat`: 新功能
- `fix`: Bug 修复
- `docs`: 文档更新
- `style`: 代码格式（不影响功能）
- `refactor`: 代码重构
- `test`: 测试相关
- `chore`: 构建/工具/配置

### 格式

```
<type>(<scope>): <subject>

<body>

<footer>
```

### 示例

```
feat(provider): add DNSPod provider support

- Implement DNSPod API integration
- Add configuration validation
- Add unit tests

Closes #45
```

```
fix(cloudflare): handle rate limit error correctly

- Add retry logic for 429 responses
- Implement exponential backoff

Fixes #67
```

## 测试要求

### 单元测试

- 新功能必须包含单元测试
- 核心逻辑测试覆盖率应 > 80%
- 测试文件命名：`<package>_test.go`

### 测试规范

```go
func TestFunctionName(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    string
        wantErr bool
    }{
        {
            name:    "valid input",
            input:   "test",
            want:    "expected",
            wantErr: false,
        },
        {
            name:    "invalid input",
            input:   "",
            want:    "",
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // 测试逻辑
        })
    }
}
```

### 运行测试

```bash
# 运行所有测试
go test ./...

# 带覆盖率
go test ./... -cover

# 覆盖率报告
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## 文档

### README 更新

如果添加了新功能，请更新 README.md：
- 更新特性列表
- 添加配置示例
- 更新使用说明

### 代码注释

- 导出标识符必须有注释
- 复杂逻辑需要解释说明
- 避免无意义的注释

## 问题反馈

### 提交 Issue

使用 GitHub Issues 报告问题或提出建议。

**Bug 报告模板：**

```markdown
### 问题描述
简要描述问题

### 复现步骤
1. 配置内容
2. 运行命令
3. 观察到的行为

### 期望行为
描述应该发生什么

### 环境信息
- OS: Linux/macOS/FreeBSD
- Go version: 1.x
- goddns version: x.x.x

### 日志
```
相关日志输出
```
```

## 发布流程

1. 更新版本号（遵循语义化版本）
2. 更新 CHANGELOG.md
3. 创建 Git tag
4. 构建并发布

## 联系方式

- GitHub Issues: 提问和讨论
- Email: 见项目 README

---

感谢你的贡献！🎉
