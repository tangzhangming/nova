package i18n

import (
	"fmt"
	"sync"
)

// Language 语言类型
type Language string

const (
	LangEnglish Language = "en"
	LangChinese Language = "zh"
)

// 全局语言设置
var (
	currentLang Language = LangEnglish
	mu          sync.RWMutex
)

// SetLanguage 设置当前语言
func SetLanguage(lang Language) {
	mu.Lock()
	defer mu.Unlock()
	currentLang = lang
}

// SetLanguageFromString 从字符串设置语言
func SetLanguageFromString(lang string) {
	switch lang {
	case "zh", "zh-cn", "zh-tw", "zh-hk", "chinese":
		SetLanguage(LangChinese)
	default:
		SetLanguage(LangEnglish)
	}
}

// GetLanguage 获取当前语言
func GetLanguage() Language {
	mu.RLock()
	defer mu.RUnlock()
	return currentLang
}

// T 翻译消息（支持格式化参数）
func T(msgID string, args ...interface{}) string {
	mu.RLock()
	lang := currentLang
	mu.RUnlock()

	var messages map[string]string
	switch lang {
	case LangChinese:
		messages = messagesZH
	default:
		messages = messagesEN
	}

	if msg, ok := messages[msgID]; ok {
		if len(args) > 0 {
			return fmt.Sprintf(msg, args...)
		}
		return msg
	}

	// 回退到英文
	if msg, ok := messagesEN[msgID]; ok {
		if len(args) > 0 {
			return fmt.Sprintf(msg, args...)
		}
		return msg
	}

	// 找不到翻译则返回原始 ID
	return msgID
}








