package session

import (
	"GopherAI/common/aihelper"
	"GopherAI/common/code"
	"GopherAI/common/observability"
	myredis "GopherAI/common/redis"
	messageDAO "GopherAI/dao/message"
	sessionDAO "GopherAI/dao/session"
	sessionFolderDAO "GopherAI/dao/session_folder"
	"GopherAI/model"
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// buildAIConfig 缂佺喍绔撮弸鍕偓鐘衬侀崹瀣灥婵瀵查崣鍌涙殶閿涘矂浼╅崗宥呮倱濮濄儱鎷板ù浣哥础闁炬崘鐭鹃柌宥咁槻閹疯壈顥婇柊宥囩枂閵?
func buildAIConfig(userName string, userID int64) map[string]interface{} {
	return map[string]interface{}{
		"apiKey":   "your-api-key", // TODO: 閸氬海鐢绘禒搴ㄥ帳缂冾喕鑵戣箛鍐╁灗閻滎垰顣ㄩ崣姗€鍣虹拠璇插絿
		"username": userName,       // MCP 缁涘膩閸ㄥ娓剁憰浣虹叀闁挸缍嬮崜宥囨暏閹寸柉闊╂禒?
		"userID":   userID,         // RAG 濡€崇€烽棁鈧憰?userID 閺屻儴顕楅弬鍥︽
	}
}

// ensureOwnedSession 缂佺喍绔撮弽锟犵崣娴兼俺鐦介弰顖氭儊鐎涙ê婀敍灞间簰閸欏﹥妲搁崥锕€鐫樻禍搴＄秼閸撳秶鏁ら幋鏋偓?
// 閺佺増宓佹惔鎾圭鐠愶絼绱扮拠婵堟埂閻╃鎷伴弶鍐鏉堝湱鏅敍灞肩瑝閼宠棄褰ч棃鐘虹箥鐞涘本妞?helper 閸掋倖鏌囨导姘崇樈閺勵垰鎯侀崥鍫熺《閵?
func ensureOwnedSession(userName string, sessionID string) (*model.Session, code.Code) {
	sess, err := sessionDAO.GetSessionByID(sessionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, code.CodeRecordNotFound
		}
		log.Println("ensureOwnedSession GetSessionByID error:", err)
		return nil, code.CodeServerBusy
	}

	if sess.UserName != userName {
		return nil, code.CodeForbidden
	}

	return sess, code.CodeSuccess
}

func ensureOwnedFolder(userID int64, folderID int64) (*model.SessionFolder, code.Code) {
	folder, err := sessionFolderDAO.GetSessionFolderByID(folderID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, code.CodeRecordNotFound
		}
		log.Println("ensureOwnedFolder GetSessionFolderByID error:", err)
		return nil, code.CodeServerBusy
	}
	if folder.UserID != userID {
		return nil, code.CodeForbidden
	}
	return folder, code.CodeSuccess
}

// persistSummaryIfChanged 閸欘亜婀幗妯款洣绾喖鐤勯崣妯哄閺冭泛娲栭崘娆愭殶閹诡喖绨遍敍宀勪缉閸忓秵鐦℃潪顔款嚞濮瑰倿鍏橀弴瀛樻煀 session閵?
func persistSummaryIfChanged(sessionID string, beforeSummary string, beforeCount int, helper *aihelper.AIHelper) code.Code {
	afterSummary, afterCount := helper.GetSummaryState()
	if beforeSummary == afterSummary && beforeCount == afterCount {
		return code.CodeSuccess
	}

	if err := sessionDAO.UpdateSessionSummary(sessionID, afterSummary, afterCount); err != nil {
		log.Println("persistSummaryIfChanged UpdateSessionSummary error:", err)
		return code.CodeServerBusy
	}

	return code.CodeSuccess
}

// persistHelperHotState 閹?helper 瑜版挸澧犻惃鍕氦闁插繒鍎归悩鑸碘偓浣告彥閻撗冨晸閸?Redis閵?
// 鏉╂瑩鍣烽弫鍛壈娑撳秵濡?Redis 瑜版挾婀￠惄鍛婄爱閿涘本澧嶆禒銉ュ晸婢惰精瑙﹂崣顏囶唶閺冦儱绻旈敍灞肩瑝闂冪粯鏌囨稉鏄忎喊婢垛晠鎽肩捄顖樷偓?
func persistHelperHotState(ctx context.Context, helper *aihelper.AIHelper) {
	if helper == nil {
		return
	}

	if err := myredis.SaveSessionHotState(ctx, helper.ExportHotState()); err != nil {
		observability.RecordRedisHotStateSaveFail()
		logSessionTrace(ctx, "hot_state_save_fail", "err=%v", err)
		log.Println("persistHelperHotState SaveSessionHotState error:", err)
	}
}

