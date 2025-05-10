//go:build embed
// +build embed

package lib

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
)

//go:embed static
var embed_static_fs embed.FS

// extract_static 提取嵌入的静态文件到目标目录
// (函数名保持用户原来的 extract_static)
func extract_static(dest_dir string) error {
	// mkdir
	if _, err := os.Stat(dest_dir); os.IsNotExist(err) {
		err := os.MkdirAll(dest_dir, 0755)
		if err != nil {
			return fmt.Errorf("创建提取目录 %s 失败: %w", dest_dir, err)
		}
		log.Printf("提取目录已创建: %s", dest_dir)
	} else {
		log.Printf("提取目录已存在: %s", dest_dir)
	}

	// extract
	// 使用本包内的全局变量 embed_static_fs
	err := fs.WalkDir(embed_static_fs, "static", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("遍历嵌入文件 %s 时出错: %w", path, err)
		}

		relPath, relErr := filepath.Rel("static", path)
		if relErr != nil {
			return fmt.Errorf("获取相对路径 '%s' (相对于 'static') 失败: %w", path, relErr)
		}

		// 如果 relPath 是 "."，表示是 "static" 目录本身，跳过
		if relPath == "." {
			return nil
		}
		destPath := filepath.Join(dest_dir, relPath)

		if d.IsDir() {
			if err := os.MkdirAll(destPath, 0755); err != nil {
				return fmt.Errorf("创建子目录 %s 失败: %w", destPath, err)
			}
			return nil
		}

		data, err := embed_static_fs.ReadFile(path)
		if err != nil {
			return fmt.Errorf("读取嵌入文件 %s 失败: %w", path, err)
		}

		if err := os.WriteFile(destPath, data, 0644); err != nil {
			return fmt.Errorf("写入文件 %s 失败: %w", destPath, err)
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("提取静态文件失败: %w", err)
	} else {
		log.Printf("嵌入式静态文件已提取到 %s", dest_dir)
	}

	return nil
}
