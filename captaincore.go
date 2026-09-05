package main

import (
	"github.com/CaptainCore/captaincore/cmd"
)

func main() {
	cmd.SetRuntimeAssets(runtimeAssets)
	cmd.Execute()
}