// syncHelperWithDatabase 閸︺劉鈧粎鎴风紒顓熺厙娑擃亙绱扮拠婵嗗閳ユ繃鐗庨崙鍡樻拱閸?helper 娑撳孩鏆熼幑顔肩氨濞戝牊浼呴悩鑸碘偓浣碘偓?
// 鐟欏嫬鍨弰顖ょ窗
// 1. DB 閺堚偓閺傜増绉烽幁顖氬嚒缂佸繐婀張顒€婀撮敍姘愁嚛閺勫孩婀伴崷鎷屽殾鐏忔垳绗夐拃钘夋倵娴?DB閿涘奔绻氶悾娆愭拱閸?buffer閵?
// 2. 閺堫剙婀撮張鈧弬鐗堢Х閹垰鍑￠拃钘夌氨閿涘奔绲?DB 閺堚偓閺傜増绉烽幁顖欑瑝閸︺劍婀伴崷甯窗鐠囧瓨妲戦張顒€婀寸紓鐑樼Х閹垽绱濋幐?DB 鐞涖儱娲栭妴?
// 3. 閺堫剙婀撮張鈧弬鐗堢Х閹垱婀拃钘夌氨閿涙俺顕╅弰搴㈡拱閸︽澘褰查懗浠嬵暙閸忓牞绱遍崣顏呮箒瑜?DB 閺堚偓閺傜増绉烽幁顖欑瘍鐡掑﹨绻冩禍鍡樻拱閸︾増娓堕崥搴濈閺夆€冲嚒閹镐椒绠欓崠鏍ㄧХ閹垱妞傞敍灞惧閸嬫矮绻氱€瑰牓鍣搁弸鍕┾偓?
func syncHelperWithDatabase(sessionID string, helper *aihelper.AIHelper) code.Code {
	latestDBMessage, err := messageDAO.GetLatestMessageBySessionID(sessionID)
	if err != nil {
		if messageDAO.IsMessageNotFoundError(err) {
			return code.CodeSuccess
		}
		log.Println("syncHelperWithDatabase GetLatestMessageBySessionID error:", err)
		return code.CodeServerBusy
	}

	// DB 閺堚偓閺傜増绉烽幁顖氬嚒閸︺劍婀伴崷甯礉鐠囧瓨妲戦張顒€婀村▽鈩冩箒閽€钘夋倵娴滃孩鏆熼幑顔肩氨閿涘奔绗夐棁鈧憰浣瑰瑏 DB 閸欏秴鎮滅憰鍡欐磰閺堫剙婀?buffer閵?
	if helper.HasMessageKey(latestDBMessage.MessageKey) {
		dbMessageCount, err := messageDAO.GetMessageCountBySessionID(sessionID)
		if err != nil {
			log.Println("syncHelperWithDatabase GetMessageCountBySessionID error:", err)
			return code.CodeServerBusy
		}

		// 閸楀厖濞囬垾婊勬付閸氬簼绔撮弶鈩冪Х閹垪鈧繂顕稉濠佺啊閿涘奔绡冩稉宥勫敩鐞涖劋鑵戦梻瀛樼梾閺堝宸遍崣锝冣偓?
		// 婵″倹鐏夐張顒€婀村鍙夊瘮娑斿懎瀵插☉鍫熶紖閺佺増鐦?DB 鐏忔埊绱濇禒宥囧姧鐟曚礁浠涙稉鈧▎鈥茬箽鐎瑰牆顕鎰┾偓?
		if int64(helper.GetPersistedMessageCount()) < dbMessageCount {
			dbMessages, err := messageDAO.GetMessagesBySessionID(sessionID)
			if err != nil {
				log.Println("syncHelperWithDatabase hole reconcile GetMessagesBySessionID error:", err)
				return code.CodeServerBusy
			}
			helper.ReconcileMessages(dbMessages)
		}
		return code.CodeSuccess
	}

	localLatestMessage := helper.GetLatestMessage()
	if localLatestMessage == nil {
		dbMessages, err := messageDAO.GetMessagesBySessionID(sessionID)
		if err != nil {
			log.Println("syncHelperWithDatabase GetMessagesBySessionID error:", err)
			return code.CodeServerBusy
		}
		helper.LoadMessages(dbMessages)
		return code.CodeSuccess
	}

	localLatestPersistedMessage := helper.GetLatestPersistedMessage()
	if localLatestPersistedMessage == nil {
		// 閺堫剙婀撮崣顏呮箒鐏忔碍婀拃钘夌氨閻ㄥ嫭绉烽幁顖涙閿涘奔绗夐懗鍊燁唨 DB 閸欏秴鎮滅憰鍡欐磰閺堫剙婀撮妴?
		return code.CodeSuccess
	}

	localLatestExistsInDB, err := messageDAO.ExistsMessageKey(localLatestMessage.MessageKey)
	if err != nil {
		log.Println("syncHelperWithDatabase ExistsMessageKey latest local error:", err)
		return code.CodeServerBusy
	}

	// 閺堫剙婀撮張鈧弬鐗堢Х閹垰鍑＄紒蹇撴躬 DB閿涘奔绲?DB 閺堚偓閺傜増绉烽幁顖氬祱娑撳秴婀張顒€婀撮敍宀冾嚛閺勫孩婀伴崷?helper 缂傝桨绨￠崥搴ｇ敾濞戝牊浼呴妴?
	if localLatestExistsInDB {
		dbMessages, err := messageDAO.GetMessagesBySessionID(sessionID)
		if err != nil {
			log.Println("syncHelperWithDatabase full DB reload error:", err)
			return code.CodeServerBusy
		}
		helper.ReconcileMessages(dbMessages)
		return code.CodeSuccess
	}

	// 鐠ф澘鍩屾潻娆撳櫡鐠囧瓨妲戦敍姘拱閸︾増娓堕弬鐗堢Х閹垰鐨婚張顏囨儰鎼存搫绱濋張顒€婀?buffer 妫板棗鍘涙禍?DB閵?
	// 鏉╂瑦妞傞崣顏呮箒瑜?DB 閺堚偓閺傜増绉烽幁顖氬嚒缂佸繋绗夐弰顖椻偓婊勬拱閸︾増娓堕崥搴濈閺夆€冲嚒閹镐椒绠欓崠鏍ㄧХ閹垪鈧繃妞傞敍?
	// 閹靛秷顕╅弰搴濊⒈鏉堢懓褰查懗浠嬪厴閸氬嫯鍤滅紓杞扮啊娑撯偓闁劌鍨庨敍宀勬付鐟曚礁浠涙穱婵嗙暓闁插秵鐎妴?
	if localLatestPersistedMessage.MessageKey == latestDBMessage.MessageKey {
		return code.CodeSuccess
	}

	dbMessages, err := messageDAO.GetMessagesBySessionID(sessionID)
	if err != nil {
		log.Println("syncHelperWithDatabase final reconcile GetMessagesBySessionID error:", err)
		return code.CodeServerBusy
	}
	helper.ReconcileMessages(dbMessages)
	return code.CodeSuccess
}

