#!/bin/bash

# Redis Operator 综合自动化测试脚本
# 包含单元测试、集成测试、端到端测试和性能测试

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# 日志函数
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_test() {
    echo -e "${PURPLE}[TEST]${NC} $1"
}

log_step() {
    echo -e "${CYAN}[STEP]${NC} $1"
}

# 全局变量
TEST_NAMESPACE="redis-operator-test"
TEST_TIMEOUT=600  # 10分钟超时
TEST_RESULTS_DIR="test-results"
START_TIME=$(date +%s)

# 测试结果统计
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0
SKIPPED_TESTS=0

# 清理函数
cleanup() {
    log_info "执行测试清理..."
    
    # 清理测试资源
    kubectl delete namespace $TEST_NAMESPACE --ignore-not-found=true --timeout=60s || true
    
    # 等待命名空间完全删除
    log_info "等待测试命名空间删除..."
    timeout 60 bash -c 'while kubectl get namespace '$TEST_NAMESPACE' >/dev/null 2>&1; do sleep 2; done' || true
    
    log_info "清理完成"
}

# 错误处理
trap cleanup EXIT

# 测试结果记录函数
record_test_result() {
    local test_name="$1"
    local result="$2"  # PASS, FAIL, SKIP
    local message="$3"
    
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    
    case $result in
        "PASS")
            PASSED_TESTS=$((PASSED_TESTS + 1))
            log_success "✓ $test_name: $message"
            ;;
        "FAIL")
            FAILED_TESTS=$((FAILED_TESTS + 1))
            log_error "✗ $test_name: $message"
            ;;
        "SKIP")
            SKIPPED_TESTS=$((SKIPPED_TESTS + 1))
            log_warning "- $test_name: $message"
            ;;
    esac
    
    # 记录到文件
    echo "$(date '+%Y-%m-%d %H:%M:%S') [$result] $test_name: $message" >> "$TEST_RESULTS_DIR/test-results.log"
}

# 初始化测试环境
init_test_environment() {
    log_step "初始化测试环境"
    
    # 创建测试结果目录
    mkdir -p "$TEST_RESULTS_DIR"
    
    # 记录测试开始时间
    echo "测试开始时间: $(date)" > "$TEST_RESULTS_DIR/test-results.log"
    
    # 检查必要的工具
    local required_tools=("kubectl" "go" "make" "jq")
    for tool in "${required_tools[@]}"; do
        if ! command -v $tool &> /dev/null; then
            record_test_result "环境检查" "FAIL" "缺少必要工具: $tool"
            exit 1
        fi
    done
    
    record_test_result "环境检查" "PASS" "所有必要工具已安装"
    
    # 检查 Kubernetes 集群连接
    if kubectl cluster-info &> /dev/null; then
        record_test_result "集群连接" "PASS" "Kubernetes 集群连接正常"
    else
        record_test_result "集群连接" "FAIL" "无法连接到 Kubernetes 集群"
        exit 1
    fi
    
    # 创建测试命名空间
    kubectl create namespace $TEST_NAMESPACE || true
    log_info "测试命名空间: $TEST_NAMESPACE"
}

# 单元测试
run_unit_tests() {
    log_step "执行单元测试"
    
    log_test "运行 Go 单元测试..."
    if go test ./internal/controller/... -v -coverprofile="$TEST_RESULTS_DIR/coverage.out" > "$TEST_RESULTS_DIR/unit-test.log" 2>&1; then
        record_test_result "单元测试" "PASS" "所有单元测试通过"
        
        # 生成覆盖率报告
        if command -v go &> /dev/null; then
            coverage=$(go tool cover -func="$TEST_RESULTS_DIR/coverage.out" | tail -1 | awk '{print $3}')
            log_info "代码覆盖率: $coverage"
            echo "代码覆盖率: $coverage" >> "$TEST_RESULTS_DIR/test-results.log"
        fi
    else
        record_test_result "单元测试" "FAIL" "单元测试失败，详见 $TEST_RESULTS_DIR/unit-test.log"
        log_error "单元测试失败，查看详细日志:"
        tail -20 "$TEST_RESULTS_DIR/unit-test.log"
    fi
}

