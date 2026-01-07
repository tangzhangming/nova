package runtime

import (
	"io"
	"os"
	"path/filepath"

	"github.com/tangzhangming/nova/internal/bytecode"
)

// ============================================================================
// Native 文件操作函数
// ============================================================================

// nativeFileRead 读取文件全部内容
func nativeFileRead(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewString("")
	}
	path := args[0].AsString()
	data, err := os.ReadFile(path)
	if err != nil {
		return bytecode.NewString("")
	}
	return bytecode.NewString(string(data))
}

// nativeFileWrite 写入文件（覆盖）
func nativeFileWrite(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.FalseValue
	}
	path := args[0].AsString()
	content := args[1].AsString()
	err := os.WriteFile(path, []byte(content), 0644)
	return bytecode.NewBool(err == nil)
}

// nativeFileAppend 追加内容到文件
func nativeFileAppend(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.FalseValue
	}
	path := args[0].AsString()
	content := args[1].AsString()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return bytecode.FalseValue
	}
	defer f.Close()
	_, err = f.WriteString(content)
	return bytecode.NewBool(err == nil)
}

// nativeFileExists 检查路径是否存在
func nativeFileExists(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	path := args[0].AsString()
	_, err := os.Stat(path)
	return bytecode.NewBool(err == nil)
}

// nativeFileDelete 删除文件
func nativeFileDelete(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	path := args[0].AsString()
	err := os.Remove(path)
	return bytecode.NewBool(err == nil)
}

// nativeFileCopy 复制文件
func nativeFileCopy(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.FalseValue
	}
	src := args[0].AsString()
	dst := args[1].AsString()

	srcFile, err := os.Open(src)
	if err != nil {
		return bytecode.FalseValue
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return bytecode.FalseValue
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return bytecode.NewBool(err == nil)
}

// nativeFileRename 重命名/移动文件
func nativeFileRename(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.FalseValue
	}
	oldPath := args[0].AsString()
	newPath := args[1].AsString()
	err := os.Rename(oldPath, newPath)
	return bytecode.NewBool(err == nil)
}

// nativeIsFile 检查是否是文件
func nativeIsFile(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	path := args[0].AsString()
	info, err := os.Stat(path)
	if err != nil {
		return bytecode.FalseValue
	}
	return bytecode.NewBool(!info.IsDir())
}

// ============================================================================
// Native 目录操作函数
// ============================================================================

// nativeDirCreate 创建单级目录
func nativeDirCreate(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	path := args[0].AsString()
	err := os.Mkdir(path, 0755)
	return bytecode.NewBool(err == nil)
}

// nativeDirCreateAll 递归创建目录
func nativeDirCreateAll(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	path := args[0].AsString()
	err := os.MkdirAll(path, 0755)
	return bytecode.NewBool(err == nil)
}

// nativeDirDelete 删除空目录
func nativeDirDelete(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	path := args[0].AsString()
	err := os.Remove(path)
	return bytecode.NewBool(err == nil)
}

// nativeDirDeleteAll 递归删除目录
func nativeDirDeleteAll(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	path := args[0].AsString()
	err := os.RemoveAll(path)
	return bytecode.NewBool(err == nil)
}

// nativeDirList 列出目录内容
func nativeDirList(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewArray([]bytecode.Value{})
	}
	path := args[0].AsString()
	entries, err := os.ReadDir(path)
	if err != nil {
		return bytecode.NewArray([]bytecode.Value{})
	}
	result := make([]bytecode.Value, len(entries))
	for i, entry := range entries {
		result[i] = bytecode.NewString(entry.Name())
	}
	return bytecode.NewArray(result)
}

// nativeIsDir 检查是否是目录
func nativeIsDir(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	path := args[0].AsString()
	info, err := os.Stat(path)
	if err != nil {
		return bytecode.FalseValue
	}
	return bytecode.NewBool(info.IsDir())
}

// ============================================================================
// Native 文件信息函数
// ============================================================================

// nativeFileSize 获取文件大小
func nativeFileSize(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewInt(-1)
	}
	path := args[0].AsString()
	info, err := os.Stat(path)
	if err != nil {
		return bytecode.NewInt(-1)
	}
	return bytecode.NewInt(info.Size())
}

// nativeFileMtime 获取修改时间
func nativeFileMtime(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewInt(-1)
	}
	path := args[0].AsString()
	info, err := os.Stat(path)
	if err != nil {
		return bytecode.NewInt(-1)
	}
	return bytecode.NewInt(info.ModTime().Unix())
}

// nativeFileAtime 获取访问时间（在某些系统上可能与修改时间相同）
func nativeFileAtime(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewInt(-1)
	}
	path := args[0].AsString()
	info, err := os.Stat(path)
	if err != nil {
		return bytecode.NewInt(-1)
	}
	// Go 标准库不直接提供 atime，使用 mtime 作为后备
	return bytecode.NewInt(info.ModTime().Unix())
}

// nativeFileCtime 获取创建时间（在某些系统上可能与修改时间相同）
func nativeFileCtime(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewInt(-1)
	}
	path := args[0].AsString()
	info, err := os.Stat(path)
	if err != nil {
		return bytecode.NewInt(-1)
	}
	// Go 标准库不直接提供 ctime，使用 mtime 作为后备
	return bytecode.NewInt(info.ModTime().Unix())
}

// nativeFilePerms 获取文件权限
func nativeFilePerms(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewInt(0)
	}
	path := args[0].AsString()
	info, err := os.Stat(path)
	if err != nil {
		return bytecode.NewInt(0)
	}
	return bytecode.NewInt(int64(info.Mode().Perm()))
}

// nativeIsReadable 检查是否可读
func nativeIsReadable(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	path := args[0].AsString()
	f, err := os.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return bytecode.FalseValue
	}
	f.Close()
	return bytecode.TrueValue
}

// nativeIsWritable 检查是否可写
func nativeIsWritable(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	path := args[0].AsString()
	info, err := os.Stat(path)
	if err != nil {
		// 文件不存在，检查父目录是否可写
		dir := filepath.Dir(path)
		f, err := os.OpenFile(dir, os.O_WRONLY, 0)
		if err != nil {
			return bytecode.FalseValue
		}
		f.Close()
		return bytecode.TrueValue
	}
	// 文件存在，尝试以写模式打开
	if info.IsDir() {
		return bytecode.NewBool(info.Mode().Perm()&0200 != 0)
	}
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return bytecode.FalseValue
	}
	f.Close()
	return bytecode.TrueValue
}

// nativeIsExecutable 检查是否可执行
func nativeIsExecutable(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	path := args[0].AsString()
	info, err := os.Stat(path)
	if err != nil {
		return bytecode.FalseValue
	}
	return bytecode.NewBool(info.Mode().Perm()&0111 != 0)
}

// nativeIsLink 检查是否是符号链接
func nativeIsLink(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	path := args[0].AsString()
	info, err := os.Lstat(path)
	if err != nil {
		return bytecode.FalseValue
	}
	return bytecode.NewBool(info.Mode()&os.ModeSymlink != 0)
}