// getOrSyncHelperWithHistory 娴兼ê鍘涙径宥囨暏瑜版挸澧犳潻娑氣柤娑擃厾娈?helper閿?
// 婵″倹鐏?helper 娑撳秴鐡ㄩ崷顭掔礉鐏忓彉绮犻弫鐗堝祦鎼存挸娲栭弨鎯у坊閸欏弶绉烽幁顖ょ幢
// 婵″倹鐏?helper 瀹告彃鐡ㄩ崷顭掔礉鐏忓崬婀紒褏鐢绘导姘崇樈閸撳秴浠涙稉鈧▎鈩冩拱閸?DB 閻ㄥ嫬鐣ㄩ崗銊ヮ嚠姒绘劑鈧?
func getOrSyncHelperWithHistory(ctx context.Context, userName string, sess *model.Session, modelType string) (*aihelper.AIHelper, code.Code) {
	if !aihelper.IsSupportedModelType(modelType) {
		return nil, code.CodeInvalidParams
	}

	sessionID := sess.ID
	manager := aihelper.GetGlobalManager()
	if helper, exists := manager.GetAIHelper(userName, sessionID); exists {
		observability.RecordHelperRecover(observability.HelperRecoverSourceProcess)
		helper.SetSummaryState(sess.ContextSummary, sess.SummaryMessageCount)
		if code_ := syncHelperWithDatabase(sessionID, helper); code_ != code.CodeSuccess {
			return nil, code_
		}
		return helper, code.CodeSuccess
	}

	helper, err := manager.GetOrCreateAIHelper(userName, sessionID, modelType, buildAIConfig(userName, sess.UserID))
	if err != nil {
		log.Println("getOrCreateHelperWithHistory GetOrCreateAIHelper error:", err)
		return nil, code.AIModelFail
	}

	// helper 妫ｆ牗顐兼潻娑樺弳瑜版挸澧犳潻娑氣柤閺冭绱濋棁鈧憰浣风矤閺佺増宓佹惔鎾虫礀閺€鐐Х閹垰宸婚崣璇х礉閹垹顦叉导姘崇樈娑撳﹣绗呴弬鍥モ偓?
	msgs, err := messageDAO.GetMessagesBySessionID(sessionID)
	if err != nil {
		log.Println("getOrCreateHelperWithHistory GetMessagesBySessionID error:", err)
		return nil, code.CodeServerBusy
	}
	if len(msgs) > 0 {
		helper.LoadMessages(msgs)
	}

	// 缁楊兛绨╂潪顔煎磳缁狙囧櫡瀵洖鍙?Redis 閻戭厾濮搁幀浣告彥閻撗嶇礉娴ｅ棜绻栭柌灞肩矝閻掓湹绻氶悾?DB 閸ョ偞鏂佹担婊€璐熼崗婊冪俺閻喓娴夐幁銏狀槻閵?
	// 娑旂喎姘ㄩ弰顖濐嚛閿涙艾鍘涙穱婵婄槈閳ユ粏鍤︾亸鎴ｅ厴閹垹顦查垾婵撶礉閸愬秶鏁?Redis 閻戭厼鎻╅悡褎濡搁張鈧潻鎴犵崶閸欙絿濮搁幀浣剿夐崶鐐存降閵?
	hotState, err := myredis.GetSessionHotState(ctx, sessionID)
	if err != nil {
		observability.RecordRedisHotStateLookup(false)
		logSessionTrace(ctx, "hot_state_read_fail", "err=%v", err)
		log.Println("getOrCreateHelperWithHistory GetSessionHotState error:", err)
	} else if hotState != nil {
		observability.RecordRedisHotStateLookup(true)
		observability.RecordHelperRecover(observability.HelperRecoverSourceRedis)
		helper.LoadHotState(hotState)
	} else {
		observability.RecordRedisHotStateLookup(false)
	}
	observability.RecordHelperRecover(observability.HelperRecoverSourceDB)
	helper.SetSummaryState(sess.ContextSummary, sess.SummaryMessageCount)
	if code_ := syncHelperWithDatabase(sessionID, helper); code_ != code.CodeSuccess {
		return nil, code_
	}

	manager.SetAIHelper(userName, sessionID, helper)
	return helper, code.CodeSuccess
}