# 构建和部署测试
run_build_tests() {
    log_step "执行构建和部署测试"
    
    # 测试 Docker 镜像构建
    log_test "测试 Docker 镜像构建..."
    if make docker-build IMG=redis-operator:test > "$TEST_RESULTS_DIR/build.log" 2>&1; then
        record_test_result "镜像构建" "PASS" "Docker 镜像构建成功"
    else
        record_test_result "镜像构建" "FAIL" "Docker 镜像构建失败"
        return 1
    fi
    
    # 测试 CRD 安装
    log_test "测试 CRD 安装..."
    if make install > "$TEST_RESULTS_DIR/crd-install.log" 2>&1; then
        record_test_result "CRD安装" "PASS" "CRD 安装成功"
    else
        record_test_result "CRD安装" "FAIL" "CRD 安装失败"
        return 1
    fi
    
    # 验证 CRD 是否正确安装
    local crds=("redissentinels.redis.github.com" "redismasterreplicas.redis.github.com" "redisinstances.redis.github.com")
    for crd in "${crds[@]}"; do
        if kubectl get crd $crd &> /dev/null; then
            record_test_result "CRD验证-$crd" "PASS" "CRD $crd 已正确安装"
        else
            record_test_result "CRD验证-$crd" "FAIL" "CRD $crd 未找到"
        fi
    done
}

# RedisSentinel 功能测试
run_redissentinel_tests() {
    log_step "执行 RedisSentinel 功能测试"
    
    # 测试内嵌 Redis 模式
    log_test "测试 RedisSentinel 内嵌 Redis 模式..."
    
    cat <<EOF | kubectl apply -f - > "$TEST_RESULTS_DIR/sentinel-embedded.log" 2>&1
apiVersion: redis.github.com/v1
kind: RedisSentinel
metadata:
  name: test-sentinel-embedded
  namespace: $TEST_NAMESPACE
spec:
  replicas: 3
  image: redis:7.0
  config:
    quorum: 2
  redis:
    master:
      replicas: 1
    replica:
      replicas: 2
  resources:
    limits:
      cpu: 100m
      memory: 128Mi
    requests:
      cpu: 50m
      memory: 64Mi
  storage:
    size: 1Gi
EOF
    
    if [[ $? -eq 0 ]]; then
        record_test_result "Sentinel创建-内嵌" "PASS" "RedisSentinel 内嵌模式创建成功"
        
        # 等待资源就绪
        log_info "等待 RedisSentinel 内嵌模式就绪..."
        if wait_for_resource_ready "redissentinel" "test-sentinel-embedded" $TEST_NAMESPACE 300; then
            record_test_result "Sentinel就绪-内嵌" "PASS" "RedisSentinel 内嵌模式就绪"
        else
            record_test_result "Sentinel就绪-内嵌" "FAIL" "RedisSentinel 内嵌模式未能就绪"
        fi
    else
        record_test_result "Sentinel创建-内嵌" "FAIL" "RedisSentinel 内嵌模式创建失败"
    fi
    
    # 测试外部引用模式
    log_test "测试 RedisSentinel 外部引用模式..."
    
    # 首先创建 RedisMasterReplica
    cat <<EOF | kubectl apply -f - > "$TEST_RESULTS_DIR/master-replica.log" 2>&1
apiVersion: redis.github.com/v1
kind: RedisMasterReplica
metadata:
  name: test-master-replica
  namespace: $TEST_NAMESPACE
spec:
  image: redis:7.0
  master:
    replicas: 1
  replica:
    replicas: 2
  resources:
    limits:
      cpu: 100m
      memory: 128Mi
    requests:
      cpu: 50m
      memory: 64Mi
  storage:
    size: 1Gi
EOF
    
    if [[ $? -eq 0 ]]; then
        record_test_result "MasterReplica创建" "PASS" "RedisMasterReplica 创建成功"
        
        # 等待 MasterReplica 就绪
        if wait_for_resource_ready "redismasterreplica" "test-master-replica" $TEST_NAMESPACE 300; then
            record_test_result "MasterReplica就绪" "PASS" "RedisMasterReplica 就绪"
            
            # 创建引用外部 MasterReplica 的 Sentinel
            cat <<EOF | kubectl apply -f - > "$TEST_RESULTS_DIR/sentinel-external.log" 2>&1
apiVersion: redis.github.com/v1
kind: RedisSentinel
metadata:
  name: test-sentinel-external
  namespace: $TEST_NAMESPACE
spec:
  replicas: 3
  image: redis:7.0
  config:
    quorum: 2
  masterReplicaRef:
    name: test-master-replica
  resources:
    limits:
      cpu: 100m
      memory: 128Mi
    requests:
      cpu: 50m
      memory: 64Mi
EOF
            
            if [[ $? -eq 0 ]]; then
                record_test_result "Sentinel创建-外部" "PASS" "RedisSentinel 外部引用模式创建成功"
                
                if wait_for_resource_ready "redissentinel" "test-sentinel-external" $TEST_NAMESPACE 300; then
                    record_test_result "Sentinel就绪-外部" "PASS" "RedisSentinel 外部引用模式就绪"
                else
                    record_test_result "Sentinel就绪-外部" "FAIL" "RedisSentinel 外部引用模式未能就绪"
                fi
            else
                record_test_result "Sentinel创建-外部" "FAIL" "RedisSentinel 外部引用模式创建失败"
            fi
        else
            record_test_result "MasterReplica就绪" "FAIL" "RedisMasterReplica 未能就绪"
        fi
    else
        record_test_result "MasterReplica创建" "FAIL" "RedisMasterReplica 创建失败"
    fi
}

