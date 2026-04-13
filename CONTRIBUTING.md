# 参与贡献

感谢你对 SubForge 的关注！以下是参与贡献的指南。

## Issue 规范

### Bug Report

提交 Bug 时请包含：

- **环境信息**：操作系统、Docker 版本（如适用）、Go 版本（如源码编译）
- **复现步骤**：从初始状态到问题出现的完整操作流程
- **期望行为**：你认为正确的行为是什么
- **实际行为**：实际发生了什么（附截图或日志）

### Feature Request

提交功能建议时请说明：

- **使用场景**：你遇到了什么问题，或想实现什么目标
- **建议方案**：你认为的解决方式（可选）
- **替代方案**：是否考虑过其他方案（可选）

## Pull Request 规范

### 流程

1. Fork 本仓库
2. 创建功能分支：`git checkout -b feature/your-feature`
3. 提交改动（见下方 Commit 规范）
4. 推送到你的 Fork：`git push origin feature/your-feature`
5. 创建 Pull Request 到 `main` 分支

### Commit 消息格式

```
<type>: <description>

[optional body]
```

Type 包括：

| Type | 说明 |
|------|------|
| `feat` | 新功能 |
| `fix` | Bug 修复 |
| `docs` | 文档变更 |
| `style` | 代码格式（不影响逻辑） |
| `refactor` | 重构（非新功能/非修复） |
| `test` | 测试相关 |
| `chore` | 构建/工具/依赖变更 |

示例：

```
feat: add node batch import from clipboard
fix: profile service rules not syncing after deletion
docs: add Docker deployment screenshot
```

### PR 描述模板

```markdown
## 变更内容

简要说明本次 PR 做了什么。

## 关联 Issue

Closes #123

## 测试

- [ ] 本地编译通过
- [ ] 相关功能已手动验证
```

## 开发环境

### 要求

- Go 1.24+
- (可选) Docker / Docker Compose

### 本地开发

```bash
# 克隆
git clone https://github.com/teacat99/SubForge.git
cd SubForge

# 编译并运行
go build -o subforge ./cmd/server/
./subforge -port 8080

# 访问 http://localhost:8080
```

前端代码内嵌在 `web/index.html`，修改后需要重新编译 Go 程序（因为使用了 `embed.FS`）。

## 代码风格

- Go 代码遵循 `gofmt` 标准格式
- 变量和函数命名使用 camelCase
- 公开的 API 方法需要简要注释
- 前端代码位于单一 HTML 文件中，保持简洁
