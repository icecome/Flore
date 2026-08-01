# 编码规范 — RSS Reader

> 本文档定义项目的编码规范，确保代码风格一致。
> 所有 AI 和开发者应遵守以下约定。

## 通用规范

### 命名约定

| 实体 | 规范 | 示例 |
|------|------|------|
| Go 文件/目录 | `snake_case` | `reader_service.go` |
| Go 类型/函数 | `PascalCase` | `ReaderService`, `GetSources` |
| Go 私有函数 | `camelCase` | `updateSourceHealth` |
| Go 变量/常量 | `camelCase` | `db`, `defaultPort` |
| TypeScript 文件 | `PascalCase.tsx`（组件）/ `camelCase.ts`（工具） | `Sidebar.tsx`, `api.ts` |
| React 组件 | `PascalCase` | `ArticleList`, `TitleBar` |
| 前端 props/state | `camelCase` | `selectedSourceId`, `onSelectFolder` |
| CSS 变量 | `--kebab-case` | `--bg-surface`, `--text-primary` |
| 接口类型 | `PascalCase` | `Source`, `FilterRule` |
| 枚举 | `PascalCase` | 无枚举，使用联合类型 |

### 注释规范

- **Go 导出函数**：必须写 `// 函数名 说明` 注释
- **Go 非导出函数**：可选，逻辑复杂时写
- **React 组件**：文件顶部写组件用途注释
- **TODO 注释**：格式 `// TODO(用户名): 说明`
- **禁止**：无意义的注释（如 `// 获取数据` 这种显而易见的不写）

## Go 后端规范

### 代码结构

```go
// 导入按以下顺序分组，组间空行
import (
    // 1. 标准库
    "fmt"
    "os"

    // 2. 第三方库
    "github.com/gin-gonic/gin"
    "gorm.io/gorm"

    // 3. 内部库
    "github.com/rss/go-server/internal/models"
)
```

### 错误处理

```go
// 好的做法：包装错误上下文
if err != nil {
    return fmt.Errorf("failed to get sources: %w", err)
}

// 好的做法：service 层返回错误，handler 层转化为 HTTP 响应
if err := h.service.DeleteSource(id); err != nil {
    c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
    return
}

// 禁止：忽略错误（除非明确不需要处理）
// _ = doSomething()  // 仅在明确不需要处理时使用
```

### Service 层规约

- 不引入 `gin.Context` 或任何 HTTP 类型
- 参数和返回值使用纯 Go 类型
- 通过 `database.DB` 全局变量获取数据库实例
- 事务操作使用 `s.db.Transaction(func(tx *gorm.DB) error { ... })`

### Handler 层规约

- 只做：参数解析 → 调用 service → 返回 JSON
- 不做：业务逻辑、数据库操作
- 错误一律返回 `{ "error": "..." }`

## 前端规范

### 组件职责

- **App.tsx**：状态管理中心，所有数据获取和状态提升在此完成
- **Sidebar**：只负责订阅源列表的渲染和选择
- **ArticleList**：只负责文章列表的渲染
- **Reader**：只负责阅读区的渲染
- **Modal 组件**：各自管理弹窗内逻辑，通过回调通知 App.tsx 刷新数据

### 状态管理

```typescript
// 好的做法：状态在 App.tsx 集中管理
const [sources, setSources] = useState<Source[]>([]);
const [items, setItems] = useState<Item[]>([]);

// 好的做法：通过 props 传递数据和回调
<Sidebar
  sources={sources}
  folders={folders}
  onSelectSource={handleSelectSource}
/>

// 禁止：引入 Redux / Zustand 等状态管理库
```

### 样式规范

```typescript
// 好的做法：使用 CSS 变量 + 内联 style 对象
const styles: Record<string, React.CSSProperties> = {
  container: {
    background: 'var(--bg-surface)',
    color: 'var(--text-primary)',
    borderRadius: 'var(--radius-md)',
  },
};

// 好的做法：CSS 变量在 index.css 中统一定义
// :root { --bg-surface: #ffffff; --text-primary: #1a1a1a; }

// 禁止：使用 Tailwind 或 styled-components
```

### 文件组织

- 每个组件一个文件，文件名与组件名一致
- 工具函数放入 `utils/` 目录
- 类型定义放入 `types.ts`
- 全局样式放入 `index.css`

## 构建规范

1. 所有构建命令通过 `npm run` 执行
2. 构建产物不提交到版本管理
3. 添加新依赖需要评估对整体打包体积的影响
4. Go 后端构建产物输出到 `dist/` 目录