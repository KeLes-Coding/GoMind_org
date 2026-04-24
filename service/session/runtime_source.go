package session

// SessionRuntimeSource 表示当前请求最终采用了哪一种运行态恢复来源。
// 第一阶段先把来源枚举显式化，后续阶段再把它接入恢复主链路和观测打点。
type SessionRuntimeSource string

const (
	// SessionRuntimeSourceProcessEphemeral 表示当前请求复用了进程内短生命周期执行对象。
	// 它只代表执行优化，不代表跨请求恢复真相源。
	SessionRuntimeSourceProcessEphemeral SessionRuntimeSource = "process_ephemeral"
	// SessionRuntimeSourceRedisHotState 表示当前请求基于 Redis 热状态重建执行器。
	SessionRuntimeSourceRedisHotState SessionRuntimeSource = "redis_hot_state"
	// SessionRuntimeSourceDatabaseRebuild 表示当前请求回退到了 MySQL 全量重建。
	SessionRuntimeSourceDatabaseRebuild SessionRuntimeSource = "database_rebuild"
)
