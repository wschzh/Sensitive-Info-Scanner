package workerproto

import (
	"time"

	"sensitivescanner/internal/types"
)

const Arg = "--extract-worker"

type Request struct {
	Path          string        `json:"path"`
	Format        string        `json:"format"`
	Levels        []types.Level `json:"levels,omitempty"`
	Mode          string        `json:"mode,omitempty"`
	MaxTextSize   int           `json:"max_text_size,omitempty"`
	MaxFindings   int           `json:"max_findings,omitempty"`
	TimeoutMillis int64         `json:"timeout_ms,omitempty"`
	MemoryLimitMB int           `json:"memory_limit_mb,omitempty"`
	StartedAt     time.Time     `json:"started_at,omitempty"`
}

type Response struct {
	Status         string             `json:"status"`
	Findings       []types.ScanResult `json:"findings,omitempty"`
	MatchedCount   int                `json:"matched_count"`
	IssueOverflow  bool               `json:"issue_overflow,omitempty"`
	Error          string             `json:"error,omitempty"`
	DurationMillis int64              `json:"duration_ms"`
}
