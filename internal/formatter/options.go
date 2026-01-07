package formatter

// Options 格式化选项
type Options struct {
	// 缩进设置
	IndentStyle string // "tabs" 或 "spaces"
	IndentSize  int    // 空格数（当使用 spaces 时）

	// 代码风格
	MaxLineLength    int  // 最大行长度
	AlignAssignments bool // 对齐赋值语句
	SpaceBeforeParen bool // 括号前是否有空格 (if (x) vs if(x))
	SpaceInsideParen bool // 括号内是否有空格 ( x ) vs (x)
	SpaceAroundOps   bool // 运算符周围是否有空格

	// 换行设置
	BraceStyle        string // "K&R", "Allman", "GNU"
	NewlineBeforeBrace bool  // 大括号前是否换行
	NewlineAfterBrace  bool  // 大括号后是否换行

	// 注释设置
	PreserveComments bool // 保留注释
	AlignComments    bool // 对齐注释

	// 导入排序
	SortImports  bool // 排序 use 语句
	GroupImports bool // 按命名空间分组 imports

	// 其他
	RemoveTrailingSpace bool // 移除行尾空格
	EnsureNewlineAtEOF  bool // 确保文件末尾有换行符
}

// DefaultOptions 返回默认格式化选项（K&R 风格 + 4空格缩进）
func DefaultOptions() *Options {
	return &Options{
		IndentStyle:         "spaces",
		IndentSize:          4,
		MaxLineLength:       100,
		AlignAssignments:    false,
		SpaceBeforeParen:    true,
		SpaceInsideParen:    false,
		SpaceAroundOps:      true,
		BraceStyle:          "K&R",
		NewlineBeforeBrace:  false,
		NewlineAfterBrace:   true,
		PreserveComments:    true,
		AlignComments:       false,
		SortImports:         true,
		GroupImports:        true,
		RemoveTrailingSpace: true,
		EnsureNewlineAtEOF:  true,
	}
}

