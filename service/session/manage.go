package session

import (
	"GopherAI/common/aihelper"
	"GopherAI/common/code"
	"GopherAI/common/mysql"
	myredis "GopherAI/common/redis"
	llmConfigDAO "GopherAI/dao/llm_config"
	sessionDAO "GopherAI/dao/session"
	sessionfolderDAO "GopherAI/dao/session_folder"
	"GopherAI/model"
	"context"
	"errors"
	"log"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	maxFolderNameLength   = 100
	maxSessionTitleLength = 100
)

// buildSessionInfo 把数据库 Session 转成前端展示结构，并尽量补齐绑定的模型配置信息。
// 如果配置已经被删除或暂时不可读，这里只保留 session 侧的基础字段，不把整个接口打失败。
func buildSessionInfo(sess model.Session) model.SessionInfo {
	info := buildSessionInfoWithConfig(sess, nil)
	if sess.LLMConfigID == nil {
		return info
	}

	config, err := llmConfigDAO.GetUserLLMConfigByID(sess.UserID, *sess.LLMConfigID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return info
		}
		return info
	}

	return buildSessionInfoWithConfig(sess, config)
}

func buildSessionInfoWithConfig(sess model.Session, config *model.UserLLMConfig) model.SessionInfo {
	info := model.SessionInfo{
		SessionID:   sess.ID,
		Title:       sess.Title,
		LLMConfigID: sess.LLMConfigID,
		ChatMode:    sess.ChatMode,
	}
	if sess.FolderID != nil {
		info.FolderID = *sess.FolderID
	}

	if config == nil {
		return info
	}

	info.LLMConfigName = config.Name
	info.Provider = config.Provider
	info.Model = config.Model
	if capability, ok := aihelper.GetProviderCapability(config.Provider); ok {
		info.ProviderCapability = &model.SessionProviderCapability{
			Provider:                 capability.Provider,
			DisplayName:              capability.DisplayName,
			IsImplemented:            capability.IsImplemented,
			SupportedChatModes:       append([]string(nil), capability.SupportedChatModes...),
			SupportsConfigTest:       capability.SupportsConfigTest,
			SupportsToolCalling:      capability.SupportsToolCalling,
			SupportsEmbedding:        capability.SupportsEmbedding,
			SupportsMultiModalFuture: capability.SupportsMultiModalFuture,
		}
	}
	return info
}

func GetSessionTree(userID int64, userName string) (*model.SessionTree, code.Code) {
	if userID <= 0 || strings.TrimSpace(userName) == "" {
		return nil, code.CodeInvalidToken
	}

	folders, err := sessionfolderDAO.GetFoldersByUserName(userName)
	if err != nil {
		log.Println("GetSessionTree GetFoldersByUserName error:", err)
		return nil, code.CodeServerBusy
	}

	sessions, err := sessionDAO.GetSessionsByUserName(userName)
	if err != nil {
		log.Println("GetSessionTree GetSessionsByUserName error:", err)
		return nil, code.CodeServerBusy
	}

	configsByID := make(map[int64]*model.UserLLMConfig)
	configIDSet := make(map[int64]struct{})
	configIDs := make([]int64, 0)
	for _, sess := range sessions {
		if sess.LLMConfigID == nil {
			continue
		}
		if _, exists := configIDSet[*sess.LLMConfigID]; exists {
			continue
		}
		configIDSet[*sess.LLMConfigID] = struct{}{}
		configIDs = append(configIDs, *sess.LLMConfigID)
	}
	if len(configIDs) > 0 {
		configs, err := llmConfigDAO.ListUserLLMConfigsByIDs(userID, configIDs)
		if err != nil {
			log.Println("GetSessionTree ListUserLLMConfigsByIDs error:", err)
			return nil, code.CodeServerBusy
		}
		for i := range configs {
			config := configs[i]
			configsByID[config.ID] = &config
		}
	}

	sessionMap := make(map[string][]model.SessionInfo, len(folders))
	ungrouped := make([]model.SessionInfo, 0)
	for _, sess := range sessions {
		var config *model.UserLLMConfig
		if sess.LLMConfigID != nil {
			config = configsByID[*sess.LLMConfigID]
		}
		info := buildSessionInfoWithConfig(sess, config)
		if sess.FolderID == nil || strings.TrimSpace(*sess.FolderID) == "" {
			ungrouped = append(ungrouped, info)
			continue
		}
		sessionMap[*sess.FolderID] = append(sessionMap[*sess.FolderID], info)
	}

	folderDetails := make([]model.SessionFolderDetail, 0, len(folders))
	for _, folder := range folders {
		folderDetails = append(folderDetails, model.SessionFolderDetail{
			ID:        folder.ID,
			Name:      folder.Name,
			Sessions:  sessionMap[folder.ID],
			CreatedAt: folder.CreatedAt,
			UpdatedAt: folder.UpdatedAt,
		})
	}

	return &model.SessionTree{
		Folders:           folderDetails,
		UngroupedSessions: ungrouped,
	}, code.CodeSuccess
}

