package common

import (
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"go.uber.org/zap"
)

var onceSerce sync.Once

// IService IService
type IService interface {
	GetAppConfig() (*AppConfigResp, error)
	// 获取短编号
	GetShortno() (string, error)
	SetShortnoUsed(shortno string, business string) error
	// 查询配置修改记录
	GetConfigHistory(pageSize uint64, id uint64) ([]*ConfigHistoryModel, error)
	// 插入配置修改记录
	AddConfigHistory(model ConfigHistoryModel) error
}

// NewService NewService
func NewService(ctx *config.Context) IService {
	return newService(ctx)
}

type service struct {
	ctx         *config.Context
	appConfigDB *AppConfigDB
	shortnoDB   *shortnoDB
	shortnoLock sync.RWMutex
}

func newService(ctx *config.Context) *service {
	// if ctx.GetConfig().ShortNo.NumOn {
	onceSerce.Do(func() {
		go runGenShortnoTask(ctx)
	})
	// }

	return &service{
		ctx:         ctx,
		appConfigDB: NewAppConfigDB(ctx),
		shortnoDB:   newShortnoDB(ctx),
	}
}

// GetAppConfig GetAppConfig
func (s *service) GetAppConfig() (*AppConfigResp, error) {
	appConfigM, err := s.appConfigDB.Query()
	if err != nil {
		return nil, err
	}

	return &AppConfigResp{
		RSAPublicKey:                   appConfigM.RSAPublicKey,
		Version:                        appConfigM.Version,
		SuperToken:                     appConfigM.SuperToken,
		SuperTokenOn:                   appConfigM.SuperTokenOn,
		WelcomeMessage:                 appConfigM.WelcomeMessage,
		NewUserJoinSystemGroup:         appConfigM.NewUserJoinSystemGroup,
		SearchByPhone:                  appConfigM.SearchByPhone,
		RegisterInviteOn:               appConfigM.RegisterInviteOn,
		SendWelcomeMessageOn:           appConfigM.SendWelcomeMessageOn,
		InviteSystemAccountJoinGroupOn: appConfigM.InviteSystemAccountJoinGroupOn,
		RegisterUserMustCompleteInfoOn: appConfigM.RegisterUserMustCompleteInfoOn,
		ChannelPinnedMessageMaxCount:   appConfigM.ChannelPinnedMessageMaxCount,
		ManequinControl:                appConfigM.ManequinControl,
		LotteryConfig:                  appConfigM.LotteryConfig,
		SearchMode:                     appConfigM.SearchMode,
		SearchShortNo:                  appConfigM.SearchShortNo,
		CreateGroup:                    appConfigM.CreateGroup,
		GroupRevokeOn:                  appConfigM.GroupRevokeOn,
		RevokeSecond:                   appConfigM.RevokeSecond,
		WebRoute:                       appConfigM.WebRoute,
		CanModifyApiUrl:                appConfigM.CanModifyApiUrl,
		ReportUrl:                      appConfigM.ReportUrl,
	}, nil
}

// 查询配置修改记录
func (s *service) GetConfigHistory(pageSize uint64, id uint64) ([]*ConfigHistoryModel, error) {
	history, err := s.appConfigDB.QueryConfigHistory(pageSize, id)
	if err != nil {
		return nil, err
	}
	return history, nil
}

// 插入配置修改记录
func (s *service) AddConfigHistory(model ConfigHistoryModel) error {
	err := s.appConfigDB.InsertConfigHistory(&model)
	if err != nil {
		return err
	}
	return err
}

func (s *service) GetShortno() (string, error) {

	s.shortnoLock.Lock() // 这里需要加锁 要不然多线程下会出现shortNo重复的问题
	defer s.shortnoLock.Unlock()

	shortnoM, err := s.shortnoDB.queryVail()
	if err != nil {
		return "", err
	}
	if shortnoM == nil {
		return "", errors.New("没有短编号可分配")
	}
	err = s.shortnoDB.updateLock(shortnoM.Shortno, 1)
	if err != nil {
		return "", err
	}
	return shortnoM.Shortno, nil
}

func (s *service) SetShortnoUsed(shortno string, business string) error {
	return s.shortnoDB.updateUsed(shortno, 1, business)
}

// 开启生成短编号任务
func runGenShortnoTask(ctx *config.Context) {
	shortnoDB := newShortnoDB(ctx)
	errorSleep := time.Second * 2
	for {
		count, err := shortnoDB.queryVailCount()
		if err != nil {
			time.Sleep(errorSleep)
			continue
		}
		if count < 10000 {
			shortnos := generateNums(ctx.GetConfig().ShortNo.NumLen, 100)
			if len(shortnos) > 0 {
				err = shortnoDB.inserts(shortnos)
				if err != nil {
					ctx.Error("添加短编号失败！", zap.Error(err))
				}
			}
		}
		time.Sleep(time.Second * 30)
	}
}

func generateNums(len int, count int) []string {
	var nums = make([]string, 0, count)
	rd := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := count; i > 0; i-- {
		var num = rd.Int63n(1e16)
		nums = append(nums, fmt.Sprintf("%016d", num)[0:len])
	}
	return nums

}

type AppConfigResp struct {
	RSAPublicKey                   string
	Version                        int
	SuperToken                     string
	SuperTokenOn                   int
	WelcomeMessage                 string // 登录欢迎语
	NewUserJoinSystemGroup         int    // 新用户是否加入系统群聊
	SearchByPhone                  int    // 是否可通过手机号搜索
	RegisterInviteOn               int    // 是否开启注册邀请
	SendWelcomeMessageOn           int    // 是否发送登录欢迎语
	InviteSystemAccountJoinGroupOn int    // 是否允许邀请系统账号进入群聊
	RegisterUserMustCompleteInfoOn int    // 是否要求注册用户必须填写完整信息
	ChannelPinnedMessageMaxCount   int    // 频道置顶消息最大数量
	ManequinControl                string // 假人控制
	LotteryConfig                  string // 抽奖配置
	SearchMode                     int
	SearchShortNo                  string
	CreateGroup                    int
	GroupRevokeOn                  int
	RevokeSecond                   int
	WebRoute                       string
	CanModifyApiUrl                int // 允许修改api地址
	ReportUrl                      string
}