// GetUserSessionsByUserName 娴犲孩鏆熼幑顔肩氨鐠囪褰囨导姘崇樈閸掓銆冮妴?
// 娴兼俺鐦介崚妤勩€冪仦鐐扮艾娑撴艾濮熼惇鐔烘祲閿涘奔绗夐懗鎴掔贩鐠ф牞绻樼粙瀣敶 helper 閻ㄥ嫮鏁撻崨钘夋噯閺堢喆鈧?
func GetUserSessionsByUserName(userName string) ([]model.SessionInfo, error) {
	sessions, err := sessionDAO.GetSessionsByUserName(userName)
	if err != nil {
		return nil, err
	}

	sessionInfos := make([]model.SessionInfo, 0, len(sessions))
	for _, sess := range sessions {
		sessionInfos = append(sessionInfos, model.SessionInfo{
			SessionID: sess.ID,
			Title:     sess.Title,
		})
	}

	return sessionInfos, nil
}

// CreateSessionAndSendMessage 閸掓稑缂撻弬棰佺窗鐠囨繂鑻熼崣鎴︹偓浣侯儑娑撯偓閺夆剝绉烽幁顖樷偓?
func CreateSessionAndSendMessage(ctx context.Context, userName string, userID int64, userQuestion string, modelType string) (string, string, code.Code) {
	requestStart := time.Now()
	if !allowChatRateLimit(ctx, userName) {
		observability.RecordRequest("create_sync", modelType, false, time.Since(requestStart))
		return "", "", code.CodeTooManyRequests
	}

	newSession := &model.Session{
		ID:       uuid.New().String(),
		UserName: userName,
		UserID:   userID,
		// 閸忓牅绻氶幐浣哄箛閺堝楠囬崫浣筋嚔娑斿绱伴悽銊╊浕閺夛繝妫舵０妯圭稊娑撹桨绱扮拠婵囩垼妫版ǜ鈧?
		Title: userQuestion,
	}
	createdSession, err := sessionDAO.CreateSession(newSession)
	if err != nil {
		log.Println("CreateSessionAndSendMessage CreateSession error:", err)
		observability.RecordRequest("create_sync", modelType, false, time.Since(requestStart))
		return "", "", code.CodeServerBusy
	}

	ctx, trace := newSessionTrace(ctx, "create_sync", createdSession.ID, modelType)
	logSessionTrace(ctx, "start", "user=%s", userName)

	result := withSessionExecutionGuard(ctx, createdSession.ID, func() codeExecutorResult {
		helper, code_ := getOrSyncHelperWithHistory(ctx, userName, createdSession, modelType)
		if code_ != code.CodeSuccess {
			return codeExecutorResult{code: code_}
		}

		beforeSummary, beforeCount := helper.GetSummaryState()
		aiResponse, err := helper.GenerateResponse(userName, ctx, userQuestion)
		if err != nil {
			log.Println("CreateSessionAndSendMessage GenerateResponse error:", err)
			return codeExecutorResult{code: code.AIModelFail}
		}
		if code_ = persistSummaryIfChanged(createdSession.ID, beforeSummary, beforeCount, helper); code_ != code.CodeSuccess {
			return codeExecutorResult{code: code_}
		}

		persistHelperHotState(ctx, helper)
		return codeExecutorResult{
			code:       code.CodeSuccess,
			aiResponse: aiResponse.Content,
		}
	})
	if result.err != nil {
		log.Println("CreateSessionAndSendMessage execution guard error:", result.err)
		logSessionTrace(ctx, "failed", "err=%v", result.err)
		observability.RecordRequest("create_sync", modelType, false, time.Since(requestStart))
		return "", "", code.CodeServerBusy
	}
	if result.busy {
		logSessionTrace(ctx, "busy", "detail=distributed_lock_busy")
		observability.RecordRequest("create_sync", modelType, false, time.Since(requestStart))
		return "", "", code.CodeTooManyRequests
	}
	if result.code != code.CodeSuccess {
		logSessionTrace(ctx, "failed", "code=%d", result.code)
		observability.RecordRequest("create_sync", modelType, false, time.Since(requestStart))
		return "", "", result.code
	}
	logSessionTrace(ctx, "success", "response_chars=%d request_id=%s", len(result.aiResponse), trace.RequestID)
	observability.RecordRequest("create_sync", modelType, true, time.Since(requestStart))

	return createdSession.ID, result.aiResponse, code.CodeSuccess
}

