package worker

import (
	"GopherAI/dao/session"
	"context"
	"log"
	"time"
)

const (
	// sessionPersistenceScanInterval 控制会话持久化水位补偿扫描频率。
	// 这里先保持和现有补偿 worker 同级别的保守节奏，避免新逻辑一上来就高频扫表。
	sessionPersistenceScanInterval = 30 * time.Second
	// sessionPersistenceBatchSize 限制单次最多处理的落后会话数量，避免积压时一次性放大 DB 压力。
	sessionPersistenceBatchSize = 100
)

// StartSessionPersistenceCompensationWorker 启动“会话持久化水位补偿 worker”。
// 它不负责重放 MQ，只负责把数据库里已经满足条件的 session_version 推进到 persisted_version。
func StartSessionPersistenceCompensationWorker(ctx context.Context) error {
	ticker := time.NewTicker(sessionPersistenceScanInterval)
	defer ticker.Stop()

	compensateSessionPersistedVersions()

	for {
		select {
		case <-ctx.Done():
			log.Println("session persistence compensation worker stopped")
			return nil
		case <-ticker.C:
			compensateSessionPersistedVersions()
		}
	}
}

// compensateSessionPersistedVersions 执行一次水位补偿扫描。
// 这里按 persisted_version+1 逐步推进，避免跳过中间尚未满足条件的版本。
func compensateSessionPersistedVersions() {
	sessions, err := session.ListSessionsWithPersistenceLag(sessionPersistenceBatchSize)
	if err != nil {
		log.Printf("ListSessionsWithPersistenceLag failed: %v", err)
		return
	}
	if len(sessions) == 0 {
		return
	}

	for _, sess := range sessions {
		for nextVersion := sess.PersistedVersion + 1; nextVersion <= sess.Version; nextVersion++ {
			advanced, err := session.TryAdvancePersistedVersionIfReady(sess.ID, nextVersion)
			if err != nil {
				log.Printf("TryAdvancePersistedVersionIfReady failed: sessionID=%s version=%d err=%v", sess.ID, nextVersion, err)
				break
			}
			if !advanced {
				break
			}
			log.Printf("Advanced persisted_version: sessionID=%s version=%d", sess.ID, nextVersion)
		}
	}
}
