package worker

import (
	myredis "GopherAI/common/redis"
	sessionDAO "GopherAI/dao/session"
	"GopherAI/model"
	sessionservice "GopherAI/service/session"
	"context"
	"log"
	"time"
)

const (
	// sessionRepairScanInterval 控制 repair worker 的扫描频率。
	sessionRepairScanInterval = 30 * time.Second
	sessionRepairBatchSize    = 100
)

// StartSessionRepairWorker 启动会话修复 worker。
// 当前它负责两类 repair：
// 1. Redis `pending_persist=true` 的热状态回放到 MySQL；
// 2. MySQL repair task 驱动的 Redis 热状态重建。
func StartSessionRepairWorker(ctx context.Context) error {
	ticker := time.NewTicker(sessionRepairScanInterval)
	defer ticker.Stop()

	runSessionRepairRound()

	for {
		select {
		case <-ctx.Done():
			log.Println("session repair worker stopped")
			return nil
		case <-ticker.C:
			runSessionRepairRound()
		}
	}
}

func runSessionRepairRound() {
	repairPendingPersistHotStates()
	rebuildDirtyHotStates()
}

func repairPendingPersistHotStates() {
	if !myredis.IsAvailable() {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	states, err := myredis.ListSessionHotStatesNeedingRepair(ctx, sessionRepairBatchSize)
	if err != nil {
		log.Printf("ListSessionHotStatesNeedingRepair failed: %v", err)
		return
	}
	if len(states) == 0 {
		return
	}

	for _, state := range states {
		if state == nil || !state.PendingPersist {
			continue
		}
		repaired, err := sessionservice.RepairPendingSessionPersistenceFromHotState(context.Background(), state)
		if err != nil {
			log.Printf("RepairPendingSessionPersistenceFromHotState failed: sessionID=%s err=%v", state.SessionID, err)
			continue
		}
		if repaired {
			log.Printf("Repaired pending session persistence from hot state: sessionID=%s", state.SessionID)
		}
	}
}

func rebuildDirtyHotStates() {
	tasks, err := sessionDAO.ListPendingSessionRepairTasks(sessionRepairBatchSize)
	if err != nil {
		log.Printf("ListPendingSessionRepairTasks failed: %v", err)
		return
	}
	if len(tasks) == 0 {
		return
	}

	for _, task := range tasks {
		if task.TaskType != model.SessionRepairTaskTypeHotStateRebuild {
			continue
		}

		err := sessionservice.RebuildSessionHotStateFromDatabase(context.Background(), task.SessionID, task.SelectionSignature)
		if err != nil {
			log.Printf("RebuildSessionHotStateFromDatabase failed: taskID=%d sessionID=%s err=%v", task.ID, task.SessionID, err)
			if markErr := sessionDAO.MarkSessionRepairTaskFailed(task.ID, err.Error()); markErr != nil {
				log.Printf("MarkSessionRepairTaskFailed failed: taskID=%d err=%v", task.ID, markErr)
			}
			continue
		}
		if err := sessionDAO.MarkSessionRepairTaskCompleted(task.ID); err != nil {
			log.Printf("MarkSessionRepairTaskCompleted failed: taskID=%d err=%v", task.ID, err)
			continue
		}
		log.Printf("Rebuilt session hot state from database: taskID=%d sessionID=%s version=%d", task.ID, task.SessionID, task.TargetVersion)
	}
}
