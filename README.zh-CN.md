# heif-meta-parser

[English](README.md)

`heif-meta-parser` 是一个用于读取 HEIF/HEIC 元信息的小型 Go 库，不解码图像像素。

它会解析必要的 ISO BMFF/HEIF 结构，用于获取主图尺寸、应用旋转信息后的尺寸、帧数、主品牌、兼容品牌、格式和 MIME 类型。

## 目前解析了什么

这个库从 BMFF box 结构中读取 HEIF 元信息。目前会提取：

- 文件类型信息：从 `ftyp` 读取主品牌和兼容品牌
- 主图 item：从 `pitm` 读取 primary item ID
- 图片 item 记录：从 `iinf` / `infe` 读取 item ID、item type、item name 和 hidden 标记
- 图片尺寸：从 `ispe` item property 读取宽高
- 旋转信息：从 `irot` item property 读取 90 度步进的旋转值
- item 与 property 的关联：从 `iprp` / `ipco` / `ipma` 读取 property association
- item 引用关系：从 `iref` 识别缩略图、辅助图和 mask 关系
- 顶层图片列表：过滤 hidden、thumbnail、auxiliary 和 mask 后得到 top-level images

这个库不会解析或解码 `mdat` 中的压缩媒体数据。

## 安装

```sh
go get github.com/turtlemeow/heif-meta-parser
```

## 核心接口

### `DecodeConfig`

如果只需要常见的图片配置，可以使用 `DecodeConfig`：

```go
package main

import (
	"fmt"
	"os"

	heifmeta "github.com/turtlemeow/heif-meta-parser"
)

func main() {
	f, err := os.Open("image.heic")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	cfg, err := heifmeta.DecodeConfig(f)
	if err != nil {
		panic(err)
	}

	fmt.Printf("%dx%d %s %s frames=%d\n",
		cfg.Width, cfg.Height, cfg.Format, cfg.MainBrand, cfg.FrameCount)
}
```

返回结构：

```go
type Config struct {
	Width            int
	Height           int
	FrameCount       int
	Format           Format
	MIME             string
	MainBrand        string
	CompatibleBrands []string
}
```

如果存在 `irot` 旋转信息，`Width` 和 `Height` 会返回应用旋转后的结果。

### `Parse`

如果需要更详细的 item 级元信息，可以使用 `Parse`：

```go
file, err := heifmeta.Parse(f)
if err != nil {
	panic(err)
}

for _, img := range file.Images {
	w, h := img.RotatedDimensions()
	fmt.Printf("id=%d primary=%t type=%s size=%dx%d\n",
		img.ID, img.Primary, img.ItemType, w, h)
}
```

返回结构：

```go
type File struct {
	MainBrand        string
	CompatibleBrands []string
	Images           []Image
	TopLevelImages   []Image
	PrimaryImage     *Image
}

type Image struct {
	ID        uint32
	Width     uint32
	Height    uint32
	Rotation  Rotation
	ItemType  string
	Name      string
	Primary   bool
	Hidden    bool
	Thumbnail bool
	Auxiliary bool
	Mask      bool
}
```

`Images` 包含所有解析到的图片 item。`TopLevelImages` 会排除 hidden、thumbnail、auxiliary 和 mask item。`PrimaryImage` 指向 HEIF `pitm` box 指定的主图 item。

### `IsSupportedBrand`

如果你已经拿到了 HEIF major brand，只想判断这个库能否把它映射为标准格式，可以使用 `IsSupportedBrand`。

## 命令行工具

```sh
go install github.com/turtlemeow/heif-meta-parser/cmd/heifmeta@latest
heifmeta testdata/sample.heic
```

仓库内置了 `testdata/sample.heic`，这是一个已清理元信息的 HEIC 文件，可用于命令行工具和测试。

## 范围

支持解析的元信息 box 包括：

- `ftyp`
- `meta`
- `pitm`
- `iinf` / `infe`
- `iprp` / `ipco` / `ipma`
- `ispe`
- `irot`
- `iref`

支持的主品牌包括：

- HEIC 静态图：`heic`、`heix`、`heim`、`heis`
- HEIF 静态图：`mif1`
- HEIC 序列图：`hevc`、`hevx`、`hevm`、`hevs`
- HEIF 序列图：`msf1`

这个库不会解码像素数据，不会校验所有 HEIF 特性，也不会解析 `mdat` 中的媒体数据。

## 许可证

MIT
