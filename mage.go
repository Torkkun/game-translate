//go:build ignore

// mage を入れずにタスクを動かすための入口: go run mage.go <タスク>
// Go が無い PC では ./scripts/mage.sh (コンテナで組み立てて手元で動かす)。
package main

import (
	"os"

	"github.com/magefile/mage/mage"
)

func main() { os.Exit(mage.Main()) }
