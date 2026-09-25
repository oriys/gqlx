# gqlx

`gqlx` 是一个用 Go 从零实现的 **GraphQL 服务端引擎**，零第三方依赖。它实现了 GraphQL 规范的核心链路，并额外提供 **Apollo Federation v2 风格的联邦网关**与 **订阅（Subscription）** 支持。

```go
import "github.com/cyo/gqlx"
```

## 特性

- **完整内核**：词法分析 → 语法分析（AST）→ 校验 → 执行
- **类型系统**：Scalar / Object / Interface / Union / Enum / InputObject，支持自定义标量与枚举映射
- **查询与变更**：字段合并、别名、片段、内联片段、`@skip` / `@include`、变量与默认值
- **内省**：`__schema` / `__type` / `__typename`
- **规范级非空传播**：非空字段解析为 null 时，null 会向上冒泡到最近的 nullable 父字段
- **订阅**：基于 channel 或 `AsyncIterator` 的流式事件
- **并发执行**：查询字段可选用 goroutine 并行解析（mutation 始终串行）
- **联邦网关**：多 subgraph schema 合并、实体跨图解析、`@key` / `@external` / `@requires` / `@provides`、递归/环检测、深度限制
- **查询计划器（Query Planner）**：按字段归属切分 selection set，生成带依赖的 fetch 计划，支持实体批处理解析

## 快速开始

### 定义 schema 并执行查询

```go
humanType := &gqlx.ObjectType{
    Name_: "Human",
    Fields_: gqlx.FieldMap{
        "name": {Type: gqlx.StringScalar},
        "age":  {Type: gqlx.IntScalar},
    },
}

queryType := &gqlx.ObjectType{
    Name_: "Query",
    Fields_: gqlx.FieldMap{
        "hero": {
            Type: humanType,
            Resolve: func(p gqlx.ResolveParams) (interface{}, error) {
                return map[string]interface{}{"name": "Luke", "age": 25}, nil
            },
        },
    },
}

schema, err := gqlx.NewSchema(gqlx.SchemaConfig{Query: queryType})
if err != nil {
    panic(err)
}

// Do = 解析 + 校验 + 执行
result := gqlx.Do(schema, `{ hero { name age } }`, nil, "")
fmt.Println(result.Data) // map[hero:map[age:25 name:Luke]]
```

也可以分步执行：

```go
doc, err := gqlx.Parse(`{ hero { name } }`)
if err != nil { /* ... */ }
if errs := gqlx.Validate(schema, doc); len(errs) > 0 { /* ... */ }

result := gqlx.Execute(gqlx.ExecuteParams{
    Schema:     schema,
    Document:   doc,
    RootValue:  nil,
    Variables:  map[string]interface{}{},
    Concurrent: true, // 可选：并行解析同层字段
})
```

### 订阅

订阅根字段的 resolver 需要返回一个 channel 或 `gqlx.AsyncIterator`：

```go
subType := &gqlx.ObjectType{
    Name_: "Subscription",
    Fields_: gqlx.FieldMap{
        "countdown": {
            Type: gqlx.IntScalar,
            Resolve: func(p gqlx.ResolveParams) (interface{}, error) {
                ch := make(chan interface{})
                go func() {
                    defer close(ch)
                    for i := 3; i > 0; i-- {
                        ch <- i
                    }
                }()
                return ch, nil
            },
        },
    },
}

schema, _ := gqlx.NewSchema(gqlx.SchemaConfig{Query: queryType, Subscription: subType})

sub, err := gqlx.DoSubscribe(schema, `subscription { countdown }`, nil, "")
if err != nil {
    panic(err)
}
defer sub.Cancel()

for result := range sub.Results {
    fmt.Println(result.Data)
}
```

自定义事件源只需实现：

```go
type AsyncIterator interface {
    Next() (interface{}, error) // io.EOF 表示流结束
    Close() error
}
```

### 联邦网关

```go
users, _ := gqlx.NewSubgraph(gqlx.SubgraphConfig{
    Name:   "users",
    Schema: gqlx.SchemaConfig{
        Query: &gqlx.ObjectType{
            Name_: "Query",
            Fields_: gqlx.FieldMap{
                "user": {
                    Type: userType,
                    Args: gqlx.ArgumentMap{"id": {Name_: "id", Type: gqlx.NewNonNull(gqlx.IDScalar)}},
                    Resolve: resolveUser,
                },
            },
        },
    },
})

orders, _ := gqlx.NewSubgraph(gqlx.SubgraphConfig{
    Name: "orders",
    Schema: gqlx.SchemaConfig{
        Query: &gqlx.ObjectType{Name_: "Query", Fields_: gqlx.FieldMap{"noop": {Type: gqlx.StringScalar}}},
    },
    Entities: []gqlx.EntityConfig{
        {
            TypeName:  "User",
            KeyFields: []string{"id"},
            Resolver: func(repr map[string]interface{}) (interface{}, error) {
                return map[string]interface{}{"id": repr["id"], "orders": loadOrders(repr["id"])}, nil
            },
        },
    },
})

gw, _ := gqlx.NewGateway(gqlx.GatewayConfig{Subgraphs: []*gqlx.Subgraph{users, orders}})
result := gw.Execute(`{ user(id: "1") { name orders } }`, nil, "")
```

