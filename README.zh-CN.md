# heif-meta-parser

[English](README.md)

`heif-meta-parser` 是一个用于读取 HEIF/HEIC 元信息的小型 Go 库，不解码图像像素。

它会解析必要的 ISO BMFF/HEIF 结构，用于获取主图尺寸、应用旋转信息后的尺寸、帧数、主品牌、兼容品牌、格式和 MIME 类型。

## 安装

```sh
go get github.com/turtlemeow/heif-meta-parser
```

## 使用

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

如果需要更详细的 item 元信息，可以使用 `Parse`：

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

## 命令行工具

```sh
go install github.com/turtlemeow/heif-meta-parser/cmd/heifmeta@latest
heifmeta image.heic
```

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
