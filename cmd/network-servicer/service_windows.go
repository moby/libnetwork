package main

import "fmt"

func marker(path, vip, eip, fwMark, file string, isDelete bool) (int, error) {
	return 1, fmt.Errorf("Unsupported")
}

func redirecter(path, eip, file string) (int, error) {
	return 1, fmt.Errorf("Unsupported")
}

func setupResolver(path, localAddress, localTCPAddress string) (int, error) {
	return 1, fmt.Errorf("Unsupported")
}
