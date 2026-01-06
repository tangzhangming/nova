package runtime

import (
	"time"

	"github.com/tangzhangming/nova/internal/bytecode"
)

// ============================================================================
// Native 时间函数
// ============================================================================

// nativeTimeNow 获取当前 Unix 时间戳（秒）
func nativeTimeNow(args []bytecode.Value) bytecode.Value {
	return bytecode.NewInt(time.Now().Unix())
}

// nativeTimeNowMs 获取当前毫秒时间戳
func nativeTimeNowMs(args []bytecode.Value) bytecode.Value {
	return bytecode.NewInt(time.Now().UnixMilli())
}

// nativeTimeNowNano 获取当前纳秒时间戳
func nativeTimeNowNano(args []bytecode.Value) bytecode.Value {
	return bytecode.NewInt(time.Now().UnixNano())
}

// nativeTimeSleep 休眠指定毫秒
func nativeTimeSleep(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NullValue
	}
	ms := args[0].AsInt()
	time.Sleep(time.Duration(ms) * time.Millisecond)
	return bytecode.NullValue
}

// nativeTimeParse 解析时间字符串
// 参数：str, format（Go 格式，如 "2006-01-02 15:04:05"）
func nativeTimeParse(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.NewInt(0)
	}
	str := args[0].AsString()
	format := "2006-01-02 15:04:05"
	if len(args) > 1 {
		format = args[1].AsString()
	}

	t, err := time.Parse(format, str)
	if err != nil {
		// 尝试常见格式
		formats := []string{
			"2006-01-02 15:04:05",
			"2006-01-02",
			"2006/01/02 15:04:05",
			"2006/01/02",
			time.RFC3339,
		}
		for _, f := range formats {
			if t, err = time.Parse(f, str); err == nil {
				break
			}
		}
		if err != nil {
			return bytecode.NewInt(0)
		}
	}
	return bytecode.NewInt(t.Unix())
}

// nativeTimeFormat 格式化时间戳
// 参数：timestamp, format（Go 格式）
func nativeTimeFormat(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.NewString("")
	}
	timestamp := args[0].AsInt()
	format := "2006-01-02 15:04:05"
	if len(args) > 1 {
		format = args[1].AsString()
	}

	t := time.Unix(timestamp, 0)
	return bytecode.NewString(t.Format(format))
}

// nativeTimeYear 获取年份
func nativeTimeYear(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewInt(int64(time.Now().Year()))
	}
	timestamp := args[0].AsInt()
	t := time.Unix(timestamp, 0)
	return bytecode.NewInt(int64(t.Year()))
}

// nativeTimeMonth 获取月份
func nativeTimeMonth(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewInt(int64(time.Now().Month()))
	}
	timestamp := args[0].AsInt()
	t := time.Unix(timestamp, 0)
	return bytecode.NewInt(int64(t.Month()))
}

// nativeTimeDay 获取日
func nativeTimeDay(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewInt(int64(time.Now().Day()))
	}
	timestamp := args[0].AsInt()
	t := time.Unix(timestamp, 0)
	return bytecode.NewInt(int64(t.Day()))
}

// nativeTimeHour 获取时
func nativeTimeHour(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewInt(int64(time.Now().Hour()))
	}
	timestamp := args[0].AsInt()
	t := time.Unix(timestamp, 0)
	return bytecode.NewInt(int64(t.Hour()))
}

// nativeTimeMinute 获取分
func nativeTimeMinute(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewInt(int64(time.Now().Minute()))
	}
	timestamp := args[0].AsInt()
	t := time.Unix(timestamp, 0)
	return bytecode.NewInt(int64(t.Minute()))
}

// nativeTimeSecond 获取秒
func nativeTimeSecond(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewInt(int64(time.Now().Second()))
	}
	timestamp := args[0].AsInt()
	t := time.Unix(timestamp, 0)
	return bytecode.NewInt(int64(t.Second()))
}

// nativeTimeWeekday 获取星期几（0=周日，1=周一...）
func nativeTimeWeekday(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewInt(int64(time.Now().Weekday()))
	}
	timestamp := args[0].AsInt()
	t := time.Unix(timestamp, 0)
	return bytecode.NewInt(int64(t.Weekday()))
}

// nativeTimeMake 构建时间戳
// 参数：year, month, day, hour, minute, second
func nativeTimeMake(args []bytecode.Value) bytecode.Value {
	if len(args) < 3 {
		return bytecode.NewInt(0)
	}
	year := int(args[0].AsInt())
	month := time.Month(args[1].AsInt())
	day := int(args[2].AsInt())
	hour := 0
	minute := 0
	second := 0
	if len(args) > 3 {
		hour = int(args[3].AsInt())
	}
	if len(args) > 4 {
		minute = int(args[4].AsInt())
	}
	if len(args) > 5 {
		second = int(args[5].AsInt())
	}

	t := time.Date(year, month, day, hour, minute, second, 0, time.Local)
	return bytecode.NewInt(t.Unix())
}


