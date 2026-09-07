package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type Page struct {
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
	Total    int64 `json:"total"`
}

type Resp struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Resp{Code: 0, Message: "ok", Data: data})
}

func OKPage(c *gin.Context, list any, page Page) {
	c.JSON(http.StatusOK, Resp{Code: 0, Message: "ok", Data: gin.H{
		"list":      list,
		"page":      page.Page,
		"page_size": page.PageSize,
		"total":     page.Total,
	}})
}

func Fail(c *gin.Context, code int, msg string) {
	c.JSON(code, Resp{Code: code, Message: msg})
}

func BadRequest(c *gin.Context, err any) {
	msg := "参数错误"
	switch v := err.(type) {
	case error:
		msg = v.Error()
	case string:
		msg = v
	}
	Fail(c, http.StatusBadRequest, msg)
}

func NotFound(c *gin.Context, msg string) {
	if msg == "" {
		msg = "资源不存在"
	}
	Fail(c, http.StatusNotFound, msg)
}

func ServerError(c *gin.Context, err error) {
	Fail(c, http.StatusInternalServerError, err.Error())
}

// QueryPage 解析分页参数
func QueryPage(c *gin.Context) (int, int) {
	page := 1
	size := 20
	if v := c.Query("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = n
		}
	}
	if v := c.Query("page_size"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
			size = n
		}
	}
	return page, size
}

func ParseID(c *gin.Context, key string) (uint, bool) {
	v := c.Param(key)
	id, err := strconv.ParseUint(v, 10, 64)
	if err != nil || id == 0 {
		return 0, false
	}
	return uint(id), true
}
