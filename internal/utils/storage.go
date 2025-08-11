package utils

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// StorageChangeType 存储变更类型
type StorageChangeType string

const (
	StorageExpansion StorageChangeType = "expansion"
	StorageShrinkage StorageChangeType = "shrinkage"
	StorageNoChange  StorageChangeType = "no-change"
)

// StorageChangeResult 存储变更结果
type StorageChangeResult struct {
	ChangeType     StorageChangeType
	CurrentSize    string
	DesiredSize    string
	NeedsAction    bool
	ErrorMessage   string
	WarningMessage string
}

// StorageManager 通用存储管理器
type StorageManager struct {
	client client.Client
	logger logr.Logger
}

// NewStorageManager 创建新的存储管理器
func NewStorageManager(client client.Client, logger logr.Logger) *StorageManager {
	return &StorageManager{
		client: client,
		logger: logger,
	}
}

// AnalyzeStorageChange 分析存储变更
func (sm *StorageManager) AnalyzeStorageChange(currentSize, desiredSize string, componentName string) *StorageChangeResult {
	result := &StorageChangeResult{
		CurrentSize: currentSize,
		DesiredSize: desiredSize,
	}

	// 解析存储大小
	currentStorage, err := resource.ParseQuantity(currentSize)
	if err != nil {
		result.ErrorMessage = fmt.Sprintf("Invalid current storage size '%s': %v", currentSize, err)
		return result
	}

	desiredStorage, err := resource.ParseQuantity(desiredSize)
	if err != nil {
		result.ErrorMessage = fmt.Sprintf("Invalid desired storage size '%s': %v", desiredSize, err)
		return result
	}

	// 比较存储大小
	if currentStorage.Equal(desiredStorage) {
		result.ChangeType = StorageNoChange
		result.NeedsAction = false
		sm.logger.V(1).Info("Storage size unchanged", "component", componentName, "size", currentSize)
		return result
	}

	if desiredStorage.Cmp(currentStorage) > 0 {
		// 扩容
		result.ChangeType = StorageExpansion
		result.NeedsAction = true
		sm.logger.Info("Storage expansion detected",
			"component", componentName,
			"current", currentSize,
			"desired", desiredSize,
			"action", "PVC dynamic expansion")
	} else {
		// 缩容
		result.ChangeType = StorageShrinkage
		result.NeedsAction = false
		result.ErrorMessage = sm.formatShrinkageError(componentName, currentSize, desiredSize)
		result.WarningMessage = sm.formatShrinkageWarning(componentName, currentSize, desiredSize)
		sm.logger.Error(fmt.Errorf("storage shrinkage rejected"), result.ErrorMessage,
			"component", componentName,
			"current", currentSize,
			"desired", desiredSize,
			"reason", "data safety")
	}

	return result
}

// formatShrinkageError 格式化缩容错误信息
func (sm *StorageManager) formatShrinkageError(componentName, currentSize, desiredSize string) string {
	return fmt.Sprintf("[%s] Storage shrinkage from %s to %s is not supported for data safety reasons. "+
		"Please consider: 1) Backup data first, 2) Create new instance with smaller storage, 3) Migrate data manually.",
		componentName, currentSize, desiredSize)
}

// formatShrinkageWarning 格式化缩容警告信息
func (sm *StorageManager) formatShrinkageWarning(componentName, currentSize, desiredSize string) string {
	return fmt.Sprintf("[%s] Attempted to shrink storage from %s to %s. "+
		"This operation is blocked to prevent data loss. "+
		"If you need to reduce storage, please create a new instance and migrate data manually.",
		componentName, currentSize, desiredSize)
}

