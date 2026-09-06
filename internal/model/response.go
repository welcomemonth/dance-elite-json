package model

// 统一响应
type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

/*
code,   含义
0,   成功
40001,参数错误
40401,资源不存在
50001,服务器内部错误
*/