// CreateStreamSessionOnly 閸欘亜鍨卞杞扮窗鐠囨繐绱濇稉宥呭絺闁焦绉烽幁顖樷偓?
// 濞翠礁绱￠崷鐑樻珯閸忓牅绗呴崣?sessionID閿涘苯鍟€瀵偓婵瀵旂紒顓熷腹濞翠降鈧?
func CreateStreamSessionOnly(userName string, userID int64, userQuestion string) (string, code.Code) {
	newSession := &model.Session{
		ID:       uuid.New().String(),
		UserName: userName,
		UserID:   userID,
		Title:    userQuestion,
	}
	createdSession, err := sessionDAO.CreateSession(newSession)
	if err != nil {
		log.Println("CreateStreamSessionOnly CreateSession error:", err)
		return "", code.CodeServerBusy
	}
	return createdSession.ID, code.CodeSuccess
}

// StreamMessageToExistingSession 閸氭垵鍑￠張澶夌窗鐠囨繂褰傞柅浣风閺夆剝绁﹀蹇旂Х閹垬鈧?
func StreamMessageToExistingSession(ctx context.Context, userName string, sessionID string, userQuestion string, modelType string, writer http.ResponseWriter) code.Code {
	requestStart := time.Now()
	ctx, _ = newSessionTrace(ctx, "chat_stream", sessionID, modelType)
	logSessionTrace(ctx, "start", "user=%s", userName)
	observability.RecordStreamActiveDelta(1)
	defer observability.RecordStreamActiveDelta(-1)
	if !allowChatRateLimit(ctx, userName) {
		observability.RecordRequest("chat_stream", modelType, false, time.Since(requestStart))
		return code.CodeTooManyRequests
	}

	flusher, ok := writer.(http.Flusher)
	if !ok {
		log.Println("StreamMessageToExistingSession: streaming unsupported")
		observability.RecordRequest("chat_stream", modelType, false, time.Since(requestStart))
		return code.CodeServerBusy
	}

	sess, code_ := ensureOwnedSession(userName, sessionID)
	if code_ != code.CodeSuccess {
		observability.RecordRequest("chat_stream", modelType, false, time.Since(requestStart))
		return code_
	}

	// 娴犲氦绻栭柌灞界磻婵铔嬮弬鎵畱閳ユ粈绱扮拠婵囧⒔鐞涘奔绻氶幎?+ 閻戭厾濮搁幀浣告礀閸愭瑢鈧繈鎽肩捄顖樷偓?
	result := withSessionExecutionGuard(ctx, sessionID, func() codeExecutorResult {
		helper, code_ := getOrSyncHelperWithHistory(ctx, userName, sess, modelType)
		if code_ != code.CodeSuccess {
			return codeExecutorResult{code: code_}
		}

		cb := func(msg string) {
			// SSE 閸楀繗顔呯憰浣圭湴濮ｅ繋閲滈悧鍥唽闁姤瀵?data 鐞涘矁绶崙鐚寸礉楠炶泛婀В蹇旑偧閸愭瑥鍙嗛崥搴ｇ彌閸?flush閵?
			_, err := writer.Write([]byte("data: " + msg + "\n\n"))
			if err != nil {
				log.Println("StreamMessageToExistingSession Write error:", err)
				return
			}
			flusher.Flush()
		}

		beforeSummary, beforeCount := helper.GetSummaryState()
		if _, err := helper.StreamResponse(userName, ctx, cb, userQuestion); err != nil {
			log.Println("StreamMessageToExistingSession StreamResponse error:", err)
			if ctx.Err() != nil {
				observability.RecordStreamDisconnect()
			}
			return codeExecutorResult{code: code.AIModelFail}
		}
		if code_ = persistSummaryIfChanged(sessionID, beforeSummary, beforeCount, helper); code_ != code.CodeSuccess {
			return codeExecutorResult{code: code_}
		}

		persistHelperHotState(ctx, helper)
		return codeExecutorResult{code: code.CodeSuccess}
	})
	if result.err != nil {
		log.Println("StreamMessageToExistingSession execution guard error:", result.err)
		logSessionTrace(ctx, "failed", "err=%v", result.err)
		observability.RecordRequest("chat_stream", modelType, false, time.Since(requestStart))
		return code.CodeServerBusy
	}
	if result.busy {
		logSessionTrace(ctx, "busy", "detail=distributed_lock_busy")
		observability.RecordRequest("chat_stream", modelType, false, time.Since(requestStart))
		return code.CodeTooManyRequests
	}
	if result.code != code.CodeSuccess {
		logSessionTrace(ctx, "failed", "code=%d", result.code)
		observability.RecordRequest("chat_stream", modelType, false, time.Since(requestStart))
		return result.code
	}

	if _, err := writer.Write([]byte("data: [DONE]\n\n")); err != nil {
		log.Println("StreamMessageToExistingSession write DONE error:", err)
		if ctx.Err() != nil {
			observability.RecordStreamDisconnect()
		}
		observability.RecordRequest("chat_stream", modelType, false, time.Since(requestStart))
		return code.AIModelFail
	}
	flusher.Flush()
	logSessionTrace(ctx, "success", "detail=stream_done")
	observability.RecordRequest("chat_stream", modelType, true, time.Since(requestStart))
	return code.CodeSuccess
}

