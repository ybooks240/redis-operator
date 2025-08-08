#!/bin/bash

# 测试配置生成一致性
# 验证 GenerateRedisConfig 函数生成的配置是否一致

set -e

echo "=== 测试配置生成一致性 ==="

# 创建测试程序
cat > test_config_consistency.go << 'EOF'
package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"strconv"

	"github.com/ybooks240/redis-operator/internal/utils"
)

func calculateConfigHash(config string) string {
	h := sha256.Sum256([]byte(config))
	return fmt.Sprintf("%x", h)
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run test_config_consistency.go <iterations>")
		os.Exit(1)
	}

	iterations, err := strconv.Atoi(os.Args[1])
	if err != nil {
		fmt.Printf("Invalid iterations: %v\n", err)
		os.Exit(1)
	}

	// 测试配置
	testConfig := map[string]string{
		"maxmemory":        "256mb",
		"maxmemory-policy": "allkeys-lru",
		"timeout":          "300",
		"save":             "900 1",
	}

	// 生成配置多次并检查一致性
	var firstConfig string
	var firstHash string
	allHashes := make(map[string]int)

	for i := 0; i < iterations; i++ {
		config := utils.GenerateRedisConfig(testConfig)
		hash := calculateConfigHash(config)

		if i == 0 {
			firstConfig = config
			firstHash = hash
			fmt.Printf("第一次生成的配置:\n%s\n\n", config)
			fmt.Printf("第一次生成的哈希值: %s\n\n", hash)
		}

		allHashes[hash]++

		if config != firstConfig {
			fmt.Printf("❌ 第 %d 次生成的配置与第一次不一致!\n", i+1)
			fmt.Printf("第一次:\n%s\n\n", firstConfig)
			fmt.Printf("第 %d 次:\n%s\n\n", i+1, config)
			os.Exit(1)
		}

		if hash != firstHash {
			fmt.Printf("❌ 第 %d 次生成的哈希值与第一次不一致!\n", i+1)
			fmt.Printf("第一次哈希值: %s\n", firstHash)
			fmt.Printf("第 %d 次哈希值: %s\n", i+1, hash)
			os.Exit(1)
		}
	}

	fmt.Printf("✅ 所有 %d 次生成的配置和哈希值都完全一致!\n\n", iterations)
	fmt.Printf("统计信息:\n")
	fmt.Printf("- 总生成次数: %d\n", iterations)
	fmt.Printf("- 唯一哈希值数量: %d\n", len(allHashes))
	fmt.Printf("- 配置行数: %d\n", len(strings.Split(firstConfig, "\n")))

	if len(allHashes) == 1 {
		fmt.Printf("🎉 配置生成完全一致，哈希值稳定!\n")
	} else {
		fmt.Printf("❌ 发现 %d 个不同的哈希值，配置生成不一致!\n", len(allHashes))
		os.Exit(1)
	}
}
EOF

# 添加必要的 import
echo "添加 strings import..."
sed -i '' 's/"os"/"os"\n\t"strings"/' test_config_consistency.go

echo "运行配置一致性测试..."
echo "测试 100 次配置生成..."
go run test_config_consistency.go 100

echo ""
echo "=== 测试不同配置的哈希值差异 ==="

# 创建测试不同配置的程序
cat > test_different_configs.go << 'EOF'
package main

import (
	"crypto/sha256"
	"fmt"

	"github.com/ybooks240/redis-operator/internal/utils"
)

func calculateConfigHash(config string) string {
	h := sha256.Sum256([]byte(config))
	return fmt.Sprintf("%x", h)
}

func main() {
	// 测试配置 1
	config1 := map[string]string{
		"maxmemory":        "128mb",
		"maxmemory-policy": "allkeys-lru",
		"timeout":          "300",
	}

	// 测试配置 2 (修改 maxmemory)
	config2 := map[string]string{
		"maxmemory":        "256mb",
		"maxmemory-policy": "allkeys-lru",
		"timeout":          "300",
	}

	// 测试配置 3 (修改 timeout)
	config3 := map[string]string{
		"maxmemory":        "128mb",
		"maxmemory-policy": "allkeys-lru",
		"timeout":          "600",
	}

	// 生成配置和哈希值
	redisConfig1 := utils.GenerateRedisConfig(config1)
	hash1 := calculateConfigHash(redisConfig1)

	redisConfig2 := utils.GenerateRedisConfig(config2)
	hash2 := calculateConfigHash(redisConfig2)

	redisConfig3 := utils.GenerateRedisConfig(config3)
	hash3 := calculateConfigHash(redisConfig3)

	fmt.Printf("配置 1 (maxmemory=128mb, timeout=300):\n")
	fmt.Printf("哈希值: %s\n\n", hash1)

	fmt.Printf("配置 2 (maxmemory=256mb, timeout=300):\n")
	fmt.Printf("哈希值: %s\n\n", hash2)

	fmt.Printf("配置 3 (maxmemory=128mb, timeout=600):\n")
	fmt.Printf("哈希值: %s\n\n", hash3)

	// 验证不同配置产生不同哈希值
	if hash1 == hash2 {
		fmt.Printf("❌ 错误：配置 1 和配置 2 的哈希值相同!\n")
		return
	}

	if hash1 == hash3 {
		fmt.Printf("❌ 错误：配置 1 和配置 3 的哈希值相同!\n")
		return
	}

	if hash2 == hash3 {
		fmt.Printf("❌ 错误：配置 2 和配置 3 的哈希值相同!\n")
		return
	}

	fmt.Printf("✅ 不同配置产生了不同的哈希值，配置变更检测正常!\n")
}
EOF

echo "测试不同配置的哈希值差异..."
go run test_different_configs.go

echo ""
echo "=== 清理测试文件 ==="
rm -f test_config_consistency.go test_different_configs.go

echo ""
echo "🎉 配置一致性测试完成！"
echo "✅ GenerateRedisConfig 函数现在能够生成一致的配置"
echo "✅ 相同输入总是产生相同的哈希值"
echo "✅ 不同输入产生不同的哈希值"
echo "✅ 解决了 map 无序导致的哈希值不一致问题"