# 状态更新优化测试
run_status_update_tests() {
    log_step "执行状态更新优化测试"
    
    log_test "运行状态更新优化测试脚本..."
    if ./test/shell/test-status-update-optimization.sh > "$TEST_RESULTS_DIR/status-update.log" 2>&1; then
        record_test_result "状态更新优化" "PASS" "状态更新优化测试通过"
    else
        record_test_result "状态更新优化" "FAIL" "状态更新优化测试失败"
        log_error "状态更新测试失败，查看详细日志:"
        tail -20 "$TEST_RESULTS_DIR/status-update.log"
    fi
}

# 性能测试
run_performance_tests() {
    log_step "执行性能测试"
    
    log_test "测试资源创建性能..."
    
    local start_time=$(date +%s)
    
    # 创建多个 RedisSentinel 实例
    for i in {1..5}; do
        cat <<EOF | kubectl apply -f - &
apiVersion: redis.github.com/v1
kind: RedisSentinel
metadata:
  name: perf-test-$i
  namespace: $TEST_NAMESPACE
spec:
  replicas: 1
  image: redis:7.0
  config:
    quorum: 1
  redis:
    master:
      replicas: 1
    replica:
      replicas: 1
  resources:
    limits:
      cpu: 50m
      memory: 64Mi
    requests:
      cpu: 25m
      memory: 32Mi
  storage:
    size: 500Mi
EOF
    done
    
    wait  # 等待所有创建操作完成
    
    local end_time=$(date +%s)
    local duration=$((end_time - start_time))
    
    log_info "创建 5 个 RedisSentinel 实例耗时: ${duration}s"
    
    if [[ $duration -lt 60 ]]; then
        record_test_result "性能测试-创建" "PASS" "资源创建性能良好 (${duration}s)"
    else
        record_test_result "性能测试-创建" "FAIL" "资源创建性能较慢 (${duration}s)"
    fi
    
    # 测试状态更新性能
    log_test "测试状态更新性能..."
    
    local update_start=$(date +%s%N)
    
    # 并发更新多个资源的注解
    for i in {1..5}; do
        kubectl annotate redissentinel perf-test-$i -n $TEST_NAMESPACE perf-test="$(date +%s)" --overwrite &
    done
    
    wait
    
    local update_end=$(date +%s%N)
    local update_duration=$(( (update_end - update_start) / 1000000 ))  # 转换为毫秒
    
    log_info "并发更新 5 个资源耗时: ${update_duration}ms"
    
    if [[ $update_duration -lt 5000 ]]; then  # 小于5秒
        record_test_result "性能测试-更新" "PASS" "状态更新性能良好 (${update_duration}ms)"
    else
        record_test_result "性能测试-更新" "FAIL" "状态更新性能较慢 (${update_duration}ms)"
    fi
}

# 故障恢复测试
run_failure_recovery_tests() {
    log_step "执行故障恢复测试"
    
    log_test "测试 Pod 故障恢复..."
    
    # 获取一个 Sentinel Pod
    local sentinel_pod=$(kubectl get pods -n $TEST_NAMESPACE -l app=redis-sentinel --no-headers -o custom-columns=":metadata.name" | head -1)
    
    if [[ -n "$sentinel_pod" ]]; then
        log_info "删除 Sentinel Pod: $sentinel_pod"
        kubectl delete pod $sentinel_pod -n $TEST_NAMESPACE
        
        # 等待 Pod 重新创建
        sleep 10
        
        # 检查 Pod 是否恢复
        if kubectl get pod $sentinel_pod -n $TEST_NAMESPACE &> /dev/null; then
            record_test_result "故障恢复-Pod" "PASS" "Pod 故障恢复成功"
        else
            # 检查是否有新的 Pod 创建
            local new_pods=$(kubectl get pods -n $TEST_NAMESPACE -l app=redis-sentinel --no-headers | wc -l)
            if [[ $new_pods -gt 0 ]]; then
                record_test_result "故障恢复-Pod" "PASS" "Pod 故障恢复成功 (新 Pod 已创建)"
            else
                record_test_result "故障恢复-Pod" "FAIL" "Pod 故障恢复失败"
            fi
        fi
    else
        record_test_result "故障恢复-Pod" "SKIP" "未找到 Sentinel Pod"
    fi
}

