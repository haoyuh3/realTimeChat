package main

import (
	"fmt"
	"github.com/gin-gonic/gin" // 导入 Gin 框架
	"net/http"                 // 导入 Go 内置的 http 包
)

func getUserHandler(c *gin.Context) {
	name := c.Param("name")
	c.String(http.StatusOK, "你好, %s!", name)
}

func helloWorldHandler(c *gin.Context) {
	c.String(http.StatusOK, "Hello World!")
}
func setupRouter() *gin.Engine {
	r := gin.Default()

	r.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "pong!",
		})
	})

	r.GET("/", helloWorldHandler)

	r.GET("user/:name", getUserHandler)

	return r
}
func main() {
	// 创建一个默认的 Gin 引擎 (它包含了 Logger 和 Recovery 中间件)
	// 如果你想要一个完全干净的引擎，可以使用 gin.New()
	r := setupRouter()

	// 4. 启动 HTTP 服务器
	// 默认情况下，它会监听 8080 端口 (r.Run() 等同于 r.Run(":8080"))
	_ = r.Run()
	fmt.Println("start server")

}
