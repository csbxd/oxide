# 使用教程：直接从 Go 调用翻译后的 Mermaid 渲染器

Oxide 直接翻译上游 Rust 库及其依赖。Go 使用生成的具名类型、静态字段方法和析构入口，不需要额外的 Rust fixture 包。图表解析、布局、SVG 与 PNG 编码仍由翻译后的 Rust 实现完成。

## 1. 构建翻译器

需要 Linux amd64/arm64、Go 1.27.1、支持安全 tar 解压的 Python 和 rustc 可用的系统链接器。以下示例假定两个项目相邻：

```text
lang/
├── oxide/
└── mermaid-rs-renderer/
    └── Cargo.toml
```

在 `oxide` 目录执行：

```sh
export OXIDE_ROOT="$PWD"
./oxide-rs/build.sh
(cd oxide-go && go build -o ../bin/oxide ./cmd/oxide)

export OXIDE_ARCH="$(go env GOHOSTARCH)"
export OXIDE_DEMO="$OXIDE_ROOT/.cache/mermaid-tutorial/$OXIDE_ARCH"
mkdir -p "$OXIDE_DEMO/go-tmp" "$OXIDE_DEMO/go-cache"
export TMPDIR="$OXIDE_DEMO/go-tmp"
export GOTMPDIR="$OXIDE_DEMO/go-tmp"
export GOCACHE="$OXIDE_DEMO/go-cache"
```

构建脚本安装固定版本的 Rust、rustc-dev、rust-src 及两个目标的标准库，无需修改系统 rustup 默认版本。完整渲染器的 MIR 与 Go 编译需要较多内存和磁盘空间，临时目录应放在空间充足的文件系统。

## 2. 直接翻译上游库

```sh
CARGO_TARGET_DIR="$OXIDE_DEMO/cargo-target" \
  "$OXIDE_ROOT/bin/oxide" translate \
  -frontend "$OXIDE_ROOT/bin/oxide-rs" \
  -manifest "$OXIDE_ROOT/../mermaid-rs-renderer/Cargo.toml" \
  -package mermaid-rs-renderer \
  -features scene \
  -target "linux/$OXIDE_ARCH" \
  -out "$OXIDE_DEMO/renderer"
```

默认 features 保持启用，`scene` 额外启用场景 API。不指定 `-roots` 时导出公开函数、重导出、方法、构造函数和可单态化的 trait 方法。泛型声明记录在清单中，需要具体 Rust 类型参数才能实例化。只需要部分接口时，可用 `-roots mermaid_rs_renderer::render` 选择入口。

输出包含：

| 文件 | 内容 |
| --- | --- |
| `oxide_gen_*.go` | 生成的 Go 代码，保留全部编号文件 |
| `oxide_alloc.bin` | 静态数据与重定位镜像，供 go:embed 使用 |
| `oxide.mir.json` | 完整 MIR、布局、函数和数据 |
| `oxide.mir.api.json` | 公开接口、类型描述、容器布局与析构清单 |

包名为 `mermaid_rs_renderer`。生成器不会创建 Go module 或应用 main。

## 3. 编写 Go 调用程序

在示例目录建立 module，将本地 runtime 加为依赖：

```sh
cd "$OXIDE_DEMO"
go mod init example.com/mermaid-demo
go mod edit -require=github.com/csbxd/oxide/oxide-go@v0.0.0
go mod edit -replace=github.com/csbxd/oxide/oxide-go="$OXIDE_ROOT/oxide-go"
mkdir -p cmd/render
```

把下面代码保存为 `cmd/render/main.go`：

