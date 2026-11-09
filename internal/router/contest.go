package router

import (
	"io"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/meetmonth/dance-elite/internal/model"
	"github.com/meetmonth/dance-elite/web"
)

// serveEmbeddedFile 返回一个从嵌入前端资源中服务单个文件的 handler。
//
// 不使用 c.FileFromFS：其底层 http.FileServer 会对以 /index.html
// 结尾的请求做 301 重定向（localRedirect("./")），导致 /contest 无法直接返回页面。
// http.ServeContent 不做该重定向，且自带 Content-Type 推断与 304 缓存。
func serveEmbeddedFile(path string) gin.HandlerFunc {
	return func(c *gin.Context) {
		f, err := web.FS.Open(path)
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		defer f.Close()

		info, err := f.Stat()
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}

		rs, ok := f.(io.ReadSeeker)
		if !ok {
			c.Status(http.StatusInternalServerError)
			return
		}
		http.ServeContent(c.Writer, c.Request, info.Name(), info.ModTime(), rs)
	}
}

// registerContestRoutes 注册音频浏览/播放页面的路由。
//
// 注意：这些路由注册在引擎根上（/contest 与 /api/contest/*），
// 而非现有的 /api/v1 group，以匹配前端硬编码的请求路径。
func (r *Router) registerContestRoutes() {
	// 前端页面与静态资源（通过 go:embed 嵌入）
	// 页面位于 /contest，其相对引用 style.css/app.js/cover.jpg 会解析到根路径
	r.engine.GET("/contest", serveEmbeddedFile("index.html"))
	r.engine.GET("/style.css", serveEmbeddedFile("style.css"))
	r.engine.GET("/app.js", serveEmbeddedFile("app.js"))
	r.engine.GET("/cover.jpg", serveEmbeddedFile("cover.jpg"))

	// 音频 API
	api := r.engine.Group("/api/contest")
	api.GET("/audios", r.getContestAudios)
	// 同时注册 HEAD，供前端用 HEAD 探测文件是否存在（区分 404 与播放失败）
	api.GET("/audio/:filename", r.getContestAudio)
	api.HEAD("/audio/:filename", r.getContestAudio)
}

// getContestAudios 返回内存中的音频列表，统一响应格式。
func (r *Router) getContestAudios(c *gin.Context) {
	c.JSON(http.StatusOK, model.Response{
		Code:    0,
		Message: "success",
		Data:    r.audioManager.List(),
	})
}

// getContestAudio 返回音频文件，支持 HTTP Range（拖动进度依赖 Range）。
//
// 文件名经启动时扫描建立的白名单校验后才取文件，杜绝路径穿越；
// 文件在运行期间被删除时返回 404。
func (r *Router) getContestAudio(c *gin.Context) {
	filename := c.Param("filename")
	path, ok := r.audioManager.Path(filename)
	if !ok {
		c.Status(http.StatusNotFound)
		return
	}

	f, err := os.Open(path)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	http.ServeContent(c.Writer, c.Request, info.Name(), info.ModTime(), f)
}
