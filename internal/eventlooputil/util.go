package eventlooputil

import "github.com/joeycumines/goroutineid"

func IsLoopThread(storedID int64) bool {
	return storedID != 0 && goroutineid.Get() == storedID
}
