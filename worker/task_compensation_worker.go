package worker

import (
	"GopherAI/common/mysql"
	"GopherAI/dao"
	"GopherAI/service/task"
	"context"
	"log"
	"time"
)

const (
	// pendingVectorTaskScanInterval 控制补偿扫描的周期。
	// 这里先取一个较保守的 30 秒：
	// 1. 足够快，能让偶发 MQ 抖动后的文件尽快恢复处理；
	// 2. 又不会因为扫表过于频繁，给数据库带来不必要压力。
	pendingVectorTaskScanInterval = 30 * time.Second
	// pendingVectorTaskBatchSize 限制单次补偿扫描最多处理多少条记录。
	// 这样即使历史上积压了一批未入队文件，也不会让一次扫描把系统瞬时流量打爆。
	pendingVectorTaskBatchSize = 100
)

// StartVectorTaskCompensationWorker 启动“向量化任务补偿 worker”。
// 这个 worker 不处理文件内容本身，它只做一件事：
// 找出已经 uploaded、但当前版本还没成功入队的文件，然后重新投递向量化任务。
func StartVectorTaskCompensationWorker(ctx context.Context) error {
	fileDAO := dao.NewFileDAO(mysql.DB)
	ticker := time.NewTicker(pendingVectorTaskScanInterval)
	defer ticker.Stop()

	// 进程启动后先立即扫一次，避免必须等到第一个 ticker 周期才开始补偿。
	compensatePendingVectorTasks(ctx, fileDAO)

	for {
		select {
		case <-ctx.Done():
			log.Println("vector task compensation worker stopped")
			return nil
		case <-ticker.C:
			compensatePendingVectorTasks(ctx, fileDAO)
		}
	}
}

// compensatePendingVectorTasks 执行一次补偿扫描。
// 这里的策略是“尽量补偿，但不放大故障”：
// 1. 单条记录失败时，只记录日志并继续处理下一条；
// 2. 每条记录都会重新校验版本号条件，避免旧版本误覆盖新版本；
// 3. 发布成功后立刻把 vector_task_queued 标成 true，避免重复补偿。
func compensatePendingVectorTasks(ctx context.Context, fileDAO *dao.FileDAO) {
	files, err := fileDAO.ListFilesPendingVectorTask(ctx, pendingVectorTaskBatchSize)
	if err != nil {
		log.Printf("ListFilesPendingVectorTask failed: %v", err)
		return
	}
	if len(files) == 0 {
		return
	}

	for _, fileAsset := range files {
		if err := task.PublishVectorizeTask(ctx, fileAsset.ID, fileAsset.Version); err != nil {
			// 补偿失败时继续维持“未成功入队”状态，并更新最近一次失败原因。
			if markErr := fileDAO.MarkVectorTaskPending(ctx, fileAsset.ID, fileAsset.Version, err.Error()); markErr != nil {
				log.Printf("MarkVectorTaskPending failed after publish error: fileID=%s version=%d err=%v markErr=%v", fileAsset.ID, fileAsset.Version, err, markErr)
				continue
			}
			log.Printf("Compensate vector task failed: fileID=%s version=%d err=%v", fileAsset.ID, fileAsset.Version, err)
			continue
		}

		if err := fileDAO.MarkVectorTaskQueued(ctx, fileAsset.ID, fileAsset.Version); err != nil {
			log.Printf("MarkVectorTaskQueued failed after compensation publish: fileID=%s version=%d err=%v", fileAsset.ID, fileAsset.Version, err)
			continue
		}
		log.Printf("Compensated vector task successfully: fileID=%s version=%d", fileAsset.ID, fileAsset.Version)
	}
}
