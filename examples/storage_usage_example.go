package examples

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/ybooks240/redis-operator/internal/utils"
)

// ExampleController 示例控制器，展示如何使用通用存储管理工具
type ExampleController struct {
	client.Client
	storageManager *utils.StorageManager
	logger         logr.Logger
}

// NewExampleController 创建示例控制器
func NewExampleController(client client.Client, logger logr.Logger) *ExampleController {
	return &ExampleController{
		Client:         client,
		storageManager: utils.NewStorageManager(client, logger),
		logger:         logger,
	}
}

// HandleStorageChange 处理存储变更的示例方法
func (c *ExampleController) HandleStorageChange(ctx context.Context, statefulSet *appsv1.StatefulSet, currentSize, desiredSize, componentName string) error {
	// 1. 分析存储变更
	storageResult := c.storageManager.AnalyzeStorageChange(currentSize, desiredSize, componentName)

	// 2. 处理错误情况
	if storageResult.ErrorMessage != "" {
		c.logger.Error(fmt.Errorf("storage change rejected"), storageResult.ErrorMessage,
			"component", componentName,
			"current", currentSize,
			"desired", desiredSize)
		return fmt.Errorf("%s", storageResult.ErrorMessage)
	}

	// 3. 处理警告情况
	if storageResult.WarningMessage != "" {
		c.logger.Info(storageResult.WarningMessage,
			"component", componentName,
			"current", currentSize,
			"desired", desiredSize)
	}

	// 4. 根据变更类型执行相应操作
	switch storageResult.ChangeType {
	case utils.StorageExpansion:
		c.logger.Info("Performing storage expansion",
			"component", componentName,
			"current", currentSize,
			"desired", desiredSize)

		// 执行 PVC 扩容
		if err := c.storageManager.ExpandStatefulSetPVCs(ctx, statefulSet, desiredSize, componentName); err != nil {
			return fmt.Errorf("failed to expand storage for %s: %w", componentName, err)
		}

		c.logger.Info("Storage expansion completed successfully",
			"component", componentName,
			"new_size", desiredSize)

	case utils.StorageShrinkage:
		// 缩容被拒绝，已在 AnalyzeStorageChange 中处理
		return fmt.Errorf("%s", storageResult.ErrorMessage)

	case utils.StorageNoChange:
		c.logger.V(1).Info("No storage change required",
			"component", componentName,
			"size", currentSize)
	}

	return nil
}

// ValidateStorageConfiguration 验证存储配置的示例方法
func (c *ExampleController) ValidateStorageConfiguration(ctx context.Context, storageClassName string) error {
	return c.storageManager.ValidateStorageClass(ctx, storageClassName)
}

// 使用示例：在控制器的 Reconcile 方法中
func (c *ExampleController) ExampleReconcile(ctx context.Context, statefulSet *appsv1.StatefulSet) error {
	// 假设从 CR 规范中获取期望的存储大小
	desiredStorageSize := "2Gi" // 从 CR 中获取
	componentName := "MyComponent"

	// 获取当前存储大小
	currentStorageSize := "1Gi" // 从 StatefulSet 或 PVC 中获取

	// 处理存储变更
	if err := c.HandleStorageChange(ctx, statefulSet, currentStorageSize, desiredStorageSize, componentName); err != nil {
		c.logger.Error(err, "Failed to handle storage change",
			"component", componentName,
			"current", currentStorageSize,
			"desired", desiredStorageSize)
		return err
	}

	return nil
}

// 错误处理示例：展示如何处理不同类型的存储错误
func (c *ExampleController) HandleStorageErrors(ctx context.Context, err error, componentName string) {
	if err == nil {
		return
	}

	// 根据错误类型进行不同处理
	errorMsg := err.Error()

	switch {
	case contains(errorMsg, "shrinkage"):
		// 存储缩容错误
		c.logger.Error(err, "Storage shrinkage attempted - operation blocked for safety",
			"component", componentName,
			"action", "Consider creating new instance with smaller storage")

	case contains(errorMsg, "invalid storage size"):
		// 无效存储大小错误
		c.logger.Error(err, "Invalid storage size specified",
			"component", componentName,
			"action", "Check storage size format (e.g., 1Gi, 500Mi)")

	case contains(errorMsg, "failed to expand PVC"):
		// PVC 扩容失败错误
		c.logger.Error(err, "PVC expansion failed",
			"component", componentName,
			"action", "Check StorageClass supports volume expansion")

	default:
		// 其他存储相关错误
		c.logger.Error(err, "Storage operation failed",
			"component", componentName)
	}
}

// contains 检查字符串是否包含子字符串
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || (len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			containsInMiddle(s, substr))))
}

func containsInMiddle(s, substr string) bool {
	for i := 1; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