func CreateFolder(userID int64, userName string, name string) (*model.SessionFolder, code.Code) {
	if userID <= 0 || strings.TrimSpace(userName) == "" {
		return nil, code.CodeInvalidToken
	}

	name = strings.TrimSpace(name)
	if name == "" || len(name) > maxFolderNameLength {
		return nil, code.CodeInvalidParams
	}

	existing, err := sessionfolderDAO.GetFolderByUserAndName(userID, name)
	if err != nil {
		log.Println("CreateFolder GetFolderByUserAndName error:", err)
		return nil, code.CodeServerBusy
	}
	if existing != nil {
		return nil, code.CodeInvalidParams
	}

	folder := &model.SessionFolder{
		ID:       uuid.NewString(),
		UserID:   userID,
		UserName: userName,
		Name:     name,
	}
	created, err := sessionfolderDAO.CreateFolder(folder)
	if err != nil {
		log.Println("CreateFolder CreateFolder error:", err)
		return nil, code.CodeServerBusy
	}
	return created, code.CodeSuccess
}

func RenameFolder(userID int64, folderID string, name string) code.Code {
	if userID <= 0 {
		return code.CodeInvalidToken
	}
	name = strings.TrimSpace(name)
	if strings.TrimSpace(folderID) == "" || name == "" || len(name) > maxFolderNameLength {
		return code.CodeInvalidParams
	}

	folder, code_ := ensureOwnedFolder(userID, folderID)
	if code_ != code.CodeSuccess {
		return code_
	}
	if folder.Name == name {
		return code.CodeSuccess
	}

	existing, err := sessionfolderDAO.GetFolderByUserAndName(userID, name)
	if err != nil {
		log.Println("RenameFolder GetFolderByUserAndName error:", err)
		return code.CodeServerBusy
	}
	if existing != nil && existing.ID != folderID {
		return code.CodeInvalidParams
	}

	if err := sessionfolderDAO.RenameFolder(userID, folderID, name); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return code.CodeRecordNotFound
		}
		log.Println("RenameFolder RenameFolder error:", err)
		return code.CodeServerBusy
	}
	return code.CodeSuccess
}

func DeleteFolder(userID int64, userName string, folderID string) code.Code {
	if userID <= 0 || strings.TrimSpace(userName) == "" {
		return code.CodeInvalidToken
	}
	if strings.TrimSpace(folderID) == "" {
		return code.CodeInvalidParams
	}

	if _, code_ := ensureOwnedFolder(userID, folderID); code_ != code.CodeSuccess {
		return code_
	}

	err := mysql.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.Session{}).
			Where("user_name = ? AND folder_id = ?", userName, folderID).
			Update("folder_id", nil).Error; err != nil {
			return err
		}
		result := tx.Where("id = ? AND user_id = ?", folderID, userID).Delete(&model.SessionFolder{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return code.CodeRecordNotFound
		}
		log.Println("DeleteFolder transaction error:", err)
		return code.CodeServerBusy
	}
	return code.CodeSuccess
}