// CreateStreamSessionAndSendMessage 閸掓稑缂撴导姘崇樈閸氬海鐝涢崡瀹犺泲濞翠礁绱￠崶鐐差槻閵?
func CreateStreamSessionAndSendMessage(ctx context.Context, userName string, userID int64, userQuestion string, modelType string, writer http.ResponseWriter) (string, code.Code) {
	sessionID, code_ := CreateStreamSessionOnly(userName, userID, userQuestion)
	if code_ != code.CodeSuccess {
		return "", code_
	}

	code_ = StreamMessageToExistingSession(ctx, userName, sessionID, userQuestion, modelType, writer)
	if code_ != code.CodeSuccess {
		return sessionID, code_
	}

	return sessionID, code.CodeSuccess
}

// ChatSend 閸氭垵鍑￠張澶夌窗鐠囨繂褰傞柅浣告倱濮濄儲绉烽幁顖樷偓?
func ChatSend(ctx context.Context, userName string, sessionID string, userQuestion string, modelType string) (string, code.Code) {
	requestStart := time.Now()
	ctx, _ = newSessionTrace(ctx, "chat_sync", sessionID, modelType)
	logSessionTrace(ctx, "start", "user=%s", userName)
	if !allowChatRateLimit(ctx, userName) {
		observability.RecordRequest("chat_sync", modelType, false, time.Since(requestStart))
		return "", code.CodeTooManyRequests
	}

	sess, code_ := ensureOwnedSession(userName, sessionID)
	if code_ != code.CodeSuccess {
		observability.RecordRequest("chat_sync", modelType, false, time.Since(requestStart))
		return "", code_
	}

	// 閺備即鎽肩捄顖氭躬鏉╂瑩鍣烽幓鎰鏉╂柨娲栭敍灞芥倵闂堛垻娈戦弮褔鈧槒绶禒鍛箽閻ｆ瑤缍旂€靛湱鍙庨敍灞界杽闂勫懍绗夋导姘晙鐠ф澘鍩岄妴?
	result := withSessionExecutionGuard(ctx, sessionID, func() codeExecutorResult {
		helper, code_ := getOrSyncHelperWithHistory(ctx, userName, sess, modelType)
		if code_ != code.CodeSuccess {
			return codeExecutorResult{code: code_}
		}

		beforeSummary, beforeCount := helper.GetSummaryState()
		aiResponse, err := helper.GenerateResponse(userName, ctx, userQuestion)
		if err != nil {
			log.Println("ChatSend GenerateResponse error:", err)
			return codeExecutorResult{code: code.AIModelFail}
		}
		if code_ = persistSummaryIfChanged(sessionID, beforeSummary, beforeCount, helper); code_ != code.CodeSuccess {
			return codeExecutorResult{code: code_}
		}

		persistHelperHotState(ctx, helper)
		return codeExecutorResult{
			code:       code.CodeSuccess,
			aiResponse: aiResponse.Content,
		}
	})
	if result.err != nil {
		log.Println("ChatSend execution guard error:", result.err)
		logSessionTrace(ctx, "failed", "err=%v", result.err)
		observability.RecordRequest("chat_sync", modelType, false, time.Since(requestStart))
		return "", code.CodeServerBusy
	}
	if result.busy {
		logSessionTrace(ctx, "busy", "detail=distributed_lock_busy")
		observability.RecordRequest("chat_sync", modelType, false, time.Since(requestStart))
		return "", code.CodeTooManyRequests
	}
	if result.code != code.CodeSuccess {
		logSessionTrace(ctx, "failed", "code=%d", result.code)
		observability.RecordRequest("chat_sync", modelType, false, time.Since(requestStart))
		return "", result.code
	}
	logSessionTrace(ctx, "success", "response_chars=%d", len(result.aiResponse))
	observability.RecordRequest("chat_sync", modelType, true, time.Since(requestStart))
	return result.aiResponse, code.CodeSuccess
}

