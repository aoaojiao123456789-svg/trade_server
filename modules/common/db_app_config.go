package common

import (
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	ldb "github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/db"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"github.com/gocraft/dbr/v2"
)

type SmsConfig struct {
	Smsbao struct {
		Pass    string `json:"pass"`
		User    string `json:"user"`
		Smsapi  string `json:"smsapi"`
		Content string `json:"content"`
	} `json:"smsbao"`
	Huiyuandx struct {
		Pass    string `json:"pass"`
		User    string `json:"user"`
		Smsapi  string `json:"smsapi"`
		Content string `json:"content"`
		Smstype string `json:"smstype"`
	} `json:"huiyuandx"`
	UseApi string `json:"use_api"`
}

type AppConfigDB struct {
	session *dbr.Session
	ctx     *config.Context
}

func NewAppConfigDB(ctx *config.Context) *AppConfigDB {

	return &AppConfigDB{
		session: ctx.DB(),
		ctx:     ctx,
	}
}

func (a *AppConfigDB) Query() (*appConfigModel, error) {
	var m *appConfigModel
	_, err := a.session.Select("*").From("app_config").OrderDesc("created_at").Load(&m)
	return m, err
}

func (a *AppConfigDB) insert(m *appConfigModel) error {
	_, err := a.session.InsertInto("app_config").Columns(util.AttrToUnderscore(m)...).Record(m).Exec()
	return err
}

func (a *AppConfigDB) updateWithMap(configMap map[string]interface{}, id int64) error {
	_, err := a.session.Update("app_config").SetMap(configMap).Where("id=?", id).Exec()
	return err
}

func (a *AppConfigDB) InsertConfigHistory(m *ConfigHistoryModel) error {
	_, err := a.session.InsertInto("config_history").Columns(util.AttrToUnderscore(m)...).Record(m).Exec()
	return err
}

func (a *AppConfigDB) QueryConfigHistory(pageSize uint64, id uint64) ([]*ConfigHistoryModel, error) {
	var m []*ConfigHistoryModel
	_, err := a.session.Select("*").From("config_history").Where("app_id = ?", id).OrderDesc("update_at").Limit(pageSize).Load(&m)
	return m, err
}

type appConfigModel struct {
	RSAPrivateKey                  string
	RSAPublicKey                   string
	Version                        int
	SuperToken                     string
	SuperTokenOn                   int
	RevokeSecond                   int    // 消息可撤回时长
	WelcomeMessage                 string // 登录欢迎语
	NewUserJoinSystemGroup         int    // 新用户是否加入系统群聊
	SearchByPhone                  int    // 是否可通过手机号搜索
	RegisterInviteOn               int    // 开启注册邀请机制
	SendWelcomeMessageOn           int    // 开启注册登录发送欢迎语
	InviteSystemAccountJoinGroupOn int    // 开启系统账号加入群聊
	RegisterUserMustCompleteInfoOn int    // 注册用户是否必须完善个人信息
	ChannelPinnedMessageMaxCount   int    // 频道置顶消息最大数量
	CanModifyApiUrl                int    // 是否可以修改API地址
	ManequinControl                string // 假人控制
	LotteryConfig                  string // 开奖配置
	SmsConfig                      string // 短信配置
	SearchMode                     int    // 搜索模式
	SearchShortNo                  string // 短号
	CreateGroup                    int    // 建群限制
	WebRoute                       string // app打开地址
	GroupRevokeOn                  int    // 群消息撤回开关
	ReportUrl                      string // 举报链接
	LineConfig                     string // 连线配置
	SesConfig                      string // SES配置

	ldb.BaseModel
}

type ConfigHistoryModel struct {
	Id          int
	OperateUid  string
	OperateName string
	UpdateValue string
	UpdateAt    int64
	Type        int
	AppId       int
}
