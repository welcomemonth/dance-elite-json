package router

import (
	"path/filepath"

	"github.com/gin-gonic/gin"
	"github.com/meetmonth/dance-elite/internal/middleware"
	"github.com/meetmonth/dance-elite/internal/pkg/httpx"
	"github.com/meetmonth/dance-elite/internal/pkg/token"
	"github.com/meetmonth/dance-elite/internal/service"
	"github.com/meetmonth/dance-elite/internal/store"
)

// Deps 是路由层所需的全部外部依赖，由组合根（main）一次性注入。
// 业务模块（T2+）落地后在此追加 service 字段，例如：
//
//	Events        *service.EventService
//	Registrations *service.RegistrationService
//
// 测试时构造 Deps 的 fake 实现即可用 httptest 对接口做测试。
type Deps struct {
	Stores        *store.Store
	Auth          *service.AuthService
	AdminAuth     *service.AdminAuthService
	Seasons       *service.SeasonService
	Events        *service.EventService
	PublicEvents  *service.PublicEventService
	Registrations *service.RegistrationService
	Materials     *service.MaterialService
	Orders        *service.OrderService
	Payments      *service.PaymentService
	Refunds       *service.RefundService
	Waitlists     *service.WaitlistService
	CSTickets     *service.CSTicketService
	ManualRegs    *service.ManualRegistrationService
	Tokens        *token.Signer

	// UploadDir 是上传文件物理根目录（配置 UPLOAD_DIR）。非空时把
	// /static/materials、/static/avatars 映射到磁盘对应目录（T13）。
	UploadDir string
}

// New 构建并返回一个已注册全局中间件的 gin.Engine。
//
// 业务路由（小程序 /api/v1、后台 /api/v1/admin、H5 /h5）将在 T2+ 按模块注册，
// 各模块 handler 只接收自己依赖的 service（依赖从 Deps 中取出后按组传参，
// 如 registerAdminRoutes(api.Group("/admin"), deps.Events, deps.Auth)）。
//
// 返回 engine 而非空值，是为了让测试能用 httptest.NewRecorder
// 直接对路由做接口级测试（无需启动真实端口）。
func New(deps Deps) *gin.Engine {
	engine := gin.New()

	// 全局中间件：Recovery -> RequestID -> AccessLog -> 统一错误渲染
	engine.Use(middleware.Recovery(), middleware.RequestID(), middleware.AccessLog(), httpx.HandleErrors())

	api := engine.Group("/api/v1")

	// 小程序端认证（T4）
	registerAuthRoutes(api, deps.Auth, deps.Tokens)

	// 小程序端赛事查询（T7）：可选登录（携带有效 token 返回"已报名"角标）。
	if deps.Tokens != nil && deps.PublicEvents != nil {
		registerEventQueryRoutes(api.Group("", middleware.OptionalUserAuth(deps.Tokens)), deps.PublicEvents)
	}

	// 小程序端报名下单（T9）：需登录，按用户限流 1 次/5 秒。
	if deps.Tokens != nil && deps.Registrations != nil {
		registerRegistrationRoutes(api, deps.Registrations, deps.Tokens)
	}

	// 小程序端参赛资料上传（T13）：需登录，仅本人有效报名可修改。
	if deps.Tokens != nil && deps.Materials != nil {
		registerMaterialRoutes(api, deps.Materials, deps.Tokens)
	}

	// 上传文件静态访问（T13）：/static/materials → UPLOAD_DIR，
	// /static/avatars → UPLOAD_DIR/avatars；关闭目录列表。
	if deps.UploadDir != "" {
		engine.StaticFS("/static/materials", gin.Dir(deps.UploadDir, false))
		engine.StaticFS("/static/avatars", gin.Dir(filepath.Join(deps.UploadDir, "avatars"), false))
	}

	// 小程序端订单支付（T10）：需登录，预下单按用户限流 10 次/分钟。
	if deps.Tokens != nil && deps.Orders != nil {
		registerOrderRoutes(api, deps.Orders, deps.Tokens)
	}

	// 支付回调与订单状态查询（T11）：回调按微信验签鉴权，状态查询需登录。
	if deps.Tokens != nil && deps.Payments != nil {
		registerPaymentRoutes(api, deps.Payments, deps.Tokens)
	}

	// 微信退款结果回调（T15）：公共接口，按验签鉴权。
	if deps.Refunds != nil {
		registerRefundNotifyRoute(api, deps.Refunds)
	}

	// 小程序端候补队列（T14）：需登录，仅本人可查看/取消。
	if deps.Tokens != nil && deps.Waitlists != nil {
		registerWaitlistRoutes(api, deps.Waitlists, deps.Tokens)
	}

	// 后台端认证（T5）
	registerAdminAuthRoutes(api, deps.AdminAuth, deps.Tokens)

	// 后台端业务（T6+）：/api/v1/admin 下全部需要 admin token。
	if deps.Tokens != nil {
		admin := api.Group("/admin", middleware.AdminAuth(deps.Tokens))
		if deps.Seasons != nil && deps.Events != nil {
			registerAdminSeasonRoutes(admin, deps.Seasons)
			registerAdminEventRoutes(admin, deps.Events)
		}
		// 后台候补管理（T14）：列表 cs 可读，写操作仅 admin / super_admin。
		if deps.Waitlists != nil {
			registerAdminWaitlistRoutes(admin, deps.Waitlists)
		}
		// 后台退款工单（T15）：登记 cs/admin 均可，处理仅 admin / super_admin。
		if deps.Refunds != nil {
			registerAdminRefundRoutes(admin, deps.Refunds)
		}
		// 后台客服工单（T16）：登记/列表 cs/admin 均可，流转仅 admin / super_admin。
		if deps.CSTickets != nil {
			registerAdminCSTicketRoutes(admin, deps.CSTickets)
		}
		// 后台人工报名（T16）：仅 admin / super_admin。
		if deps.ManualRegs != nil {
			registerAdminManualRegistrationRoutes(admin, deps.ManualRegs)
		}
	}

	// 健康检查
	api.GET("/ping", func(c *gin.Context) {
		httpx.OK(c, gin.H{"message": "pong"})
	})

	return engine
}