func MoveSessionToFolder(userID int64, userName string, sessionID string, folderID string) code.Code {
	if userID <= 0 || strings.TrimSpace(userName) == "" {
		return code.CodeInvalidToken
	}
	if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(folderID) == "" {
		return code.CodeInvalidParams
	}

	if _, code_ := ensureOwnedSession(userName, sessionID); code_ != code.CodeSuccess {
		return code_
	}
	if _, code_ := ensureOwnedFolder(userID, folderID); code_ != code.CodeSuccess {
		return code_
	}

	folderIDCopy := folderID
	if err := sessionDAO.UpdateSessionFolder(userName, sessionID, &folderIDCopy); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return code.CodeRecordNotFound
		}
		log.Println("MoveSessionToFolder UpdateSessionFolder error:", err)
		return code.CodeServerBusy
	}
	return code.CodeSuccess
}

func RemoveSessionFromFolder(userName string, sessionID string) code.Code {
	if strings.TrimSpace(userName) == "" || strings.TrimSpace(sessionID) == "" {
		return code.CodeInvalidParams
	}

	if _, code_ := ensureOwnedSession(userName, sessionID); code_ != code.CodeSuccess {
		return code_
	}
	if err := sessionDAO.UpdateSessionFolder(userName, sessionID, nil); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return code.CodeRecordNotFound
		}
		log.Println("RemoveSessionFromFolder UpdateSessionFolder error:", err)
		return code.CodeServerBusy
	}
	return code.CodeSuccess
}

func RenameSession(userName string, sessionID string, title string) code.Code {
	if strings.TrimSpace(userName) == "" {
		return code.CodeInvalidToken
	}
	if strings.TrimSpace(sessionID) == "" {
		return code.CodeInvalidParams
	}
	title = strings.TrimSpace(title)
	if title == "" || len(title) > maxSessionTitleLength {
		return code.CodeInvalidParams
	}

	if _, code_ := ensureOwnedSession(userName, sessionID); code_ != code.CodeSuccess {
		return code_
	}
	if err := sessionDAO.UpdateSessionTitle(userName, sessionID, title); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return code.CodeRecordNotFound
		}
		log.Println("RenameSession UpdateSessionTitle error:", err)
		return code.CodeServerBusy
	}
	return code.CodeSuccess
}

func DeleteSession(userName string, sessionID string) code.Code {
	if strings.TrimSpace(userName) == "" {
		return code.CodeInvalidToken
	}
	if strings.TrimSpace(sessionID) == "" {
		return code.CodeInvalidParams
	}

	if _, code_ := ensureOwnedSession(userName, sessionID); code_ != code.CodeSuccess {
		return code_
	}
	if err := sessionDAO.SoftDeleteSession(userName, sessionID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return code.CodeRecordNotFound
		}
		log.Println("DeleteSession SoftDeleteSession error:", err)
		return code.CodeServerBusy
	}

	// 会话删除成功后，需要同步清理进程内 helper、Redis 热状态和仍在运行的流式任务。
	aihelper.GetGlobalManager().RemoveAIHelper(userName, sessionID)
	if _, stopCode := globalActiveStreamRegistry.stop(userName, sessionID); stopCode != code.CodeSuccess && stopCode != code.CodeChatNotRunning {
		log.Println("DeleteSession stop active stream error:", stopCode)
	}
	if err := myredis.DeleteSessionHotState(context.Background(), sessionID); err != nil {
		log.Println("DeleteSession DeleteSessionHotState error:", err)
	}
	if err := myredis.DeleteSessionLock(context.Background(), sessionID); err != nil {
		log.Println("DeleteSession DeleteSessionLock error:", err)
	}
	if err := myredis.DeleteSessionOwnerLease(context.Background(), sessionID); err != nil {
		log.Println("DeleteSession DeleteSessionOwnerLease error:", err)
	}
	if err := sessionDAO.DeleteSessionRepairTasksBySessionID(sessionID); err != nil {
		log.Println("DeleteSession DeleteSessionRepairTasksBySessionID error:", err)
	}
	return code.CodeSuccess
}

func ensureOwnedFolder(userID int64, folderID string) (*model.SessionFolder, code.Code) {
	folder, err := sessionfolderDAO.GetFolderByID(folderID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, code.CodeRecordNotFound
		}
		log.Println("ensureOwnedFolder GetFolderByID error:", err)
		return nil, code.CodeServerBusy
	}
	if folder.UserID != userID {
		return nil, code.CodeForbidden
	}
	return folder, code.CodeSuccess
}
