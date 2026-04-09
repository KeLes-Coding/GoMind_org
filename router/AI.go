package router

import (
	"GopherAI/controller/session"
	"GopherAI/controller/tts"
	"GopherAI/middleware/ratelimit"

	"github.com/gin-gonic/gin"
)

func AIRouter(r *gin.RouterGroup) {

	// Chat-related routes.
	{
		r.GET("/configs", session.ListLLMConfigs)
		r.GET("/configs/meta", session.GetLLMConfigMeta)
		r.GET("/configs/:id", session.GetLLMConfig)
		r.POST("/configs", session.CreateLLMConfig)
		r.POST("/configs/test", session.TestLLMConfig)
		r.PUT("/configs/:id", session.UpdateLLMConfig)
		r.DELETE("/configs/:id", session.DeleteLLMConfig)
		r.POST("/configs/:id/default", session.SetDefaultLLMConfig)

		r.GET("/chat/sessions", session.GetUserSessionsByUserName)
		r.GET("/chat/session/:id", session.GetSessionInfo)
		r.GET("/chat/session-tree", session.GetSessionTree)
		r.GET("/chat/observability", session.GetAIObservability)
		r.POST("/chat/send-new-session", ratelimit.LimitChatByIP(), ratelimit.LimitChatByUser(), session.CreateSessionAndSendMessage)
		r.POST("/chat/send", ratelimit.LimitChatByIP(), ratelimit.LimitChatByUser(), session.ChatSend)
		r.POST("/chat/history", session.ChatHistory)
		// stop is paired with streaming chat so the client can explicitly stop generation.
		r.POST("/chat/stop", session.StopStream)
		r.POST("/chat/resume-stream", session.ResumeStream)
		r.POST("/chat/folder/create", session.CreateFolder)
		r.POST("/chat/folder/rename", session.RenameFolder)
		r.POST("/chat/folder/delete", session.DeleteFolder)
		r.POST("/chat/session/move", session.MoveSessionToFolder)
		r.POST("/chat/session/remove-from-folder", session.RemoveSessionFromFolder)
		r.POST("/chat/session/rename", session.RenameSession)
		r.POST("/chat/session/delete", session.DeleteSession)

		// TTS routes.
		r.POST("/chat/tts", tts.CreateTTSTask)
		r.GET("/chat/tts/query", tts.QueryTTSTask)

		r.POST("/chat/send-stream-new-session", ratelimit.LimitChatByIP(), ratelimit.LimitChatByUser(), session.CreateStreamSessionAndSendMessage)
		r.POST("/chat/send-stream", ratelimit.LimitChatByIP(), ratelimit.LimitChatByUser(), session.ChatStreamSend)
	}

}
