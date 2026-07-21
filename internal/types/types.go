// Package types 定义扫描器全包共享的数据结构。
package types

import "time"

// Level 敏感信息级别。
type Level string

const (
	Critical Level = "critical"
	High     Level = "high"
	Medium   Level = "medium"
	Low      Level = "low"
)

// AllLevels 按优先级排序的全部级别。
var AllLevels = []Level{Critical, High, Medium, Low}

// LevelInfo 级别展示信息。
type LevelInfo struct {
	Name     string // 中文名
	Color    string // 前端染色用
	Priority int    // 排序优先级（1 最高）
}

// LevelConfig 各级别展示配置。
var LevelConfig = map[Level]LevelInfo{
	Critical: {Name: "严重", Color: "#e74c3c", Priority: 1},
	High:     {Name: "高级", Color: "#e67e22", Priority: 2},
	Medium:   {Name: "中级", Color: "#f1c40f", Priority: 3},
	Low:      {Name: "低级", Color: "#3498db", Priority: 4},
}

// ScanResult 单条扫描结果。
type ScanResult struct {
	FilePath    string    `json:"file_path"`
	PatternName string    `json:"pattern_name"`
	Level       Level     `json:"level"`
	LineNumber  int       `json:"line_number"`
	LineContent string    `json:"line_content"`
	MatchedText string    `json:"matched_text"`
	Timestamp   time.Time `json:"timestamp"`
}

// ScanStatistics 扫描统计。
type ScanStatistics struct {
	TotalFiles       int           `json:"total_files"`
	ScannedFiles     int           `json:"scanned_files"`
	FilesWithIssues  int           `json:"files_with_issues"`
	TotalIssues      int           `json:"total_issues"`
	IssuesByLevel    map[Level]int `json:"issues_by_level"`
	ScanDuration     float64       `json:"scan_duration"`
	TruncatedCount   int           `json:"truncated_count"`              // 因超 MaxResults 未保留的结果数
	TruncatedByLevel map[Level]int `json:"truncated_by_level,omitempty"` // 各级别被截断的数量
}

// NewScanStatistics 构造一个初始化好 map 的统计结构。
func NewScanStatistics() ScanStatistics {
	return ScanStatistics{
		IssuesByLevel:    map[Level]int{},
		TruncatedByLevel: map[Level]int{},
	}
}
