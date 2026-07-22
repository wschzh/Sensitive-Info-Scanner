//go:build gui

// Package main 是 Web 图形界面入口（用 -tags gui 编译）。
// 启动内嵌 HTTP 服务并自动打开默认浏览器。
package main

import (
	"fmt"
	"os"
	"time"

	"sensitivescanner/internal/web"
	"sensitivescanner/internal/worker"
	"sensitivescanner/internal/workerproto"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == workerproto.Arg {
		os.Exit(worker.RunExtractWorker(os.Stdin, os.Stdout))
	}

	srv := web.NewServer()
	addr, err := srv.Start()
	if err != nil {
		fmt.Println("启动服务失败:", err)
		return
	}
	url := "http://" + addr
	fmt.Println("敏感信息扫描工具已启动:", url)
	fmt.Println("（如浏览器未自动打开，请手动访问上面的地址；按 Ctrl+C 退出）")

	// 等服务就绪后开浏览器
	time.Sleep(200 * time.Millisecond)
	if err := web.OpenBrowser(url); err != nil {
		fmt.Println("浏览器未自动打开，请手动访问:", url)
	}

	// 阻塞直到 /api/exit（Web 页面「退出程序」按钮）触发；退出后进程结束，Windows 下可正常删除 exe
	<-srv.Done()
}
