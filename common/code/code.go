package code

// Code 表示统一业务返回码。
// 这里继续沿用原项目“1000 成功、2xxx 参数和业务校验、3xxx 权限、4xxx 系统侧、5xxx AI 侧”的分层方式，
// 只是补充了 stop / timeout / cancelled 这些中断处理需要的新语义。
type Code int64

const (
	CodeSuccess Code = 1000

	CodeInvalidParams    Code = 2001
	CodeUserExist        Code = 2002
	CodeUserNotExist     Code = 2003
	CodeInvalidPassword  Code = 2004
	CodeNotMatchPassword Code = 2005
	CodeInvalidToken     Code = 2006
	CodeNotLogin         Code = 2007
	CodeInvalidCaptcha   Code = 2008
	CodeRecordNotFound   Code = 2009
	CodeIllegalPassword  Code = 2010
	CodeTooManyRequests  Code = 2011
	// CodeChatNotRunning 表示用户请求 stop 时，当前会话并没有正在运行的流式任务。
	CodeChatNotRunning Code = 2012

	CodeForbidden Code = 3001

	CodeServerBusy Code = 4001
	// CodeRequestTimeout 表示请求在业务规定时间内没有完成，被超时机制中断。
	CodeRequestTimeout Code = 4002

	AIModelNotFind    Code = 5001
	AIModelCannotOpen Code = 5002
	AIModelFail       Code = 5003
	// AIModelCancelled 表示模型执行链路被主动取消，而不是模型本身执行失败。
	AIModelCancelled Code = 5004

	TTSFail Code = 6001
)

var msg = map[Code]string{
	CodeSuccess: "success",

	CodeInvalidParams:    "请求参数错误",
	CodeUserExist:        "用户已存在",
	CodeUserNotExist:     "用户不存在",
	CodeInvalidPassword:  "用户名或密码错误",
	CodeNotMatchPassword: "两次密码不一致",
	CodeInvalidToken:     "无效的Token",
	CodeNotLogin:         "用户未登录",
	CodeInvalidCaptcha:   "验证码错误",
	CodeRecordNotFound:   "记录不存在",
	CodeIllegalPassword:  "密码不合法",
	CodeTooManyRequests:  "请求过于频繁",
	CodeChatNotRunning:   "当前没有正在执行的对话任务",

	CodeForbidden: "权限不足",

	CodeServerBusy:     "服务繁忙",
	CodeRequestTimeout: "请求超时",

	AIModelNotFind:    "模型不存在",
	AIModelCannotOpen: "无法打开模型",
	AIModelFail:       "模型运行失败",
	AIModelCancelled:  "对话已取消",
	TTSFail:           "语音服务失败",
}

func (code Code) Code() int64 {
	return int64(code)
}

func (code Code) Msg() string {
	if m, ok := msg[code]; ok {
		return m
	}
	return msg[CodeServerBusy]
}
