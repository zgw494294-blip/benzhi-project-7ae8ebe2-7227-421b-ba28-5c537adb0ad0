package main

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

func validateAddr(addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("监听地址无效: %w", err)
	}
	if host != "127.0.0.1" && host != "localhost" {
		return fmt.Errorf("仅允许回环地址")
	}
	n, e := strconv.Atoi(port)
	if e != nil || n < 1024 || n > 65535 {
		return fmt.Errorf("端口必须在1024-65535")
	}
	return nil
}
func normalizeAddr(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return "127.0.0.1" + addr
	}
	return addr
}