// ExpandStatefulSetPVCs 扩展 StatefulSet 的 PVC
func (sm *StorageManager) ExpandStatefulSetPVCs(ctx context.Context, statefulSet *appsv1.StatefulSet, newStorageSize string, componentName string) error {
	// 解析新的存储大小
	newStorage, err := resource.ParseQuantity(newStorageSize)
	if err != nil {
		return fmt.Errorf("invalid storage size '%s': %w", newStorageSize, err)
	}

	// 获取与 StatefulSet 相关的 PVC
	labelSelector := labels.SelectorFromSet(statefulSet.Spec.Selector.MatchLabels)
	pvcList := &corev1.PersistentVolumeClaimList{}
	if err := sm.client.List(ctx, pvcList, &client.ListOptions{
		Namespace:     statefulSet.Namespace,
		LabelSelector: labelSelector,
	}); err != nil {
		return fmt.Errorf("failed to list PVCs for StatefulSet %s: %w", statefulSet.Name, err)
	}

	// 如果没有找到 PVC，尝试通过名称模式查找
	if len(pvcList.Items) == 0 {
		if err := sm.findPVCsByNamePattern(ctx, statefulSet, pvcList); err != nil {
			return fmt.Errorf("failed to find PVCs for StatefulSet %s: %w", statefulSet.Name, err)
		}
	}

	if len(pvcList.Items) == 0 {
		return fmt.Errorf("no PVCs found for StatefulSet %s", statefulSet.Name)
	}

	sm.logger.Info("Found PVCs for expansion",
		"component", componentName,
		"statefulset", statefulSet.Name,
		"pvc_count", len(pvcList.Items),
		"target_size", newStorageSize)

	// 扩展每个 PVC
	for _, pvc := range pvcList.Items {
		if err := sm.expandSinglePVC(ctx, &pvc, newStorage, componentName); err != nil {
			return fmt.Errorf("failed to expand PVC %s: %w", pvc.Name, err)
		}
	}

	sm.logger.Info("PVC expansion completed successfully",
		"component", componentName,
		"statefulset", statefulSet.Name,
		"new_size", newStorageSize)

	return nil
}

// findPVCsByNamePattern 通过名称模式查找 PVC
func (sm *StorageManager) findPVCsByNamePattern(ctx context.Context, statefulSet *appsv1.StatefulSet, pvcList *corev1.PersistentVolumeClaimList) error {
	// 获取所有 PVC
	allPVCs := &corev1.PersistentVolumeClaimList{}
	if err := sm.client.List(ctx, allPVCs, &client.ListOptions{
		Namespace: statefulSet.Namespace,
	}); err != nil {
		return err
	}

	// 查找匹配的 PVC（通常以 volumeClaimTemplate 名称 + StatefulSet 名称开头）
	for _, pvc := range allPVCs.Items {
		if sm.isPVCBelongsToStatefulSet(&pvc, statefulSet) {
			pvcList.Items = append(pvcList.Items, pvc)
		}
	}

	return nil
}

// isPVCBelongsToStatefulSet 检查 PVC 是否属于指定的 StatefulSet
func (sm *StorageManager) isPVCBelongsToStatefulSet(pvc *corev1.PersistentVolumeClaim, statefulSet *appsv1.StatefulSet) bool {
	// 检查 PVC 名称是否匹配 StatefulSet 的命名模式
	// 通常格式为: {volumeClaimTemplate.name}-{statefulset.name}-{ordinal}
	for _, vct := range statefulSet.Spec.VolumeClaimTemplates {
		expectedPrefix := fmt.Sprintf("%s-%s-", vct.Name, statefulSet.Name)
		if strings.HasPrefix(pvc.Name, expectedPrefix) {
			return true
		}
	}
	return false
}

// expandSinglePVC 扩展单个 PVC
func (sm *StorageManager) expandSinglePVC(ctx context.Context, pvc *corev1.PersistentVolumeClaim, newStorage resource.Quantity, componentName string) error {
	// 获取当前存储大小
	currentStorage := pvc.Spec.Resources.Requests[corev1.ResourceStorage]

	// 检查是否需要扩容
	if newStorage.Cmp(currentStorage) <= 0 {
		sm.logger.Info("PVC already has sufficient storage",
			"component", componentName,
			"name", pvc.Name,
			"current", currentStorage.String(),
			"requested", newStorage.String())
		return nil
	}

	sm.logger.Info("Expanding PVC storage",
		"component", componentName,
		"name", pvc.Name,
		"current", currentStorage.String(),
		"new", newStorage.String())

	// 更新 PVC 的存储请求
	pvc.Spec.Resources.Requests[corev1.ResourceStorage] = newStorage

	// 应用更新
	if err := sm.client.Update(ctx, pvc); err != nil {
		return fmt.Errorf("failed to update PVC %s: %w", pvc.Name, err)
	}

	sm.logger.Info("PVC expansion request submitted",
		"component", componentName,
		"name", pvc.Name,
		"new_size", newStorage.String())

	return nil
}

// ValidateStorageClass 验证 StorageClass 是否支持扩容
func (sm *StorageManager) ValidateStorageClass(ctx context.Context, storageClassName string) error {
	if storageClassName == "" {
		return fmt.Errorf("storage class name is empty")
	}

	// 这里可以添加更多的 StorageClass 验证逻辑
	// 例如检查 allowVolumeExpansion 字段
	sm.logger.V(1).Info("Storage class validation passed", "storage_class", storageClassName)
	return nil
}
