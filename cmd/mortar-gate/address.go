package main

import (
	"errors"
	"fmt"
	"net"
	"strconv"
)

// validateAddress 限制服务只使用高位回环监听地址。
func validateAddress(address string) error {
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("监听地址格式无效: %w", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1024 || port > 65535 {
		return errors.New("监听端口必须是 1024 到 65535 的高位端口")
	}
	if host == "localhost" {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("监听地址必须使用回环主机，得到 %q", host)
	}
	return nil
}
