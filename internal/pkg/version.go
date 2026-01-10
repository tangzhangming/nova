// version.go - 语义化版本处理
//
// 实现语义化版本（Semantic Versioning）的解析和比较。
//
// 版本格式：MAJOR.MINOR.PATCH[-PRERELEASE][+BUILD]
// 例如：1.2.3, 1.0.0-alpha, 2.1.0-beta.1+build.123

package pkg

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Version 语义化版本
type Version struct {
	Major      int
	Minor      int
	Patch      int
	Prerelease string
	Build      string
	Raw        string
}

// 版本正则表达式
var versionRegex = regexp.MustCompile(
	`^v?(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$`)

// ParseVersion 解析版本字符串
func ParseVersion(s string) (*Version, error) {
	// 特殊处理 latest
	if s == "latest" {
		return &Version{
			Major: 999999,
			Minor: 999999,
			Patch: 999999,
			Raw:   s,
		}, nil
	}
	
	matches := versionRegex.FindStringSubmatch(s)
	if matches == nil {
		return nil, fmt.Errorf("invalid version: %s", s)
	}
	
	major, _ := strconv.Atoi(matches[1])
	minor, _ := strconv.Atoi(matches[2])
	patch, _ := strconv.Atoi(matches[3])
	
	return &Version{
		Major:      major,
		Minor:      minor,
		Patch:      patch,
		Prerelease: matches[4],
		Build:      matches[5],
		Raw:        s,
	}, nil
}

// String 转换为字符串
func (v *Version) String() string {
	if v.Raw != "" {
		return v.Raw
	}
	
	s := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Prerelease != "" {
		s += "-" + v.Prerelease
	}
	if v.Build != "" {
		s += "+" + v.Build
	}
	return s
}

// Compare 比较两个版本
// 返回：-1 (v < other), 0 (v == other), 1 (v > other)
func (v *Version) Compare(other *Version) int {
	// 比较主版本号
	if v.Major != other.Major {
		if v.Major < other.Major {
			return -1
		}
		return 1
	}
	
	// 比较次版本号
	if v.Minor != other.Minor {
		if v.Minor < other.Minor {
			return -1
		}
		return 1
	}
	
	// 比较修订号
	if v.Patch != other.Patch {
		if v.Patch < other.Patch {
			return -1
		}
		return 1
	}
	
	// 比较预发布版本
	// 没有预发布标识的版本优先级更高
	if v.Prerelease == "" && other.Prerelease != "" {
		return 1
	}
	if v.Prerelease != "" && other.Prerelease == "" {
		return -1
	}
	if v.Prerelease != other.Prerelease {
		return comparePrerelease(v.Prerelease, other.Prerelease)
	}
	
	return 0
}

// LessThan 小于
func (v *Version) LessThan(other *Version) bool {
	return v.Compare(other) < 0
}

// LessThanOrEqual 小于等于
func (v *Version) LessThanOrEqual(other *Version) bool {
	return v.Compare(other) <= 0
}

// GreaterThan 大于
func (v *Version) GreaterThan(other *Version) bool {
	return v.Compare(other) > 0
}

// GreaterThanOrEqual 大于等于
func (v *Version) GreaterThanOrEqual(other *Version) bool {
	return v.Compare(other) >= 0
}

// Equal 等于
func (v *Version) Equal(other *Version) bool {
	return v.Compare(other) == 0
}

// IsPrerelease 是否是预发布版本
func (v *Version) IsPrerelease() bool {
	return v.Prerelease != ""
}

// comparePrerelease 比较预发布版本
func comparePrerelease(a, b string) int {
	partsA := strings.Split(a, ".")
	partsB := strings.Split(b, ".")
	
	minLen := len(partsA)
	if len(partsB) < minLen {
		minLen = len(partsB)
	}
	
	for i := 0; i < minLen; i++ {
		cmp := comparePrereleaseIdentifier(partsA[i], partsB[i])
		if cmp != 0 {
			return cmp
		}
	}
	
	// 更长的预发布版本优先级更高
	if len(partsA) < len(partsB) {
		return -1
	}
	if len(partsA) > len(partsB) {
		return 1
	}
	
	return 0
}