# 等待资源就绪的辅助函数
wait_for_resource_ready() {
    local resource_type="$1"
    local resource_name="$2"
    local namespace="$3"
    local timeout="$4"
    
    local start_time=$(date +%s)
    
    while true; do
        local current_time=$(date +%s)
        local elapsed=$((current_time - start_time))
        
        if [[ $elapsed -gt $timeout ]]; then
            log_warning "等待 $resource_type/$resource_name 就绪超时 (${timeout}s)"
            return 1
        fi
        
        # 检查资源状态
        local ready=$(kubectl get $resource_type $resource_name -n $namespace -o jsonpath='{.status.ready}' 2>/dev/null || echo "false")
        
        if [[ "$ready" == "true" ]]; then
            log_info "$resource_type/$resource_name 已就绪 (耗时 ${elapsed}s)"
            return 0
        fi
        
        log_info "等待 $resource_type/$resource_name 就绪... (已等待 ${elapsed}s)"
        sleep 10
    done
}

# 生成测试报告
generate_test_report() {
    log_step "生成测试报告"
    
    local end_time=$(date +%s)
    local total_duration=$((end_time - START_TIME))
    
    # 创建 HTML 报告
    cat > "$TEST_RESULTS_DIR/test-report.html" <<EOF
<!DOCTYPE html>
<html>
<head>
    <title>Redis Operator 测试报告</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; }
        .header { background-color: #f0f0f0; padding: 20px; border-radius: 5px; }
        .summary { margin: 20px 0; }
        .pass { color: green; }
        .fail { color: red; }
        .skip { color: orange; }
        table { border-collapse: collapse; width: 100%; }
        th, td { border: 1px solid #ddd; padding: 8px; text-align: left; }
        th { background-color: #f2f2f2; }
    </style>
</head>
<body>
    <div class="header">
        <h1>Redis Operator 自动化测试报告</h1>
        <p>测试时间: $(date)</p>
        <p>测试持续时间: ${total_duration}s</p>
    </div>
    
    <div class="summary">
        <h2>测试摘要</h2>
        <p>总测试数: $TOTAL_TESTS</p>
        <p class="pass">通过: $PASSED_TESTS</p>
        <p class="fail">失败: $FAILED_TESTS</p>
        <p class="skip">跳过: $SKIPPED_TESTS</p>
        <p>成功率: $(( PASSED_TESTS * 100 / TOTAL_TESTS ))%</p>
    </div>
    
    <h2>详细结果</h2>
    <pre>
EOF
    
    cat "$TEST_RESULTS_DIR/test-results.log" >> "$TEST_RESULTS_DIR/test-report.html"
    
    cat >> "$TEST_RESULTS_DIR/test-report.html" <<EOF
    </pre>
</body>
</html>
EOF
    
    log_success "测试报告已生成: $TEST_RESULTS_DIR/test-report.html"
}

# 主函数
main() {
    log_info "开始 Redis Operator 综合自动化测试"
    log_info "测试命名空间: $TEST_NAMESPACE"
    log_info "测试超时: $TEST_TIMEOUT 秒"
    
    # 初始化测试环境
    init_test_environment
    
    # 执行各种测试
    run_unit_tests
    run_build_tests
    run_redissentinel_tests
    run_status_update_tests
    run_performance_tests
    run_failure_recovery_tests
    
    # 生成测试报告
    generate_test_report
    
    # 输出最终结果
    log_info "测试完成！"
    log_info "总测试数: $TOTAL_TESTS"
    log_success "通过: $PASSED_TESTS"
    log_error "失败: $FAILED_TESTS"
    log_warning "跳过: $SKIPPED_TESTS"
    
    if [[ $FAILED_TESTS -eq 0 ]]; then
        log_success "所有测试通过！ 🎉"
        exit 0
    else
        log_error "有 $FAILED_TESTS 个测试失败 ❌"
        exit 1
    fi
}

# 运行主函数
main "$@"