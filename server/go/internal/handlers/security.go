package handlers

import (
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
)

// IsLocalOrigin 判断请求来源是否为可信本地源。
// 与 CORS AllowOriginFunc 共用同一逻辑，保证"跨域拦截"与"CSRF 校验"口径一致。
// 仅按 hostname 匹配 127.0.0.1/localhost/wails.localhost，忽略 scheme 与端口，
// 以同时覆盖各平台 WebView 源（Windows/Linux 生产为 http://wails.localhost、
// macOS 生产为 wails://wails.localhost、dev 模式带端口），并天然拒绝
// wails.localhost.evil.com 这类子域欺骗与外网源。
func IsLocalOrigin(origin string) bool {
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	switch u.Hostname() {
	case "127.0.0.1", "localhost", "wails.localhost":
		return true
	}
	return false
}

// CSRFProtection 返回跨站请求伪造防护中间件，注册在 CORS 中间件之后。
//
// 防护模型（针对绑定 127.0.0.1 的本地后端）：
//   - 读方法（GET/HEAD）与预检（OPTIONS）不拦截，避免影响 <img>/<link> 等只读消费；
//   - 有 Origin 头的浏览器写请求必须同时满足：Origin 为可信本地源、且携带自定义头
//     X-Requested-With: XMLHttpRequest。跨域简单请求无法携带自定义头（需预检且 CORS
//     拒绝外源），因此可阻断恶意网页对本地后端的 CSRF；
//   - Origin 为 "null"（file:// 页面、沙箱 iframe）一律拒绝写请求；
//   - 无 Origin 头的请求视为非浏览器客户端（curl、桌面壳 Go 客户端、后端互调），放行。
func CSRFProtection() gin.HandlerFunc {
	return func(c *gin.Context) {
		method := c.Request.Method
		if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
			c.Next()
			return
		}

		origin := c.GetHeader("Origin")
		if origin == "" {
			// 非浏览器客户端，信任本地调用方
			c.Next()
			return
		}
		if origin == "null" || !IsLocalOrigin(origin) {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: untrusted origin"})
			c.Abort()
			return
		}
		if c.GetHeader("X-Requested-With") != "XMLHttpRequest" {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: missing csrf header"})
			c.Abort()
			return
		}
		c.Next()
	}
}

// setProxyCORS 为只读透传代理设置跨域响应头。
// 代理端点经 <img>/<link> 消费，浏览器渲染不需要 CORS；仅在请求方为可信本地源时反射
// Origin（供 fetch/canvas 等需要读取响应的场景），不再无条件返回 "*"。
func setProxyCORS(c *gin.Context) {
	origin := c.GetHeader("Origin")
	if origin != "" && IsLocalOrigin(origin) && origin != "null" {
		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Vary", "Origin")
	}
}
