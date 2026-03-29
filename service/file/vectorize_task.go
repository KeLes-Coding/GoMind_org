package file

import (
	"GopherAI/dao"
	"GopherAI/model"
	"GopherAI/service/task"
	"context"
	"fmt"
)

// publishVectorizeTaskWithCompensation 统一封装“投递向量化任务 + 维护补偿状态”的逻辑。
// 之所以抽成单独方法，而不是让每个上传/重试/重建索引调用点各自写一遍，
// 是因为这里有一套必须保持一致的状态机：
// 1. 投递成功时，要把 vector_task_queued 标成 true；
// 2. 投递失败时，要把 vector_task_queued 标成 false，并记录失败原因；
// 3. 所有入口都必须按同一规则维护，补偿 worker 才能可靠地扫到真正需要补偿的数据。
func publishVectorizeTaskWithCompensation(ctx context.Context, fileDAO *dao.FileDAO, fileAsset *model.FileAsset) error {
	if fileAsset == nil {
		return fmt.Errorf("file asset is required")
	}

	if err := task.PublishVectorizeTask(ctx, fileAsset.ID, fileAsset.Version); err != nil {
		// MQ 发布失败时，文件上传本身仍然可以视为成功，
		// 但必须明确把“当前版本尚未成功入队”的状态写回数据库，
		// 否则后面的补偿 worker 无法准确识别这条资产。
		if markErr := fileDAO.MarkVectorTaskPending(ctx, fileAsset.ID, fileAsset.Version, err.Error()); markErr != nil {
			return fmt.Errorf("publish vectorize task failed: %w; mark pending failed: %v", err, markErr)
		}
		return err
	}

	// 只有在 broker 发布确认已经成功返回后，才能把当前版本标成“已成功入队”。
	if err := fileDAO.MarkVectorTaskQueued(ctx, fileAsset.ID, fileAsset.Version); err != nil {
		return fmt.Errorf("mark vectorize task queued failed: %w", err)
	}
	fileAsset.VectorTaskQueued = true
	fileAsset.VectorTaskErrMsg = ""
	return nil
}