// GetChatHistory 娴犲孩鏆熼幑顔肩氨鐠囪褰囬崢鍡楀蕉濞戝牊浼呴妴?
// 閸樺棗褰堕幒銉ュ經瀵缚鐨熼崣顖涗划婢跺秵鈧冩嫲娑撯偓閼峰瓨鈧嶇礉閸ョ姵顒濊箛鍛淬€忔禒銉︽殶閹诡喖绨辨稉顓犳畱濞戝牊浼呯拋鏉跨秿娑撳搫鍣妴?
func GetChatHistory(userName string, sessionID string) ([]model.History, code.Code) {
	if _, code_ := ensureOwnedSession(userName, sessionID); code_ != code.CodeSuccess {
		return nil, code_
	}

	messages, err := messageDAO.GetMessagesBySessionID(sessionID)
	if err != nil {
		log.Println("GetChatHistory GetMessagesBySessionID error:", err)
		return nil, code.CodeServerBusy
	}

	history := make([]model.History, 0, len(messages))
	for _, msg := range messages {
		// 閻╁瓨甯存担璺ㄦ暏閹镐椒绠欓崠鏍畱 IsUser 鐎涙顔岄敍宀勪缉閸忓秴鍟€闁俺绻冩總鍥т紦娴ｅ秶瀵藉ù瀣Х閹垵闊╂禒濮愨偓?
		history = append(history, model.History{
			IsUser:  msg.IsUser,
			Content: msg.Content,
			Status:  msg.Status,
		})
	}

	return history, code.CodeSuccess
}

// ChatStreamSend 閸氭垵鍑￠張澶夌窗鐠囨繂褰傞柅浣圭ウ瀵繑绉烽幁顖樷偓?
func ChatStreamSend(ctx context.Context, userName string, sessionID string, userQuestion string, modelType string, writer http.ResponseWriter) code.Code {
	return StreamMessageToExistingSession(ctx, userName, sessionID, userQuestion, modelType, writer)
}

// RenameSession renames a session owned by the current user.
func RenameSession(userName string, sessionID string, title string) code.Code {
	if _, code_ := ensureOwnedSession(userName, sessionID); code_ != code.CodeSuccess {
		return code_
	}

	title = strings.TrimSpace(title)
	if title == "" || len(title) > 100 {
		return code.CodeInvalidParams
	}

	if err := sessionDAO.UpdateSessionTitle(sessionID, title); err != nil {
		log.Println("RenameSession UpdateSessionTitle error:", err)
		return code.CodeServerBusy
	}

	return code.CodeSuccess
}

// DeleteSession soft-deletes a session and clears in-memory helper state.
func DeleteSession(userName string, sessionID string) code.Code {
	if _, code_ := ensureOwnedSession(userName, sessionID); code_ != code.CodeSuccess {
		return code_
	}

	if err := sessionDAO.DeleteSession(sessionID); err != nil {
		log.Println("DeleteSession DeleteSession error:", err)
		return code.CodeServerBusy
	}

	aihelper.GetGlobalManager().RemoveAIHelper(userName, sessionID)
	return code.CodeSuccess
}

