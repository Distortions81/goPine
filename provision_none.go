//go:build !provision

package main

const provisioningBuild = false

func prepareFirmware(func(int)) error { return nil }
