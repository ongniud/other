package tsdb

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage"
)

// PromQLExecutor 封装了 PromQL 执行功能
type PromQLExecutor struct {
	engine    *promql.Engine
	queryable storage.Queryable
}

// NewPromQLExecutor 创建新的 PromQL 执行器
func NewPromQLExecutor(db *InMemoryDB) *PromQLExecutor {
	// 创建 PromQL 引擎配置
	opts := promql.EngineOpts{
		MaxSamples:           1000000,         // 最大样本数
		Timeout:              2 * time.Minute, // 查询超时
		ActiveQueryTracker:   nil,             // 查询跟踪器
		LookbackDelta:        5 * time.Minute, // 回溯窗口
		EnableAtModifier:     true,            // 启用 @ 修饰符
		EnableNegativeOffset: true,            // 启用负偏移
	}

	// 创建 Queryable 适配器
	queryable := storage.QueryableFunc(func(mint, maxt int64) (storage.Querier, error) {
		return db.Querier(mint, maxt)
	})

	// 创建 PromQL 引擎
	engine := promql.NewEngine(opts)

	return &PromQLExecutor{
		engine:    engine,
		queryable: queryable,
	}
}

// ExecuteInstantQuery 执行即时查询
func (e *PromQLExecutor) ExecuteInstantQuery(ctx context.Context, query string, ts time.Time) (promql.Vector, error) {
	// 解析查询
	qry, err := e.engine.NewInstantQuery(ctx, e.queryable, nil, query, ts)
	if err != nil {
		return nil, fmt.Errorf("query parse error: %w", err)
	}
	defer qry.Close()

	// 执行查询
	res := qry.Exec(ctx)
	if res.Err != nil {
		return nil, fmt.Errorf("query execution error: %w", res.Err)
	}

	fmt.Println(res)
	// 处理结果
	switch v := res.Value.(type) {
	case promql.Vector:
		return v, nil
	case promql.Scalar:
		return promql.Vector{promql.Sample{
			Metric: labels.Labels{},
			T:      ts.UnixMilli(),
			F:      v.V,
		}}, nil
	case promql.String:
		return nil, fmt.Errorf("string results not supported in vector output")
	default:
		return nil, fmt.Errorf("unsupported result type: %T", v)
	}
}

// ExecuteRangeQuery 执行范围查询
func (e *PromQLExecutor) ExecuteRangeQuery(
	ctx context.Context,
	query string,
	start, end time.Time,
	step time.Duration,
) (promql.Matrix, error) {
	// 解析查询
	qry, err := e.engine.NewRangeQuery(ctx, e.queryable, nil, query, start, end, step)
	if err != nil {
		return nil, fmt.Errorf("query parse error: %w", err)
	}
	defer qry.Close()

	// 执行查询
	res := qry.Exec(ctx)
	if res.Err != nil {
		return nil, fmt.Errorf("query execution error: %w", res.Err)
	}

	// 处理结果
	switch v := res.Value.(type) {
	case promql.Matrix:
		return v, nil
	default:
		return nil, fmt.Errorf("unsupported result type for range query: %T", res.Value)
	}
}

// QueryResultFormatter 格式化查询结果
type QueryResultFormatter struct{}

// FormatVector 格式化向量结果
func (f *QueryResultFormatter) FormatVector(vec promql.Vector) string {
	var result string
	for _, sample := range vec {
		result += fmt.Sprintf("%s => %v @[%v]\n", sample.Metric, sample.F, sample.T)
	}
	return result
}

// FormatMatrix 格式化矩阵结果
func (f *QueryResultFormatter) FormatMatrix(mat promql.Matrix) string {
	var result strings.Builder
	for _, series := range mat {
		result.WriteString(fmt.Sprintf("%s:\n", series.Metric))
		for _, point := range series.Floats {
			result.WriteString(fmt.Sprintf("  %v @[%v]\n", point.F, time.UnixMilli(point.T).UTC()))
		}
		for _, point := range series.Histograms {
			result.WriteString(fmt.Sprintf("  histogram @[%v]\n", time.UnixMilli(point.T).UTC()))
		}
	}
	return result.String()
}
