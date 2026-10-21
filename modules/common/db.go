package common

import (
	dbs "github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/db"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/util"
	"github.com/gocraft/dbr/v2"
	"strings"
)

type db struct {
	session *dbr.Session
}

func newDB(session *dbr.Session) *db {
	return &db{
		session: session,
	}
}

func (d *db) searchRevokeById(id int64) (int, int, string, error) {
	type appInfo struct {
		RevokeSecond  int    `db:"revoke_second"`
		GroupRevokeOn int    `db:"group_revoke_on"`
		WebRoute      string `db:"web_route"`
	}

	var data appInfo

	_, err := d.session.
		Select("revoke_second", "group_revoke_on, web_route").
		From("workplace_app").
		Where("id = ?", id).
		Limit(1).
		Load(&data)

	if err != nil {
		return 0, 0, "", err
	}

	return data.RevokeSecond, data.GroupRevokeOn, data.WebRoute, nil
}

// 添加版本升级
func (d *db) insertAppVersion(m *appVersionModel) (int64, error) {
	result, err := d.session.InsertInto("app_version").Columns(util.AttrToUnderscore(m)...).Record(m).Exec()
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	return id, err
}

// 更新版本升级
func (d *db) updateAppVersion(m *appVersionModel, id int64) error {
	_, err := d.session.Update("app_version").SetMap(map[string]interface{}{
		"app_version":      m.AppVersion,
		"update_desc":      m.UpdateDesc,
		"download_url":     m.DownloadURL,
		"hot_download_url": m.HotDownloadURL,
	}).Where("id=?", id).Exec()
	return err
}

// 删除版本升级
func (d *db) deleteAppVersion(id int64) error {
	_, err := d.session.DeleteFrom("app_version").Where("id=?", id).Exec()
	return err
}

// 查询某个系统的最新版本
func (d *db) queryNewVersion(os string, clientId int) (*appVersionModel, error) {
	var model *appVersionModel
	q := d.session.Select("*").From("app_version").Where("os=?", os).OrderDir("created_at", false).Limit(1)
	if clientId > 0 {
		q.Where("client_id =?", clientId)
	}
	_, err := q.Load(&model)
	return model, err
}

// 查询版本升级列表
func (d *db) queryAppVersionListWithPage(pageSize, page uint64, platforms []string) ([]*appVersionModel, error) {
	var models []*appVersionModel
	session := d.session.Select("*").From("app_version")

	// 多平台精准匹配 OS
	if len(platforms) > 0 {
		var conds []string
		var args []interface{}
		for _, p := range platforms {
			conds = append(conds, "OS = ?")
			args = append(args, p)
		}
		where := strings.Join(conds, " OR ")
		session = session.Where(where, args...)
	}

	// 分页 + 排序
	_, err := session.
		Offset((page-1)*pageSize).
		Limit(pageSize).
		OrderDir("updated_at", false). // false 表示 DESC
		Load(&models)

	return models, err
}

// 模糊查询用户数量
func (d *db) queryCount(platforms []string) (int64, error) {
	var count int64

	session := d.session.Select("COUNT(*)").From("app_version")

	if len(platforms) > 0 {
		var conds []string
		var args []interface{}
		for _, p := range platforms {
			conds = append(conds, "OS = ?")
			args = append(args, p)
		}
		where := strings.Join(conds, " OR ")
		session = session.Where(where, args...)
	}

	_, err := session.Load(&count)
	return count, err
}

// 查询所有背景图片
func (d *db) queryChatBgs() ([]*chatBgModel, error) {
	var models []*chatBgModel
	_, err := d.session.Select("*").From("chat_bg").Load(&models)
	return models, err
}

// 查询app模块
func (d *db) queryAppModule() ([]*appModuleModel, error) {
	var list []*appModuleModel
	_, err := d.session.Select("*").From("app_module").OrderDir("created_at", true).Load(&list)
	return list, err
}

// 查询某个app模块
func (d *db) queryAppModuleWithSid(sid string) (*appModuleModel, error) {
	var m *appModuleModel
	_, err := d.session.Select("*").From("app_module").Where("sid=?", sid).Load(&m)
	return m, err
}

// 新增app模块
func (d *db) insertAppModule(m *appModuleModel) (int64, error) {
	result, err := d.session.InsertInto("app_module").Columns(util.AttrToUnderscore(m)...).Record(m).Exec()
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	return id, err
}

// 修改app模块
func (d *db) updateAppModule(m *appModuleModel) error {
	_, err := d.session.Update("app_module").SetMap(map[string]interface{}{
		"name":   m.Name,
		"desc":   m.Desc,
		"status": m.Status,
	}).Where("id=?", m.Id).Exec()
	return err
}

// 配置修改记录

// 删除模块
func (d *db) deleteAppModule(sid string) error {
	_, err := d.session.DeleteFrom("app_module").Where("sid=?", sid).Exec()
	return err
}

type chatBgModel struct {
	Cover string // 封面
	Url   string // 图片地址
	IsSvg int    // 1 svg图片 0 普通图片
	dbs.BaseModel
}

type appVersionModel struct {
	AppVersion     string // app版本
	OS             string // android | ios
	IsForce        int    // 是否强制更新 1:是
	UpdateDesc     string // 更新说明
	DownloadURL    string // 下载地址
	HotDownloadURL string // 热更新地址
	Signature      string // 安装包签名
	ClientId       int    // 应用唯一ID
	AppName        string // 应用名称
	dbs.BaseModel
}

type appModuleModel struct {
	SID    string // 模块ID
	Name   string // 模块名称
	Desc   string // 介绍
	Status int    // 状态
	dbs.BaseModel
}
