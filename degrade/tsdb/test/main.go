package main

import (
	"context"
	"fmt"
	"github.com/ongniud/other/degrade/tsdb"
	"github.com/prometheus/prometheus/model/labels"
	"time"
)

func main() {
	// 创建内存数据库
	db := tsdb.NewInMemoryDB()

	// 创建 PromQL 执行器
	executor := tsdb.NewPromQLExecutor(db)

	// 写入丰富的测试数据
	appender := db.Appender()
	now := time.Now()

	// 1. 写入 CPU 使用率数据 (每台主机每分钟一个点)
	for i := 0; i < 6; i++ {
		t := now.Add(-time.Duration(5-i) * time.Minute)
		// 主机1的CPU数据
		appender.Append(0, labels.FromStrings("__name__", "cpu_usage", "instance", "host1", "job", "node"), t.UnixMilli(), 0.25+float64(i)*0.02)
		// 主机2的CPU数据
		appender.Append(0, labels.FromStrings("__name__", "cpu_usage", "instance", "host2", "job", "node"), t.UnixMilli(), 0.40-float64(i)*0.03)
		// 主机3的CPU数据
		appender.Append(0, labels.FromStrings("__name__", "cpu_usage", "instance", "host3", "job", "node"), t.UnixMilli(), 0.30+float64(i)*0.01)
	}

	// 2. 写入内存使用数据
	for i := 0; i < 6; i++ {
		t := now.Add(-time.Duration(5-i) * time.Minute)
		appender.Append(0, labels.FromStrings("__name__", "memory_usage", "instance", "host1", "job", "node"), t.UnixMilli(), 0.6-float64(i)*0.02)
		appender.Append(0, labels.FromStrings("__name__", "memory_usage", "instance", "host2", "job", "node"), t.UnixMilli(), 0.7-float64(i)*0.03)
	}

	// 3. 写入HTTP请求数据
	for i := 0; i < 6; i++ {
		t := now.Add(-time.Duration(5-i) * time.Minute)
		appender.Append(0, labels.FromStrings("__name__", "http_requests_total", "instance", "host1", "job", "node", "status", "200"), t.UnixMilli(), float64(1000+i*50))
		appender.Append(0, labels.FromStrings("__name__", "http_requests_total", "instance", "host1", "job", "node", "status", "500"), t.UnixMilli(), float64(10+i))
	}

	// 4. 写入磁盘空间数据
	for i := 0; i < 6; i++ {
		t := now.Add(-time.Duration(5-i) * time.Minute)
		appender.Append(0, labels.FromStrings("__name__", "disk_used", "instance", "host1", "job", "node", "device", "sda1"), t.UnixMilli(), 50.0+float64(i))
		appender.Append(0, labels.FromStrings("__name__", "disk_total", "instance", "host1", "job", "node", "device", "sda1"), t.UnixMilli(), 100.0)
	}

	appender.Commit()

	formatter := tsdb.QueryResultFormatter{}

	// 示例1: 基本查询 - 获取特定主机的CPU使用率
	fmt.Println("\n1. 主机1的CPU使用率:")
	matrix, err := executor.ExecuteRangeQuery(
		context.Background(),
		"cpu_usage{instance='host1'}",
		now.Add(-10*time.Minute),
		now,
		1*time.Minute,
	)
	if err != nil {
		panic(err)
	}
	fmt.Println(formatter.FormatMatrix(matrix))

	// 示例2: 聚合查询 - 计算所有主机的平均CPU使用率
	fmt.Println("\n2. 所有主机的平均CPU使用率:")
	//matrix, err = executor.ExecuteRangeQuery(
	//	context.Background(),
	//	"avg(cpu_usage) by (instance)",
	//	now.Add(-10*time.Minute),
	//	now,
	//	1*time.Minute,
	//)
	query1 := `avg by(instance) (avg_over_time(cpu_usage[10m]))`
	vector1, _ := executor.ExecuteInstantQuery(context.Background(), query1, now)
	fmt.Println(formatter.FormatVector(vector1))

	// 示例3: 数学运算 - 计算CPU和内存使用率的加权和
	fmt.Println("\n3. CPU和内存的加权使用率(CPU 70%, 内存30%):")
	matrix, err = executor.ExecuteRangeQuery(
		context.Background(),
		"cpu_usage * 0.7 + memory_usage * 0.3",
		now.Add(-10*time.Minute),
		now,
		1*time.Minute,
	)
	fmt.Println(formatter.FormatMatrix(matrix))

	// 示例4: 变化率计算 - HTTP请求速率
	fmt.Println("\n4. HTTP请求速率(每分钟):")
	matrix, err = executor.ExecuteRangeQuery(
		context.Background(),
		"rate(http_requests_total[1m])",
		now.Add(-10*time.Minute),
		now,
		1*time.Minute,
	)
	fmt.Println(formatter.FormatMatrix(matrix))

	// 示例5: 条件告警 - 高CPU使用率主机
	fmt.Println("\n5. CPU使用率超过30%的主机:")
	vector, err := executor.ExecuteInstantQuery(
		context.Background(),
		"cpu_usage > 0.3",
		now,
	)
	fmt.Println(formatter.FormatVector(vector))

	// 示例6: 磁盘使用百分比
	fmt.Println("\n6. 磁盘使用百分比:")
	matrix, err = executor.ExecuteRangeQuery(
		context.Background(),
		"(disk_used / disk_total) * 100",
		now.Add(-10*time.Minute),
		now,
		1*time.Minute,
	)
	fmt.Println(formatter.FormatMatrix(matrix))

	// 示例7: 错误率计算
	fmt.Println("\n7. HTTP错误率:")
	matrix, err = executor.ExecuteRangeQuery(
		context.Background(),
		`sum(rate(http_requests_total{status=~"5.."}[1m])) by (instance) / sum(rate(http_requests_total[1m])) by (instance)`,
		now.Add(-10*time.Minute),
		now,
		1*time.Minute,
	)
	fmt.Println(formatter.FormatMatrix(matrix))

	// 示例8: 时间比较 - 当前与5分钟前的CPU使用率差异
	fmt.Println("\n8. 当前与5分钟前的CPU使用率差异:")
	vector, err = executor.ExecuteInstantQuery(
		context.Background(),
		"cpu_usage - (cpu_usage offset 5m)",
		now,
	)
	fmt.Println(formatter.FormatVector(vector))

	// 示例9: 预测 - 基于最近10分钟数据预测5分钟后的CPU使用率
	fmt.Println("\n9. 预测5分钟后的CPU使用率:")
	vector, err = executor.ExecuteInstantQuery(
		context.Background(),
		"predict_linear(cpu_usage[10m], 5*60)",
		now,
	)
	fmt.Println(formatter.FormatVector(vector))

	// 示例10: 复杂逻辑 - 高负载主机检测
	fmt.Println("\n10. 高负载主机(CPU>30%且内存>60%):")
	vector, err = executor.ExecuteInstantQuery(
		context.Background(),
		"(cpu_usage > 0.3) and (memory_usage > 0.6)",
		now,
	)
	fmt.Println(formatter.FormatVector(vector))
}
