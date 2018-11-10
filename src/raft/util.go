package raft

import (
	"fmt"
	// "log"
)

const (
	WIDTH_PREFIX = 36
)

// Debugging
const Debug = 0

func DPrintf(format string, a ...interface{}) (n int, err error) {
	// if Debug > 0 {
	// 	outputParts := strings.Split(format, "]")
	// 	a = append(a, outputParts[0]+"]")
	// 	a = append(a[len(a)-1:len(a)], a[0:len(a)-1])
	// 	log.Printf("%-"+strconv.Itoa(WIDTH_PREFIX)+"s"+outputParts[1], a...)
	// 	// log.Printf(format, a...)
	// }

	if Debug > 0 {
		if a[0] != "[persist]" {
			fmt.Printf(format, a...)
			// log.Printf(format, a...)
		}
	}
	return
}
