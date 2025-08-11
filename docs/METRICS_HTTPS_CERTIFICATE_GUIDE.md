# Redis Operator Metrics HTTPS 证书配置指南

## 概述

本指南详细说明如何为 Redis Operator 的 metrics 端点配置 HTTPS 认证，包括使用 cert-manager 自动管理证书和手动证书配置两种方案。

## 当前状态

目前 Redis Operator 配置为 HTTP 模式（`--metrics-secure=false`），这是为了简化部署和调试。但在生产环境中，建议启用 HTTPS 认证以提高安全性。

## 方案一：使用 cert-manager（推荐）

### 1. 安装 cert-manager

```bash
# 安装 cert-manager
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.13.0/cert-manager.yaml

# 验证安装
kubectl get pods -n cert-manager
```

### 2. 启用证书配置

修改 `config/default/kustomization.yaml`：

```yaml
resources:
- ../crd
- ../rbac
- ../manager
- ../certmanager  # 取消注释
- ../prometheus
- metrics_service.yaml

patches:
# 启用 HTTPS metrics
- path: manager_metrics_patch.yaml
  target:
    kind: Deployment

# 启用证书管理
- path: cert_metrics_manager_patch.yaml  # 取消注释
  target:
    kind: Deployment
```

### 3. 创建 cert-manager 配置

创建 `config/certmanager/` 目录和相关文件：

**config/certmanager/kustomization.yaml**:
```yaml
resources:
- certificate.yaml
- issuer.yaml

configurations:
- kustomizeconfig.yaml
```

**config/certmanager/certificate.yaml**:
```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  labels:
    app.kubernetes.io/name: redis-operator
    app.kubernetes.io/managed-by: kustomize
  name: metrics-certs
  namespace: system
spec:
  dnsNames:
  - SERVICE_NAME.SERVICE_NAMESPACE.svc
  - SERVICE_NAME.SERVICE_NAMESPACE.svc.cluster.local
  issuerRef:
    kind: Issuer
    name: selfsigned-issuer
  secretName: metrics-server-cert
```

**config/certmanager/issuer.yaml**:
```yaml
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  labels:
    app.kubernetes.io/name: redis-operator
    app.kubernetes.io/managed-by: kustomize
  name: selfsigned-issuer
  namespace: system
spec:
  selfSigned: {}
```

### 4. 修改 Manager 配置

在 `config/manager/manager.yaml` 中移除 `--metrics-secure=false` 参数：

```yaml
args:
  - --leader-elect
  - --health-probe-bind-address=:8081
  - --metrics-bind-address=:8443
  # 移除 --metrics-secure=false
  - --enable-metrics-collection=true
```

### 5. 更新 ServiceMonitor

恢复 `config/prometheus/monitor.yaml` 的 HTTPS 配置：

```yaml
spec:
  endpoints:
    - path: /metrics
      port: https
      scheme: https
      bearerTokenFile: /var/run/secrets/kubernetes.io/serviceaccount/token
      tlsConfig:
        ca:
          secret:
            name: metrics-server-cert
            key: ca.crt
        cert:
          secret:
            name: metrics-server-cert
            key: tls.crt
        keySecret:
          name: metrics-server-cert
          key: tls.key
        serverName: redis-operator-controller-manager-metrics-service.redis-operator-system.svc
```

### 6. 启用 kustomization 替换

在 `config/default/kustomization.yaml` 中取消注释 replacements 部分，用于自动配置服务名称和命名空间。

## 方案二：手动证书管理

### 1. 生成自签名证书

```bash
# 创建私钥
openssl genrsa -out metrics-server.key 2048

# 创建证书签名请求
openssl req -new -key metrics-server.key -out metrics-server.csr -subj "/CN=redis-operator-controller-manager-metrics-service.redis-operator-system.svc"

# 生成自签名证书
openssl x509 -req -in metrics-server.csr -signkey metrics-server.key -out metrics-server.crt -days 365

# 创建 Kubernetes Secret
kubectl create secret tls metrics-server-cert \
  --cert=metrics-server.crt \
  --key=metrics-server.key \
  -n redis-operator-system
```

