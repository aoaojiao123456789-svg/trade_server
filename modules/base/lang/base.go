package lang

const (
	ZhCN string = "zh-CN"
	EnUS string = "en-US"
)

/*msg2 := lang.T(lang.EnUS, "不存在的提示")
fmt.Println(msg2) // 输出原样: 不存在的提示*/

func T(language string, zhCN string) string {
	switch language {
	case EnUS:
		if v, ok := EnUSMap[zhCN]; ok {
			return v
		}
		return zhCN

	case ZhCN:
		fallthrough
	default:
		if v, ok := ZhCNMap[zhCN]; ok {
			return v
		}
		return zhCN
	}
}
