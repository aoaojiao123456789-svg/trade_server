package common

// CodeType 验证码类型
type CodeType int

const GroupBeginId = 100000000

const NomalGroupLimit = 10
const SuperGroupLimit = 1000

const (
	// CodeTypeRegister 注册
	CodeTypeRegister CodeType = iota
	// CodeTypePayPWD 支付密码
	CodeTypePayPWD
	// CodeTypeForgetLoginPWD 忘记登录密码
	CodeTypeForgetLoginPWD
	// CodeTypeCheckMobile 校验指定手机号是否正确
	CodeTypeCheckMobile
	// DestroyAccount 注销账号
	CodeTypeDestroyAccount
	// CodeTypeLogon 登录
	CodeTypeLogon
)

const (
	SettingUnknown        uint8 = 0
	SettingReceiptEnabled uint8 = 1 << 7 // 是否开启回执
	SettingSignal         uint8 = 1 << 5 // 是否开启signal加密
	SettingNoEncrypt      uint8 = 1 << 4 // 是否不加密
	SettingTopic          uint8 = 1 << 3 // 是否有topic
	SettingStream         uint8 = 1 << 1 // 是否开启流

)

const (
	// CacheKeySMSCode 短信验证码的缓存key
	CacheKeySMSCode string = "smscode:"
	// CacheKeyAwsSesCode 邮箱验证码的缓存key
	CacheKeyAwsSesCode string = "awsSescode:"
)

const (
	// 用户头像缓存版本
	CacheUserAvatarVersion = "avatar_version:%s"
	// 群头像缓存版本
	CacheGroupAvatarVersion = "group_avatar_version:%s"
	// 用户注册和登录标记
	CacheUserRegisterAndLoginMark = "user_register_login_mark:%s"
	// 群ID自增
	CacheGroupIDAutoIncrement = "group_id_auto_increment"
)