// comparePrereleaseIdentifier 比较单个预发布标识符
func comparePrereleaseIdentifier(a, b string) int {
	// 数字标识符始终小于非数字标识符
	numA, errA := strconv.Atoi(a)
	numB, errB := strconv.Atoi(b)
	
	if errA == nil && errB == nil {
		// 都是数字，数值比较
		if numA < numB {
			return -1
		}
		if numA > numB {
			return 1
		}
		return 0
	}
	
	if errA == nil {
		return -1 // 数字 < 非数字
	}
	if errB == nil {
		return 1 // 非数字 > 数字
	}
	
	// 都是字符串，字典序比较
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

// ============================================================================
// 版本约束
// ============================================================================

// Constraint 版本约束
type Constraint struct {
	operator string
	version  *Version
}

// ConstraintSet 版本约束集合
type ConstraintSet struct {
	constraints []*Constraint
}

// ParseConstraint 解析版本约束
// 支持的格式：
//   - "1.2.3" 精确匹配
//   - ">=1.2.3" 大于等于
//   - "<=1.2.3" 小于等于
//   - ">1.2.3" 大于
//   - "<1.2.3" 小于
//   - "^1.2.3" 兼容（同主版本号）
//   - "~1.2.3" 近似（同主次版本号）
//   - "1.2.x" 或 "1.2.*" 通配符
func ParseConstraint(s string) (*ConstraintSet, error) {
	s = strings.TrimSpace(s)
	
	// 处理多个约束（用空格或逗号分隔）
	parts := regexp.MustCompile(`[,\s]+`).Split(s, -1)
	
	constraints := make([]*Constraint, 0)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		
		c, err := parseSingleConstraint(part)
		if err != nil {
			return nil, err
		}
		constraints = append(constraints, c)
	}
	
	return &ConstraintSet{constraints: constraints}, nil
}

// parseSingleConstraint 解析单个约束
func parseSingleConstraint(s string) (*Constraint, error) {
	// 提取操作符
	var operator string
	var versionStr string
	
	switch {
	case strings.HasPrefix(s, ">="):
		operator = ">="
		versionStr = s[2:]
	case strings.HasPrefix(s, "<="):
		operator = "<="
		versionStr = s[2:]
	case strings.HasPrefix(s, ">"):
		operator = ">"
		versionStr = s[1:]
	case strings.HasPrefix(s, "<"):
		operator = "<"
		versionStr = s[1:]
	case strings.HasPrefix(s, "^"):
		operator = "^"
		versionStr = s[1:]
	case strings.HasPrefix(s, "~"):
		operator = "~"
		versionStr = s[1:]
	case strings.HasPrefix(s, "="):
		operator = "="
		versionStr = s[1:]
	default:
		operator = "="
		versionStr = s
	}
	
	// 处理通配符
	versionStr = strings.Replace(versionStr, "x", "0", -1)
	versionStr = strings.Replace(versionStr, "*", "0", -1)
	
	version, err := ParseVersion(versionStr)
	if err != nil {
		return nil, err
	}
	
	return &Constraint{
		operator: operator,
		version:  version,
	}, nil
}

// Check 检查版本是否满足约束集
func (cs *ConstraintSet) Check(v *Version) bool {
	for _, c := range cs.constraints {
		if !c.check(v) {
			return false
		}
	}
	return true
}

// check 检查版本是否满足单个约束
func (c *Constraint) check(v *Version) bool {
	switch c.operator {
	case "=":
		return v.Equal(c.version)
	case ">":
		return v.GreaterThan(c.version)
	case ">=":
		return v.GreaterThanOrEqual(c.version)
	case "<":
		return v.LessThan(c.version)
	case "<=":
		return v.LessThanOrEqual(c.version)
	case "^":
		// 兼容：同主版本号
		return v.Major == c.version.Major && v.GreaterThanOrEqual(c.version)
	case "~":
		// 近似：同主次版本号
		return v.Major == c.version.Major &&
			v.Minor == c.version.Minor &&
			v.GreaterThanOrEqual(c.version)
	default:
		return v.Equal(c.version)
	}
}

// String 转换为字符串
func (cs *ConstraintSet) String() string {
	parts := make([]string, len(cs.constraints))
	for i, c := range cs.constraints {
		parts[i] = c.operator + c.version.String()
	}
	return strings.Join(parts, " ")
}

// ============================================================================
// 辅助函数
// ============================================================================

// sortVersions 对版本字符串切片进行排序
func sortVersions(versions []string) {
	sort.Slice(versions, func(i, j int) bool {
		vi, err1 := ParseVersion(versions[i])
		vj, err2 := ParseVersion(versions[j])
		
		if err1 != nil || err2 != nil {
			return versions[i] < versions[j]
		}
		
		return vi.LessThan(vj)
	})
}

// getMajorVersion 获取主版本号
func getMajorVersion(versionStr string) int {
	v, err := ParseVersion(versionStr)
	if err != nil {
		return 0
	}
	return v.Major
}

// SelectVersion 从版本列表中选择满足约束的最高版本
func SelectVersion(versions []string, constraint string) (string, error) {
	cs, err := ParseConstraint(constraint)
	if err != nil {
		return "", err
	}
	
	// 解析并过滤版本
	matching := make([]*Version, 0)
	for _, vs := range versions {
		v, err := ParseVersion(vs)
		if err != nil {
			continue
		}
		if cs.Check(v) {
			matching = append(matching, v)
		}
	}
	
	if len(matching) == 0 {
		return "", fmt.Errorf("no version satisfies constraint: %s", constraint)
	}
	
	// 排序并选择最高版本
	sort.Slice(matching, func(i, j int) bool {
		return matching[i].LessThan(matching[j])
	})
	
	return matching[len(matching)-1].String(), nil
}

// IsValidVersion 检查是否是有效的版本字符串
func IsValidVersion(s string) bool {
	_, err := ParseVersion(s)
	return err == nil
}
