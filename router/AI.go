package router

import (
	"GopherAI/controller/session"
	"GopherAI/controller/tts"
	"GopherAI/middleware/ratelimit"

	"github.com/gin-gonic/gin"
)

func AIRouter(r *gin.RouterGroup) {

	// 聊天相关接口
	{
		r.GET("/chat/sessions", session.GetUserSessionsByUserName)
		r.GET("/chat/observability", session.GetAIObservability)
		r.POST("/chat/send-new-session", ratelimit.LimitChatByIP(), ratelimit.LimitChatByUser(), session.CreateSessionAndSendMessage)
		r.POST("/chat/send", ratelimit.LimitChatByIP(), ratelimit.LimitChatByUser(), session.ChatSend)
		r.POST("/chat/history", session.ChatHistory)
		r.GET("/chat/session-tree", session.GetSessionTree)
		r.POST("/chat/folder/create", session.CreateFolder)
		r.POST("/chat/folder/rename", session.RenameFolder)
		r.POST("/chat/folder/delete", session.DeleteFolder)
		r.POST("/chat/session/move", session.MoveSession)
		r.POST("/chat/session/remove-from-folder", session.RemoveSessionFromFolder)
		r.POST("/chat/session/rename", session.RenameSession)
		r.POST("/chat/session/delete", session.DeleteSession)
		// stop 接口用于主动终止当前会话的流式生成。
		// 它和 send-stream 配套使用，目标是把“只能被动断开连接”升级成“显式停止当前回答”。
		r.POST("/chat/stop", session.StopStream)

		// TTS相关接口
		r.POST("/chat/tts", tts.CreateTTSTask)
		r.GET("/chat/tts/query", tts.QueryTTSTask)

		r.POST("/chat/send-stream-new-session", ratelimit.LimitChatByIP(), ratelimit.LimitChatByUser(), session.CreateStreamSessionAndSendMessage)
		r.POST("/chat/send-stream", ratelimit.LimitChatByIP(), ratelimit.LimitChatByUser(), session.ChatStreamSend)
	}

}
