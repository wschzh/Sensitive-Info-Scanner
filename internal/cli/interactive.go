// Package cli 提供命令行交互式扫描模式（对齐原 cli.py 的交互流程）。
package cli

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"sensitivescanner/internal/report"
	"sensitivescanner/internal/scanner"
	"sensitivescanner/internal/types"
)

// RunInteractive 启动交互式扫描，返回退出码。
func RunInteractive() int {
	r := bufio.NewReader(os.Stdin)
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("敏感信息扫描工具 - 交互模式")
	fmt.Println(strings.Repeat("=", 60))

	// 1. 扫描路径
	var path string
	for {
		fmt.Print("\n请输入要扫描的路径（文件或目录）: ")
		path = readLine(r)
		if path == "" {
			fmt.Println("路径不能为空")
			continue
		}
		if _, err := os.Stat(path); err != nil {
			fmt.Printf("路径不存在: %s\n", path)
			continue
		}
		break
	}

	// 2. 递归
	recursive := true
	fmt.Print("\n是否递归扫描子目录？(y/n, 默认y): ")
	if strings.HasPrefix(strings.ToLower(readLine(r)), "n") {
		recursive = false
	}

	// 3. 最大文件大小
	maxSize := 10
	for {
		fmt.Print("\n请输入最大文件大小(MB, 默认10): ")
		s := readLine(r)
		if s == "" {
			break
		}
		n, err := strconv.Atoi(s)
		if err != nil || n <= 0 {
			fmt.Println("请输入有效的正整数")
			continue
		}
		maxSize = n
		break
	}

	// 4. 敏感级别
	fmt.Println("\n可选敏感级别：1=严重 2=高级 3=中级 4=低级（多选用逗号分隔，默认全部）")
	levels := promptLevels(r)

	// 5. 报告格式
	format := report.Text
	for {
		fmt.Print("\n报告格式(text/json, 默认text): ")
		s := strings.ToLower(readLine(r))
		if s == "" {
			break
		}
		if s == "text" || s == "json" {
			format = report.Format(s)
			break
		}
		fmt.Println("请输入 text 或 json")
	}

	// 6. 输出文件
	fmt.Print("\n报告输出文件路径（可选，回车输出到控制台）: ")
	output := readLine(r)

	cfg := scanner.Config{
		MaxFileSize: int64(maxSize) * 1024 * 1024,
		ScanLevels:  levels,
	}
	s := scanner.New(cfg)

	fmt.Printf("\n开始扫描: %s\n", path)
	if fi, _ := os.Stat(path); fi.IsDir() {
		s.ScanDirectory(path, recursive)
	} else {
		s.ScanSingle(path)
	}

	content, err := report.Generate(s, format, output)
	if err != nil {
		fmt.Fprintf(os.Stderr, "写报告失败: %v\n", err)
	}
	if output == "" {
		fmt.Println(content)
	} else {
		stats := s.Stats()
		fmt.Printf("完成: 扫描 %d 个文件，发现 %d 个敏感信息，报告已保存到 %s\n",
			stats.ScannedFiles, stats.TotalIssues, output)
	}
	return 0
}

func readLine(r *bufio.Reader) string {
	s, _ := r.ReadString('\n')
	return strings.TrimSpace(s)
}

func promptLevels(r *bufio.Reader) []types.Level {
	all := types.AllLevels
	for {
		fmt.Print("请输入级别编号（例如 1,2,3）: ")
		s := readLine(r)
		if s == "" {
			return nil
		}
		var levels []types.Level
		ok := true
		for _, part := range strings.Split(s, ",") {
			n, err := strconv.Atoi(strings.TrimSpace(part))
			if err != nil || n < 1 || n > len(all) {
				ok = false
				break
			}
			levels = append(levels, all[n-1])
		}
		if ok && len(levels) > 0 {
			return levels
		}
		fmt.Println("输入无效，请用逗号分隔的数字（1-4）")
	}
}