### 2. 应用证书补丁

在 `config/default/kustomization.yaml` 中启用证书补丁：

```yaml
patches:
- path: manager_metrics_patch.yaml
  target:
    kind: Deployment
- path: cert_metrics_manager_patch.yaml
  target:
    kind: Deployment
```

## 验证 HTTPS 配置

### 1. 检查证书 Secret

```bash
kubectl get secret metrics-server-cert -n redis-operator-system
kubectl describe secret metrics-server-cert -n redis-operator-system
```

### 2. 检查 Pod 配置

```bash
kubectl get pod -n redis-operator-system -o yaml | grep -A 10 volumeMounts
kubectl logs <pod-name> -n redis-operator-system | grep metrics
```

应该看到：
```
INFO controller-runtime.metrics Serving metrics server {"bindAddress": ":8443", "secure": true}
```

### 3. 测试 HTTPS 访问

```bash
# 端口转发
kubectl port-forward -n redis-operator-system svc/redis-operator-controller-manager-metrics-service 8443:8443

# 使用证书访问
curl --cacert /path/to/ca.crt https://localhost:8443/metrics

# 或跳过证书验证（仅测试用）
curl -k https://localhost:8443/metrics
```

## ServiceMonitor 配置对比

### HTTP 模式（当前）
```yaml
spec:
  endpoints:
    - path: /metrics
      port: https
      scheme: http
```

### HTTPS 模式（推荐生产环境）
```yaml
spec:
  endpoints:
    - path: /metrics
      port: https
      scheme: https
      bearerTokenFile: /var/run/secrets/kubernetes.io/serviceaccount/token
      tlsConfig:
        insecureSkipVerify: true  # 开发环境
        # 或使用证书验证（生产环境）
        ca:
          secret:
            name: metrics-server-cert
            key: ca.crt
```

## 部署步骤

### 启用 HTTPS 的完整部署

1. **安装 cert-manager**（如果使用方案一）
2. **修改配置文件**（如上所述）
3. **重新部署**：
   ```bash
   make deploy IMG=controller:latest
   ```
4. **验证配置**：
   ```bash
   kubectl get certificate -n redis-operator-system
   kubectl get secret metrics-server-cert -n redis-operator-system
   ```

## 故障排除

### 1. 证书未生成

```bash
# 检查 cert-manager 状态
kubectl get pods -n cert-manager

# 检查证书状态
kubectl describe certificate metrics-certs -n redis-operator-system

# 检查 Issuer 状态
kubectl describe issuer selfsigned-issuer -n redis-operator-system
```

### 2. TLS 握手失败

- 检查证书的 DNS 名称是否匹配服务名称
- 验证证书是否已正确挂载到 Pod
- 检查 ServiceMonitor 的 `serverName` 配置

### 3. Prometheus 无法抓取指标

- 检查 ServiceMonitor 的 TLS 配置
- 验证 Prometheus 是否有访问证书 Secret 的权限
- 检查网络策略是否阻止了连接

## 安全建议

1. **生产环境**：
   - 使用 cert-manager 自动管理证书
   - 启用证书验证（`insecureSkipVerify: false`）
   - 定期轮换证书

2. **开发环境**：
   - 可以使用自签名证书
   - 允许跳过证书验证以简化调试

3. **网络安全**：
   - 配置网络策略限制 metrics 端点访问
   - 使用 RBAC 控制 ServiceAccount 权限

## 总结

虽然当前的 HTTP 配置简化了部署和调试，但在生产环境中强烈建议启用 HTTPS 认证。cert-manager 提供了自动化的证书管理解决方案，是生产环境的最佳选择。

根据您的需求选择合适的方案：
- **开发/测试环境**：保持当前的 HTTP 配置
- **生产环境**：启用 cert-manager 和 HTTPS 认证
- **混合环境**：使用环境变量或不同的 kustomization 配置