```go
package main

import (
    "errors"
    "fmt"
    "os"
    "strings"

    renderer "example.com/mermaid-demo/renderer"
    oxide "github.com/csbxd/oxide/oxide-go/runtime"
)

func rustError(ctx *oxide.Context, value renderer.Ref__Anyhow_Error) error {
    message := value.Display(ctx)
    defer message.Drop(ctx)
    return errors.New(strings.Clone(message.Ref().String()))
}

func run() error {
    if len(os.Args) != 4 {
        return errors.New("usage: render input.mmd output.svg output.png")
    }
    source, err := os.ReadFile(os.Args[1])
    if err != nil {
        return err
    }
    ctx := oxide.NewContext()
    defer ctx.Close()
    mark := ctx.Mark()
    defer ctx.Restore(mark)

    input := renderer.Borrow__Str(ctx.CopyString(string(source)))
    result := renderer.Render(ctx, input)
    defer result.Drop(ctx)
    if result.Ref().Variant().String() == "Err" {
        return rustError(ctx, result.Ref().Field__Err__0())
    }
    svg := result.Ref().Field__Ok__0()
    if err := os.WriteFile(os.Args[2], svg.Bytes(), 0644); err != nil {
        return err
    }

    config := renderer.Default__RenderConfig(ctx)
    defer config.Drop(ctx)
    theme := renderer.Theme_Modern(ctx)
    defer theme.Drop(ctx)
    path := renderer.Borrow__Std_Path_Path(ctx.CopyBytes([]byte(os.Args[3])))
    png := renderer.WriteOutputPng(ctx, svg.Borrow(), path, config.Ref(), theme.Ref())
    defer png.Drop(ctx)
    if png.Ref().Variant().String() == "Err" {
        return rustError(ctx, png.Ref().Field__Err__0())
    }
    return nil
}

func main() {
    if err := run(); err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
}
```

编译运行：

```sh
GOWORK=off GOGC=25 GOMEMLIMIT=20GiB GOMAXPROCS=4 \
  go build -p=1 -mod=mod -o render ./cmd/render
./render "$OXIDE_ROOT/fixtures/renderer/cases/flowchart.mmd" result.svg result.png
```

`svg.Bytes()` 直接借用 Rust String 的内容。必须在 `result.Drop` 之前完成读取。`strings.Clone(value.Ref().String())` 创建独立 Go 字符串，适合错误返回或长期保存。

## 4. 修改 Rust 类型并管理所有权

每个 Rust 类型生成真正的具名布局类型 `Rust__Name`，并区分稳定存储上的拥有值 `Value__Name`、共享引用 `Ref__Name` 和可变位置 `Mut__Name`。函数、字段和枚举载荷都有具体 Go 类型；错误类型、不可见字段和不满足 trait 的方法由 Go 编译器拒绝。公开别名保持 Rust 的类型身份，大小相同的不同 Rust 类型仍然不同。接口没有运行时类型表、字符串字段查找或动态方法回调。

```go
options := renderer.Default__RenderOptions(ctx)
font := options.Mut().Field__Theme().Field__FontFamily()
font.Replace(ctx, renderer.String__Alloc_String_String(ctx, "sans-serif"))
options.Mut().Field__Layout().Field__NodeSpacing().Set(60)

input := renderer.Borrow__Str(ctx.CopyString(source))
result := renderer.RenderWithOptions(ctx, input, options)
// options 已按值移交给 Rust，不能再读取或 Drop。
defer result.Drop(ctx)
```

`Replace` 先保存右侧值，再原地运行旧字段的 Rust Drop，最后移入新值；即使旧析构 panic，新值也会安装到目标位置。零大小拥有值即便共享地址，也不会省略 Drop。`Init` 只用于未初始化或已被消费的存储，`Move(ctx)` 将一个可变位置移入新的稳定存储。`New__Name(ctx)` 只预留空间，不能假定全零是有效 Rust 值；有效默认值由 `Default__Name(ctx)` 调用真实 Rust 实现产生。

`Vec__Name(ctx, capacity)` 先分配容量，再通过 `value.Mut().InitAt(index, owner)` 初始化，最后用 `SetLen` 发布长度。枚举构造形如 `New__Name__Some(ctx, payload)`，采用真实 Rust tag/niche，并静态检查参数类型。`value.Ref().Variant()` 返回具名标签类型；`Field__Some__0()` 等方法只允许读取对应的活动 variant。自定义 allocator 容器没有全局 allocator 构造入口。

HashMap/BTreeMap 通过 `value.Ref().Iter(ctx)` 或 `value.Mut().Iter(ctx)` 调用实际 Rust 迭代器，再使用 `iterator.Mut().Next(ctx)`。返回值是具名 `Option<Item>`；Map 的 Item 是两个引用组成的 tuple，逐项使用具名字段方法和 `Deref`。集合必须活得比迭代器更久。

