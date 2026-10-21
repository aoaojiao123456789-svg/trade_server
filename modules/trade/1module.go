package openapi

import (
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/config"
	"github.com/TangSengDaoDao/TangSengDaoDaoServerLib/pkg/register"
)

func init() {
	register.AddModule(func(ctx interface{}) register.Module {
		x := ctx.(*config.Context)
		api := New(x)
		return register.Module{
			Name: "openapi",
			SetupAPI: func() register.APIRouter {
				return api
			},
			Start: func() error {
				api.syncApiData()
				return nil
			},
			Stop: func() error {
				api.stopApiData()
				return nil
			},
		}
	})
}
