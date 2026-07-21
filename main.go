// Package main 是敏感信息扫描工具的命令行入口。
//
// 用法：
//
//	scanner <路径> [选项]      直接扫描
//	scanner -i                 交互模式
//
// Web 图形界面见 cmd/gui（需 -tags gui 编译）。
package main

import (
	"flag"
	"fmt"
	"os"

	"sensitivescanner/internal/cli"
	"sensitivescanner/internal/report"
	"sensitivescanner/internal/scanner"
	"sensitivescanner/internal/types"
)

// levelFlag 实现 flag.Value，支持 -level 多选。
type levelFlag struct{ levels []types.Level }

func (l *levelFlag) String() string { return "" }
func (l *levelFlag) Set(v string) error {
	switch v {
	case "critical":
		l.levels = append(l.levels, types.Critical)
	case "high":
		l.levels = append(l.levels, types.High)
	case "medium":
		l.levels = append(l.levels, types.Medium)
	case "low":
		l.levels = append(l.levels, types.Low)
	default:
		return fmt.Errorf("未知级别 %q（可选: critical/high/medium/low）", v)
	}
	return nil
}

func main() {
	interactive := flag.Bool("i", false, "进入交互模式")
	recursive := flag.Bool("recursive", true, "递归扫描子目录")
	output := flag.String("o", "", "输出报告文件路径")
	format := flag.String("f", "text", "报告格式 (text/json)")
	maxSize := flag.Int("max-size", 10*1024*1024, "最大文件大小(字节)")
	workers := flag.Int("workers", 0, "并发 worker 数(0=CPU 核数)")
	maxResults := flag.Int("max-results", 100000, "内存保留结果上限")
	var levels levelFlag
	flag.Var(&levels, "level", "扫描级别(可多选: critical/high/medium/low)")
	flag.Parse()

	if *interactive {
		os.Exit(cli.RunInteractive())
	}

	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "用法: scanner <路径> [选项]  或  scanner -i 进入交互模式")
		fmt.Fprintln(os.Stderr, "选项: -recursive -o <文件> -f text|json --max-size <字节> -level <级别>")
		os.Exit(1)
	}
	path := args[0]

	if _, err := os.Stat(path); err != nil {
		fmt.Fprintf(os.Stderr, "错误: 路径不存在: %s\n", path)
		os.Exit(1)
	}

	s := scanner.New(scanner.Config{
		MaxFileSize: int64(*maxSize),
		ScanLevels:  levels.levels,
		Workers:     *workers,
		MaxResults:  *maxResults,
	})

	fmt.Printf("开始扫描: %s\n", path)
	if fi, _ := os.Stat(path); fi.IsDir() {
		s.ScanDirectory(path, *recursive)
	} else {
		s.ScanSingle(path)
	}

	if *output == "" {
		if err := report.GenerateTo(s, report.Format(*format), os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "生成报告失败: %v\n", err)
		}
	} else {
		if err := report.GenerateToFile(s, report.Format(*format), *output); err != nil {
			fmt.Fprintf(os.Stderr, "写报告失败: %v\n", err)
		} else {
			stats := s.Stats()
			fmt.Printf("完成: 扫描 %d 个文件，发现 %d 个敏感信息，报告已保存到 %s\n",
				stats.ScannedFiles, stats.TotalIssues, *output)
		}
	}
}