依赖图存在 `serde_json` 且具体类型满足 trait 时，会生成 `Ref__Name.JSON(ctx)`、`JSONValue(ctx)` 和 `FromJSON__Name(ctx, input)`。这些方法执行真实 Rust serde 实现；`JSONValue` 调用 `to_value(&value)`，保留其浮点转换和对象键序。返回的 Rust Result 需要检查和 Drop；反序列化结果若借用了原始输入，输入的 frame 必须继续有效。

Go 检查类型和方法集，但无法执行 Rust 的线性所有权、完整借用和生命周期检查。复制拥有值的 Go 句柄不会执行 Rust Clone；所有权转移后，任何相同位拷贝都不能再次使用或释放。共享引用没有 Drop 和修改方法。每种类型与权限使用零大小私有标记，阻止普通 Go 显式转换把共享引用变成可变引用或拥有值；薄句柄仍为 8 字节，DST 视图为 16 字节。字段、索引、字符串与切片视图不能超过其拥有者的生命周期。从共享引用取得的 `Bytes()` / `Span().Bytes()` 是零拷贝视图，不能通过它写入；Go 切片类型无法表达只读借用。原始指针字段的地址读取和 null 初始化需要显式 unsafe 存储操作。

`RustSize__Name` / `RustAlign__Name` 来自 rustc。Go 对齐上限为 8，所以超对齐对象必须通过 Context 或生成的分配器保存，不能把普通 Go 布局变量当成已满足 Rust 对齐的对象。packed 字段可以按字节读写或移到对齐位置；生成接口拒绝把未对齐位置作为 Rust 引用传入。DST 通过带 metadata 的具名引用表示，不伪造固定尺寸。外部存储可显式接入 `UnsafeRef__Name` / `UnsafeMut__Name` / `UnsafeValue__Name`，由调用者保证初始化、所有权与生存期。

## 5. 单独分配和释放对象存储

Context 管理稳定的自动存储。对返回的拥有值先 Drop，再 Restore；Context.Close 不会自动寻找并释放 Rust String、Vec 等拥有的堆内存。

如果对象需要超过当前 Context frame 的生命周期，可以单独分配其存储：

```go
saved := renderer.Alloc__Theme()
mark := ctx.Mark()
saved.Init(renderer.Theme_Modern(ctx))
ctx.Restore(mark)

// saved 的对象头与 Rust 内部内存仍然有效。
saved.Mut().Field__FontSize().Set(18)
saved.Close(ctx)
```

`Value__Name.Drop(ctx)` 原地运行 Rust 析构；`Storage__Name.Free()` 只释放对象头的分配。`Storage__Name.Close(ctx)` 顺序完成两者，析构 panic 时也释放对象头。借用的 Field / Index 视图没有 Free 方法。

Rust 的 `Default`、`Display`、`Debug` 由真实 trait 实现完成。格式化返回的 Rust String 也需要 Drop。不满足 trait 的类型没有对应方法。原生 CLI 的 `Run` 可以直接调用；结束进程时应使用生成的 `RustExit(ctx, code)`，让 Rust 清理缓冲 stdout，再执行实际进程退出。

## 6. 验证与边界

```sh
cd "$OXIDE_ROOT"
python3 fixtures/renderer/test.py
python3 fixtures/renderer/test.py --stage native
python3 fixtures/renderer/test.py --stage export
python3 fixtures/renderer/test.py --stage go
```

验收目录中的 Rust 程序仅作为原生对照，不参与翻译输入。测试直接调用原库，比较完整 SVG/PNG、公开 API、CLI 进程行为和内存计数。每个架构使用同架构原生 Rust 参考。

只重新运行 Go 后端可以使用：

```sh
"$OXIDE_ROOT/bin/oxide" emit \
  -mir "$OXIDE_DEMO/renderer/oxide.mir.json" \
  -package mermaid_rs_renderer \
  -out "$OXIDE_DEMO/renderer"
```

Oxide 保留编译器给出的布局、析构和已验证语义。稳定的 Context 存储是模拟 Rust 自动存储；它不等于原生机器栈位置，也不代表已覆盖全部 Rust unsafe、并发或异步行为。零分配验收指预热后 Go 堆计数为零，Rust 自身仍可进行堆分配。支持范围及最新双架构结果见 [里程碑](../MILESTONES.md)。