func GetSessionTree(userID int64, userName string) (*model.SessionListTreeResponse, code.Code) {
	folders, err := sessionFolderDAO.GetSessionFoldersByUserID(userID)
	if err != nil {
		log.Println("GetSessionTree GetSessionFoldersByUserID error:", err)
		return nil, code.CodeServerBusy
	}

	sessions, err := sessionDAO.GetSessionsByUserName(userName)
	if err != nil {
		log.Println("GetSessionTree GetSessionsByUserName error:", err)
		return nil, code.CodeServerBusy
	}

	folderMap := make(map[int64]*model.SessionFolderInfo, len(folders))
	response := &model.SessionListTreeResponse{
		Folders:           make([]model.SessionFolderInfo, 0, len(folders)),
		UngroupedSessions: make([]model.SessionTreeItem, 0),
	}

	for _, folder := range folders {
		response.Folders = append(response.Folders, model.SessionFolderInfo{
			ID:       folder.ID,
			Name:     folder.Name,
			Sessions: make([]model.SessionTreeItem, 0),
		})
		folderMap[folder.ID] = &response.Folders[len(response.Folders)-1]
	}

	for _, sess := range sessions {
		item := model.SessionTreeItem{SessionID: sess.ID, Title: sess.Title}
		if sess.FolderID != nil {
			if folderInfo, ok := folderMap[*sess.FolderID]; ok {
				folderInfo.Sessions = append(folderInfo.Sessions, item)
				continue
			}
		}
		response.UngroupedSessions = append(response.UngroupedSessions, item)
	}

	return response, code.CodeSuccess
}

func CreateSessionFolder(userID int64, userName string, name string) (*model.SessionFolderInfo, code.Code) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 100 {
		return nil, code.CodeInvalidParams
	}

	folder, err := sessionFolderDAO.CreateSessionFolder(&model.SessionFolder{
		UserID:   userID,
		UserName: userName,
		Name:     name,
	})
	if err != nil {
		log.Println("CreateSessionFolder CreateSessionFolder error:", err)
		return nil, code.CodeServerBusy
	}

	return &model.SessionFolderInfo{
		ID:       folder.ID,
		Name:     folder.Name,
		Sessions: make([]model.SessionTreeItem, 0),
	}, code.CodeSuccess
}

func RenameSessionFolder(userID int64, folderID int64, name string) code.Code {
	if _, code_ := ensureOwnedFolder(userID, folderID); code_ != code.CodeSuccess {
		return code_
	}

	name = strings.TrimSpace(name)
	if name == "" || len(name) > 100 {
		return code.CodeInvalidParams
	}

	if err := sessionFolderDAO.UpdateSessionFolderName(folderID, name); err != nil {
		log.Println("RenameSessionFolder UpdateSessionFolderName error:", err)
		return code.CodeServerBusy
	}

	return code.CodeSuccess
}

func DeleteSessionFolder(userID int64, folderID int64) code.Code {
	if _, code_ := ensureOwnedFolder(userID, folderID); code_ != code.CodeSuccess {
		return code_
	}

	if err := sessionDAO.ClearSessionFolderIDByFolderID(folderID); err != nil {
		log.Println("DeleteSessionFolder ClearSessionFolderIDByFolderID error:", err)
		return code.CodeServerBusy
	}
	if err := sessionFolderDAO.DeleteSessionFolder(folderID); err != nil {
		log.Println("DeleteSessionFolder DeleteSessionFolder error:", err)
		return code.CodeServerBusy
	}

	return code.CodeSuccess
}

func MoveSessionToFolder(userID int64, userName string, sessionID string, folderID int64) code.Code {
	if _, code_ := ensureOwnedSession(userName, sessionID); code_ != code.CodeSuccess {
		return code_
	}
	if _, code_ := ensureOwnedFolder(userID, folderID); code_ != code.CodeSuccess {
		return code_
	}

	if err := sessionDAO.UpdateSessionFolderID(sessionID, &folderID); err != nil {
		log.Println("MoveSessionToFolder UpdateSessionFolderID error:", err)
		return code.CodeServerBusy
	}

	return code.CodeSuccess
}

func RemoveSessionFromFolder(userName string, sessionID string) code.Code {
	if _, code_ := ensureOwnedSession(userName, sessionID); code_ != code.CodeSuccess {
		return code_
	}

	if err := sessionDAO.UpdateSessionFolderID(sessionID, nil); err != nil {
		log.Println("RemoveSessionFromFolder UpdateSessionFolderID error:", err)
		return code.CodeServerBusy
	}

	return code.CodeSuccess
}