字段上的联邦元数据：

```go
&gqlx.FieldDefinition{
    Type:     gqlx.StringScalar,
    External: true,              // @external：该字段由其它 subgraph 拥有
    Requires: []string{"price"}, // @requires：解析前需要同类型的 price
    Provides: []string{"name"},  // @provides：返回类型对外提供的字段
}
```

`@requires` / `@provides` 在网关组合时会做存在性校验，阻止拼写错误。

### 查询计划器（Query Planner）

`Gateway.Execute` 采用「按需解析」（resolver 遇到跨图字段时即时调 reference resolver）。对于生产场景，可以使用查询计划器先把查询切分成一组 fetch，再执行：

```go
plan, err := gw.Plan(`{
    user(id: "1") {
        name
        orders { quantity product { name } }
    }
}`)
if err != nil { panic(err) }
fmt.Println(plan.Describe())
// Parallel
//   Fetch(service=users, selection={user})
//     Flatten(path=[user])
//       Fetch[entity User](service=orders, selection={orders})
//         Flatten(path=[orders])
//           Fetch[entity Order](service=products, selection={product})

result := gw.ExecutePlan(plan, doc, nil, "")
// 或者一步到位：
result = gw.ExecutePlanned(query, nil, "")
```

计划器的工作流程：

1. **Selection splitting**：按 `fieldOwner` 把一个 selection set 拆成「本 subgraph 可解析」与「属于其它 subgraph」两部分。
2. **实体边界**：跨 subgraph 的字段必须是 `@key` 实体；父 fetch 会**注入** `__typename` 与 key 字段用于构造 representation。
3. **Flatten**：子 fetch 挂到父结果的具体响应路径上，逐实体解析并 merge。
4. **批处理**：如果 `EntityConfig.BatchResolver` 存在，同一路径下的所有实体一次解析，避免 N+1。
5. **Response shaping**：最后按原始 operation 重新装配响应，注入的 key / `__typename` 不会泄漏。

```go
Entities: []gqlx.EntityConfig{
    {
        TypeName:  "User",
        KeyFields: []string{"id"},
        Resolver:  resolveOne,
        BatchResolver: func(reprs []map[string]interface{}) ([]interface{}, error) {
            return batchLoadUsers(reprs) // 一次批量加载
        },
    },
}
```

## 错误与结果

```go
type Result struct {
    Data   interface{}
    Errors []*GraphQLError
}
```

非空传播示例：若 `user.name` 是 `String!` 而 resolver 返回 null，则 `user` 会整体变为 null（若 `user` 可空），并附带一条 `Cannot return null for non-nullable field.` 错误；若一直冒泡到根，则 `Data` 为 null。

## 包结构

| 文件 | 职责 |
|------|------|
| `lexer.go` / `token.go` | 词法分析 |
| `parser.go` / `ast.go` | 语法分析与 AST |
| `validation.go` | 查询校验 |
| `executor.go` | 执行引擎（查询 / 变更、非空传播、内省、并发） |
| `subscription.go` | 订阅运行时（channel / AsyncIterator） |
| `planner.go` | 联邦查询计划器与计划执行器 |
| `schema.go` / `types.go` | Schema 与类型系统 |
| `values.go` / `scalars.go` | 值、变量、参数与标量 |
| `directives.go` | 内置指令 |
| `federation.go` / `gateway.go` | 联邦 subgraph 与网关 |
| `errors.go` | 错误类型与格式化 |

## 局限

- 联邦提供两条执行路径：`Gateway.Execute`（按需解析）与 `Gateway.ExecutePlanned`（查询计划器）。计划器覆盖查询/变更的 root 分组、实体切分、多级 flatten、批处理、别名/指令/片段；但**尚未**处理：`@requires` / `@provides` 在计划期的跨图预取、抽象类型（Interface/Union）跨 subgraph 的字段切分、以及订阅的计划。`Gateway.Execute` 仍可处理这些场景中的一部分。
- 没有实现 Apollo 的 `_entities` / `_service` 端点与 SDL 组合 —— 本库的 subgraph 是进程内的 Go schema，fetch 即函数调用。
- 订阅需要由调用方自行管理传输层（如 WebSocket / SSE）；本库只负责事件解析与分发。
- 未内置 HTTP 服务器与持久化查询（persisted queries）。

## 测试

```sh
go test ./...
go test -race ./...
go test -bench . -benchmem
```
